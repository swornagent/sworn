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

const passAuthorityAttentionCode = "pass_authority_stop"

// PreparedVerdictAdmission is a process-local, non-authorizing capability for
// one exact succeeded verifier result and deterministic admission command.
// PASS still requires a separately fresh CurrentPASSAdmissionPermit.
type PreparedVerdictAdmission struct {
	issuer           *leaseIssuer
	commandID        string
	runID            string
	workID           string
	stateRevision    int64
	record           protocol.EncodedRecord
	facts            engine.VerdictAdmissionFacts
	submissionID     string
	submissionDigest string
	dispatchID       string
	dispatchDigest   string
	effectID         string
	assessmentDigest string
	outcome          engine.VerdictOutcome
	reviewEpoch      int64
}

type preparedVerdictAdmission = PreparedVerdictAdmission

type controlledVerdictAdmissionAuthorization struct {
	ownership    *ControllerOwnership
	controllerID string
	prepared     PreparedVerdictAdmission
	authority    *policy.Authority
	permit       policy.CurrentPASSAdmissionPermit
	request      policy.PASSAdmissionPermitRequest
	requirePASS  bool
}

type controlledAttentionAuthorization struct {
	ownership    *ControllerOwnership
	controllerID string
	prepared     PreparedVerdictAdmission
	facts        engine.DeliveryAttentionFacts
}

// PrepareControlledVerdictAdmission reconstructs the exact succeeded
// assessment and engine-stamped Baton verdict without mutating Store state.
// For PASS it also returns the exact request which must be freshly authorized.
func (s *Store) PrepareControlledVerdictAdmission(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	runID string,
	workID string,
	commandID string,
) (engine.State, protocol.ExactPlan, policy.PASSAdmissionPermitRequest, PreparedVerdictAdmission, error) {
	zero := func(err error) (
		engine.State, protocol.ExactPlan, policy.PASSAdmissionPermitRequest, PreparedVerdictAdmission, error,
	) {
		return engine.State{}, protocol.ExactPlan{}, policy.PASSAdmissionPermitRequest{}, PreparedVerdictAdmission{}, err
	}
	if s == nil || s.readOnly || ownership == nil || !engine.ValidID(commandID) {
		return zero(errors.New("verdict preparation requires a writable owned Store and valid command id"))
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("validate verdict preparation ownership: %w", err))
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero(fmt.Errorf("begin verdict preparation: %w", err))
	}
	defer transaction.Rollback() //nolint:errcheck
	prepared, closure, err := s.deriveVerdictAdmission(ctx, transaction, runID, workID, commandID)
	if err != nil {
		return zero(err)
	}
	prepared.issuer = s.leaseIssuer
	prepared.stateRevision = closure.state.Revision
	var request policy.PASSAdmissionPermitRequest
	if prepared.outcome == engine.VerdictPass {
		request = policy.PASSAdmissionPermitRequest{
			ControllerID: controllerID, RunID: runID, StateRevision: closure.state.Revision,
			WorkID: workID, WorkAttempt: closure.work.Attempt, Contract: closure.contract,
			SubmissionID: prepared.submissionID, SubmissionDigest: prepared.submissionDigest,
			VerifierEffectID: prepared.effectID, DispatchID: prepared.dispatchID,
			DispatchDigest: prepared.dispatchDigest, AssessmentDigest: prepared.assessmentDigest,
			Outcome: string(engine.VerdictPass),
		}
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return zero(fmt.Errorf("revalidate verdict preparation ownership: %w", err))
	}
	if err := transaction.Commit(); err != nil {
		return zero(fmt.Errorf("finish verdict preparation: %w", err))
	}
	return closure.state, closure.plan, request, prepared, nil
}

