package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

type controlledCheckAuthorization struct {
	ownership *ControllerOwnership
	authority *policy.Authority
	permit    policy.CurrentCheckPermit
	request   policy.CheckPermitRequest
}

// AuthorizedCheckLease is the only capability which can enter one current-
// authorized local check attempt. It is process-local and shared by value
// copies through one atomic consumption state.
type AuthorizedCheckLease struct {
	issuer        *leaseIssuer
	effect        Effect
	capability    *effectCapabilityState
	ownership     *ControllerOwnership
	authority     *policy.Authority
	permit        policy.CurrentCheckPermit
	permitRequest policy.CheckPermitRequest
}

// PreparedAuthorizedCheckLease proves that current authority and the durable
// source head were rejoined immediately before worker execution. Completion
// remains bankable history and therefore does not depend on permit freshness.
type PreparedAuthorizedCheckLease struct {
	issuer        *leaseIssuer
	effect        Effect
	capability    *effectCapabilityState
	control       *Store
	ownership     *ControllerOwnership
	permitRequest policy.CheckPermitRequest
	planDigest    string
}

// PendingCheckPermitRequest derives the complete identity of one exact next
// policy-ordered pending check under active controller ownership. It grants no
// execution capability; the controller must freshly authorize the returned
// request and present that permit to ClaimControlledCheck.
func (s *Store) PendingCheckPermitRequest(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	runID string,
	workID string,
	effectID string,
) (engine.State, protocol.ExactPlan, policy.CheckPermitRequest, error) {
	zero := func(err error) (engine.State, protocol.ExactPlan, policy.CheckPermitRequest, error) {
		return engine.State{}, protocol.ExactPlan{}, policy.CheckPermitRequest{}, err
	}
	if s == nil || s.readOnly || ownership == nil {
		return zero(errors.New("pending check selection requires a writable owned Store"))
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("validate pending check selector ownership: %w", err))
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero(fmt.Errorf("begin pending check selection: %w", err))
	}
	defer transaction.Rollback() //nolint:errcheck
	state, found, err := loadState(ctx, transaction, runID)
	if err != nil {
		return zero(err)
	}
	if !found || state.Phase != engine.PhaseActive || state.RunID != runID {
		return zero(errors.New("pending check selection requires an active delivery"))
	}
	attempt, err := loadExactAttempt(ctx, transaction, state, workID, engine.WorkChecking)
	if err != nil {
		return zero(err)
	}
	contract, exists := attempt.plan.Work(workID)
	if !exists || contract.Digest() == "" {
		return zero(errors.New("pending check selection lacks its exact work contract"))
	}
	payload, effects, err := loadExactCheckBatch(ctx, transaction, state, attempt)
	if err != nil {
		return zero(err)
	}
	if payload.RuntimeManifestDigest != s.localCheckRuntimeManifestDigest {
		return zero(errors.New("pending check selection does not match the configured runtime"))
	}
	selected := -1
	for index := range effects {
		if effects[index].ID == effectID {
			selected = index
			break
		}
	}
	if selected < 0 || effects[selected].State != EffectPending {
		return zero(ErrNoPendingEffect)
	}
	for index := 0; index < selected; index++ {
		if effects[index].State != EffectSucceeded {
			return zero(errors.New("pending check selection is not policy-ordered"))
		}
	}
	request, err := engine.ParseLocalCheckEffectRequest(effects[selected].Request)
	if err != nil {
		return zero(err)
	}
	permitRequest := policy.CheckPermitRequest{
		ControllerID: controllerID, RunID: runID, StateRevision: state.Revision,
		WorkID: workID, WorkAttempt: attempt.number, Contract: contract,
		BuilderEffectID: payload.BuilderEffectID, CheckEffectID: effects[selected].ID,
		CheckID: request.CheckID, DefinitionDigest: request.DefinitionDigest,
		RuntimeManifestDigest: request.RuntimeManifestDigest,
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("revalidate pending check selector ownership: %w", err))
	}
	if err := transaction.Commit(); err != nil {
		return zero(fmt.Errorf("finish pending check selection: %w", err))
	}
	return state, attempt.plan, permitRequest, nil
}

func (lease AuthorizedCheckLease) effectLease() EffectLease {
	return EffectLease{issuer: lease.issuer, effect: cloneEffect(lease.effect)}
}

func (lease PreparedAuthorizedCheckLease) effectLease() EffectLease {
	return EffectLease{issuer: lease.issuer, effect: cloneEffect(lease.effect)}
}

