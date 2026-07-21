package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const verifierWorkspaceDescription = "fresh-read-only-materialization"

// PreparedVerifierDispatch is an opaque, process-local Store capability for
// one exact not-yet-persisted verifier dispatch. Preparing it grants no
// authority and creates no effect; current authority must bind and consume it
// in ApplyControlledVerifierDispatch.
type PreparedVerifierDispatch struct {
	issuer        *leaseIssuer
	commandID     string
	runID         string
	workID        string
	stateRevision int64
	record        protocol.EncodedRecord
	facts         engine.VerifierDispatchFacts
}

type preparedVerifierDispatch = PreparedVerifierDispatch

type controlledVerifierDispatchAuthorization struct {
	ownership *ControllerOwnership
	authority *policy.Authority
	permit    policy.CurrentVerifierExecutionPermit
	request   policy.VerifierExecutionPermitRequest
	prepared  PreparedVerifierDispatch
}

type exactReviewClosure struct {
	state      engine.State
	work       engine.Work
	plan       protocol.ExactPlan
	contract   protocol.ExactWorkContract
	submission protocol.ExactSubmission
	candidate  repo.Candidate
}

// PrepareControlledVerifierDispatch derives a fresh immutable Baton dispatch
// and its complete authority request from current Store truth. The returned
// capability is valid only for this Store instance and exact command ID.
func (s *Store) PrepareControlledVerifierDispatch(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	runID string,
	workID string,
	commandID string,
) (engine.State, protocol.ExactPlan, policy.VerifierExecutionPermitRequest, PreparedVerifierDispatch, error) {
	zero := func(err error) (
		engine.State, protocol.ExactPlan, policy.VerifierExecutionPermitRequest, PreparedVerifierDispatch, error,
	) {
		return engine.State{}, protocol.ExactPlan{}, policy.VerifierExecutionPermitRequest{}, PreparedVerifierDispatch{}, err
	}
	if s == nil || s.readOnly || ownership == nil {
		return zero(errors.New("verifier dispatch preparation requires a writable owned Store"))
	}
	if !engine.ValidDigest(s.verifierProfileDigest) || !protocol.ValidNonEmpty(s.verifierAgent) || s.repository == nil {
		return zero(errors.New("verifier dispatch preparation requires an immutable verifier configuration"))
	}
	if !engine.ValidID(commandID) {
		return zero(errors.New("verifier dispatch preparation requires a valid command id"))
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("validate verifier dispatch ownership: %w", err))
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero(fmt.Errorf("begin verifier dispatch preparation: %w", err))
	}
	defer transaction.Rollback() //nolint:errcheck
	closure, err := s.loadExactReviewClosure(ctx, transaction, runID, workID)
	if err != nil {
		return zero(err)
	}
	if closure.work.VerificationEpoch >= engine.MaximumVerificationEpoch {
		return zero(errors.New("fresh verification epoch ceiling reached"))
	}
	if err := validateVerifierRedispatch(ctx, transaction, closure.work); err != nil {
		return zero(err)
	}
	dispatchID := derivedID("eff", commandID, 0)
	createdAt := s.now().UTC().Truncate(time.Microsecond)
	record, err := protocol.BuildVerifierDispatch(protocol.VerifierDispatchInput{
		Submission: closure.submission,
		DispatchID: dispatchID,
		Workspace:  verifierWorkspaceDescription,
		CreatedAt:  createdAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return zero(fmt.Errorf("construct exact verifier dispatch: %w", err))
	}
	pointer := protocol.Artifact{Ref: record.Digest, MediaType: "application/json", Digest: record.Digest}
	epoch := closure.work.VerificationEpoch + 1
	facts := engine.VerifierDispatchFacts{
		PlanDigest:   closure.state.PlanDigest,
		SubmissionID: closure.work.SubmissionID, SubmissionDigest: closure.work.SubmissionDigest,
		Candidate:  closure.submission.View().Candidate,
		DispatchID: dispatchID, DispatchReceipt: pointer,
		VerifierProfileDigest: s.verifierProfileDigest, Agent: s.verifierAgent,
		VerificationEpoch: epoch,
	}
	prepared := PreparedVerifierDispatch{
		issuer: s.leaseIssuer, commandID: commandID, runID: runID, workID: workID,
		stateRevision: closure.state.Revision, record: cloneEncodedRecord(record), facts: facts,
	}
	request := policy.VerifierExecutionPermitRequest{
		ControllerID: controllerID, RunID: runID, StateRevision: closure.state.Revision,
		WorkID: workID, WorkAttempt: closure.work.Attempt, Contract: closure.contract,
		SubmissionID: closure.work.SubmissionID, SubmissionDigest: closure.work.SubmissionDigest,
		VerifierEffectID: dispatchID, DispatchID: dispatchID, DispatchDigest: record.Digest,
		VerifierProfileDigest: s.verifierProfileDigest,
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("revalidate verifier dispatch ownership: %w", err))
	}
	if err := transaction.Commit(); err != nil {
		return zero(fmt.Errorf("finish verifier dispatch preparation: %w", err))
	}
	return closure.state, closure.plan, request, prepared, nil
}

