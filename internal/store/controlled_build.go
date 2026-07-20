package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

type controlledBuildAuthorization struct {
	ownership *ControllerOwnership
	authority *policy.Authority
	permit    policy.CurrentBuildPermit
	request   policy.BuildPermitRequest
}

// ApplyControlledBuild is the only Store boundary which accepts a
// build.dispatch command. The active controller, current authority permit,
// durable authority-source head, current run revision, exact work contract,
// and configured builder are rejoined inside the command transaction before
// any idempotency, state, event, or effect row can be written.
func (s *Store) ApplyControlledBuild(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentBuildPermit,
	request policy.BuildPermitRequest,
	command engine.Command,
) (ApplyResult, error) {
	authorization := controlledBuildAuthorization{
		ownership: ownership, authority: authority, permit: permit, request: request,
	}
	if err := s.validateControlledBuildCapability(authorization); err != nil {
		return ApplyResult{}, err
	}
	return s.applyCommand(ctx, command, &authorization)
}

// ClaimControlledBuild claims only the unique runnable build effect whose
// engine-owned request and causal command exactly match the current permit.
// Other pending builds, including same-run selector mismatches, remain
// completely unchanged.
func (s *Store) ClaimControlledBuild(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentBuildPermit,
	request policy.BuildPermitRequest,
) (AuthorizedBuildLease, error) {
	authorization := controlledBuildAuthorization{
		ownership: ownership, authority: authority, permit: permit, request: request,
	}
	if err := s.validateControlledBuildCapability(authorization); err != nil {
		return AuthorizedBuildLease{}, err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return AuthorizedBuildLease{}, fmt.Errorf("begin controlled build claim: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	state, _, contract, err := s.validateControlledBuildState(
		ctx, transaction, authorization, engine.WorkActive,
	)
	if err != nil {
		return AuthorizedBuildLease{}, err
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT pending.effect_id, pending.run_id, pending.command_id, pending.ordinal,
		       pending.kind, pending.request_json, pending.state, pending.attempt,
		       pending.owner_id, pending.receipt_json, pending.last_error,
		       pending.created_at_us, pending.started_at_us, pending.completed_at_us
		FROM effects AS pending
		WHERE pending.state = 'pending' AND pending.run_id = ? AND pending.kind = ?
		  AND NOT EXISTS (
			SELECT 1 FROM effects AS earlier
			WHERE earlier.command_id = pending.command_id
			  AND earlier.ordinal < pending.ordinal
			  AND earlier.state != 'succeeded'
		  )
		ORDER BY pending.created_at_us, pending.command_id, pending.ordinal, pending.effect_id`,
		request.RunID, engine.EffectBuild,
	)
	if err != nil {
		return AuthorizedBuildLease{}, fmt.Errorf("select controlled pending builds: %w", err)
	}
	var candidates []Effect
	for rows.Next() {
		effect, scanErr := scanEffect(rows)
		if scanErr != nil {
			_ = rows.Close()
			return AuthorizedBuildLease{}, scanErr
		}
		candidates = append(candidates, effect)
	}
	if iterationErr := rows.Err(); iterationErr != nil {
		_ = rows.Close()
		return AuthorizedBuildLease{}, fmt.Errorf("iterate controlled pending builds: %w", iterationErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return AuthorizedBuildLease{}, fmt.Errorf("close controlled pending builds: %w", closeErr)
	}
	var matched []Effect
	for _, effect := range candidates {
		exact, matchErr := s.controlledBuildEffectMatches(
			ctx, transaction, effect, state, contract, authorization.request,
		)
		if matchErr != nil {
			return AuthorizedBuildLease{}, matchErr
		}
		if exact {
			matched = append(matched, effect)
		}
	}
	if len(matched) == 0 {
		return AuthorizedBuildLease{}, ErrNoPendingEffect
	}
	if len(matched) != 1 {
		return AuthorizedBuildLease{}, errors.New("controlled build selector is ambiguous")
	}
	// Rejoin the current durable head and active ownership immediately before
	// the compare-and-swap which creates the external-attempt capability.
	if _, _, _, err := s.validateControlledBuildState(
		ctx, transaction, authorization, engine.WorkActive,
	); err != nil {
		return AuthorizedBuildLease{}, err
	}
	effect := matched[0]
	now := s.now().UTC().UnixMicro()
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'running', attempt = attempt + 1, owner_id = ?,
		    started_at_us = ?, completed_at_us = NULL, receipt_json = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'pending'`, request.ControllerID, now, effect.ID)
	if err != nil {
		return AuthorizedBuildLease{}, fmt.Errorf("claim controlled build %q: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "claim controlled build "+effect.ID); err != nil {
		return AuthorizedBuildLease{}, err
	}
	effect, err = loadEffect(ctx, transaction, effect.ID)
	if err != nil {
		return AuthorizedBuildLease{}, err
	}
	claimReceipt, err := s.claimReceipt(effect)
	if err != nil {
		return AuthorizedBuildLease{}, err
	}
	if err := insertObservation(ctx, transaction, effect, "claimed", claimReceipt, "", now); err != nil {
		return AuthorizedBuildLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AuthorizedBuildLease{}, fmt.Errorf("commit controlled build claim: %w", err)
	}
	return AuthorizedBuildLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect),
		capability: newEffectCapabilityState(effectCapabilityClaimed), ownership: ownership,
		authority: authority, permit: permit, permitRequest: request,
	}, nil
}

