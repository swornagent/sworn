package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
)

type Outcome string

const (
	OutcomeApplied  Outcome = "applied"
	OutcomeRejected Outcome = "rejected"
)

var ErrIdempotencyConflict = errors.New("command idempotency conflict")

type ApplyResult struct {
	CommandID    string   `json:"command_id"`
	RunID        string   `json:"run_id"`
	Outcome      Outcome  `json:"outcome"`
	Revision     int64    `json:"revision"`
	EventID      string   `json:"event_id,omitempty"`
	EffectIDs    []string `json:"effect_ids,omitempty"`
	ErrorCode    string   `json:"error_code,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
	Replayed     bool     `json:"replayed,omitempty"`
}

func (s *Store) Apply(ctx context.Context, command engine.Command) (ApplyResult, error) {
	if s.readOnly {
		return ApplyResult{}, errors.New("control store is read-only")
	}
	if !engine.ValidID(command.ID) || !engine.ValidID(command.RunID) {
		return ApplyResult{}, errors.New("valid command and run ids are required for durable idempotency")
	}
	request, err := json.Marshal(command)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("encode command: %w", err)
	}
	requestDigest := commandDigest(command)
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin command transaction: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck

	if priorDigest, prior, found, err := loadCommandResult(ctx, transaction, command.ID); err != nil {
		return ApplyResult{}, err
	} else if found {
		if priorDigest != requestDigest {
			return ApplyResult{}, fmt.Errorf("%w: command %q was already used for a different request", ErrIdempotencyConflict, command.ID)
		}
		prior.Replayed = true
		return prior, nil
	}

	current, found, err := loadState(ctx, transaction, command.RunID)
	if err != nil {
		return ApplyResult{}, err
	}
	var currentPointer *engine.State
	if found {
		currentPointer = &current
	}
	decision, reduceErr := engine.Reduce(currentPointer, command)
	if reduceErr != nil {
		if rejection, ok := engine.RejectionOf(reduceErr); ok {
			result := ApplyResult{
				CommandID:    command.ID,
				RunID:        command.RunID,
				Outcome:      OutcomeRejected,
				Revision:     revisionOf(currentPointer),
				ErrorCode:    rejection.Code,
				ErrorMessage: rejection.Message,
			}
			if err := insertCommand(ctx, transaction, command, request, requestDigest, result, s.now().UTC().UnixMicro()); err != nil {
				return ApplyResult{}, err
			}
			if err := transaction.Commit(); err != nil {
				return ApplyResult{}, fmt.Errorf("commit rejected command: %w", err)
			}
			return result, nil
		}
		return ApplyResult{}, reduceErr
	}
	if err := decision.State.Validate(); err != nil {
		return ApplyResult{}, fmt.Errorf("reducer returned invalid state: %w", err)
	}
	if command.Kind == engine.CommandDispatchChecks {
		if !found {
			return ApplyResult{}, errors.New("checks.dispatch requires an existing delivery")
		}
		if err := s.validateCheckDispatchDecision(ctx, transaction, current, command, decision); err != nil {
			return ApplyResult{}, fmt.Errorf("validate checks.dispatch preconditions: %w", err)
		}
	}
	stateJSON, err := json.Marshal(decision.State)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("encode next state: %w", err)
	}
	now := s.now().UTC().UnixMicro()
	eventID := derivedID("evt", command.ID, 0)
	effectIDs := make([]string, len(decision.Effects))
	for index := range decision.Effects {
		effectIDs[index] = derivedID("eff", command.ID, index)
	}
	result := ApplyResult{
		CommandID: command.ID,
		RunID:     command.RunID,
		Outcome:   OutcomeApplied,
		Revision:  decision.State.Revision,
		EventID:   eventID,
		EffectIDs: effectIDs,
	}
	if err := insertCommand(ctx, transaction, command, request, requestDigest, result, now); err != nil {
		return ApplyResult{}, err
	}
	if !found {
		_, err = transaction.ExecContext(ctx, `
			INSERT INTO runs (
				run_id, delivery_id, repository_id, target_ref, plan_digest,
				revision, phase, terminal, state_json, created_at_us, updated_at_us
			) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
			decision.State.RunID, decision.State.DeliveryID, decision.State.Repository,
			decision.State.TargetRef, decision.State.PlanDigest, decision.State.Revision,
			decision.State.Phase, stateJSON, now, now,
		)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("insert run state: %w", err)
		}
	} else {
		result, err := transaction.ExecContext(ctx, `
			UPDATE runs
			SET revision = ?, phase = ?, state_json = ?, updated_at_us = ?
			WHERE run_id = ? AND revision = ?`,
			decision.State.Revision, decision.State.Phase, stateJSON, now,
			command.RunID, current.Revision,
		)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("update run state: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return ApplyResult{}, fmt.Errorf("read state update result: %w", err)
		}
		if changed != 1 {
			return ApplyResult{}, errors.New("run revision changed during command transaction")
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO events (event_id, run_id, command_id, revision, ordinal, kind, data_json, recorded_at_us)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		eventID, command.RunID, command.ID, decision.State.Revision,
		decision.Event.Kind, []byte(decision.Event.Data), now,
	); err != nil {
		return ApplyResult{}, fmt.Errorf("insert event: %w", err)
	}
	for index, effect := range decision.Effects {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO effects (
				effect_id, run_id, command_id, ordinal, kind, request_json,
				state, attempt, created_at_us
			) VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, ?)`,
			effectIDs[index], command.RunID, command.ID, index,
			effect.Kind, []byte(effect.Request), now,
		); err != nil {
			return ApplyResult{}, fmt.Errorf("insert effect %d: %w", index, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return ApplyResult{}, fmt.Errorf("commit command: %w", err)
	}
	return result, nil
}

func insertCommand(
	ctx context.Context,
	transaction *sql.Tx,
	command engine.Command,
	request []byte,
	requestDigest string,
	result ApplyResult,
	now int64,
) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode command result: %w", err)
	}
	var errorCode, errorMessage any
	if result.Outcome == OutcomeRejected {
		errorCode, errorMessage = result.ErrorCode, result.ErrorMessage
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, error_code, error_message, recorded_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		command.ID, command.RunID, command.Kind, command.ExpectedRevision,
		requestDigest, request, result.Outcome, resultJSON,
		errorCode, errorMessage, now,
	); err != nil {
		return fmt.Errorf("insert command: %w", err)
	}
	return nil
}

func loadCommandResult(ctx context.Context, query rowQuerier, commandID string) (string, ApplyResult, bool, error) {
	var requestDigest string
	var encoded []byte
	err := query.QueryRowContext(ctx,
		"SELECT request_digest, result_json FROM commands WHERE command_id = ?", commandID,
	).Scan(&requestDigest, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ApplyResult{}, false, nil
	}
	if err != nil {
		return "", ApplyResult{}, false, fmt.Errorf("load command %q: %w", commandID, err)
	}
	var result ApplyResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return "", ApplyResult{}, false, fmt.Errorf("decode command %q result: %w", commandID, err)
	}
	return requestDigest, result, true, nil
}

func revisionOf(state *engine.State) int64 {
	if state == nil {
		return engine.NoRevision
	}
	return state.Revision
}

func derivedID(prefix, commandID string, ordinal int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", prefix, commandID, ordinal)))
	return prefix + "-" + hex.EncodeToString(sum[:])
}

func digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commandDigest(command engine.Command) string {
	hasher := sha256.New()
	bind := func(value []byte) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		hasher.Write(size[:])
		hasher.Write(value)
	}
	bind([]byte(command.ID))
	bind([]byte(command.RunID))
	bind([]byte(command.Kind))
	var revision [8]byte
	binary.BigEndian.PutUint64(revision[:], uint64(command.ExpectedRevision))
	hasher.Write(revision[:])
	bind(command.Payload)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}