// ApplyControlledVerdictAdmission admits only a non-PASS truthful assessment.
// It deliberately accepts no authority object: historical FAIL, SPEC_BLOCK,
// and INCONCLUSIVE results remain bankable after later authority loss.
func (s *Store) ApplyControlledVerdictAdmission(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	prepared PreparedVerdictAdmission,
	command engine.Command,
) (ApplyResult, error) {
	authorization := controlledVerdictAdmissionAuthorization{
		ownership: ownership, controllerID: controllerID, prepared: prepared,
	}
	if err := s.validateControlledVerdictCapability(authorization); err != nil {
		return ApplyResult{}, err
	}
	return s.applyCommand(ctx, command, &authorization)
}

// ApplyControlledPASSAdmission admits PASS only with a separately fresh
// current-authority capability bound to the exact assessment and dispatch.
func (s *Store) ApplyControlledPASSAdmission(
	ctx context.Context,
	ownership *ControllerOwnership,
	authority *policy.Authority,
	permit policy.CurrentPASSAdmissionPermit,
	request policy.PASSAdmissionPermitRequest,
	prepared PreparedVerdictAdmission,
	command engine.Command,
) (ApplyResult, error) {
	authorization := controlledVerdictAdmissionAuthorization{
		ownership: ownership, controllerID: request.ControllerID, prepared: prepared,
		authority: authority, permit: permit, request: request, requirePASS: true,
	}
	if err := s.validateControlledVerdictCapability(authorization); err != nil {
		return ApplyResult{}, err
	}
	return s.applyCommand(ctx, command, &authorization)
}

func (s *Store) validateControlledVerdictCapability(
	authorization controlledVerdictAdmissionAuthorization,
) error {
	prepared := authorization.prepared
	if s == nil || authorization.ownership == nil || prepared.issuer == nil ||
		prepared.issuer != s.leaseIssuer || !engine.ValidID(authorization.controllerID) {
		return errors.New("controlled verdict admission requires Store ownership and exact preparation")
	}
	if err := authorization.ownership.ValidateActive(s, authorization.controllerID); err != nil {
		return fmt.Errorf("validate verdict admission ownership: %w", err)
	}
	if prepared.outcome == engine.VerdictPass {
		if !authorization.requirePASS || authorization.authority == nil {
			return errors.New("PASS verdict admission requires fresh current authority")
		}
		if err := authorization.authority.RequireLedger(s); err != nil {
			return fmt.Errorf("validate PASS authority ledger: %w", err)
		}
		if err := authorization.authority.ValidatePASSAdmissionPermit(
			authorization.permit, authorization.request,
		); err != nil {
			return fmt.Errorf("validate current PASS admission permit: %w", err)
		}
		request := authorization.request
		if request.ControllerID != authorization.controllerID || request.RunID != prepared.runID ||
			request.StateRevision != prepared.stateRevision || request.WorkID != prepared.workID ||
			request.SubmissionID != prepared.submissionID || request.SubmissionDigest != prepared.submissionDigest ||
			request.VerifierEffectID != prepared.effectID || request.DispatchID != prepared.dispatchID ||
			request.DispatchDigest != prepared.dispatchDigest ||
			request.AssessmentDigest != prepared.assessmentDigest || request.Outcome != string(engine.VerdictPass) {
			return errors.New("PASS permit does not match the exact prepared verdict")
		}
	} else if authorization.requirePASS || authorization.authority != nil {
		return errors.New("non-PASS verdict must not consume PASS authority")
	}
	return nil
}

