package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/swornagent/sworn/internal/engine"
)

// ControlledBuildDispatchSelector is the non-authorizing identity used to
// recover one already-durable build dispatch outcome. CommandID remains the
// idempotency identity; the other fields prevent an occupied ID from being
// interpreted in a foreign controller, run, work, or builder context.
type ControlledBuildDispatchSelector struct {
	ControllerID          string
	CommandID             string
	RunID                 string
	WorkID                string
	BuilderDispatchDigest string
}

// ConvergeControlledBuildDispatch returns the exact durable result of one
// controlled build dispatch without reauthorizing historical work. A false
// found result means only that no command row occupies selector.CommandID.
// Any occupied but non-matching or incomplete command fails closed.
func (s *Store) ConvergeControlledBuildDispatch(
	ctx context.Context,
	ownership *ControllerOwnership,
	selector ControlledBuildDispatchSelector,
) (ApplyResult, bool, error) {
	if s == nil || s.readOnly {
		return ApplyResult{}, false, errors.New("controlled build convergence requires a writable Store")
	}
	if ownership == nil {
		return ApplyResult{}, false, ErrInvalidControllerOwnership
	}
	if !engine.ValidID(selector.ControllerID) || !engine.ValidID(selector.CommandID) ||
		!engine.ValidID(selector.RunID) || !engine.ValidID(selector.WorkID) ||
		!engine.ValidDigest(selector.BuilderDispatchDigest) ||
		selector.BuilderDispatchDigest != s.builderDispatchDigest {
		return ApplyResult{}, false, errors.New("controlled build convergence selector is invalid")
	}
	if err := ownership.ValidateActive(s, selector.ControllerID); err != nil {
		return ApplyResult{}, false, fmt.Errorf("validate controlled build convergence ownership: %w", err)
	}

	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("begin controlled build convergence: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck

	result, found, err := s.loadControlledBuildDispatchOutcome(ctx, transaction, selector)
	if err != nil {
		return ApplyResult{}, false, err
	}
	if err := ownership.ValidateActive(s, selector.ControllerID); err != nil {
		return ApplyResult{}, false, fmt.Errorf("revalidate controlled build convergence ownership: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return ApplyResult{}, false, fmt.Errorf("finish controlled build convergence: %w", err)
	}
	if !found {
		return ApplyResult{}, false, nil
	}
	result.Replayed = true
	return result, true, nil
}

func (s *Store) loadControlledBuildDispatchOutcome(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledBuildDispatchSelector,
) (ApplyResult, bool, error) {
	var runID, kind, requestDigest, outcome string
	var expectedRevision, recordedAtUS, eventCount, effectCount int64
	var requestJSON, resultJSON []byte
	var errorCode, errorMessage sql.NullString
	err := transaction.QueryRowContext(ctx, `
		SELECT run_id, kind, expected_revision, request_digest, request_json,
		       outcome, result_json, error_code, error_message, recorded_at_us,
		       (SELECT COUNT(*) FROM events WHERE command_id = commands.command_id),
		       (SELECT COUNT(*) FROM effects WHERE command_id = commands.command_id)
		FROM commands WHERE command_id = ?`, selector.CommandID,
	).Scan(
		&runID, &kind, &expectedRevision, &requestDigest, &requestJSON,
		&outcome, &resultJSON, &errorCode, &errorMessage, &recordedAtUS,
		&eventCount, &effectCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ApplyResult{}, false, nil
	}
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("load controlled build command %q: %w", selector.CommandID, err)
	}

	command, err := decodeExactControlledBuildJSON[engine.Command](
		requestJSON, "id", "run_id", "kind", "expected_revision", "payload",
	)
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("decode controlled build command %q: %w", selector.CommandID, err)
	}
	if runID != selector.RunID || kind != string(engine.CommandDispatchBuild) ||
		command.ID != selector.CommandID || command.RunID != selector.RunID ||
		command.Kind != engine.CommandDispatchBuild || command.ExpectedRevision != expectedRevision ||
		expectedRevision < 0 || requestDigest != commandDigest(command) {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its controlled build selector",
			ErrIdempotencyConflict, selector.CommandID,
		)
	}
	payload, err := parseExactDispatchBuildPayload(command.Payload)
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("validate controlled build command payload: %w", err)
	}
	if payload.WorkID != selector.WorkID || !engine.ValidDigest(payload.DispatchDigest) ||
		payload.BuilderDispatchDigest != selector.BuilderDispatchDigest {
		return ApplyResult{}, false, fmt.Errorf(
			"%w: command %q does not match its controlled build selector",
			ErrIdempotencyConflict, selector.CommandID,
		)
	}

	result, err := decodeExactControlledBuildJSON[ApplyResult](resultJSON)
	if err != nil {
		return ApplyResult{}, false, fmt.Errorf("decode controlled build result %q: %w", selector.CommandID, err)
	}
	if result.CommandID != selector.CommandID || result.RunID != selector.RunID ||
		result.Replayed || string(result.Outcome) != outcome {
		return ApplyResult{}, false, errors.New("controlled build result does not match its command")
	}

	if result.Outcome != OutcomeApplied {
		return ApplyResult{}, false, fmt.Errorf("controlled build command has unsupported outcome %q", result.Outcome)
	}
	if expectedRevision == math.MaxInt64 || result.Revision != expectedRevision+1 ||
		result.EventID != derivedID("evt", selector.CommandID, 0) ||
		len(result.EffectIDs) != 1 ||
		result.EffectIDs[0] != derivedID("eff", selector.CommandID, 0) ||
		result.ErrorCode != "" || result.ErrorMessage != "" ||
		errorCode.Valid || errorMessage.Valid || eventCount != 1 || effectCount != 1 {
		return ApplyResult{}, false, errors.New("applied controlled build result is inconsistent")
	}
	if err := requireExactJSONMembers(resultJSON,
		"command_id", "run_id", "outcome", "revision", "event_id", "effect_ids",
	); err != nil {
		return ApplyResult{}, false, fmt.Errorf("validate applied controlled build result: %w", err)
	}
	buildRequest, err := validateAppliedControlledBuildClosure(
		ctx, transaction, selector, command, result, recordedAtUS,
	)
	if err != nil {
		return ApplyResult{}, false, err
	}
	if err := s.validateConvergedControlledBuildPlan(
		ctx, transaction, selector, payload, result, buildRequest,
	); err != nil {
		return ApplyResult{}, false, err
	}
	return result, true, nil
}

