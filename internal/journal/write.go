package journal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) RegisterRun(ctx context.Context, run Run) error {
	if err := validateIdentity(run.ID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(run.Release, "release"); err != nil {
		return err
	}
	if err := validateDigest(run.ManifestDigest); err != nil {
		return err
	}
	if !filepath.IsAbs(run.Repository) || filepath.Clean(run.Repository) != run.Repository {
		return fail("INVALID_REPOSITORY", nil)
	}
	if !strings.HasPrefix(run.TargetRef, "refs/heads/") ||
		strings.ContainsAny(run.TargetRef, "\x00\n\r ") {
		return fail("INVALID_TARGET_REF", nil)
	}
	createdAt, err := canonicalTime(run.CreatedAt)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO runs(
				run_id, manifest_digest, repository, release_id, target_ref, created_at
			) VALUES(?, ?, ?, ?, ?, ?)`,
			run.ID, run.ManifestDigest, run.Repository, run.Release, run.TargetRef, createdAt,
		)
		if err != nil {
			return dbError(err)
		}
		var observed Run
		var observedAt string
		if err := conn.QueryRowContext(
			ctx,
			`SELECT run_id, manifest_digest, repository, release_id, target_ref, created_at
			 FROM runs WHERE run_id = ?`,
			run.ID,
		).Scan(
			&observed.ID,
			&observed.ManifestDigest,
			&observed.Repository,
			&observed.Release,
			&observed.TargetRef,
			&observedAt,
		); err != nil {
			return dbError(err)
		}
		if observed.ID != run.ID ||
			observed.ManifestDigest != run.ManifestDigest ||
			observed.Repository != run.Repository ||
			observed.Release != run.Release ||
			observed.TargetRef != run.TargetRef {
			return fail("RUN_CONFLICT", nil)
		}
		return nil
	})
}

func (s *Store) RecordCommand(ctx context.Context, command Command) error {
	if err := validateIdentity(command.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(command.ReplayKey, "replay_key"); err != nil {
		return err
	}
	if err := validateIdentity(command.Kind, "command_kind"); err != nil {
		return err
	}
	if len(command.Payload) == 0 || len(command.Payload) > MaxPayloadBytes {
		return fail("RESOURCE_LIMIT", nil)
	}
	createdAt, err := canonicalTime(command.CreatedAt)
	if err != nil {
		return err
	}
	body := append([]byte(nil), command.Payload...)
	bodyDigest := digest(body)
	return s.immediate(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO commands(
				run_id, replay_key, kind, payload_digest, payload, created_at
			) VALUES(?, ?, ?, ?, ?, ?)`,
			command.RunID,
			command.ReplayKey,
			command.Kind,
			bodyDigest,
			body,
			createdAt,
		)
		if err != nil {
			return dbError(err)
		}
		var kind, observedDigest, observedAt string
		var observedBody []byte
		if err := conn.QueryRowContext(
			ctx,
			`SELECT kind, payload_digest, payload, created_at
			 FROM commands WHERE run_id = ? AND replay_key = ?`,
			command.RunID,
			command.ReplayKey,
		).Scan(&kind, &observedDigest, &observedBody, &observedAt); err != nil {
			return dbError(err)
		}
		if kind != command.Kind || observedDigest != bodyDigest ||
			!bytes.Equal(observedBody, body) {
			return fail("REPLAY_CONFLICT", nil)
		}
		return nil
	})
}

