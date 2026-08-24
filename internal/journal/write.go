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

func prepareRun(run Run) (string, error) {
	if err := validateIdentity(run.ID, "run"); err != nil {
		return "", err
	}
	if err := validateIdentity(run.Release, "release"); err != nil {
		return "", err
	}
	if err := validateDigest(run.ManifestDigest); err != nil {
		return "", err
	}
	if !filepath.IsAbs(run.Repository) || filepath.Clean(run.Repository) != run.Repository {
		return "", fail("INVALID_REPOSITORY", nil)
	}
	if !strings.HasPrefix(run.TargetRef, "refs/heads/") ||
		strings.ContainsAny(run.TargetRef, "\x00\n\r ") {
		return "", fail("INVALID_TARGET_REF", nil)
	}
	createdAt, err := canonicalTime(run.CreatedAt)
	if err != nil {
		return "", err
	}
	return createdAt, nil
}

func registerRunOnConnection(ctx context.Context, conn *sql.Conn, run Run, createdAt string) error {
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
}

func (s *Store) RegisterRun(ctx context.Context, run Run) error {
	createdAt, err := prepareRun(run)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		return registerRunOnConnection(ctx, conn, run, createdAt)
	})
}

type preparedCommand struct {
	body      []byte
	digest    string
	createdAt string
}

func prepareCommand(command Command) (preparedCommand, error) {
	if err := validateIdentity(command.RunID, "run"); err != nil {
		return preparedCommand{}, err
	}
	if err := validateIdentity(command.ReplayKey, "replay_key"); err != nil {
		return preparedCommand{}, err
	}
	if err := validateIdentity(command.Kind, "command_kind"); err != nil {
		return preparedCommand{}, err
	}
	if len(command.Payload) == 0 || len(command.Payload) > MaxPayloadBytes {
		return preparedCommand{}, fail("RESOURCE_LIMIT", nil)
	}
	createdAt, err := canonicalTime(command.CreatedAt)
	if err != nil {
		return preparedCommand{}, err
	}
	body := append([]byte(nil), command.Payload...)
	return preparedCommand{body: body, digest: digest(body), createdAt: createdAt}, nil
}