func (s *Store) prepareVerdictAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledVerdictAdmissionAuthorization,
	command engine.Command,
) (*preparedVerdictAdmission, error) {
	if err := s.validateControlledVerdictCapability(authorization); err != nil {
		return nil, err
	}
	prepared := authorization.prepared
	if command.Kind != engine.CommandAdmitVerdict || command.ID != prepared.commandID ||
		command.RunID != prepared.runID || command.ExpectedRevision != prepared.stateRevision {
		return nil, errors.New("verdict admission command does not match its exact preparation")
	}
	payload, err := decodeExactControlledBuildJSON[engine.AdmitVerdictPayload](command.Payload, "work_id")
	if err != nil || payload.WorkID != prepared.workID {
		return nil, errors.New("verdict admission payload does not match its exact preparation")
	}
	current, closure, err := s.deriveVerdictAdmission(
		ctx, transaction, prepared.runID, prepared.workID, prepared.commandID,
	)
	if err != nil {
		return nil, err
	}
	if current.stateRevision != prepared.stateRevision || current.outcome != prepared.outcome ||
		current.facts != prepared.facts || current.submissionID != prepared.submissionID ||
		current.submissionDigest != prepared.submissionDigest || current.dispatchID != prepared.dispatchID ||
		current.dispatchDigest != prepared.dispatchDigest || current.effectID != prepared.effectID ||
		current.assessmentDigest != prepared.assessmentDigest || current.reviewEpoch != prepared.reviewEpoch ||
		current.record.Kind != prepared.record.Kind || current.record.Digest != prepared.record.Digest ||
		!bytes.Equal(current.record.CanonicalJSON, prepared.record.CanonicalJSON) {
		return nil, errors.New("prepared verdict no longer matches current durable review truth")
	}
	if authorization.requirePASS {
		if closure.contract.Digest() != authorization.request.Contract.Digest() ||
			closure.work.Attempt != authorization.request.WorkAttempt {
			return nil, errors.New("PASS admission no longer matches its exact work contract")
		}
		if err := validateCurrentPASSPermitHead(ctx, transaction, authorization.permit); err != nil {
			return nil, err
		}
		if err := s.validatePASSArtifactClosure(ctx, transaction, closure); err != nil {
			return nil, err
		}
	}
	if err := authorization.ownership.ValidateActive(s, authorization.controllerID); err != nil {
		return nil, fmt.Errorf("revalidate verdict admission ownership: %w", err)
	}
	if err := s.validateControlledVerdictCapability(authorization); err != nil {
		return nil, err
	}
	current.issuer = s.leaseIssuer
	return &current, nil
}

func (s *Store) deriveVerdictAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	runID string,
	workID string,
	commandID string,
) (PreparedVerdictAdmission, exactReviewClosure, error) {
	closure, err := s.loadExactReviewClosure(ctx, transaction, runID, workID)
	if err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{}, err
	}
	effect, dispatchDigest, err := s.loadHistoricalVerifierEffect(
		ctx, transaction, closure, closure.work.VerificationDispatchID, EffectSucceeded,
	)
	if err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{}, err
	}
	if err := validateBoundEffectResult(
		ctx, journalResultResolver{query: transaction}, effect, effect.Result,
	); err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{}, err
	}
	request, _ := engine.ParseVerifierEffectRequest(effect.Request)
	result, _ := engine.ParseVerifierEffectResult(effect.Result)
	if !journalContains(effect, result.StartedAt, result.CompletedAt) {
		return PreparedVerdictAdmission{}, exactReviewClosure{},
			errors.New("verifier review timestamps fall outside its journal lease")
	}
	assessmentBytes, err := protocol.ResolveArtifact(
		ctx, journalResultResolver{query: transaction}, result.Assessment,
		protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{}, err
	}
	assessment, err := protocol.ParseVerifierAssessment(assessmentBytes)
	if err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{}, err
	}
	assessmentRecord := assessment.Record()
	kind, canonical, err := loadRecord(ctx, transaction, assessmentRecord.Digest)
	if err != nil || kind != assessmentRecord.Kind || !bytes.Equal(canonical, assessmentRecord.CanonicalJSON) {
		return PreparedVerdictAdmission{}, exactReviewClosure{},
			errors.New("verifier assessment canonical record is unavailable or invalid")
	}
	dispatchMedia, dispatchBytes, err := loadArtifact(ctx, transaction, dispatchDigest)
	if err != nil || dispatchMedia != "application/json" {
		return PreparedVerdictAdmission{}, exactReviewClosure{},
			errors.New("verifier dispatch artifact is unavailable or invalid")
	}
	dispatchPointer := protocol.Artifact{
		Ref: dispatchDigest, MediaType: "application/json", Digest: dispatchDigest,
	}
	verdictID := derivedID("verdict", commandID, 0)
	record, err := protocol.BuildDeliveryVerdict(
		protocol.VerdictBindingInput{
			Plan: closure.plan, Submission: closure.submission,
			DispatchReceipt: dispatchPointer, Dispatch: dispatchBytes,
		},
		protocol.VerdictStamp{
			VerdictID: verdictID, Agent: request.Agent,
			StartedAt: result.StartedAt, CompletedAt: result.CompletedAt,
		},
		assessment,
	)
	if err != nil {
		return PreparedVerdictAdmission{}, exactReviewClosure{},
			fmt.Errorf("construct exact delivery verdict: %w", err)
	}
	outcome := engine.VerdictOutcome(assessment.View().Outcome)
	prepared := PreparedVerdictAdmission{
		commandID: commandID, runID: runID, workID: workID, stateRevision: closure.state.Revision,
		record: cloneEncodedRecord(record),
		facts: engine.VerdictAdmissionFacts{
			DispatchID: effect.ID, VerificationEpoch: request.VerificationEpoch,
			VerdictBinding: engine.VerdictBinding{
				VerdictID: verdictID, VerdictDigest: record.Digest, Verdict: outcome,
			},
		},
		submissionID: closure.work.SubmissionID, submissionDigest: closure.work.SubmissionDigest,
		dispatchID: effect.ID, dispatchDigest: dispatchDigest, effectID: effect.ID,
		assessmentDigest: assessmentRecord.Digest, outcome: outcome, reviewEpoch: request.VerificationEpoch,
	}
	return prepared, closure, nil
}

func persistVerdictAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	command engine.Command,
	eventID string,
	eventRevision int64,
	prepared preparedVerdictAdmission,
	now int64,
) error {
	if command.ID != prepared.commandID || command.RunID != prepared.runID ||
		eventRevision <= prepared.stateRevision {
		return errors.New("persisted verdict identity differs from its preparation")
	}
	if err := putRecordTransaction(ctx, transaction, prepared.record, now, "delivery verdict"); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO verdict_records (
			verdict_id, digest, submission_id, submission_digest, dispatch_id,
			verifier_effect_id, assessment_digest, outcome, run_id, command_id,
			event_id, event_revision, review_epoch, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prepared.facts.VerdictID, prepared.record.Digest,
		prepared.submissionID, prepared.submissionDigest, prepared.dispatchID,
		prepared.effectID, prepared.assessmentDigest, prepared.outcome,
		command.RunID, command.ID, eventID, eventRevision, prepared.reviewEpoch, now,
	); err != nil {
		return fmt.Errorf("insert atomic verdict identity: %w", err)
	}
	return nil
}

func (s *Store) validatePASSArtifactClosure(
	ctx context.Context,
	transaction *sql.Tx,
	closure exactReviewClosure,
) error {
	resolver := journalResultResolver{query: transaction}
	if _, err := protocol.ResolveExactLocalChecks(ctx, resolver, closure.plan, closure.work.ID); err != nil {
		return fmt.Errorf("resolve PASS policy closure: %w", err)
	}
	submission := closure.submission.View()
	pointers := []protocol.Artifact{submission.AuthorityReceipt}
	for _, check := range submission.Checks {
		if check.Outcome != "pass" {
			return fmt.Errorf("PASS submission contains non-passing check %q", check.ID)
		}
		pointers = append(pointers, check.Receipt)
	}
	for _, evidence := range submission.Evidence {
		pointers = append(pointers, evidence.Artifact)
	}
	for _, pointer := range pointers {
		if pointer.Ref != pointer.Digest {
			return errors.New("PASS artifact closure contains a non-CAS pointer")
		}
		mediaType, _, err := loadArtifact(ctx, transaction, pointer.Digest)
		if err != nil || mediaType != pointer.MediaType {
			return fmt.Errorf("PASS artifact %q is unavailable or has the wrong media type", pointer.Digest)
		}
	}
	return nil
}

