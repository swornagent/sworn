package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
)

type preparedAdmission struct {
	record   protocol.EncodedRecord
	facts    engine.AdmissionFacts
	workID   string
	attempt  int64
	delivery string
}

func (s *Store) prepareAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	current engine.State,
	command engine.Command,
	createdAt time.Time,
) (*preparedAdmission, error) {
	if s.repository == nil || s.repository.Binding().RepositoryID != current.Repository {
		return nil, errors.New("submission admission requires the immutable configured repository")
	}
	var intent engine.AdmitSubmissionPayload
	if err := json.Unmarshal(command.Payload, &intent); err != nil {
		return nil, fmt.Errorf("decode submission admission intent: %w", err)
	}
	attempt, err := loadExactAttempt(ctx, transaction, current, intent.WorkID, engine.WorkChecking)
	if err != nil {
		return nil, err
	}
	payload, effects, err := loadExactCheckBatch(ctx, transaction, current, attempt)
	if err != nil {
		return nil, err
	}
	resolver := journalResultResolver{query: transaction}
	_, build, err := resolveAttemptBuilder(
		ctx, resolver, current, attempt, payload.BuilderEffectID, s.builderDispatchDigest,
	)
	if err != nil {
		return nil, err
	}
	builderEffect, err := loadEffect(ctx, transaction, payload.BuilderEffectID)
	if err != nil || builderEffect.CompletedAtUS > createdAt.UnixMicro() ||
		!journalContains(builderEffect, build.Builder.StartedAt, build.Builder.CompletedAt) {
		return nil, errors.New("builder timestamps fall outside its journal lease")
	}
	authority, err := loadHistoricalAuthority(ctx, transaction, current, attempt.plan, build.Builder)
	if err != nil {
		return nil, err
	}
	measurements := make([]protocol.MeasuredCheck, len(effects))
	for index, effect := range effects {
		journal, err := resolver.SucceededEffect(ctx, effect.ID)
		if err != nil {
			return nil, fmt.Errorf("resolve admitted check %d: %w", index, err)
		}
		request, _ := engine.ParseLocalCheckEffectRequest(journal.Request)
		result, _ := engine.ParseLocalCheckEffectResult(journal.Result)
		if result.Outcome != engine.LocalCheckOutcomePass || effect.CompletedAtUS <= 0 ||
			effect.CompletedAtUS > createdAt.UnixMicro() {
			return nil, fmt.Errorf("check %q is not a completed passing journal fact", effect.ID)
		}
		_, receiptBytes, err := resolver.Artifact(ctx, result.Receipt.Digest)
		if err != nil {
			return nil, fmt.Errorf("resolve check %q receipt chronology: %w", effect.ID, err)
		}
		receipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
		if err != nil || !journalContains(effect, receipt.StartedAt, receipt.CompletedAt) {
			return nil, fmt.Errorf("check %q timestamps fall outside its journal lease", effect.ID)
		}
		measurements[index] = protocol.MeasuredCheck{
			RunID: effect.ID, RuntimeManifestDigest: request.RuntimeManifestDigest, Receipt: result.Receipt,
		}
	}
	record, err := protocol.BuildSubmission(ctx, s.repository, resolver, protocol.SubmissionInput{
		Attempt: attempt.number, CreatedAt: createdAt, Plan: attempt.plan, WorkID: attempt.workID,
		AuthorityReceipt: authority, Builder: build.Builder, Candidate: build.Candidate,
		MeasuredChecks: measurements,
	})
	if err != nil {
		return nil, fmt.Errorf("construct exact submission: %w", err)
	}
	submissionID, err := protocol.SubmissionID(current.DeliveryID, attempt.workID, attempt.number)
	if err != nil {
		return nil, err
	}
	if record.Kind != protocol.SubmissionSchemaVersion || record.Digest != protocol.RawDigest(record.CanonicalJSON) {
		return nil, errors.New("submission constructor returned an invalid canonical record")
	}
	return &preparedAdmission{
		record: record, workID: attempt.workID, attempt: attempt.number, delivery: current.DeliveryID,
		facts: engine.AdmissionFacts{
			SubmissionID: submissionID, SubmissionDigest: record.Digest,
			CandidateCommit: build.Candidate.Commit,
		},
	}, nil
}

func journalContains(effect Effect, started, completed string) bool {
	if effect.StartedAtUS <= 0 || effect.CompletedAtUS < effect.StartedAtUS {
		return false
	}
	journalStart := time.UnixMicro(effect.StartedAtUS).UTC().Format(time.RFC3339Nano)
	journalEnd := time.UnixMicro(effect.CompletedAtUS).UTC().Format(time.RFC3339Nano)
	startOrder, startErr := protocol.CompareDateTimes(journalStart, started)
	endOrder, endErr := protocol.CompareDateTimes(completed, journalEnd)
	return startErr == nil && endErr == nil && startOrder <= 0 && endOrder <= 0
}