func (s *Store) controlledBuildEffectMatches(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
	state engine.State,
	contract protocol.ExactWorkContract,
	request policy.BuildPermitRequest,
) (bool, error) {
	buildRequest, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil || buildRequest.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		effect.DeliveryRunID != request.RunID || effect.Kind != string(engine.EffectBuild) ||
		effect.Ordinal != 0 || effect.ID != derivedID("eff", effect.CommandID, 0) ||
		buildRequest.DeliveryRunID != request.RunID || buildRequest.DeliveryID != state.DeliveryID ||
		buildRequest.WorkID != request.WorkID || buildRequest.WorkAttempt != request.WorkAttempt ||
		buildRequest.DispatchDigest != contract.Digest() ||
		buildRequest.BuilderDispatchDigest != request.BuilderDispatchDigest {
		return false, nil
	}
	var kind, requestDigest, outcome string
	var encoded []byte
	err = query.QueryRowContext(ctx, `
		SELECT kind, request_digest, request_json, outcome
		FROM commands WHERE command_id = ?`, effect.CommandID,
	).Scan(&kind, &requestDigest, &encoded, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load controlled build command: %w", err)
	}
	var command engine.Command
	if err := json.Unmarshal(encoded, &command); err != nil {
		return false, nil
	}
	payload, err := parseExactDispatchBuildPayload(command.Payload)
	if err != nil || kind != string(engine.CommandDispatchBuild) || outcome != string(OutcomeApplied) ||
		command.ID != effect.CommandID || command.RunID != request.RunID ||
		command.Kind != engine.CommandDispatchBuild || command.ExpectedRevision != request.StateRevision-1 ||
		requestDigest != commandDigest(command) || payload.WorkID != request.WorkID ||
		payload.DispatchDigest != contract.Digest() ||
		payload.BuilderDispatchDigest != request.BuilderDispatchDigest {
		return false, nil
	}
	return true, nil
}

func (s *Store) validateAuthorizedBuildLeaseTransaction(
	ctx context.Context,
	query rowQuerier,
	lease AuthorizedBuildLease,
) error {
	if lease.issuer == nil || lease.issuer != s.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityClaimed || lease.ownership == nil ||
		lease.authority == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID {
		return errors.New("build operation requires a current authorized build lease")
	}
	authorization := controlledBuildAuthorization{
		ownership: lease.ownership, authority: lease.authority,
		permit: lease.permit, request: lease.permitRequest,
	}
	state, _, contract, err := s.validateControlledBuildState(
		ctx, query, authorization, engine.WorkActive,
	)
	if err != nil {
		return err
	}
	effect, err := loadEffect(ctx, query, lease.effect.ID)
	if err != nil {
		return err
	}
	if err := requireRunningLease(effect, lease.effectLease()); err != nil {
		return err
	}
	exact, err := s.controlledBuildEffectMatches(
		ctx, query, effect, state, contract, lease.permitRequest,
	)
	if err != nil {
		return err
	}
	if !exact {
		return errors.New("authorized build lease no longer matches its exact dispatch")
	}
	if _, _, err := s.validateNativeBuildAttempt(ctx, query, effect); err != nil {
		return err
	}
	return s.validateControlledBuildCapability(authorization)
}