func validateCurrentPASSPermitHead(
	ctx context.Context,
	query rowQuerier,
	permit policy.CurrentPASSAdmissionPermit,
) error {
	facts := permit.Facts()
	return validateCurrentAuthorityFactsHead(
		ctx, query, facts.SourceRef, facts.SourceVersion, facts.SourceDigest, "PASS admission",
	)
}

// ApplyControlledPASSAttention records the mandatory delivery-level attention
// latch after a current-authority/control stop prevented PASS admission. It
// preserves the reviewable row and creates no verdict or effect.
func (s *Store) ApplyControlledPASSAttention(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	prepared PreparedVerdictAdmission,
	command engine.Command,
) (ApplyResult, error) {
	if prepared.outcome != engine.VerdictPass {
		return ApplyResult{}, errors.New("PASS attention requires an exact prepared PASS assessment")
	}
	authorization := controlledAttentionAuthorization{
		ownership: ownership, controllerID: controllerID, prepared: prepared,
		facts: engine.DeliveryAttentionFacts{
			Message: controlledPASSAttentionMessage,
		},
	}
	if err := s.validateControlledAttentionCapability(authorization); err != nil {
		return ApplyResult{}, err
	}
	return s.applyCommand(ctx, command, &authorization)
}

func (s *Store) validateControlledAttentionCapability(authorization controlledAttentionAuthorization) error {
	if s == nil || authorization.ownership == nil || authorization.prepared.issuer == nil ||
		authorization.prepared.issuer != s.leaseIssuer ||
		authorization.prepared.outcome != engine.VerdictPass ||
		!protocol.ValidNonEmpty(authorization.facts.Message) {
		return errors.New("controlled PASS attention requires exact Store preparation and ownership")
	}
	return authorization.ownership.ValidateActive(s, authorization.controllerID)
}

func (s *Store) validateControlledAttentionTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	authorization controlledAttentionAuthorization,
	command engine.Command,
) error {
	if err := s.validateControlledAttentionCapability(authorization); err != nil {
		return err
	}
	prepared := authorization.prepared
	if command.Kind != engine.CommandRaiseDeliveryAttention || command.RunID != prepared.runID ||
		command.ExpectedRevision != prepared.stateRevision || command.ID == prepared.commandID {
		return errors.New("PASS attention command does not match its exact prepared state")
	}
	payload, err := decodeExactControlledBuildJSON[engine.RaiseDeliveryAttentionPayload](
		command.Payload, "work_id", "code",
	)
	if err != nil || payload.WorkID != prepared.workID || payload.Code != passAuthorityAttentionCode {
		return errors.New("PASS attention payload does not match its exact control stop")
	}
	current, _, err := s.deriveVerdictAdmission(
		ctx, transaction, prepared.runID, prepared.workID, prepared.commandID,
	)
	if err != nil || current.outcome != engine.VerdictPass ||
		current.record.Digest != prepared.record.Digest || current.effectID != prepared.effectID {
		return errors.New("PASS attention no longer matches the current exact assessment")
	}
	return authorization.ownership.ValidateActive(s, authorization.controllerID)
}

// PASSAttentionCommand builds the only accepted intent payload for
// ApplyControlledPASSAttention. The message remains Store-derived.
func PASSAttentionCommand(commandID, runID, workID string, expectedRevision int64) (engine.Command, error) {
	payload, err := protocol.EncodeCanonical(engine.RaiseDeliveryAttentionPayload{
		WorkID: workID, Code: passAuthorityAttentionCode,
	})
	if err != nil {
		return engine.Command{}, err
	}
	return engine.Command{
		ID: commandID, RunID: runID, Kind: engine.CommandRaiseDeliveryAttention,
		ExpectedRevision: expectedRevision, Payload: json.RawMessage(payload),
	}, nil
}
