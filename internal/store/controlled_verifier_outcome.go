package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
)

const controlledPASSAttentionMessage = "current authority or control state prevented PASS admission"

// Convergence selectors retain durable command intent plus the logical
// submission and verification epoch. Dynamic digests, verifier configuration,
// assessment, outcome, and Baton identities are reconstructed from the
// occupied command's immutable journal.
type ControlledVerifierDispatchSelector struct {
	ControllerID      string
	CommandID         string
	RunID             string
	WorkID            string
	SubmissionID      string
	VerificationEpoch int64
}

type ControlledVerdictAdmissionSelector struct {
	ControllerID      string
	CommandID         string
	RunID             string
	WorkID            string
	SubmissionID      string
	VerificationEpoch int64
}

type ControlledPASSAttentionSelector struct {
	ControllerID      string
	CommandID         string
	RunID             string
	WorkID            string
	SubmissionID      string
	VerificationEpoch int64
}

// ConvergenceSelector is a convenience for retaining durable dispatch intent
// before apply. It exports no authority or process-local capability.
func (prepared PreparedVerifierDispatch) ConvergenceSelector(
	controllerID string,
) (ControlledVerifierDispatchSelector, error) {
	selector := ControlledVerifierDispatchSelector{
		ControllerID: controllerID, CommandID: prepared.commandID,
		RunID: prepared.runID, WorkID: prepared.workID,
		SubmissionID: prepared.facts.SubmissionID, VerificationEpoch: prepared.facts.VerificationEpoch,
	}
	if !validConvergenceIntent(
		controllerID, prepared.commandID, prepared.runID, prepared.workID,
		prepared.facts.SubmissionID, prepared.facts.VerificationEpoch,
	) {
		return ControlledVerifierDispatchSelector{}, errors.New("prepared verifier dispatch has no valid convergence intent")
	}
	return selector, nil
}

// ConvergenceSelector is a convenience for retaining durable verdict intent
// before apply. The actual outcome is deliberately absent and is recovered
// from the committed assessment and verdict event.
func (prepared PreparedVerdictAdmission) ConvergenceSelector(
	controllerID string,
) (ControlledVerdictAdmissionSelector, error) {
	selector := ControlledVerdictAdmissionSelector{
		ControllerID: controllerID, CommandID: prepared.commandID,
		RunID: prepared.runID, WorkID: prepared.workID,
		SubmissionID: prepared.submissionID, VerificationEpoch: prepared.reviewEpoch,
	}
	if !validConvergenceIntent(
		controllerID, prepared.commandID, prepared.runID, prepared.workID,
		prepared.submissionID, prepared.reviewEpoch,
	) {
		return ControlledVerdictAdmissionSelector{}, errors.New("prepared verdict has no valid convergence intent")
	}
	return selector, nil
}

// PASSAttentionConvergenceSelector retains the distinct durable attention
// command intent while proving locally that the preparation describes PASS.
func (prepared PreparedVerdictAdmission) PASSAttentionConvergenceSelector(
	controllerID string,
	attentionCommandID string,
) (ControlledPASSAttentionSelector, error) {
	if _, err := prepared.ConvergenceSelector(controllerID); err != nil ||
		prepared.outcome != engine.VerdictPass || !engine.ValidID(attentionCommandID) ||
		attentionCommandID == prepared.commandID {
		return ControlledPASSAttentionSelector{}, errors.New("prepared PASS verdict has no valid attention convergence intent")
	}
	return ControlledPASSAttentionSelector{
		ControllerID: controllerID, CommandID: attentionCommandID,
		RunID: prepared.runID, WorkID: prepared.workID,
		SubmissionID: prepared.submissionID, VerificationEpoch: prepared.reviewEpoch,
	}, nil
}

type convergedAppliedCommand struct {
	command engine.Command
	result  ApplyResult
}

type historicalVerifierDispatch struct {
	command convergedAppliedCommand
	effect  Effect
	request engine.VerifierEffectRequest
}