func (s *Store) EnsureEffect(ctx context.Context, effect Effect) error {
	for label, value := range map[string]string{
		"run": effect.RunID, "effect": effect.ID, "replay_key": effect.ReplayKey,
		"effect_kind": effect.Kind,
	} {
		if err := validateIdentity(value, label); err != nil {
			return err
		}
	}
	if err := validateDigest(effect.BeforeDigest); err != nil {
		return err
	}
	if err := validateDigest(effect.ExpectedDigest); err != nil {
		return err
	}
	updatedAt, err := canonicalTime(effect.UpdatedAt)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO effects(
				run_id, effect_id, replay_key, kind, state,
				before_digest, expected_digest, updated_at
			) VALUES(?, ?, ?, ?, 'pending', ?, ?, ?)`,
			effect.RunID,
			effect.ID,
			effect.ReplayKey,
			effect.Kind,
			effect.BeforeDigest,
			effect.ExpectedDigest,
			updatedAt,
		)
		if err != nil {
			return dbError(err)
		}
		var replayKey, kind, state, before, expected, observedAt string
		if err := conn.QueryRowContext(
			ctx,
			`SELECT replay_key, kind, state, before_digest, expected_digest, updated_at
			 FROM effects WHERE run_id = ? AND effect_id = ?`,
			effect.RunID,
			effect.ID,
		).Scan(&replayKey, &kind, &state, &before, &expected, &observedAt); err != nil {
			return dbError(err)
		}
		if replayKey != effect.ReplayKey || kind != effect.Kind ||
			before != effect.BeforeDigest || expected != effect.ExpectedDigest {
			return fail("EFFECT_CONFLICT", nil)
		}
		return nil
	})
}

func (s *Store) Claim(ctx context.Context, runID, effectID string, now time.Time, lease time.Duration) (Claim, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Claim{}, err
	}
	if err := validateIdentity(effectID, "effect"); err != nil {
		return Claim{}, err
	}
	if lease <= 0 || lease > MaxLease {
		return Claim{}, fail("INVALID_LEASE", nil)
	}
	acquired, err := canonicalTime(now)
	if err != nil {
		return Claim{}, err
	}
	expiresAt := now.Add(lease)
	expires, err := canonicalTime(expiresAt)
	if err != nil {
		return Claim{}, err
	}
	token, err := randomToken()
	if err != nil {
		return Claim{}, err
	}
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var state string
		var current sql.NullString
		if err := conn.QueryRowContext(
			ctx,
			`SELECT state, current_claim FROM effects
			 WHERE run_id = ? AND effect_id = ?`,
			runID,
			effectID,
		).Scan(&state, &current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fail("EFFECT_NOT_FOUND", nil)
			}
			return dbError(err)
		}
		if state != string(Pending) || current.Valid {
			return fail("EFFECT_NOT_CLAIMABLE", nil)
		}
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO claims(
				run_id, effect_id, token, acquired_at, expires_at
			) VALUES(?, ?, ?, ?, ?)`,
			runID,
			effectID,
			token,
			acquired,
			expires,
		); err != nil {
			return dbError(err)
		}
		result, err := conn.ExecContext(
			ctx,
			`UPDATE effects
			 SET state = 'claimed', current_claim = ?, updated_at = ?
			 WHERE run_id = ? AND effect_id = ?
			   AND state = 'pending' AND current_claim IS NULL`,
			token,
			acquired,
			runID,
			effectID,
		)
		if err != nil {
			return dbError(err)
		}
		return requireRows(result, "STALE_CLAIM")
	})
	if err != nil {
		return Claim{}, err
	}
	return Claim{
		RunID: runID, EffectID: effectID, Token: token,
		AcquiredAt: now.UTC(), ExpiresAt: expiresAt.UTC(),
	}, nil
}