// ApplyControlledVerifierDispatch atomically persists the dispatch record,
// raw CAS artifact, command, event, pending effect, and next reducer state.
func (s *Store) ApplyControlledVerifierDispatch(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentVerifierExecutionPermit,
	request policy.VerifierExecutionPermitRequest,
	prepared PreparedVerifierDispatch,
	command engine.Command,
) (ApplyResult, error) {
	authorization := controlledVerifierDispatchAuthorization{
		ownership: ownership, authority: authority, permit: permit, request: request, prepared: prepared,
	}
	if err := s.validateControlledVerifierDispatchCapability(authorization); err != nil {
		return ApplyResult{}, err
	}
	return s.applyCommand(ctx, command, &authorization)
}

func (s *Store) validateControlledVerifierDispatchCapability(
	authorization controlledVerifierDispatchAuthorization,
) error {
	prepared, request := authorization.prepared, authorization.request
	if s == nil || authorization.ownership == nil || authorization.authority == nil ||
		prepared.issuer == nil || prepared.issuer != s.leaseIssuer {
		return errors.New("controlled verifier dispatch requires Store ownership, preparation, and current authority")
	}
	if err := authorization.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("validate verifier controller ownership: %w", err)
	}
	if err := authorization.authority.RequireLedger(s); err != nil {
		return fmt.Errorf("validate verifier authority ledger: %w", err)
	}
	if err := authorization.authority.ValidateVerifierExecutionPermit(authorization.permit, request); err != nil {
		return fmt.Errorf("validate current verifier execution permit: %w", err)
	}
	if s.repository == nil || request.VerifierProfileDigest != s.verifierProfileDigest ||
		prepared.facts.VerifierProfileDigest != s.verifierProfileDigest ||
		prepared.facts.Agent != s.verifierAgent || request.DispatchID != prepared.facts.DispatchID ||
		request.VerifierEffectID != prepared.facts.DispatchID || request.DispatchDigest != prepared.record.Digest {
		return errors.New("controlled verifier dispatch does not match immutable process configuration")
	}
	return nil
}

func (s *Store) validateControlledVerifierDispatchTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledVerifierDispatchAuthorization,
	command engine.Command,
) error {
	if err := s.validateControlledVerifierDispatchCapability(authorization); err != nil {
		return err
	}
	if err := validateCurrentVerifierPermitHead(ctx, transaction, authorization.permit); err != nil {
		return err
	}
	prepared, request := authorization.prepared, authorization.request
	if command.Kind != engine.CommandDispatchVerifier || command.ID != prepared.commandID ||
		command.RunID != prepared.runID || command.RunID != request.RunID ||
		command.ExpectedRevision != prepared.stateRevision || command.ExpectedRevision != request.StateRevision {
		return errors.New("controlled verifier command does not match its preparation and permit")
	}
	payload, err := decodeExactControlledBuildJSON[engine.DispatchVerifierPayload](command.Payload, "work_id")
	if err != nil || payload.WorkID != prepared.workID || payload.WorkID != request.WorkID {
		return errors.New("controlled verifier payload does not match its exact preparation")
	}
	if derivedID("eff", command.ID, 0) != prepared.facts.DispatchID {
		return errors.New("controlled verifier dispatch identity is not derived from its command")
	}
	closure, err := s.loadExactReviewClosure(ctx, transaction, request.RunID, request.WorkID)
	if err != nil {
		return err
	}
	if closure.state.Revision != request.StateRevision || closure.work.Attempt != request.WorkAttempt ||
		closure.contract.Digest() != request.Contract.Digest() ||
		closure.work.SubmissionID != request.SubmissionID || closure.work.SubmissionDigest != request.SubmissionDigest ||
		closure.work.VerificationEpoch+1 != prepared.facts.VerificationEpoch {
		return errors.New("controlled verifier dispatch no longer matches current reviewable truth")
	}
	if err := validateVerifierRedispatch(ctx, transaction, closure.work); err != nil {
		return err
	}
	dispatch, err := protocol.ParseVerifierDispatch(prepared.record.CanonicalJSON)
	if err != nil || prepared.record.Kind != protocol.ControlReceiptSchemaVersion ||
		prepared.record.Digest != protocol.RawDigest(prepared.record.CanonicalJSON) ||
		dispatch.DispatchID != request.DispatchID || dispatch.SubmissionDigest != request.SubmissionDigest ||
		dispatch.Candidate != closure.submission.View().Candidate {
		return errors.New("prepared verifier dispatch does not match its exact current submission")
	}
	if err := authorization.ownership.ValidateActive(s, request.ControllerID); err != nil {
		return fmt.Errorf("revalidate verifier dispatch ownership: %w", err)
	}
	return s.validateControlledVerifierDispatchCapability(authorization)
}