func (s *Store) ConvergeControlledVerifierDispatch(
	ctx context.Context,
	ownership *ControllerOwnership,
	selector ControlledVerifierDispatchSelector,
) (ApplyResult, bool, error) {
	if !s.validConvergenceSelector(
		selector.ControllerID, selector.CommandID, selector.RunID, selector.WorkID,
		selector.SubmissionID, selector.VerificationEpoch,
	) {
		return ApplyResult{}, false, errors.New("controlled verifier convergence selector is invalid")
	}
	return s.convergeControlledOutcome(ctx, ownership, selector.ControllerID, "verifier dispatch",
		func(transaction *sql.Tx) (ApplyResult, bool, error) {
			dispatch, found, err := s.loadHistoricalVerifierDispatch(ctx, transaction, selector)
			return dispatch.command.result, found, err
		})
}

func (s *Store) ConvergeControlledVerdictAdmission(
	ctx context.Context,
	ownership *ControllerOwnership,
	selector ControlledVerdictAdmissionSelector,
) (ApplyResult, bool, error) {
	if !s.validConvergenceSelector(
		selector.ControllerID, selector.CommandID, selector.RunID, selector.WorkID,
		selector.SubmissionID, selector.VerificationEpoch,
	) {
		return ApplyResult{}, false, errors.New("controlled verdict convergence selector is invalid")
	}
	return s.convergeControlledOutcome(ctx, ownership, selector.ControllerID, "verdict admission",
		func(transaction *sql.Tx) (ApplyResult, bool, error) {
			return s.loadControlledVerdictAdmissionOutcome(ctx, transaction, selector)
		})
}

func (s *Store) ConvergeControlledPASSAttention(
	ctx context.Context,
	ownership *ControllerOwnership,
	selector ControlledPASSAttentionSelector,
) (ApplyResult, bool, error) {
	if !s.validConvergenceSelector(
		selector.ControllerID, selector.CommandID, selector.RunID, selector.WorkID,
		selector.SubmissionID, selector.VerificationEpoch,
	) {
		return ApplyResult{}, false, errors.New("controlled PASS attention convergence selector is invalid")
	}
	return s.convergeControlledOutcome(ctx, ownership, selector.ControllerID, "PASS attention",
		func(transaction *sql.Tx) (ApplyResult, bool, error) {
			return s.loadControlledPASSAttentionOutcome(ctx, transaction, selector)
		})
}

func (s *Store) validConvergenceSelector(
	controllerID, commandID, runID, workID, submissionID string,
	verificationEpoch int64,
) bool {
	return s != nil && !s.readOnly &&
		validConvergenceIntent(controllerID, commandID, runID, workID, submissionID, verificationEpoch)
}

func validConvergenceIntent(
	controllerID, commandID, runID, workID, submissionID string,
	verificationEpoch int64,
) bool {
	return engine.ValidID(controllerID) && engine.ValidID(commandID) &&
		engine.ValidID(runID) && engine.ValidID(workID) && engine.ValidID(submissionID) &&
		verificationEpoch >= 1 && verificationEpoch <= engine.MaximumVerificationEpoch
}

func (s *Store) convergeControlledOutcome(
	ctx context.Context,
	ownership *ControllerOwnership,
	controllerID string,
	label string,
	load func(*sql.Tx) (ApplyResult, bool, error),
) (ApplyResult, bool, error) {
	if ownership == nil {
		return ApplyResult{}, false, ErrInvalidControllerOwnership
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return ApplyResult{}, false, fmt.Errorf("validate controlled %s convergence ownership: %w", label, err)
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("begin controlled %s convergence: %w", label, err)
	}
	defer transaction.Rollback() //nolint:errcheck
	result, found, err := load(transaction)
	if err != nil {
		return ApplyResult{}, false, err
	}
	if err := ownership.ValidateActive(s, controllerID); err != nil {
		return ApplyResult{}, false, fmt.Errorf("revalidate controlled %s convergence ownership: %w", label, err)
	}
	if err := transaction.Commit(); err != nil {
		return ApplyResult{}, false, fmt.Errorf("finish controlled %s convergence: %w", label, err)
	}
	if !found {
		return ApplyResult{}, false, nil
	}
	result.Replayed = true
	return result, true, nil
}

