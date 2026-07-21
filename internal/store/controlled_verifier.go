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

type controlledVerifierExecutionAuthorization struct {
	ownership *ControllerOwnership
	authority *policy.Authority
	permit    policy.CurrentVerifierExecutionPermit
	request   policy.VerifierExecutionPermitRequest
}

// AuthorizedVerifierLease is the only capability which can claim one exact
// current-authorized verifier effect.
type AuthorizedVerifierLease struct {
	issuer        *leaseIssuer
	effect        Effect
	capability    *effectCapabilityState
	ownership     *ControllerOwnership
	authority     *policy.Authority
	permit        policy.CurrentVerifierExecutionPermit
	permitRequest policy.VerifierExecutionPermitRequest
}

// PreparedAuthorizedVerifierLease proves the last current-authority gate was
// joined to the running journal attempt. Permit expiry after the external turn
// begins does not erase a truthful result.
type PreparedAuthorizedVerifierLease struct {
	issuer        *leaseIssuer
	effect        Effect
	capability    *effectCapabilityState
	control       *Store
	ownership     *ControllerOwnership
	permitRequest policy.VerifierExecutionPermitRequest
	planDigest    string
}

func (lease AuthorizedVerifierLease) effectLease() EffectLease {
	return EffectLease{issuer: lease.issuer, effect: cloneEffect(lease.effect)}
}

func (lease PreparedAuthorizedVerifierLease) effectLease() EffectLease {
	return EffectLease{issuer: lease.issuer, effect: cloneEffect(lease.effect)}
}

// PendingVerifierExecutionPermitRequest derives the exact pending verifier
// request under current active ownership. It grants no execution capability.
func (s *Store) PendingVerifierExecutionPermitRequest(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	runID string,
	workID string,
) (engine.State, protocol.ExactPlan, policy.VerifierExecutionPermitRequest, error) {
	zero := func(err error) (engine.State, protocol.ExactPlan, policy.VerifierExecutionPermitRequest, error) {
		return engine.State{}, protocol.ExactPlan{}, policy.VerifierExecutionPermitRequest{}, err
	}
	if s == nil || s.readOnly || ownership == nil {
		return zero(errors.New("pending verifier selection requires a writable owned Store"))
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("validate pending verifier ownership: %w", err))
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero(fmt.Errorf("begin pending verifier selection: %w", err))
	}
	defer transaction.Rollback() //nolint:errcheck
	closure, err := s.loadExactReviewClosure(ctx, transaction, runID, workID)
	if err != nil {
		return zero(err)
	}
	effect, dispatchDigest, err := s.loadExactVerifierEffect(
		ctx, transaction, closure, closure.work.VerificationDispatchID, EffectPending,
	)
	if err != nil {
		return zero(err)
	}
	request := policy.VerifierExecutionPermitRequest{
		ControllerID: controllerID, RunID: runID, StateRevision: closure.state.Revision,
		WorkID: workID, WorkAttempt: closure.work.Attempt, Contract: closure.contract,
		SubmissionID: closure.work.SubmissionID, SubmissionDigest: closure.work.SubmissionDigest,
		VerifierEffectID: effect.ID, DispatchID: effect.ID, DispatchDigest: dispatchDigest,
		VerifierProfileDigest: s.verifierProfileDigest,
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("revalidate pending verifier ownership: %w", err))
	}
	if err := transaction.Commit(); err != nil {
		return zero(fmt.Errorf("finish pending verifier selection: %w", err))
	}
	return closure.state, closure.plan, request, nil
}