func recordCommandOnConnection(ctx context.Context, conn *sql.Conn, command Command,
	prepared preparedCommand) error {
	_, err := conn.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO commands(
				run_id, replay_key, kind, payload_digest, payload, created_at
			) VALUES(?, ?, ?, ?, ?, ?)`,
		command.RunID,
		command.ReplayKey,
		command.Kind,
		prepared.digest,
		prepared.body,
		prepared.createdAt,
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
	if kind != command.Kind || observedDigest != prepared.digest ||
		!bytes.Equal(observedBody, prepared.body) {
		return fail("REPLAY_CONFLICT", nil)
	}
	return nil
}

func (s *Store) RecordCommand(ctx context.Context, command Command) error {
	prepared, err := prepareCommand(command)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		return recordCommandOnConnection(ctx, conn, command, prepared)
	})
}

// RecordCommandEffect records one immutable command and its first pending
// effect in the same transaction. Callers must never treat the command alone
// as evidence that the effect was admitted.
func (s *Store) RecordCommandEffect(
	ctx context.Context,
	command Command,
	effect Effect,
) error {
	preparedCommand, err := prepareCommand(command)
	if err != nil {
		return err
	}
	updatedAt, err := prepareEffect(effect)
	if err != nil {
		return err
	}
	if command.RunID != effect.RunID || command.ReplayKey != effect.ReplayKey {
		return fail("COMMAND_EFFECT_CONFLICT", nil)
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := recordCommandOnConnection(
			ctx,
			conn,
			command,
			preparedCommand,
		); err != nil {
			return err
		}
		return ensureEffectOnConnection(ctx, conn, effect, updatedAt)
	})
}

// RegisterRunCommandsEffect atomically establishes a run, its immutable
// manifest, and the first authority effect. It is the delegated bootstrap
// seam; no observer can see a registered delegated run without its pending
// authority intent.
func (s *Store) RegisterRunCommandsEffect(
	ctx context.Context,
	run Run,
	manifest Command,
	authority Command,
	effect Effect,
) error {
	createdAt, err := prepareRun(run)
	if err != nil {
		return err
	}
	preparedManifest, err := prepareCommand(manifest)
	if err != nil {
		return err
	}
	preparedAuthority, err := prepareCommand(authority)
	if err != nil {
		return err
	}
	updatedAt, err := prepareEffect(effect)
	if err != nil {
		return err
	}
	if run.ID != manifest.RunID || run.ID != authority.RunID ||
		authority.RunID != effect.RunID || authority.ReplayKey != effect.ReplayKey {
		return fail("COMMAND_EFFECT_CONFLICT", nil)
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := registerRunOnConnection(ctx, conn, run, createdAt); err != nil {
			return err
		}
		if err := recordCommandOnConnection(ctx, conn, manifest, preparedManifest); err != nil {
			return err
		}
		if err := recordCommandOnConnection(ctx, conn, authority, preparedAuthority); err != nil {
			return err
		}
		return ensureEffectOnConnection(ctx, conn, effect, updatedAt)
	})
}

func prepareEffect(effect Effect) (string, error) {
	for label, value := range map[string]string{
		"run": effect.RunID, "effect": effect.ID, "replay_key": effect.ReplayKey,
		"effect_kind": effect.Kind,
	} {
		if err := validateIdentity(value, label); err != nil {
			return "", err
		}
	}
	if err := validateDigest(effect.BeforeDigest); err != nil {
		return "", err
	}
	if err := validateDigest(effect.ExpectedDigest); err != nil {
		return "", err
	}
	updatedAt, err := canonicalTime(effect.UpdatedAt)
	if err != nil {
		return "", err
	}
	return updatedAt, nil
}

func ensureEffectOnConnection(ctx context.Context, conn *sql.Conn, effect Effect,
	updatedAt string) error {
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
}

func (s *Store) EnsureEffect(ctx context.Context, effect Effect) error {
	updatedAt, err := prepareEffect(effect)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		return ensureEffectOnConnection(ctx, conn, effect, updatedAt)
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
	expiresAt := now.Add(lease)
	token, err := randomToken()
	if err != nil {
		return Claim{}, err
	}
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		value, claimErr := claimOnConnection(ctx, conn, runID, effectID, now, lease, token)
		if claimErr == nil {
			token = value.Token
		}
		return claimErr
	})
	if err != nil {
		return Claim{}, err
	}
	return Claim{
		RunID: runID, EffectID: effectID, Token: token,
		AcquiredAt: now.UTC(), ExpiresAt: expiresAt.UTC(),
	}, nil
}

func claimOnConnection(ctx context.Context, conn *sql.Conn, runID, effectID string,
	now time.Time, lease time.Duration, token string) (Claim, error) {
	acquired, err := canonicalTime(now)
	if err != nil {
		return Claim{}, err
	}
	expiresAt := now.Add(lease)
	expires, err := canonicalTime(expiresAt)
	if err != nil {
		return Claim{}, err
	}
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
			return Claim{}, fail("EFFECT_NOT_FOUND", nil)
		}
		return Claim{}, dbError(err)
	}
	if state != string(Pending) || current.Valid {
		return Claim{}, fail("EFFECT_NOT_CLAIMABLE", nil)
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
		return Claim{}, dbError(err)
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
		return Claim{}, dbError(err)
	}
	if err := requireRows(result, "STALE_CLAIM"); err != nil {
		return Claim{}, err
	}
	return Claim{
		RunID: runID, EffectID: effectID, Token: token,
		AcquiredAt: now.UTC(), ExpiresAt: expiresAt.UTC(),
	}, nil
}

func (s *Store) ClaimOwned(ctx context.Context, owner OwnerLease, effectID string,
	now time.Time, lease time.Duration) (Claim, error) {
	token, err := randomToken()
	if err != nil {
		return Claim{}, err
	}
	var claim Claim
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		projection, err := projectionOnConnection(ctx, conn, owner.RunID)
		if err != nil || projection.Desired != "running" {
			return fail("CONTROL_STOPPED", err)
		}
		if err := checkOwner(ctx, conn, owner, now); err != nil {
			return err
		}
		var claimErr error
		claim, claimErr = claimOnConnection(ctx, conn, owner.RunID, effectID, now, lease, token)
		return claimErr
	})
	return claim, err
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
	if completion.ExpectedEventOffset != nil && *completion.ExpectedEventOffset < 0 {
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
		// The stored body is evidence for the digest, so a present body
		// must match it before any truncation; the digest always stays
		// over the full marshaled bytes. An absent body is valid (the
		// success path and historical callers carry none) and enforces
		// nothing. Oversize bodies are bounded at write time by the marked
		// partial prefix, never rejected: rejecting would lose the
		// evidence this slice exists to keep.
		if len(completion.Attempt.ObservationBody) > 0 &&
			digest(completion.Attempt.ObservationBody) !=
				completion.Attempt.ObservationDigest {
			return fail("OBSERVATION_DIGEST_MISMATCH", nil)
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

// CompleteWithChild atomically completes one effect and admits at most one
// deterministic child command/effect. It is used for outcome transitions whose
// durable decision and next effect must never be observed separately.
func (s *Store) CompleteWithChild(
	ctx context.Context,
	completion Completion,
	childCommand *Command,
	childEffect *Effect,
) error {
	if err := validateCompletion(completion); err != nil {
		return err
	}
	if (childCommand == nil) != (childEffect == nil) {
		return fail("COMMAND_EFFECT_CONFLICT", nil)
	}
	var prepared preparedCommand
	var updatedAt string
	var err error
	if childCommand != nil {
		prepared, err = prepareCommand(*childCommand)
		if err != nil {
			return err
		}
		updatedAt, err = prepareEffect(*childEffect)
		if err != nil {
			return err
		}
		if childCommand.RunID != completion.RunID || childEffect.RunID != completion.RunID || childCommand.RunID != childEffect.RunID || childCommand.ReplayKey != childEffect.ReplayKey {
			return fail("COMMAND_EFFECT_CONFLICT", nil)
		}
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := completeOnConnection(ctx, conn, completion); err != nil {
			return err
		}
		if childCommand == nil {
			return nil
		}
		if err := recordCommandOnConnection(ctx, conn, *childCommand, prepared); err != nil {
			return err
		}
		return ensureEffectOnConnection(ctx, conn, *childEffect, updatedAt)
	})
}

func (s *Store) CompleteOwned(ctx context.Context, owner OwnerLease, completion Completion) error {
	if err := validateCompletion(completion); err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, owner, completion.At); err != nil {
			return err
		}
		return completeOnConnection(ctx, conn, completion)
	})
}

func completeOnConnection(ctx context.Context, conn *sql.Conn, completion Completion) error {
	at, _ := canonicalTime(completion.At)
	if completion.ExpectedEventOffset != nil {
		var current int64
		if err := conn.QueryRowContext(
			ctx,
			"SELECT COALESCE(max(event_offset), 0) FROM events WHERE run_id = ?",
			completion.RunID,
		).Scan(&current); err != nil {
			return dbError(err)
		}
		if current != *completion.ExpectedEventOffset {
			return fail("STALE_COMPLETION", nil)
		}
	}
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
		// The bound is the same MaxPayloadBytes the usage receipt already
		// meets. An oversize body is stored as its explicit prefix with
		// observation_partial = 1 — a marked partial, never a silent
		// truncation — and the digest remains over the full bytes.
		body := append([]byte(nil), attempt.ObservationBody...)
		partial := 0
		if len(body) > MaxPayloadBytes {
			body = body[:MaxPayloadBytes]
			partial = 1
		}
		if len(body) == 0 {
			body = nil
		}
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO attempts(
				run_id, effect_id, attempt, responsibility, transport_status,
				observation_digest, usage_digest, usage, handoff_digest, created_at,
				observation_body, observation_partial
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)`,
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
			body,
			partial,
		); err != nil {
			return dbError(err)
		}
	}
	for _, receipt := range completion.Receipts {
		body := append([]byte(nil), receipt.Body...)
		binding := []byte(
			completion.RunID + "\x00" + completion.EffectID + "\x00" +
				receipt.Kind + "\x00",
		)
		receiptDigest := digest(append(binding, body...))
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

func (s *Store) Reconcile(ctx context.Context, completion Completion,
	disposition RecoveryDisposition) error {
	return s.reconcile(ctx, OwnerLease{}, completion, disposition)
}

func (s *Store) ReconcileOwned(ctx context.Context, owner OwnerLease, completion Completion,
	disposition RecoveryDisposition) error {
	return s.reconcile(ctx, owner, completion, disposition)
}

// ReconcileManyOwned applies one recovery disposition to a related set of
// claimed effects in a single transaction. It is used when partially
// reconciling a parent/child effect group would itself create a false recovery
// state.
func (s *Store) ReconcileManyOwned(
	ctx context.Context,
	owner OwnerLease,
	completions []Completion,
	disposition RecoveryDisposition,
) error {
	if len(completions) == 0 {
		return fail("INVALID_RECOVERY", nil)
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
	type preparedRecovery struct {
		completion Completion
		at         string
		eventKind  string
	}
	prepared := make([]preparedRecovery, len(completions))
	seen := make(map[string]bool, len(completions))
	for index, completion := range completions {
		if err := validateIdentity(completion.RunID, "run"); err != nil {
			return err
		}
		if owner.Token != "" && completion.RunID != owner.RunID {
			return fail("OWNER_FENCED", nil)
		}
		if err := validateIdentity(completion.EffectID, "effect"); err != nil {
			return err
		}
		if seen[completion.EffectID] {
			return fail("INVALID_RECOVERY", nil)
		}
		seen[completion.EffectID] = true
		if len(completion.Token) != 64 {
			return fail("INVALID_CLAIM_TOKEN", nil)
		}
		if len(completion.EventBody) > MaxEventBytes {
			return fail("RESOURCE_LIMIT", nil)
		}
		at, err := canonicalTime(completion.At)
		if err != nil {
			return err
		}
		eventKind := completion.EventKind
		if eventKind == "" {
			eventKind = "effect_reconciled"
		}
		if err := validateIdentity(eventKind, "event_kind"); err != nil {
			return err
		}
		prepared[index] = preparedRecovery{
			completion: completion,
			at:         at,
			eventKind:  eventKind,
		}
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		for _, item := range prepared {
			completion := item.completion
			if owner.Token != "" {
				if err := checkOwner(
					ctx, conn, owner, completion.At); err != nil {
					return err
				}
			}
			result, err := conn.ExecContext(
				ctx,
				`UPDATE effects SET
					state = ?, current_claim = NULL, updated_at = ?
				 WHERE run_id = ? AND effect_id = ?
				   AND state = 'claimed' AND current_claim = ?`,
				string(state),
				item.at,
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
				item.at,
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
			if err := appendEvent(
				ctx,
				conn,
				completion.RunID,
				item.eventKind,
				completion.EventBody,
				item.at,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) reconcile(ctx context.Context, owner OwnerLease, completion Completion,
	disposition RecoveryDisposition) error {
	if disposition == RecoveryAllNew {
		if completion.State != Succeeded {
			return fail("INVALID_RECOVERY", nil)
		}
		if err := validateCompletion(completion); err != nil {
			return err
		}
		return s.immediate(ctx, func(conn *sql.Conn) error {
			if owner.Token != "" {
				if completion.RunID != owner.RunID {
					return fail("OWNER_FENCED", nil)
				}
				if err := checkOwner(ctx, conn, owner, completion.At); err != nil {
					return err
				}
			}
			return completeOnConnection(ctx, conn, completion)
		})
	}
	return s.ReconcileManyOwned(
		ctx, owner, []Completion{completion}, disposition)
}

func appendEvent(ctx context.Context, conn *sql.Conn, runID, kind string,
	body []byte, at string) error {
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

func (s *Store) AppendEvent(ctx context.Context, runID, kind string,
	body []byte, at time.Time) error {
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

// AppendEventOnce atomically admits a deterministic identity command and its
// informational event. The command replay key is the uniqueness boundary, so
// concurrent recovery paths either append the exact event once or observe the
// already-admitted fact without a check-then-append race.
func (s *Store) AppendEventOnce(
	ctx context.Context,
	identity Command,
	eventKind string,
	eventBody []byte,
	at time.Time,
) error {
	prepared, err := prepareCommand(identity)
	if err != nil {
		return err
	}
	if err := validateIdentity(eventKind, "event_kind"); err != nil {
		return err
	}
	if len(eventBody) > MaxEventBytes {
		return fail("RESOURCE_LIMIT", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		result, insertErr := conn.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO commands(
				run_id, replay_key, kind, payload_digest, payload, created_at
			) VALUES(?, ?, ?, ?, ?, ?)`,
			identity.RunID, identity.ReplayKey, identity.Kind,
			prepared.digest, prepared.body, prepared.createdAt,
		)
		if insertErr != nil {
			return dbError(insertErr)
		}
		var kind, observedDigest string
		var observedBody []byte
		if queryErr := conn.QueryRowContext(
			ctx,
			`SELECT kind, payload_digest, payload FROM commands
			 WHERE run_id = ? AND replay_key = ?`,
			identity.RunID, identity.ReplayKey,
		).Scan(&kind, &observedDigest, &observedBody); queryErr != nil {
			return dbError(queryErr)
		}
		if kind != identity.Kind || observedDigest != prepared.digest ||
			!bytes.Equal(observedBody, prepared.body) {
			return fail("REPLAY_CONFLICT", nil)
		}
		inserted, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return dbError(rowsErr)
		}
		if inserted == 0 {
			return nil
		}
		return appendEvent(ctx, conn, identity.RunID, eventKind, eventBody, timestamp)
	})
}

func completionError(err error) error {
	if err == nil {
		return nil
	}
	return fail("COMPLETION_FAILED", fmt.Errorf("%T", err))
}