func (s *Store) validatePreparedAuthorizedBuildTransaction(
	ctx context.Context,
	query rowQuerier,
	lease PreparedAuthorizedBuildLease,
) error {
	request := lease.permitRequest
	if lease.issuer == nil || lease.issuer != s.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityConsumed || lease.control != s ||
		lease.ownership == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != request.ControllerID {
		return errors.New("build operation requires a prepared authorized build lease")
	}
	if err := lease.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("validate prepared build ownership: %w", err)
	}
	state, found, err := loadState(ctx, query, request.RunID)
	if err != nil {
		return err
	}
	if !found || state.Phase != engine.PhaseActive || state.RunID != request.RunID ||
		state.Revision != request.StateRevision || state.PlanDigest != lease.planDigest {
		return errors.New("prepared build no longer matches its delivery state")
	}
	plan, err := loadExactPlan(ctx, query, state.PlanDigest)
	if err != nil {
		return fmt.Errorf("load prepared build plan: %w", err)
	}
	target := plan.Target()
	if plan.Record().Digest != lease.planDigest || plan.DeliveryID() != state.DeliveryID ||
		target.Repository != state.Repository || target.Ref != state.TargetRef ||
		!slices.Equal(plan.WorkIDs(), stateWorkIDsForBuildGate(state.Work)) {
		return errors.New("prepared build state no longer matches its exact plan")
	}
	contract, exists := plan.Work(request.WorkID)
	if !exists || contract.Digest() == "" || contract.Digest() != request.Contract.Digest() {
		return errors.New("prepared build no longer matches its exact contract")
	}
	matchedWork := false
	for _, work := range state.Work {
		matchedWork = matchedWork || work.ID == request.WorkID && work.State == engine.WorkActive &&
			work.Attempt == request.WorkAttempt
	}
	if !matchedWork {
		return errors.New("prepared build no longer matches its active work attempt")
	}
	effect, err := loadEffect(ctx, query, lease.effect.ID)
	if err != nil {
		return err
	}
	if err := requireRunningLease(effect, lease.effectLease()); err != nil {
		return err
	}
	exact, err := s.controlledBuildEffectMatches(ctx, query, effect, state, contract, request)
	if err != nil {
		return err
	}
	if !exact {
		return errors.New("prepared build no longer matches its exact dispatch")
	}
	if len(effect.Result) == 0 {
		if _, _, err := s.validateNativeBuildAttempt(ctx, query, effect); err != nil {
			return err
		}
	} else {
		identity, err := loadBuildAttemptIdentity(ctx, query, effect)
		buildRequest, requestErr := engine.ParseBuildEffectRequest(effect.Request)
		if err != nil || requestErr != nil || s.repository == nil ||
			s.repository.Binding().RepositoryID != state.Repository ||
			buildRequest.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
			buildRequest.BuilderDispatchDigest != s.builderDispatchDigest ||
			identity.BuilderDispatchDigest != s.builderDispatchDigest {
			return errors.New("prepared build result no longer matches its native attempt")
		}
	}
	if err := lease.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("revalidate prepared build ownership: %w", err)
	}
	return nil
}

func (lease PreparedAuthorizedBuildLease) validatePreparedTransaction(
	ctx context.Context,
	query *sql.Tx,
	control *Store,
) error {
	return control.validatePreparedAuthorizedBuildTransaction(ctx, query, lease)
}

func (s *Store) validateControlledBuildCapability(authorization controlledBuildAuthorization) error {
	if s == nil || authorization.ownership == nil || authorization.authority == nil {
		return errors.New("controlled build requires Store ownership and current authority")
	}
	if err := authorization.ownership.ValidateActive(s, authorization.request.ControllerID); err != nil {
		return fmt.Errorf("validate active controller ownership: %w", err)
	}
	if err := authorization.authority.RequireLedger(s); err != nil {
		return fmt.Errorf("validate controlled build authority ledger: %w", err)
	}
	if err := authorization.authority.ValidateBuildPermit(authorization.permit, authorization.request); err != nil {
		return fmt.Errorf("validate current build permit: %w", err)
	}
	if s.builderDispatchDigest == "" ||
		authorization.request.BuilderDispatchDigest != s.builderDispatchDigest {
		return errors.New("controlled build does not match the configured builder dispatch")
	}
	return nil
}

func (s *Store) validateControlledBuildTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledBuildAuthorization,
	command engine.Command,
) error {
	if err := s.validateControlledBuildCapability(authorization); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, _, contract, err := s.validateControlledBuildState(
		ctx, transaction, authorization, engine.WorkReady,
	)
	if err != nil {
		return err
	}
	request := authorization.request
	if command.Kind != engine.CommandDispatchBuild || command.RunID != request.RunID ||
		command.ExpectedRevision != request.StateRevision || !engine.ValidID(command.ID) {
		return errors.New("controlled build command does not match its permit")
	}
	payload, err := parseExactDispatchBuildPayload(command.Payload)
	if err != nil {
		return err
	}
	if payload.WorkID != request.WorkID || payload.DispatchDigest != contract.Digest() ||
		payload.BuilderDispatchDigest != request.BuilderDispatchDigest {
		return errors.New("controlled build payload does not match its exact permit")
	}
	// Recheck the in-memory capabilities after all transactional reads. A
	// concurrent source-head advance will either be observed above or prevent
	// this read transaction from being promoted to the command write.
	return s.validateControlledBuildCapability(authorization)
}