func validateCompletion(completion Completion) error {
	if err := validateIdentity(completion.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(completion.EffectID, "effect"); err != nil {
		return err
	}
	if len(completion.Token) != 64 {
		return fail("INVALID_CLAIM_TOKEN", nil)
	}
	if completion.State != Succeeded && completion.State != OperationalFailed {
		return fail("INVALID_COMPLETION", nil)
	}
	if len(completion.Result) > MaxPayloadBytes ||
		len(completion.EventBody) > MaxEventBytes {
		return fail("RESOURCE_LIMIT", nil)
	}
	if err := validateIdentity(completion.EventKind, "event_kind"); err != nil {
		return err
	}
	if completion.State == Succeeded && completion.ErrorCode != "" {
		return fail("INVALID_COMPLETION", nil)
	}
	if completion.State == OperationalFailed {
		if err := validateIdentity(completion.ErrorCode, "error_code"); err != nil {
			return err
		}
		if len(completion.Receipts) != 0 {
			return fail("VERDICT_ON_OPERATIONAL_FAILURE", nil)
		}
	}
	if completion.Attempt != nil {
		if completion.Attempt.Number < 1 {
			return fail("INVALID_ATTEMPT", nil)
		}
		for label, value := range map[string]string{
			"responsibility": completion.Attempt.Responsibility,
			"transport":      completion.Attempt.TransportStatus,
		} {
			if err := validateIdentity(value, label); err != nil {
				return err
			}
		}
		if err := validateDigest(completion.Attempt.ObservationDigest); err != nil {
			return err
		}
		if len(completion.Attempt.Usage) == 0 ||
			len(completion.Attempt.Usage) > MaxPayloadBytes {
			return fail("INVALID_ATTEMPT", nil)
		}
		if completion.Attempt.HandoffDigest != "" {
			if err := validateDigest(completion.Attempt.HandoffDigest); err != nil {
				return err
			}
		}
	}
	for _, receipt := range completion.Receipts {
		if err := validateIdentity(receipt.Kind, "receipt_kind"); err != nil {
			return err
		}
		if len(receipt.Body) == 0 || len(receipt.Body) > MaxPayloadBytes {
			return fail("INVALID_RECEIPT", nil)
		}
	}
	_, err := canonicalTime(completion.At)
	return err
}

func (s *Store) Complete(ctx context.Context, completion Completion) error {
	if err := validateCompletion(completion); err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		return completeOnConnection(ctx, conn, completion)
	})
}