func persistVerifierDispatch(
	ctx context.Context,
	transaction *sql.Tx,
	command engine.Command,
	prepared preparedVerifierDispatch,
	effectID string,
	now int64,
) error {
	record := cloneEncodedRecord(prepared.record)
	if effectID != prepared.facts.DispatchID || command.ID != prepared.commandID ||
		command.RunID != prepared.runID {
		return errors.New("persisted verifier dispatch identity differs from its preparation")
	}
	if err := putRecordTransaction(ctx, transaction, record, now, "verifier dispatch"); err != nil {
		return err
	}
	if err := putArtifact(ctx, transaction, record.Digest, "application/json", record.CanonicalJSON, now); err != nil {
		return fmt.Errorf("persist verifier dispatch artifact: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prepared.facts.DispatchID, record.Digest, record.Digest,
		prepared.facts.SubmissionID, prepared.facts.SubmissionDigest,
		effectID, prepared.facts.VerifierProfileDigest, command.RunID, command.ID,
		prepared.facts.VerificationEpoch, now,
	); err != nil {
		return fmt.Errorf("insert atomic verifier dispatch identity: %w", err)
	}
	return nil
}

func (s *Store) loadExactReviewClosure(
	ctx context.Context,
	transaction *sql.Tx,
	runID string,
	workID string,
) (exactReviewClosure, error) {
	state, found, err := loadState(ctx, transaction, runID)
	if err != nil {
		return exactReviewClosure{}, err
	}
	if !found || state.Phase != engine.PhaseActive {
		return exactReviewClosure{}, errors.New("verifier review requires an active delivery")
	}
	var work engine.Work
	foundWork := false
	for _, candidate := range state.Work {
		if candidate.ID == workID {
			work, foundWork = candidate, true
			break
		}
	}
	if !foundWork || (work.State != engine.WorkReviewable && work.State != engine.WorkRetry) ||
		!engine.ValidID(work.SubmissionID) || !engine.ValidDigest(work.SubmissionDigest) {
		return exactReviewClosure{}, errors.New("work is not reviewable or awaiting an exact verifier retry")
	}
	plan, err := loadExactPlan(ctx, transaction, state.PlanDigest)
	if err != nil {
		return exactReviewClosure{}, fmt.Errorf("load verifier delivery plan: %w", err)
	}
	target := plan.Target()
	if plan.DeliveryID() != state.DeliveryID || target.Repository != state.Repository ||
		target.Ref != state.TargetRef || !slices.Equal(plan.WorkIDs(), stateWorkIDsForBuildGate(state.Work)) {
		return exactReviewClosure{}, errors.New("verifier state does not match its exact plan")
	}
	contract, exists := plan.Work(workID)
	if !exists || contract.Digest() == "" {
		return exactReviewClosure{}, errors.New("verifier review lacks its exact work contract")
	}
	var submissionDigest string
	var submissionAttempt int64
	err = transaction.QueryRowContext(ctx, `
		SELECT digest, attempt FROM submission_records
		WHERE submission_id = ? AND run_id = ? AND delivery_id = ? AND work_id = ?`,
		work.SubmissionID, state.RunID, state.DeliveryID, workID,
	).Scan(&submissionDigest, &submissionAttempt)
	if errors.Is(err, sql.ErrNoRows) {
		return exactReviewClosure{}, errors.New("reviewable work lacks its atomic submission identity")
	}
	if err != nil {
		return exactReviewClosure{}, fmt.Errorf("load reviewable submission identity: %w", err)
	}
	kind, submissionBytes, err := loadRecord(ctx, transaction, submissionDigest)
	if err != nil {
		return exactReviewClosure{}, err
	}
	submission, err := protocol.ParseSubmission(submissionBytes)
	if err != nil {
		return exactReviewClosure{}, fmt.Errorf("parse reviewable submission: %w", err)
	}
	view := submission.View()
	if kind != protocol.SubmissionSchemaVersion || submission.Record().Digest != work.SubmissionDigest ||
		submissionDigest != work.SubmissionDigest || view.SubmissionID != work.SubmissionID ||
		view.DeliveryID != state.DeliveryID || view.WorkID != workID ||
		view.Attempt != work.Attempt || submissionAttempt != work.Attempt ||
		view.PlanDigest != state.PlanDigest || view.ContractDigest != contract.Digest() ||
		view.Candidate.Repository != state.Repository || view.Candidate.Commit != work.CandidateCommit {
		return exactReviewClosure{}, errors.New("reviewable submission does not match current engine and plan truth")
	}
	builder, err := (journalResultResolver{query: transaction}).SucceededEffect(ctx, view.Builder.RunID)
	if err != nil {
		return exactReviewClosure{}, fmt.Errorf("resolve reviewable submission builder: %w", err)
	}
	buildRequest, requestErr := engine.ParseBuildEffectRequest(builder.Request)
	buildResult, resultErr := engine.ParseBuildEffectResult(builder.Result)
	if requestErr != nil || resultErr != nil || builder.DeliveryRunID != state.RunID ||
		buildRequest.DeliveryID != state.DeliveryID || buildRequest.WorkID != workID ||
		buildRequest.WorkAttempt != work.Attempt || buildRequest.DispatchDigest != contract.Digest() ||
		buildResult.Candidate.RepositoryID != view.Candidate.Repository ||
		buildResult.Candidate.Commit != view.Candidate.Commit || buildResult.Candidate.Tree != view.Candidate.Tree ||
		!slices.Equal(buildResult.Candidate.ChangedPaths, view.ChangedPaths) {
		return exactReviewClosure{}, errors.New("reviewable submission candidate does not match its retained builder result")
	}
	if s.repository == nil || s.repository.Binding().RepositoryID != state.Repository {
		return exactReviewClosure{}, errors.New("verifier review requires the immutable configured repository")
	}
	if err := s.repository.VerifyCandidate(ctx, buildResult.Candidate, contract.View().Scope); err != nil {
		return exactReviewClosure{}, fmt.Errorf("verify retained review candidate: %w", err)
	}
	if _, err := protocol.ResolveExactLocalChecks(
		ctx, journalResultResolver{query: transaction}, plan, workID,
	); err != nil {
		return exactReviewClosure{}, fmt.Errorf("resolve verifier policy closure: %w", err)
	}
	return exactReviewClosure{
		state: state, work: work, plan: plan, contract: contract,
		submission: submission, candidate: buildResult.Candidate,
	}, nil
}

func validateVerifierRedispatch(ctx context.Context, query rowQuerier, work engine.Work) error {
	if work.VerificationDispatchID == "" {
		return nil
	}
	effect, err := loadEffect(ctx, query, work.VerificationDispatchID)
	if err != nil {
		return fmt.Errorf("load current verifier dispatch effect: %w", err)
	}
	if effect.Kind != string(engine.EffectVerifier) {
		return errors.New("current verification dispatch is not a verifier effect")
	}
	switch effect.State {
	case EffectFailed:
		return nil
	case EffectPending, EffectRunning:
		return errors.New("current verifier dispatch is still executable")
	case EffectUnknown:
		return errors.New("current verifier dispatch has an ambiguous interrupted model turn")
	case EffectSucceeded:
		if work.State != engine.WorkRetry || work.Verdict != engine.VerdictInconclusive {
			return errors.New("current verifier result must be admitted before another dispatch")
		}
		var count int
		err := query.QueryRowContext(ctx, `
			SELECT count(*) FROM verdict_records
			WHERE dispatch_id = ? AND verdict_id = ? AND digest = ? AND outcome = 'INCONCLUSIVE'`,
			work.VerificationDispatchID, work.VerdictID, work.VerdictDigest,
		).Scan(&count)
		if err != nil || count != 1 {
			return errors.New("verification retry lacks its exact admitted INCONCLUSIVE verdict")
		}
		return nil
	default:
		return errors.New("current verifier dispatch has an unsupported journal state")
	}
}

func validateCurrentVerifierPermitHead(
	ctx context.Context,
	query rowQuerier,
	permit policy.CurrentVerifierExecutionPermit,
) error {
	facts := permit.Facts()
	return validateCurrentAuthorityFactsHead(
		ctx, query, facts.SourceRef, facts.SourceVersion, facts.SourceDigest, "verifier execution",
	)
}

func validateCurrentAuthorityFactsHead(
	ctx context.Context,
	query rowQuerier,
	sourceRef string,
	sourceVersion int64,
	sourceDigest string,
	label string,
) error {
	var version int64
	var digest, status string
	err := query.QueryRowContext(ctx, `
		SELECT source_version, source_digest, status
		FROM authority_source_snapshots
		WHERE source_ref = ? ORDER BY source_version DESC LIMIT 1`, sourceRef,
	).Scan(&version, &digest, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("current %s permit has no durable authority source head", label)
	}
	if err != nil {
		return fmt.Errorf("load current authority source head: %w", err)
	}
	if status != "active" || version != sourceVersion || digest != sourceDigest {
		return fmt.Errorf("current %s permit was superseded in the control ledger", label)
	}
	return nil
}

func cloneEncodedRecord(record protocol.EncodedRecord) protocol.EncodedRecord {
	record.CanonicalJSON = bytes.Clone(record.CanonicalJSON)
	return record
}