func loadConvergedAppliedCommand(
	ctx context.Context,
	transaction *sql.Tx,
	commandID string,
) (convergedAppliedCommand, bool, error) {
	var runID, kind, requestDigest, outcome string
	var expectedRevision int64
	var requestJSON, resultJSON []byte
	var errorCode, errorMessage sql.NullString
	err := transaction.QueryRowContext(ctx, `
		SELECT run_id, kind, expected_revision, request_digest, request_json,
		       outcome, result_json, error_code, error_message
		FROM commands WHERE command_id = ?`, commandID,
	).Scan(
		&runID, &kind, &expectedRevision, &requestDigest, &requestJSON,
		&outcome, &resultJSON, &errorCode, &errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return convergedAppliedCommand{}, false, nil
	}
	if err != nil {
		return convergedAppliedCommand{}, false, fmt.Errorf("load converged command %q: %w", commandID, err)
	}
	command, err := decodeExactControlledBuildJSON[engine.Command](
		requestJSON, "id", "run_id", "kind", "expected_revision", "payload",
	)
	if err != nil || command.ID != commandID || command.RunID != runID ||
		string(command.Kind) != kind || command.ExpectedRevision != expectedRevision ||
		expectedRevision < 0 || requestDigest != commandDigest(command) {
		return convergedAppliedCommand{}, false, fmt.Errorf(
			"%w: command %q does not match its durable request", ErrIdempotencyConflict, commandID,
		)
	}
	result, err := decodeExactControlledBuildJSON[ApplyResult](resultJSON)
	if err != nil || outcome != string(OutcomeApplied) || errorCode.Valid || errorMessage.Valid ||
		result.CommandID != commandID || result.RunID != runID || result.Outcome != OutcomeApplied ||
		result.Replayed || expectedRevision == math.MaxInt64 || result.Revision != expectedRevision+1 ||
		result.EventID != derivedID("evt", commandID, 0) || result.ErrorCode != "" || result.ErrorMessage != "" {
		return convergedAppliedCommand{}, false, errors.New("converged command result is incomplete or inconsistent")
	}
	return convergedAppliedCommand{command: command, result: result}, true, nil
}

func loadConvergedEvent(
	ctx context.Context,
	transaction *sql.Tx,
	command convergedAppliedCommand,
	kind string,
) ([]byte, error) {
	var eventID, runID, eventKind string
	var revision, ordinal int64
	var data []byte
	err := transaction.QueryRowContext(ctx, `
		SELECT event_id, run_id, revision, ordinal, kind, data_json
		FROM events WHERE command_id = ? AND ordinal = 0`, command.command.ID,
	).Scan(&eventID, &runID, &revision, &ordinal, &eventKind, &data)
	if err != nil || eventID != command.result.EventID || runID != command.command.RunID ||
		revision != command.result.Revision || ordinal != 0 || eventKind != kind {
		return nil, fmt.Errorf("converged %s event is incomplete or inconsistent", kind)
	}
	return data, nil
}