// ClaimControlledVerifier claims only the exact pending verifier selected by a
// freshly resolved current-authority permit.
func (s *Store) ClaimControlledVerifier(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentVerifierExecutionPermit,
	request policy.VerifierExecutionPermitRequest,
) (AuthorizedVerifierLease, error) {
	authorization := controlledVerifierExecutionAuthorization{
		ownership: ownership, authority: authority, permit: permit, request: request,
	}
	if err := s.validateControlledVerifierExecutionCapability(authorization); err != nil {
		return AuthorizedVerifierLease{}, err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return AuthorizedVerifierLease{}, fmt.Errorf("begin controlled verifier claim: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	// Rejoin current authority and ownership immediately before the effect CAS.
	_, _, effect, err := s.validateControlledVerifierExecutionState(
		ctx, transaction, authorization, EffectPending,
	)
	if err != nil {
		return AuthorizedVerifierLease{}, err
	}
	now := s.now().UTC().UnixMicro()
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'running', attempt = attempt + 1, owner_id = ?,
		    started_at_us = ?, completed_at_us = NULL, receipt_json = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'pending'`, request.ControllerID, now, effect.ID)
	if err != nil {
		return AuthorizedVerifierLease{}, fmt.Errorf("claim controlled verifier %q: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "claim controlled verifier "+effect.ID); err != nil {
		return AuthorizedVerifierLease{}, err
	}
	effect, err = loadEffect(ctx, transaction, effect.ID)
	if err != nil {
		return AuthorizedVerifierLease{}, err
	}
	claimReceipt, err := s.claimReceipt(effect)
	if err != nil {
		return AuthorizedVerifierLease{}, err
	}
	if len(claimReceipt) == 0 {
		return AuthorizedVerifierLease{}, errors.New("controlled verifier claim lacks its attempt identity")
	}
	if err := insertObservation(ctx, transaction, effect, "claimed", claimReceipt, "", now); err != nil {
		return AuthorizedVerifierLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AuthorizedVerifierLease{}, fmt.Errorf("commit controlled verifier claim: %w", err)
	}
	return AuthorizedVerifierLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect),
		capability: newEffectCapabilityState(effectCapabilityClaimed), ownership: ownership,
		authority: authority, permit: permit, permitRequest: request,
	}, nil
}

// PrepareAuthorizedVerifierExecution is the last current-authority gate before
// a verifier worker receives a one-shot external-execution capability.
func (s *Store) PrepareAuthorizedVerifierExecution(
	ctx context.Context,
	lease AuthorizedVerifierLease,
) (PreparedAuthorizedVerifierLease, error) {
	if s.readOnly {
		return PreparedAuthorizedVerifierLease{}, errors.New("control store is read-only")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PreparedAuthorizedVerifierLease{}, fmt.Errorf("begin authorized verifier preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := s.validateAuthorizedVerifierLeaseTransaction(ctx, transaction, lease); err != nil {
		return PreparedAuthorizedVerifierLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return PreparedAuthorizedVerifierLease{}, fmt.Errorf("finish authorized verifier preparation: %w", err)
	}
	if lease.capability == nil || !lease.capability.phase.CompareAndSwap(
		effectCapabilityClaimed, effectCapabilityPrepared,
	) {
		return PreparedAuthorizedVerifierLease{}, errors.New("authorized verifier lease was already prepared")
	}
	return PreparedAuthorizedVerifierLease{
		issuer: s.leaseIssuer, effect: cloneEffect(lease.effect), capability: lease.capability,
		control: s, ownership: lease.ownership, permitRequest: lease.permitRequest,
		planDigest: lease.permit.Facts().PlanDigest,
	}, nil
}

// RunVerifier retains active ownership for the entire synchronous worker call
// and consumes the execution capability exactly once across all value copies.
func (lease PreparedAuthorizedVerifierLease) RunVerifier(
	run func(engine.JournalEffect) (json.RawMessage, error),
) (result json.RawMessage, resultErr error) {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer || lease.ownership == nil ||
		lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID || run == nil {
		return nil, errors.New("verifier execution requires a prepared Store capability")
	}
	resultErr = lease.ownership.withActiveOperation(
		lease.control, lease.permitRequest.ControllerID, func() error {
			if !lease.capability.phase.CompareAndSwap(effectCapabilityPrepared, effectCapabilityConsumed) {
				return errors.New("prepared verifier execution capability was already consumed")
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

// BindAuthorizedVerifierResult binds one exact assessment result after the
// one-shot verifier call. Current permit expiry cannot erase observed truth.
func (s *Store) BindAuthorizedVerifierResult(
	ctx context.Context,
	lease PreparedAuthorizedVerifierLease,
	result json.RawMessage,
) error {
	return s.bindEffectResult(ctx, lease.effectLease(), result, lease)
}

// CompleteAuthorizedVerifier closes the prepared verifier attempt from its
// already-bound typed result.
func (s *Store) CompleteAuthorizedVerifier(
	ctx context.Context,
	lease PreparedAuthorizedVerifierLease,
) error {
	return s.completeEffect(ctx, lease.effectLease(), lease)
}

func (s *Store) validateControlledVerifierExecutionCapability(
	authorization controlledVerifierExecutionAuthorization,
) error {
	if s == nil || authorization.ownership == nil || authorization.authority == nil {
		return errors.New("controlled verifier execution requires Store ownership and current authority")
	}
	request := authorization.request
	if err := authorization.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("validate verifier execution ownership: %w", err)
	}
	if err := authorization.authority.RequireLedger(s); err != nil {
		return fmt.Errorf("validate verifier execution authority ledger: %w", err)
	}
	if err := authorization.authority.ValidateVerifierExecutionPermit(authorization.permit, request); err != nil {
		return fmt.Errorf("validate current verifier execution permit: %w", err)
	}
	if !engine.ValidDigest(s.verifierProfileDigest) || request.VerifierProfileDigest != s.verifierProfileDigest ||
		request.DispatchID != request.VerifierEffectID || s.repository == nil {
		return errors.New("controlled verifier execution does not match immutable process configuration")
	}
	return nil
}

func (s *Store) validateControlledVerifierExecutionState(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledVerifierExecutionAuthorization,
	want EffectState,
) (exactReviewClosure, string, Effect, error) {
	zero := func(err error) (exactReviewClosure, string, Effect, error) {
		return exactReviewClosure{}, "", Effect{}, err
	}
	if err := s.validateControlledVerifierExecutionCapability(authorization); err != nil {
		return zero(err)
	}
	if err := validateCurrentVerifierPermitHead(ctx, transaction, authorization.permit); err != nil {
		return zero(err)
	}
	request, facts := authorization.request, authorization.permit.Facts()
	closure, err := s.loadExactReviewClosure(ctx, transaction, request.RunID, request.WorkID)
	if err != nil {
		return zero(err)
	}
	if closure.state.Revision != request.StateRevision || closure.state.PlanDigest != facts.PlanDigest ||
		closure.work.Attempt != request.WorkAttempt || closure.contract.Digest() != request.Contract.Digest() ||
		closure.contract.Digest() != facts.WorkContractDigest ||
		closure.work.SubmissionID != request.SubmissionID || closure.work.SubmissionDigest != request.SubmissionDigest ||
		closure.work.VerificationDispatchID != request.DispatchID {
		return zero(errors.New("controlled verifier execution does not match current review truth"))
	}
	effect, dispatchDigest, err := s.loadExactVerifierEffect(
		ctx, transaction, closure, request.VerifierEffectID, want,
	)
	if err != nil {
		return zero(err)
	}
	if dispatchDigest != request.DispatchDigest {
		return zero(errors.New("controlled verifier execution does not match its dispatch digest"))
	}
	return closure, dispatchDigest, effect, nil
}

func (s *Store) validateAuthorizedVerifierLeaseTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	lease AuthorizedVerifierLease,
) error {
	if lease.issuer == nil || lease.issuer != s.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityClaimed || lease.ownership == nil ||
		lease.authority == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID {
		return errors.New("verifier operation requires a current authorized verifier lease")
	}
	authorization := controlledVerifierExecutionAuthorization{
		ownership: lease.ownership, authority: lease.authority,
		permit: lease.permit, request: lease.permitRequest,
	}
	_, _, effect, err := s.validateControlledVerifierExecutionState(
		ctx, transaction, authorization, EffectRunning,
	)
	if err != nil {
		return err
	}
	return requireRunningLease(effect, lease.effectLease())
}

func (lease PreparedAuthorizedVerifierLease) validatePreparedTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	control *Store,
) error {
	if lease.issuer == nil || lease.issuer != control.leaseIssuer || lease.capability == nil ||
		lease.capability.phase.Load() != effectCapabilityConsumed || lease.control != control ||
		lease.ownership == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID {
		return errors.New("verifier operation requires a consumed prepared verifier lease")
	}
	if err := lease.ownership.ValidateActive(control, lease.permitRequest.ControllerID); err != nil {
		return fmt.Errorf("validate prepared verifier ownership: %w", err)
	}
	closure, err := control.loadExactReviewClosure(
		ctx, transaction, lease.permitRequest.RunID, lease.permitRequest.WorkID,
	)
	if err != nil {
		return err
	}
	if closure.state.Revision != lease.permitRequest.StateRevision ||
		closure.state.PlanDigest != lease.planDigest || closure.work.Attempt != lease.permitRequest.WorkAttempt ||
		closure.work.SubmissionID != lease.permitRequest.SubmissionID ||
		closure.work.SubmissionDigest != lease.permitRequest.SubmissionDigest {
		return errors.New("prepared verifier no longer matches its delivery state")
	}
	effect, dispatchDigest, err := control.loadExactVerifierEffect(
		ctx, transaction, closure, lease.effect.ID, EffectRunning,
	)
	if err != nil {
		return err
	}
	if dispatchDigest != lease.permitRequest.DispatchDigest ||
		!bytes.Equal(effect.Request, lease.effect.Request) {
		return errors.New("prepared verifier no longer matches its exact effect")
	}
	if err := requireRunningLease(effect, lease.effectLease()); err != nil {
		return err
	}
	return lease.ownership.ValidateActive(control, lease.permitRequest.ControllerID)
}

func (s *Store) loadExactVerifierEffect(
	ctx context.Context,
	query rowQuerier,
	closure exactReviewClosure,
	effectID string,
	want EffectState,
) (Effect, string, error) {
	return s.loadExactVerifierEffectWithConfiguration(ctx, query, closure, effectID, want, true)
}

// loadHistoricalVerifierEffect validates immutable dispatch and result truth
// without treating the verifier profile selected by the current process as a
// retroactive validity rule. Current-profile equality is an execution gate;
// once an exact result succeeded, later configuration rotation must not erase
// a bankable historical assessment.
func (s *Store) loadHistoricalVerifierEffect(
	ctx context.Context,
	query rowQuerier,
	closure exactReviewClosure,
	effectID string,
	want EffectState,
) (Effect, string, error) {
	return s.loadExactVerifierEffectWithConfiguration(ctx, query, closure, effectID, want, false)
}

func (s *Store) loadExactVerifierEffectWithConfiguration(
	ctx context.Context,
	query rowQuerier,
	closure exactReviewClosure,
	effectID string,
	want EffectState,
	requireCurrentConfiguration bool,
) (Effect, string, error) {
	if !engine.ValidID(effectID) || closure.work.VerificationDispatchID != effectID {
		return Effect{}, "", errors.New("verifier effect identity does not match current work")
	}
	effect, err := loadEffect(ctx, query, effectID)
	if err != nil {
		return Effect{}, "", err
	}
	if effect.State != want || effect.DeliveryRunID != closure.state.RunID ||
		effect.Kind != string(engine.EffectVerifier) || effect.Ordinal != 0 ||
		effect.ID != derivedID("eff", effect.CommandID, 0) {
		return Effect{}, "", fmt.Errorf("verifier effect %q is not the exact %s current effect", effectID, want)
	}
	request, err := engine.ParseVerifierEffectRequest(effect.Request)
	if err != nil || request.DeliveryRunID != closure.state.RunID ||
		request.DeliveryID != closure.state.DeliveryID || request.WorkID != closure.work.ID ||
		request.WorkAttempt != closure.work.Attempt || request.PlanDigest != closure.state.PlanDigest ||
		request.SubmissionID != closure.work.SubmissionID || request.SubmissionDigest != closure.work.SubmissionDigest ||
		request.Candidate != closure.submission.View().Candidate || request.DispatchID != effect.ID ||
		request.DispatchReceipt.Ref != request.DispatchReceipt.Digest ||
		(requireCurrentConfiguration &&
			(request.VerifierProfileDigest != s.verifierProfileDigest || request.Agent != s.verifierAgent)) ||
		request.VerificationEpoch != closure.work.VerificationEpoch {
		return Effect{}, "", errors.New("verifier effect request does not match its current exact review")
	}
	var digest, artifactDigest, submissionID, submissionDigest, profileDigest, runID, commandID string
	var epoch int64
	err = query.QueryRowContext(ctx, `
		SELECT digest, artifact_digest, submission_id, submission_digest,
		       profile_digest, run_id, command_id, review_epoch
		FROM verifier_dispatch_records WHERE dispatch_id = ? AND effect_id = ?`, effect.ID, effect.ID,
	).Scan(
		&digest, &artifactDigest, &submissionID, &submissionDigest,
		&profileDigest, &runID, &commandID, &epoch,
	)
	if err != nil || digest != artifactDigest || digest != request.DispatchReceipt.Digest ||
		submissionID != request.SubmissionID || submissionDigest != request.SubmissionDigest ||
		profileDigest != request.VerifierProfileDigest || runID != effect.DeliveryRunID ||
		commandID != effect.CommandID || epoch != request.VerificationEpoch {
		return Effect{}, "", errors.New("verifier effect lacks its exact immutable dispatch identity")
	}
	mediaType, dispatchBytes, err := loadArtifact(ctx, query, digest)
	if err != nil || mediaType != "application/json" {
		return Effect{}, "", errors.New("verifier dispatch artifact is unavailable or invalid")
	}
	dispatch, err := protocol.ParseVerifierDispatch(dispatchBytes)
	if err != nil || dispatch.DispatchID != effect.ID || dispatch.SubmissionDigest != request.SubmissionDigest ||
		dispatch.Candidate != request.Candidate {
		return Effect{}, "", errors.New("verifier dispatch artifact does not match its exact effect")
	}
	return effect, digest, nil
}

func loadVerifierAttemptIdentity(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) (engine.VerifierAttemptIdentity, error) {
	var encoded []byte
	if err := query.QueryRowContext(ctx, `
		SELECT receipt_json FROM effect_observations
		WHERE effect_id = ? AND attempt = ? AND kind = 'claimed'`,
		effect.ID, effect.Attempt,
	).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return engine.VerifierAttemptIdentity{}, errors.New("verifier attempt lacks its durable attempt witness")
		}
		return engine.VerifierAttemptIdentity{}, fmt.Errorf("load verifier attempt identity: %w", err)
	}
	identity, err := engine.ParseVerifierAttemptIdentity(encoded)
	if err != nil {
		return engine.VerifierAttemptIdentity{}, fmt.Errorf("validate verifier attempt identity: %w", err)
	}
	request, err := engine.ParseVerifierEffectRequest(effect.Request)
	if err != nil || identity.EffectID != effect.ID || identity.EffectAttempt != effect.Attempt ||
		identity.DispatchID != request.DispatchID ||
		identity.DispatchDigest != request.DispatchReceipt.Digest ||
		identity.VerifierProfileDigest != request.VerifierProfileDigest || identity.Agent != request.Agent ||
		identity.VerificationEpoch != request.VerificationEpoch {
		return engine.VerifierAttemptIdentity{}, errors.New("verifier attempt identity does not match its journal")
	}
	return identity, nil
}