func completeOnConnection(ctx context.Context, conn *sql.Conn, completion Completion) error {
	at, _ := canonicalTime(completion.At)
	resultDigest := digest(completion.Result)
	result, err := conn.ExecContext(
		ctx,
		`UPDATE effects SET
			state = ?, current_claim = NULL, result_digest = ?, result = ?,
			error_code = NULLIF(?, ''), updated_at = ?
		 WHERE run_id = ? AND effect_id = ?
		   AND state = 'claimed' AND current_claim = ?`,
		string(completion.State),
		resultDigest,
		append([]byte(nil), completion.Result...),
		completion.ErrorCode,
		at,
		completion.RunID,
		completion.EffectID,
		completion.Token,
	)
	if err != nil {
		return dbError(err)
	}
	if err := requireRows(result, "STALE_COMPLETION"); err != nil {
		return err
	}
	result, err = conn.ExecContext(
		ctx,
		`UPDATE claims SET completed_at = ?, outcome = ?
		 WHERE run_id = ? AND effect_id = ? AND token = ?
		   AND completed_at IS NULL`,
		at,
		string(completion.State),
		completion.RunID,
		completion.EffectID,
		completion.Token,
	)
	if err != nil {
		return dbError(err)
	}
	if err := requireRows(result, "STALE_COMPLETION"); err != nil {
		return err
	}
	if completion.Attempt != nil {
		attempt := completion.Attempt
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO attempts(
				run_id, effect_id, attempt, responsibility, transport_status,
				observation_digest, usage_digest, usage, handoff_digest, created_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			completion.RunID,
			completion.EffectID,
			attempt.Number,
			attempt.Responsibility,
			attempt.TransportStatus,
			attempt.ObservationDigest,
			digest(attempt.Usage),
			append([]byte(nil), attempt.Usage...),
			attempt.HandoffDigest,
			at,
		); err != nil {
			return dbError(err)
		}
	}
	for _, receipt := range completion.Receipts {
		body := append([]byte(nil), receipt.Body...)
		receiptDigest := digest(body)
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO receipts(
				digest, run_id, effect_id, kind, body, created_at
			) VALUES(?, ?, ?, ?, ?, ?)`,
			receiptDigest,
			completion.RunID,
			completion.EffectID,
			receipt.Kind,
			body,
			at,
		); err != nil {
			return dbError(err)
		}
	}
	return appendEvent(
		ctx,
		conn,
		completion.RunID,
		completion.EventKind,
		completion.EventBody,
		at,
	)
}

func (s *Store) Reconcile(
	ctx context.Context,
	completion Completion,
	disposition RecoveryDisposition,
) error {
	if disposition == RecoveryAllNew {
		if completion.State != Succeeded {
			return fail("INVALID_RECOVERY", nil)
		}
		if err := validateCompletion(completion); err != nil {
			return err
		}
		return s.immediate(ctx, func(conn *sql.Conn) error {
			return completeOnConnection(ctx, conn, completion)
		})
	}
	if err := validateIdentity(completion.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(completion.EffectID, "effect"); err != nil {
		return err
	}
	if len(completion.Token) != 64 {
		return fail("INVALID_CLAIM_TOKEN", nil)
	}
	at, err := canonicalTime(completion.At)
	if err != nil {
		return err
	}
	var state EffectState
	var claimOutcome string
	switch disposition {
	case RecoveryAllOld:
		state, claimOutcome = Pending, string(RecoveryAllOld)
	case RecoveryAmbiguous:
		state, claimOutcome = Uncertain, string(RecoveryAmbiguous)
	default:
		return fail("INVALID_RECOVERY", nil)
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(
			ctx,
			`UPDATE effects SET
				state = ?, current_claim = NULL, updated_at = ?
			 WHERE run_id = ? AND effect_id = ?
			   AND state = 'claimed' AND current_claim = ?`,
			string(state),
			at,
			completion.RunID,
			completion.EffectID,
			completion.Token,
		)
		if err != nil {
			return dbError(err)
		}
		if err := requireRows(result, "STALE_COMPLETION"); err != nil {
			return err
		}
		result, err = conn.ExecContext(
			ctx,
			`UPDATE claims SET completed_at = ?, outcome = ?
			 WHERE run_id = ? AND effect_id = ? AND token = ?
			   AND completed_at IS NULL`,
			at,
			claimOutcome,
			completion.RunID,
			completion.EffectID,
			completion.Token,
		)
		if err != nil {
			return dbError(err)
		}
		if err := requireRows(result, "STALE_COMPLETION"); err != nil {
			return err
		}
		body := completion.EventBody
		if len(body) > MaxEventBytes {
			return fail("RESOURCE_LIMIT", nil)
		}
		eventKind := completion.EventKind
		if eventKind == "" {
			eventKind = "effect_reconciled"
		}
		if err := validateIdentity(eventKind, "event_kind"); err != nil {
			return err
		}
		return appendEvent(ctx, conn, completion.RunID, eventKind, body, at)
	})
}

func appendEvent(
	ctx context.Context,
	conn *sql.Conn,
	runID, kind string,
	body []byte,
	at string,
) error {
	if len(body) > MaxEventBytes {
		return fail("RESOURCE_LIMIT", nil)
	}
	_, err := conn.ExecContext(
		ctx,
		`INSERT INTO events(run_id, kind, body_digest, body, created_at)
		 VALUES(?, ?, ?, ?, ?)`,
		runID,
		kind,
		digest(body),
		append([]byte{}, body...),
		at,
	)
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) AppendEvent(
	ctx context.Context,
	runID, kind string,
	body []byte,
	at time.Time,
) error {
	if err := validateIdentity(runID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(kind, "event_kind"); err != nil {
		return err
	}
	if len(body) > MaxEventBytes {
		return fail("RESOURCE_LIMIT", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		return appendEvent(ctx, conn, runID, kind, body, timestamp)
	})
}

func completionError(err error) error {
	if err == nil {
		return nil
	}
	return fail("COMPLETION_FAILED", fmt.Errorf("%T", err))
}