func (s *Store) loadHistoricalVerifierDispatch(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledVerifierDispatchSelector,
) (historicalVerifierDispatch, bool, error) {
	command, found, err := loadConvergedAppliedCommand(ctx, transaction, selector.CommandID)
	if err != nil || !found {
		return historicalVerifierDispatch{}, found, err
	}
	if command.command.RunID != selector.RunID || command.command.Kind != engine.CommandDispatchVerifier {
		return historicalVerifierDispatch{}, false, fmt.Errorf(
			"%w: command %q does not match its verifier selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	payload, err := decodeExactControlledBuildJSON[engine.DispatchVerifierPayload](
		command.command.Payload, "work_id",
	)
	if err != nil || payload.WorkID != selector.WorkID {
		return historicalVerifierDispatch{}, false, fmt.Errorf(
			"%w: command %q does not match its verifier selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	dispatchID := derivedID("eff", selector.CommandID, 0)
	if !slices.Equal(command.result.EffectIDs, []string{dispatchID}) {
		return historicalVerifierDispatch{}, false, errors.New("verifier dispatch result lacks its exact effect")
	}
	eventJSON, err := loadConvergedEvent(ctx, transaction, command, "verifier.dispatched")
	if err != nil {
		return historicalVerifierDispatch{}, false, err
	}
	event, err := decodeExactControlledBuildJSON[struct {
		engine.DispatchVerifierPayload
		engine.VerifierDispatchFacts
	}](eventJSON,
		"work_id", "plan_digest", "submission_id", "submission_digest", "candidate",
		"dispatch_id", "dispatch_receipt", "verifier_profile_digest", "agent", "verification_epoch",
	)
	if err != nil || event.WorkID != selector.WorkID || event.DispatchID != dispatchID ||
		event.SubmissionID != selector.SubmissionID || event.VerificationEpoch != selector.VerificationEpoch ||
		event.DispatchReceipt.Ref != event.DispatchReceipt.Digest ||
		event.DispatchReceipt.MediaType != "application/json" {
		return historicalVerifierDispatch{}, false, errors.New("verifier dispatch event facts are inconsistent")
	}
	effect, err := loadEffect(ctx, transaction, dispatchID)
	if err != nil || effect.DeliveryRunID != selector.RunID || effect.CommandID != selector.CommandID ||
		effect.Ordinal != 0 || effect.Kind != string(engine.EffectVerifier) {
		return historicalVerifierDispatch{}, false, errors.New("verifier dispatch effect is inconsistent")
	}
	request, err := engine.ParseVerifierEffectRequest(effect.Request)
	if err != nil || request.DeliveryRunID != selector.RunID || request.WorkID != selector.WorkID ||
		request.PlanDigest != event.PlanDigest || request.SubmissionID != event.SubmissionID ||
		request.SubmissionDigest != event.SubmissionDigest || request.Candidate != event.Candidate ||
		request.DispatchID != dispatchID || request.DispatchReceipt != event.DispatchReceipt ||
		request.VerifierProfileDigest != event.VerifierProfileDigest || request.Agent != event.Agent ||
		request.VerificationEpoch != event.VerificationEpoch ||
		request.VerificationEpoch < 1 || request.VerificationEpoch > engine.MaximumVerificationEpoch {
		return historicalVerifierDispatch{}, false, errors.New("verifier effect request does not match its event")
	}
	var digest, artifactDigest, submissionID, submissionDigest, effectID, profileDigest, runID, commandID string
	var reviewEpoch int64
	err = transaction.QueryRowContext(ctx, `
		SELECT digest, artifact_digest, submission_id, submission_digest,
		       effect_id, profile_digest, run_id, command_id, review_epoch
		FROM verifier_dispatch_records WHERE dispatch_id = ?`, dispatchID,
	).Scan(
		&digest, &artifactDigest, &submissionID, &submissionDigest,
		&effectID, &profileDigest, &runID, &commandID, &reviewEpoch,
	)
	if err != nil || digest != artifactDigest || digest != request.DispatchReceipt.Digest ||
		submissionID != request.SubmissionID || submissionDigest != request.SubmissionDigest ||
		effectID != dispatchID || profileDigest != request.VerifierProfileDigest ||
		runID != selector.RunID || commandID != selector.CommandID || reviewEpoch != request.VerificationEpoch {
		return historicalVerifierDispatch{}, false, errors.New("verifier dispatch identity is inconsistent")
	}
	return historicalVerifierDispatch{command: command, effect: effect, request: request}, true, nil
}

func (s *Store) loadControlledVerdictAdmissionOutcome(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledVerdictAdmissionSelector,
) (ApplyResult, bool, error) {
	command, found, err := loadConvergedAppliedCommand(ctx, transaction, selector.CommandID)
	if err != nil || !found {
		return ApplyResult{}, found, err
	}
	if command.command.RunID != selector.RunID || command.command.Kind != engine.CommandAdmitVerdict {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its verdict selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	payload, err := decodeExactControlledBuildJSON[engine.AdmitVerdictPayload](command.command.Payload, "work_id")
	if err != nil || payload.WorkID != selector.WorkID {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its verdict selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	if len(command.result.EffectIDs) != 0 {
		return ApplyResult{}, false, errors.New("verdict admission result unexpectedly contains effects")
	}
	eventJSON, err := loadConvergedEvent(ctx, transaction, command, "verdict.admitted")
	if err != nil {
		return ApplyResult{}, false, err
	}
	event, err := decodeExactControlledBuildJSON[struct {
		engine.AdmitVerdictPayload
		engine.VerdictAdmissionFacts
	}](eventJSON, "work_id", "dispatch_id", "verification_epoch", "verdict_id", "verdict_digest", "verdict")
	if err != nil || event.WorkID != selector.WorkID || event.VerdictID != derivedID("verdict", selector.CommandID, 0) ||
		!validConvergedVerdictOutcome(event.Verdict) {
		return ApplyResult{}, false, errors.New("verdict admission event facts are inconsistent")
	}
	var verdictID, verdictDigest, submissionID, submissionDigest, dispatchID string
	var effectID, assessmentDigest, outcome, runID, commandID, eventID string
	var eventRevision, reviewEpoch int64
	err = transaction.QueryRowContext(ctx, `
		SELECT verdict_id, digest, submission_id, submission_digest, dispatch_id,
		       verifier_effect_id, assessment_digest, outcome, run_id, command_id,
		       event_id, event_revision, review_epoch
		FROM verdict_records WHERE command_id = ?`, selector.CommandID,
	).Scan(
		&verdictID, &verdictDigest, &submissionID, &submissionDigest, &dispatchID,
		&effectID, &assessmentDigest, &outcome, &runID, &commandID,
		&eventID, &eventRevision, &reviewEpoch,
	)
	if err != nil || verdictID != event.VerdictID || verdictDigest != event.VerdictDigest ||
		dispatchID != event.DispatchID || effectID != event.DispatchID || outcome != string(event.Verdict) ||
		runID != selector.RunID || commandID != selector.CommandID || eventID != command.result.EventID ||
		eventRevision != command.result.Revision || reviewEpoch != event.VerificationEpoch {
		return ApplyResult{}, false, errors.New("verdict identity is inconsistent")
	}
	var dispatchCommandID string
	if err := transaction.QueryRowContext(ctx,
		"SELECT command_id FROM verifier_dispatch_records WHERE dispatch_id = ?", dispatchID,
	).Scan(&dispatchCommandID); err != nil {
		return ApplyResult{}, false, errors.New("verdict lacks its dispatch identity")
	}
	dispatch, dispatchFound, err := s.loadHistoricalVerifierDispatch(ctx, transaction, ControlledVerifierDispatchSelector{
		ControllerID: selector.ControllerID, CommandID: dispatchCommandID,
		RunID: selector.RunID, WorkID: selector.WorkID,
		SubmissionID: selector.SubmissionID, VerificationEpoch: selector.VerificationEpoch,
	})
	if err != nil || !dispatchFound {
		if err == nil {
			err = errors.New("verdict lacks its historical dispatch")
		}
		return ApplyResult{}, false, err
	}
	if dispatch.command.result.Revision >= command.result.Revision ||
		dispatch.effect.State != EffectSucceeded || dispatch.request.DispatchID != dispatchID ||
		dispatch.request.SubmissionID != submissionID || dispatch.request.SubmissionDigest != submissionDigest ||
		dispatch.request.SubmissionID != selector.SubmissionID ||
		dispatch.request.VerificationEpoch != reviewEpoch || reviewEpoch != selector.VerificationEpoch {
		return ApplyResult{}, false, errors.New("verdict does not match its succeeded verifier dispatch")
	}
	assessment, _, err := loadHistoricalVerifierAssessment(
		ctx, transaction, dispatch.effect, assessmentDigest,
	)
	if err != nil || engine.VerdictOutcome(assessment.View().Outcome) != event.Verdict {
		return ApplyResult{}, false, errors.New("verdict does not match its exact assessment")
	}
	return command.result, true, nil
}

func (s *Store) loadControlledPASSAttentionOutcome(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledPASSAttentionSelector,
) (ApplyResult, bool, error) {
	command, found, err := loadConvergedAppliedCommand(ctx, transaction, selector.CommandID)
	if err != nil || !found {
		return ApplyResult{}, found, err
	}
	if command.command.RunID != selector.RunID || command.command.Kind != engine.CommandRaiseDeliveryAttention {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its PASS attention selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	payload, err := decodeExactControlledBuildJSON[engine.RaiseDeliveryAttentionPayload](
		command.command.Payload, "work_id", "code",
	)
	if err != nil || payload.WorkID != selector.WorkID || payload.Code != passAuthorityAttentionCode ||
		len(command.result.EffectIDs) != 0 {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its PASS attention selector", ErrIdempotencyConflict, selector.CommandID,
		)
	}
	eventJSON, err := loadConvergedEvent(ctx, transaction, command, "delivery.attention_raised")
	if err != nil {
		return ApplyResult{}, false, err
	}
	event, err := decodeExactControlledBuildJSON[struct {
		WorkID  string `json:"work_id"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}](eventJSON, "work_id", "code", "message")
	if err != nil || event.WorkID != selector.WorkID || event.Code != passAuthorityAttentionCode ||
		event.Message != controlledPASSAttentionMessage {
		return ApplyResult{}, false, errors.New("PASS attention event facts are inconsistent")
	}
	var dispatchCommandID string
	err = transaction.QueryRowContext(ctx, `
		SELECT command.command_id
		FROM events AS event
		JOIN commands AS command ON command.command_id = event.command_id
		WHERE event.run_id = ? AND event.kind = 'verifier.dispatched'
		  AND event.ordinal = 0 AND event.revision <= ?
		  AND json_extract(command.request_json, '$.payload.work_id') = ?
		ORDER BY event.revision DESC LIMIT 1`,
		selector.RunID, command.command.ExpectedRevision, selector.WorkID,
	).Scan(&dispatchCommandID)
	if err != nil {
		return ApplyResult{}, false, errors.New("PASS attention lacks a preceding verifier dispatch")
	}
	dispatch, dispatchFound, err := s.loadHistoricalVerifierDispatch(ctx, transaction, ControlledVerifierDispatchSelector{
		ControllerID: selector.ControllerID, CommandID: dispatchCommandID,
		RunID: selector.RunID, WorkID: selector.WorkID,
		SubmissionID: selector.SubmissionID, VerificationEpoch: selector.VerificationEpoch,
	})
	if err != nil || !dispatchFound {
		if err == nil {
			err = errors.New("PASS attention lacks its historical verifier dispatch")
		}
		return ApplyResult{}, false, err
	}
	if dispatch.command.result.Revision >= command.result.Revision || dispatch.effect.State != EffectSucceeded {
		return ApplyResult{}, false, errors.New("PASS attention is out of order with its verifier dispatch")
	}
	assessment, _, err := loadHistoricalVerifierAssessment(ctx, transaction, dispatch.effect, "")
	if err != nil || engine.VerdictOutcome(assessment.View().Outcome) != engine.VerdictPass {
		return ApplyResult{}, false, errors.New("PASS attention does not bind an exact PASS assessment")
	}
	var earlierVerdicts int
	if err := transaction.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM verdict_records
		WHERE dispatch_id = ? AND event_revision <= ?`, dispatch.request.DispatchID, command.result.Revision,
	).Scan(&earlierVerdicts); err != nil || earlierVerdicts != 0 {
		return ApplyResult{}, false, errors.New("PASS attention is out of order with verdict admission")
	}
	return command.result, true, nil
}

func loadHistoricalVerifierAssessment(
	ctx context.Context,
	transaction *sql.Tx,
	effect Effect,
	canonicalDigest string,
) (protocol.ExactVerifierAssessment, engine.VerifierEffectResult, error) {
	if err := validateBoundEffectResult(
		ctx, journalResultResolver{query: transaction}, effect, effect.Result,
	); err != nil {
		return protocol.ExactVerifierAssessment{}, engine.VerifierEffectResult{}, err
	}
	result, err := engine.ParseVerifierEffectResult(effect.Result)
	if err != nil || result.DispatchID != effect.ID || !journalContains(effect, result.StartedAt, result.CompletedAt) {
		return protocol.ExactVerifierAssessment{}, engine.VerifierEffectResult{},
			errors.New("verifier assessment does not match its journal result")
	}
	raw, err := protocol.ResolveArtifact(
		ctx, journalResultResolver{query: transaction}, result.Assessment,
		protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil {
		return protocol.ExactVerifierAssessment{}, engine.VerifierEffectResult{}, err
	}
	assessment, err := protocol.ParseVerifierAssessment(raw)
	if err != nil {
		return protocol.ExactVerifierAssessment{}, engine.VerifierEffectResult{}, err
	}
	record := assessment.Record()
	if canonicalDigest != "" && record.Digest != canonicalDigest {
		return protocol.ExactVerifierAssessment{}, engine.VerifierEffectResult{},
			errors.New("assessment canonical digest does not match its verdict identity")
	}
	return assessment, result, nil
}

func validConvergedVerdictOutcome(outcome engine.VerdictOutcome) bool {
	return outcome == engine.VerdictPass || outcome == engine.VerdictFail ||
		outcome == engine.VerdictSpecBlock || outcome == engine.VerdictInconclusive
}