func validateAppliedControlledBuildClosure(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledBuildDispatchSelector,
	command engine.Command,
	result ApplyResult,
	recordedAtUS int64,
) (engine.BuildEffectRequest, error) {
	var eventID, runID, kind string
	var revision, ordinal, eventRecordedAtUS int64
	var data []byte
	err := transaction.QueryRowContext(ctx, `
		SELECT event_id, run_id, revision, ordinal, kind, data_json, recorded_at_us
		FROM events WHERE command_id = ? ORDER BY ordinal LIMIT 1`,
		selector.CommandID,
	).Scan(
		&eventID, &runID, &revision, &ordinal, &kind, &data, &eventRecordedAtUS,
	)
	if err != nil {
		return engine.BuildEffectRequest{}, fmt.Errorf("load controlled build dispatch event: %w", err)
	}
	if eventID != result.EventID || runID != selector.RunID ||
		revision != result.Revision || ordinal != 0 ||
		kind != "build.dispatched" || !bytes.Equal(data, command.Payload) ||
		eventRecordedAtUS != recordedAtUS {
		return engine.BuildEffectRequest{}, errors.New("controlled build dispatch event is incomplete or inconsistent")
	}

	effect, err := loadEffect(ctx, transaction, result.EffectIDs[0])
	if err != nil {
		return engine.BuildEffectRequest{}, err
	}
	if effect.DeliveryRunID != selector.RunID ||
		effect.CommandID != selector.CommandID || effect.Ordinal != 0 ||
		effect.Kind != string(engine.EffectBuild) || effect.CreatedAtUS != recordedAtUS {
		return engine.BuildEffectRequest{}, errors.New("controlled build effect is inconsistent with its command")
	}
	buildRequest, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil || buildRequest.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		buildRequest.DeliveryRunID != selector.RunID || buildRequest.WorkID != selector.WorkID ||
		buildRequest.BuilderDispatchDigest != selector.BuilderDispatchDigest {
		return engine.BuildEffectRequest{}, errors.New("controlled build effect request is inconsistent")
	}
	return buildRequest, nil
}

func (s *Store) validateConvergedControlledBuildPlan(
	ctx context.Context,
	transaction *sql.Tx,
	selector ControlledBuildDispatchSelector,
	payload engine.DispatchBuildPayload,
	result ApplyResult,
	buildRequest engine.BuildEffectRequest,
) error {
	state, found, err := loadState(ctx, transaction, selector.RunID)
	if err != nil {
		return err
	}
	if !found || state.Phase != engine.PhaseActive || state.Revision < result.Revision {
		return errors.New("controlled build outcome no longer has a monotonic active run")
	}
	plan, err := loadExactPlan(ctx, transaction, state.PlanDigest)
	if err != nil {
		return fmt.Errorf("load converged controlled build plan: %w", err)
	}
	workIDs, target := plan.WorkIDs(), plan.Target()
	if plan.Record().Digest != state.PlanDigest || plan.DeliveryID() != state.DeliveryID ||
		target.Repository != state.Repository || target.Ref != state.TargetRef ||
		!slices.Equal(workIDs, stateWorkIDsForBuildGate(state.Work)) {
		return errors.New("controlled build outcome state does not match its exact plan")
	}
	contract, exists := plan.Work(selector.WorkID)
	workIndex := slices.Index(workIDs, selector.WorkID)
	if !exists || workIndex < 0 || contract.Digest() == "" || contract.Digest() != payload.DispatchDigest {
		return errors.New("controlled build outcome does not match its exact work contract")
	}

	currentWork := state.Work[workIndex]
	if buildRequest.DeliveryID != state.DeliveryID ||
		buildRequest.DispatchDigest != payload.DispatchDigest ||
		currentWork.Attempt < buildRequest.WorkAttempt {
		return errors.New("controlled build outcome attempt is inconsistent with current state")
	}
	intendedAttempt := currentWork.Attempt
	if currentWork.State == engine.WorkReady {
		intendedAttempt++
	}
	if buildRequest.WorkAttempt != intendedAttempt {
		return fmt.Errorf(
			"%w: command %q names build attempt %d, current intended attempt is %d",
			ErrIdempotencyConflict, selector.CommandID,
			buildRequest.WorkAttempt, intendedAttempt,
		)
	}
	if state.Revision == result.Revision &&
		(currentWork.State != engine.WorkActive || currentWork.Attempt != buildRequest.WorkAttempt) {
		return errors.New("current state does not contain the newly dispatched build attempt")
	}
	return nil
}