func (s *Store) validateControlledBuildState(
	ctx context.Context,
	query rowQuerier,
	authorization controlledBuildAuthorization,
	wantState engine.WorkState,
) (engine.State, protocol.ExactPlan, protocol.ExactWorkContract, error) {
	if err := s.validateControlledBuildCapability(authorization); err != nil {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{}, err
	}
	if err := validateCurrentPermitHead(ctx, query, authorization.permit); err != nil {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{}, err
	}
	request := authorization.request
	state, found, err := loadState(ctx, query, request.RunID)
	if err != nil {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{}, err
	}
	if !found {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			errors.New("controlled build run does not exist")
	}
	permitFacts := authorization.permit.Facts()
	if state.Phase != engine.PhaseActive || state.RunID != request.RunID ||
		state.Revision != request.StateRevision || state.PlanDigest != permitFacts.PlanDigest {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			errors.New("controlled build does not match the current delivery state")
	}
	plan, err := loadExactPlan(ctx, query, state.PlanDigest)
	if err != nil {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			fmt.Errorf("load controlled build plan: %w", err)
	}
	target := plan.Target()
	if plan.Record().Digest != permitFacts.PlanDigest || plan.DeliveryID() != state.DeliveryID ||
		target.Repository != state.Repository || target.Ref != state.TargetRef ||
		!slices.Equal(plan.WorkIDs(), stateWorkIDsForBuildGate(state.Work)) {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			errors.New("controlled build state does not match its exact plan")
	}
	contract, exists := plan.Work(request.WorkID)
	if !exists || contract.View().ID != request.WorkID || contract.Digest() == "" ||
		contract.Digest() != request.Contract.Digest() || contract.Digest() != permitFacts.WorkContractDigest {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			errors.New("controlled build does not match its exact work contract")
	}
	matchedWork := false
	for _, work := range state.Work {
		attemptMatches := work.Attempt == request.WorkAttempt
		if wantState == engine.WorkReady {
			attemptMatches = protocol.ValidPositiveSafeInteger(request.WorkAttempt) &&
				work.Attempt == request.WorkAttempt-1
		}
		matchedWork = matchedWork || work.ID == request.WorkID && work.State == wantState && attemptMatches
	}
	if !matchedWork {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{},
			fmt.Errorf("controlled build does not match the %s work attempt", wantState)
	}
	return state, plan, contract, nil
}

func validateCurrentPermitHead(
	ctx context.Context,
	query rowQuerier,
	permit policy.CurrentBuildPermit,
) error {
	facts := permit.Facts()
	var version int64
	var digest, status string
	err := query.QueryRowContext(ctx, `
		SELECT source_version, source_digest, status
		FROM authority_source_snapshots
		WHERE source_ref = ? ORDER BY source_version DESC LIMIT 1`, facts.SourceRef,
	).Scan(&version, &digest, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("current build permit has no durable authority source head")
	}
	if err != nil {
		return fmt.Errorf("load current authority source head: %w", err)
	}
	if status != "active" || version != facts.SourceVersion || digest != facts.SourceDigest {
		return errors.New("current build permit was superseded in the control ledger")
	}
	return nil
}

func parseExactDispatchBuildPayload(encoded json.RawMessage) (engine.DispatchBuildPayload, error) {
	payload, err := decodeExactControlledBuildJSON[engine.DispatchBuildPayload](
		encoded, "work_id", "dispatch_digest", "builder_dispatch_digest",
	)
	if err != nil {
		return engine.DispatchBuildPayload{}, fmt.Errorf("validate controlled build payload: %w", err)
	}
	return payload, nil
}

func decodeExactControlledBuildJSON[T any](encoded []byte, names ...string) (T, error) {
	var value T
	if _, err := protocol.CanonicalizeJSON(encoded); err != nil {
		return value, err
	}
	if len(names) != 0 {
		if err := requireExactJSONMembers(encoded, names...); err != nil {
			return value, err
		}
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return value, err
	}
	return value, nil
}

func requireExactJSONMembers(encoded []byte, names ...string) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return err
	}
	if len(members) != len(names) {
		return errors.New("JSON has unexpected members")
	}
	for _, name := range names {
		if _, exists := members[name]; !exists {
			return fmt.Errorf("JSON is missing %q", name)
		}
	}
	return nil
}

func stateWorkIDsForBuildGate(work []engine.Work) []string {
	ids := make([]string, len(work))
	for index := range work {
		ids[index] = work[index].ID
	}
	return ids
}