// RunCheck retains active controller ownership for the complete synchronous
// worker call and permits exactly one value copy to enter it.
func (lease PreparedAuthorizedCheckLease) RunCheck(
	run func(engine.JournalEffect) (json.RawMessage, error),
) (result json.RawMessage, resultErr error) {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer || lease.ownership == nil ||
		lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID || run == nil {
		return nil, errors.New("check execution requires a prepared Store capability")
	}
	resultErr = lease.ownership.withActiveOperation(
		lease.control, lease.permitRequest.ControllerID, func() error {
			if !lease.capability.phase.CompareAndSwap(
				effectCapabilityPrepared, effectCapabilityConsumed,
			) {
				return errors.New("prepared check execution capability was already consumed")
			}
			var err error
			result, err = run(journalEffect(lease.effect))
			return err
		},
	)
	if resultErr != nil {
		return nil, resultErr
	}
	return result, nil
}

// ClaimControlledCheck claims only the exact next policy-ordered local check
// selected by a fresh current-authority permit. The claim itself grants the
// synchronous worker capability; no generic lease can execute this effect.
func (s *Store) ClaimControlledCheck(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentCheckPermit,
	request policy.CheckPermitRequest,
) (AuthorizedCheckLease, error) {
	authorization := controlledCheckAuthorization{
		ownership: ownership, authority: authority, permit: permit, request: request,
	}
	if err := s.validateControlledCheckCapability(authorization); err != nil {
		return AuthorizedCheckLease{}, err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return AuthorizedCheckLease{}, fmt.Errorf("begin controlled check claim: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	_, _, _, effect, err := s.validateControlledCheckState(ctx, transaction, authorization, EffectPending)
	if err != nil {
		return AuthorizedCheckLease{}, err
	}
	// Rejoin ownership, the permit and durable current head immediately before
	// the CAS which creates an executable attempt.
	if _, _, _, _, err := s.validateControlledCheckState(
		ctx, transaction, authorization, EffectPending,
	); err != nil {
		return AuthorizedCheckLease{}, err
	}
	now := s.now().UTC().UnixMicro()
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'running', attempt = attempt + 1, owner_id = ?,
		    started_at_us = ?, completed_at_us = NULL, receipt_json = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'pending'`, request.ControllerID, now, effect.ID)
	if err != nil {
		return AuthorizedCheckLease{}, fmt.Errorf("claim controlled check %q: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "claim controlled check "+effect.ID); err != nil {
		return AuthorizedCheckLease{}, err
	}
	effect, err = loadEffect(ctx, transaction, effect.ID)
	if err != nil {
		return AuthorizedCheckLease{}, err
	}
	claimReceipt, err := s.claimReceipt(effect)
	if err != nil {
		return AuthorizedCheckLease{}, err
	}
	if len(claimReceipt) == 0 {
		return AuthorizedCheckLease{}, errors.New("controlled check claim lacks its attempt identity")
	}
	if err := insertObservation(ctx, transaction, effect, "claimed", claimReceipt, "", now); err != nil {
		return AuthorizedCheckLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AuthorizedCheckLease{}, fmt.Errorf("commit controlled check claim: %w", err)
	}
	return AuthorizedCheckLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect),
		capability: newEffectCapabilityState(effectCapabilityClaimed), ownership: ownership,
		authority: authority, permit: permit, permitRequest: request,
	}, nil
}

// PrepareAuthorizedCheckExecution is the last current-authority gate before
// the local worker receives an external-execution capability.
func (s *Store) PrepareAuthorizedCheckExecution(
	ctx context.Context,
	lease AuthorizedCheckLease,
) (PreparedAuthorizedCheckLease, error) {
	if s.readOnly {
		return PreparedAuthorizedCheckLease{}, errors.New("control store is read-only")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PreparedAuthorizedCheckLease{}, fmt.Errorf("begin authorized check preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := s.validateAuthorizedCheckLeaseTransaction(ctx, transaction, lease); err != nil {
		return PreparedAuthorizedCheckLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return PreparedAuthorizedCheckLease{}, fmt.Errorf("finish authorized check preparation: %w", err)
	}
	if lease.capability == nil || !lease.capability.phase.CompareAndSwap(
		effectCapabilityClaimed, effectCapabilityPrepared,
	) {
		return PreparedAuthorizedCheckLease{}, errors.New("authorized check lease was already prepared")
	}
	return PreparedAuthorizedCheckLease{
		issuer: s.leaseIssuer, effect: cloneEffect(lease.effect), capability: lease.capability,
		control: s, ownership: lease.ownership, permitRequest: lease.permitRequest,
		planDigest: lease.permit.Facts().PlanDigest,
	}, nil
}

// BindAuthorizedCheckResult binds a typed check result after the synchronous
// worker has consumed the exact prepared capability.
func (s *Store) BindAuthorizedCheckResult(
	ctx context.Context,
	lease PreparedAuthorizedCheckLease,
	result json.RawMessage,
) error {
	if err := s.validatePreparedAuthorizedCheckLease(lease); err != nil {
		return err
	}
	return s.bindEffectResult(ctx, lease.effectLease(), result, lease)
}

// CompleteAuthorizedCheck closes the exact prepared check attempt from its
// already bound typed result.
func (s *Store) CompleteAuthorizedCheck(
	ctx context.Context,
	lease PreparedAuthorizedCheckLease,
) error {
	if err := s.validatePreparedAuthorizedCheckLease(lease); err != nil {
		return err
	}
	return s.completeEffect(ctx, lease.effectLease(), lease)
}

func (s *Store) validatePreparedAuthorizedCheckLease(
	lease PreparedAuthorizedCheckLease,
) error {
	if lease.issuer == nil || lease.issuer != s.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityConsumed || lease.control != s ||
		lease.ownership == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID {
		return errors.New("check operation requires a consumed authorized check lease")
	}
	if err := lease.ownership.ValidateActive(s, lease.permitRequest.ControllerID); err != nil {
		return fmt.Errorf("validate prepared check ownership: %w", err)
	}
	return nil
}

func (lease PreparedAuthorizedCheckLease) validatePreparedTransaction(
	ctx context.Context,
	query *sql.Tx,
	control *Store,
) error {
	if err := control.validatePreparedAuthorizedCheckLease(lease); err != nil {
		return err
	}
	request := lease.permitRequest
	state, found, err := loadState(ctx, query, request.RunID)
	if err != nil {
		return err
	}
	if !found || state.Phase != engine.PhaseActive || state.RunID != request.RunID ||
		state.Revision != request.StateRevision || state.PlanDigest != lease.planDigest {
		return errors.New("prepared check no longer matches its delivery state")
	}
	attempt, err := loadExactAttempt(ctx, query, state, request.WorkID, engine.WorkChecking)
	if err != nil {
		return err
	}
	contract, exists := attempt.plan.Work(request.WorkID)
	if !exists || contract.Digest() != request.Contract.Digest() || attempt.number != request.WorkAttempt {
		return errors.New("prepared check no longer matches its exact work attempt")
	}
	_, effects, err := loadExactCheckBatch(ctx, query, state, attempt)
	if err != nil {
		return err
	}
	for _, effect := range effects {
		if effect.ID != request.CheckEffectID {
			continue
		}
		if err := requireRunningLease(effect, lease.effectLease()); err != nil {
			return err
		}
		if err := lease.ownership.ValidateActive(control, request.ControllerID); err != nil {
			return fmt.Errorf("revalidate prepared check ownership: %w", err)
		}
		return nil
	}
	return errors.New("prepared check no longer matches its exact effect")
}

func (s *Store) validateAuthorizedCheckLeaseTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	lease AuthorizedCheckLease,
) error {
	if lease.issuer == nil || lease.issuer != s.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityClaimed || lease.ownership == nil ||
		lease.authority == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID {
		return errors.New("check operation requires a current authorized check lease")
	}
	authorization := controlledCheckAuthorization{
		ownership: lease.ownership, authority: lease.authority,
		permit: lease.permit, request: lease.permitRequest,
	}
	_, _, _, effect, err := s.validateControlledCheckState(
		ctx, transaction, authorization, EffectRunning,
	)
	if err != nil {
		return err
	}
	if err := requireRunningLease(effect, lease.effectLease()); err != nil {
		return err
	}
	return nil
}

func (s *Store) validateControlledCheckCapability(
	authorization controlledCheckAuthorization,
) error {
	if s == nil || authorization.ownership == nil || authorization.authority == nil {
		return errors.New("controlled check requires Store ownership and current authority")
	}
	request := authorization.request
	if err := authorization.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("validate active controller ownership: %w", err)
	}
	if err := authorization.authority.RequireLedger(s); err != nil {
		return fmt.Errorf("validate controlled check authority ledger: %w", err)
	}
	if err := authorization.authority.ValidateCheckPermit(authorization.permit, request); err != nil {
		return fmt.Errorf("validate current check permit: %w", err)
	}
	if !engine.ValidDigest(s.localCheckRuntimeManifestDigest) ||
		request.RuntimeManifestDigest != s.localCheckRuntimeManifestDigest {
		return errors.New("controlled check does not match the configured local runtime")
	}
	return nil
}

func (s *Store) validateControlledCheckState(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledCheckAuthorization,
	wantEffectState EffectState,
) (engine.State, protocol.ExactPlan, protocol.ExactWorkContract, Effect, error) {
	zero := func(err error) (engine.State, protocol.ExactPlan, protocol.ExactWorkContract, Effect, error) {
		return engine.State{}, protocol.ExactPlan{}, protocol.ExactWorkContract{}, Effect{}, err
	}
	if err := s.validateControlledCheckCapability(authorization); err != nil {
		return zero(err)
	}
	if err := validateCurrentCheckPermitHead(ctx, transaction, authorization.permit); err != nil {
		return zero(err)
	}
	request, facts := authorization.request, authorization.permit.Facts()
	state, found, err := loadState(ctx, transaction, request.RunID)
	if err != nil {
		return zero(err)
	}
	if !found || state.Phase != engine.PhaseActive || state.RunID != request.RunID ||
		state.Revision != request.StateRevision || state.PlanDigest != facts.PlanDigest {
		return zero(errors.New("controlled check does not match the current delivery state"))
	}
	attempt, err := loadExactAttempt(ctx, transaction, state, request.WorkID, engine.WorkChecking)
	if err != nil {
		return zero(err)
	}
	contract, exists := attempt.plan.Work(request.WorkID)
	if !exists || contract.Digest() == "" || contract.Digest() != request.Contract.Digest() ||
		contract.Digest() != facts.WorkContractDigest || attempt.number != request.WorkAttempt {
		return zero(errors.New("controlled check does not match its exact work contract and attempt"))
	}
	payload, effects, err := loadExactCheckBatch(ctx, transaction, state, attempt)
	if err != nil {
		return zero(err)
	}
	if payload.BuilderEffectID != request.BuilderEffectID ||
		payload.RuntimeManifestDigest != request.RuntimeManifestDigest {
		return zero(errors.New("controlled check does not match its exact dispatch batch"))
	}
	selected := -1
	for index := range effects {
		if effects[index].ID == request.CheckEffectID {
			selected = index
			break
		}
	}
	if selected < 0 {
		return zero(errors.New("controlled check effect is absent from its exact dispatch batch"))
	}
	for index := 0; index < selected; index++ {
		if effects[index].State != EffectSucceeded {
			return zero(errors.New("controlled check is not the next policy-ordered effect"))
		}
	}
	effect := effects[selected]
	if effect.State != wantEffectState {
		return zero(fmt.Errorf("controlled check effect %q is %s, want %s", effect.ID, effect.State, wantEffectState))
	}
	checkRequest, err := engine.ParseLocalCheckEffectRequest(effect.Request)
	if err != nil || checkRequest.DeliveryRunID != request.RunID ||
		checkRequest.WorkID != request.WorkID || checkRequest.WorkAttempt != request.WorkAttempt ||
		checkRequest.BuilderEffectID != request.BuilderEffectID || checkRequest.CheckID != request.CheckID ||
		checkRequest.DefinitionDigest != request.DefinitionDigest ||
		checkRequest.RuntimeManifestDigest != request.RuntimeManifestDigest ||
		facts.CheckEffectID != request.CheckEffectID || facts.CheckID != request.CheckID ||
		facts.DefinitionDigest != request.DefinitionDigest ||
		facts.RuntimeManifestDigest != request.RuntimeManifestDigest {
		return zero(errors.New("controlled check effect does not match its permit"))
	}
	if wantEffectState == EffectRunning {
		loaded, err := loadEffect(ctx, transaction, effect.ID)
		if err != nil {
			return zero(err)
		}
		if !bytes.Equal(loaded.Request, effect.Request) {
			return zero(errors.New("controlled check running request changed"))
		}
		effect = loaded
	}
	return state, attempt.plan, contract, effect, nil
}

func validateCurrentCheckPermitHead(
	ctx context.Context,
	query rowQuerier,
	permit policy.CurrentCheckPermit,
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
		return errors.New("current check permit has no durable authority source head")
	}
	if err != nil {
		return fmt.Errorf("load current authority source head: %w", err)
	}
	if status != "active" || version != facts.SourceVersion || digest != facts.SourceDigest {
		return errors.New("current check permit was superseded in the control ledger")
	}
	return nil
}