func loadExactCheckBatch(
	ctx context.Context,
	transaction *sql.Tx,
	current engine.State,
	attempt exactAttempt,
) (engine.DispatchChecksPayload, []Effect, error) {
	var commandID, requestDigest, kind, outcome string
	var eventData, requestJSON []byte
	err := transaction.QueryRowContext(ctx, `
		SELECT event.command_id, event.data_json, command.request_digest,
		       command.request_json, command.kind, command.outcome
		FROM events AS event
		JOIN commands AS command ON command.command_id = event.command_id
		WHERE event.run_id = ? AND event.revision = ? AND event.ordinal = 0
		  AND event.kind = 'checks.dispatched'`, current.RunID, current.Revision,
	).Scan(&commandID, &eventData, &requestDigest, &requestJSON, &kind, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return engine.DispatchChecksPayload{}, nil, errors.New("current checking state lacks its dispatch event")
	}
	if err != nil {
		return engine.DispatchChecksPayload{}, nil, fmt.Errorf("load current check dispatch: %w", err)
	}
	var dispatched engine.Command
	if err := json.Unmarshal(requestJSON, &dispatched); err != nil {
		return engine.DispatchChecksPayload{}, nil, fmt.Errorf("decode current check dispatch command: %w", err)
	}
	if kind != string(engine.CommandDispatchChecks) || outcome != string(OutcomeApplied) ||
		dispatched.ID != commandID || dispatched.RunID != current.RunID ||
		dispatched.Kind != engine.CommandDispatchChecks || dispatched.ExpectedRevision != current.Revision-1 ||
		requestDigest != commandDigest(dispatched) || !bytes.Equal(eventData, dispatched.Payload) {
		return engine.DispatchChecksPayload{}, nil, errors.New("current check dispatch event does not match its accepted command")
	}
	if _, err := protocol.CanonicalizeJSON(eventData); err != nil {
		return engine.DispatchChecksPayload{}, nil, fmt.Errorf("validate current check dispatch payload: %w", err)
	}
	var payload engine.DispatchChecksPayload
	decoder := json.NewDecoder(bytes.NewReader(eventData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return engine.DispatchChecksPayload{}, nil, fmt.Errorf("decode exact check dispatch payload: %w", err)
	}
	requirements := attempt.checks.Requirements()
	if payload.WorkID != attempt.workID || len(payload.Checks) != len(requirements) {
		return engine.DispatchChecksPayload{}, nil, errors.New("current dispatch does not exactly cover the work policy")
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE command_id = ? ORDER BY ordinal`, commandID)
	if err != nil {
		return engine.DispatchChecksPayload{}, nil, fmt.Errorf("load current check effects: %w", err)
	}
	var effects []Effect
	for rows.Next() {
		effect, scanErr := scanEffect(rows)
		if scanErr != nil {
			rows.Close()
			return engine.DispatchChecksPayload{}, nil, scanErr
		}
		effects = append(effects, effect)
	}
	iterationErr := rows.Err()
	closeErr := rows.Close()
	if iterationErr != nil || closeErr != nil {
		return engine.DispatchChecksPayload{}, nil, errors.New("iterate current check effects")
	}
	if len(effects) != len(requirements) {
		return engine.DispatchChecksPayload{}, nil, errors.New("current dispatch effect batch is incomplete or contains extras")
	}
	for index, requirement := range requirements {
		selected, effect := payload.Checks[index], effects[index]
		expected, encodeErr := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
			SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
			DeliveryRunID: current.RunID, DeliveryID: current.DeliveryID,
			WorkID: attempt.workID, WorkAttempt: attempt.number, BuilderEffectID: payload.BuilderEffectID,
			CheckID: requirement.CheckID, DefinitionDigest: requirement.Definition.Digest,
			RuntimeManifestDigest: payload.RuntimeManifestDigest,
		})
		if encodeErr != nil || selected.CheckID != requirement.CheckID ||
			selected.DefinitionDigest != requirement.Definition.Digest || effect.Ordinal != int64(index) ||
			effect.ID != derivedID("eff", commandID, index) || effect.DeliveryRunID != current.RunID || effect.CommandID != commandID ||
			effect.Kind != string(engine.EffectLocalCheck) || !bytes.Equal(effect.Request, expected) {
			return engine.DispatchChecksPayload{}, nil, fmt.Errorf("current check effect %d differs from the exact policy dispatch", index)
		}
	}
	return payload, effects, nil
}

func persistAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	command engine.Command,
	admission preparedAdmission,
	now int64,
) error {
	if err := putRecordTransaction(ctx, transaction, admission.record, now, "submission"); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO submission_records (
			submission_id, delivery_id, work_id, attempt, digest, run_id, command_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		admission.facts.SubmissionID, admission.delivery, admission.workID, admission.attempt,
		admission.record.Digest, command.RunID, command.ID,
	); err != nil {
		return fmt.Errorf("insert atomic submission identity: %w", err)
	}
	return nil
}
