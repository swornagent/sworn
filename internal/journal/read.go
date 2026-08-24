package journal

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC {
		return time.Time{}, fail("CORRUPT_JOURNAL", nil)
	}
	return parsed, nil
}

func (s *Store) Effect(ctx context.Context, runID, effectID string) (Effect, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Effect{}, err
	}
	if err := validateIdentity(effectID, "effect"); err != nil {
		return Effect{}, err
	}
	if s == nil {
		return Effect{}, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return Effect{}, fail("CLOSED", nil)
	}
	return effectOnConnection(ctx, s.conn, runID, effectID)
}

func effectOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID, effectID string,
) (Effect, error) {
	var value Effect
	var state, updatedAt string
	var currentClaim, resultDigest, errorCode, claimExpires sql.NullString
	var result []byte
	err := conn.QueryRowContext(
		ctx,
		`SELECT e.run_id, e.effect_id, e.replay_key, e.kind, e.state,
		        e.before_digest, e.expected_digest, e.current_claim,
		        e.result_digest, e.result, e.error_code, e.updated_at,
		        c.expires_at
		 FROM effects e
		 LEFT JOIN claims c ON c.run_id = e.run_id
		  AND c.effect_id = e.effect_id
		  AND c.token = e.current_claim
		  AND c.completed_at IS NULL
		 WHERE e.run_id = ? AND e.effect_id = ?`,
		runID,
		effectID,
	).Scan(
		&value.RunID,
		&value.ID,
		&value.ReplayKey,
		&value.Kind,
		&state,
		&value.BeforeDigest,
		&value.ExpectedDigest,
		&currentClaim,
		&resultDigest,
		&result,
		&errorCode,
		&updatedAt,
		&claimExpires,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Effect{}, fail("EFFECT_NOT_FOUND", nil)
	}
	if err != nil {
		return Effect{}, dbError(err)
	}
	value.State = EffectState(state)
	value.CurrentClaim = currentClaim.String
	value.ResultDigest = resultDigest.String
	value.Result = append([]byte(nil), result...)
	value.ErrorCode = errorCode.String
	value.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Effect{}, err
	}
	if value.State == Claimed {
		// A claimed effect must carry exactly one live claim row: the token
		// in current_claim and the claim's expiry are the one place the
		// engine can see a claim's lifetime. Anything else is
		// engine-impossible and fails closed (the currentOwnerOnConnection
		// precedent for the runtime.owner effect).
		if value.CurrentClaim == "" || !claimExpires.Valid {
			return Effect{}, fail("CORRUPT_JOURNAL", nil)
		}
		expires, err := parseTime(claimExpires.String)
		if err != nil {
			return Effect{}, err
		}
		value.CurrentClaimExpiresAt = expires
	}
	if value.ResultDigest != "" && digest(value.Result) != value.ResultDigest {
		return Effect{}, fail("CORRUPT_JOURNAL", nil)
	}
	return value, nil
}

// LoadSealedProposal returns the exact plan bytes stored at seal time as the
// planner.sealed_plan child of a dispatch attempt. It intentionally reads
// that child rather than parent receipts, so the bytes remain available
// after either parent success or operational failure.
func (s *Store) LoadSealedProposal(
	ctx context.Context,
	runID, parentEffectID string,
) ([]byte, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	if err := validateIdentity(parentEffectID, "effect"); err != nil {
		return nil, err
	}
	childID := parentEffectID + "/sealed-proposal"
	if err := validateIdentity(childID, "effect"); err != nil {
		return nil, err
	}
	effect, err := s.Effect(ctx, runID, childID)
	if err != nil {
		return nil, err
	}
	if effect.Kind != "planner.sealed_plan" || effect.State != Succeeded {
		return nil, fail("SEALED_PROPOSAL_NOT_READY", nil)
	}
	return append([]byte(nil), effect.Result...), nil
}

func (s *Store) ClaimedEffects(ctx context.Context, runID string) ([]Effect, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return nil, fail("CLOSED", nil)
	}
	rows, err := s.conn.QueryContext(
		ctx,
		`SELECT effect_id FROM effects
		 WHERE run_id = ? AND state = 'claimed'
		 ORDER BY effect_id`,
		runID,
	)
	if err != nil {
		return nil, dbError(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, dbError(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, dbError(err)
	}
	result := make([]Effect, 0, len(ids))
	for _, id := range ids {
		effect, err := effectOnConnection(ctx, s.conn, runID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, effect)
	}
	return result, nil
}

// EffectChildren returns the effects whose IDs are direct descendants of
// effectID (prefix effectID+"/"). Child effects are durable checkpoints or
// sealed evidence; recovery treats their presence conservatively and never
// mistakes a child for completion evidence of its claimed parent.
func (s *Store) EffectChildren(
	ctx context.Context,
	runID, effectID string,
) ([]Effect, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	if err := validateIdentity(effectID, "effect"); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return nil, fail("CLOSED", nil)
	}
	rows, err := s.conn.QueryContext(
		ctx,
		`SELECT effect_id FROM effects
		 WHERE run_id = ? AND effect_id LIKE ? AND effect_id != ?
		 ORDER BY effect_id`,
		runID,
		effectID+"/%",
		effectID,
	)
	if err != nil {
		return nil, dbError(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, dbError(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, dbError(err)
	}
	result := make([]Effect, 0, len(ids))
	for _, id := range ids {
		effect, err := effectOnConnection(ctx, s.conn, runID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, effect)
	}
	return result, nil
}

// EffectCompletionEvidence reports whether one effect carries journaled
// attempt rows or receipt rows. completeOnConnection writes both atomically
// with the terminal state and current_claim=NULL, so either row on a
// still-claimed effect is engine-impossible and must fail closed upstream.
func (s *Store) EffectCompletionEvidence(
	ctx context.Context,
	runID, effectID string,
) (attempts bool, receipts bool, err error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return false, false, err
	}
	if err := validateIdentity(effectID, "effect"); err != nil {
		return false, false, err
	}
	if s == nil {
		return false, false, fail("CLOSED", nil)
	}
	err = s.readTransaction(ctx, func(conn *sql.Conn) error {
		for _, table := range []string{"attempts", "receipts"} {
			var count int64
			queryErr := conn.QueryRowContext(
				ctx,
				`SELECT count(*) FROM `+table+
					` WHERE run_id = ? AND effect_id = ?`,
				runID,
				effectID,
			).Scan(&count)
			if queryErr != nil {
				return dbError(queryErr)
			}
			if table == "attempts" {
				attempts = count != 0
			} else {
				receipts = count != 0
			}
		}
		return nil
	})
	if err != nil {
		return false, false, err
	}
	return attempts, receipts, nil
}

const MaxWindowEvents = 1024

type Window struct {
	Snapshot           Snapshot
	Control            ControlProjection
	Owner              OwnerLease
	OwnerPresent       bool
	ThroughEventOffset int64
	HasMoreEvents      bool
}

func snapshotOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	afterOffset, throughOffset int64,
	eventLimit int,
) (Snapshot, error) {
	var result Snapshot
	var runAt string
	err := conn.QueryRowContext(
		ctx,
		`SELECT run_id, manifest_digest, repository, release_id, target_ref, created_at
		 FROM runs WHERE run_id = ?`,
		runID,
	).Scan(
		&result.Run.ID,
		&result.Run.ManifestDigest,
		&result.Run.Repository,
		&result.Run.Release,
		&result.Run.TargetRef,
		&runAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, fail("RUN_NOT_FOUND", nil)
	}
	if err != nil {
		return Snapshot{}, dbError(err)
	}
	result.Run.CreatedAt, err = parseTime(runAt)
	if err != nil {
		return Snapshot{}, err
	}

	commandRows, err := conn.QueryContext(
		ctx,
		`SELECT replay_key, kind, payload_digest, payload, created_at
		 FROM commands WHERE run_id = ? ORDER BY replay_key`,
		runID,
	)
	if err != nil {
		return Snapshot{}, dbError(err)
	}
	for commandRows.Next() {
		var command Command
		var createdAt, payloadDigest string
		command.RunID = runID
		if err := commandRows.Scan(
			&command.ReplayKey,
			&command.Kind,
			&payloadDigest,
			&command.Payload,
			&createdAt,
		); err != nil {
			_ = commandRows.Close()
			return Snapshot{}, dbError(err)
		}
		command.CreatedAt, err = parseTime(createdAt)
		if err != nil || digest(command.Payload) != payloadDigest {
			_ = commandRows.Close()
			return Snapshot{}, fail("CORRUPT_JOURNAL", nil)
		}
		command.Payload = append([]byte(nil), command.Payload...)
		result.Commands = append(result.Commands, command)
	}
	if err := commandRows.Close(); err != nil {
		return Snapshot{}, dbError(err)
	}

	effectRows, err := conn.QueryContext(
		ctx,
		`SELECT effect_id FROM effects WHERE run_id = ? ORDER BY effect_id`,
		runID,
	)
	if err != nil {
		return Snapshot{}, dbError(err)
	}
	var effectIDs []string
	for effectRows.Next() {
		var effectID string
		if err := effectRows.Scan(&effectID); err != nil {
			_ = effectRows.Close()
			return Snapshot{}, dbError(err)
		}
		effectIDs = append(effectIDs, effectID)
	}
	if err := effectRows.Close(); err != nil {
		return Snapshot{}, dbError(err)
	}
	for _, effectID := range effectIDs {
		effect, err := effectOnConnection(ctx, conn, runID, effectID)
		if err != nil {
			return Snapshot{}, err
		}
		result.Effects = append(result.Effects, effect)
	}

	eventRows, err := conn.QueryContext(
		ctx,
		`SELECT event_offset, kind, body_digest, body, created_at
		 FROM events
		 WHERE run_id = ? AND event_offset > ? AND event_offset <= ?
		 ORDER BY event_offset
		 LIMIT ?`,
		runID,
		afterOffset,
		throughOffset,
		eventLimit,
	)
	if err != nil {
		return Snapshot{}, dbError(err)
	}
	for eventRows.Next() {
		var event Event
		var createdAt string
		event.RunID = runID
		if err := eventRows.Scan(
			&event.Offset,
			&event.Kind,
			&event.BodyDigest,
			&event.Body,
			&createdAt,
		); err != nil {
			_ = eventRows.Close()
			return Snapshot{}, dbError(err)
		}
		event.CreatedAt, err = parseTime(createdAt)
		if err != nil || digest(event.Body) != event.BodyDigest {
			_ = eventRows.Close()
			return Snapshot{}, fail("CORRUPT_JOURNAL", nil)
		}
		event.Body = append([]byte(nil), event.Body...)
		result.Events = append(result.Events, event)
	}
	if err := eventRows.Close(); err != nil {
		return Snapshot{}, dbError(err)
	}
	return result, nil
}

func eventHighWatermark(ctx context.Context, conn *sql.Conn, runID string) (int64, error) {
	var result int64
	if err := conn.QueryRowContext(
		ctx,
		"SELECT COALESCE(max(event_offset), 0) FROM events WHERE run_id = ?",
		runID,
	).Scan(&result); err != nil {
		return 0, dbError(err)
	}
	return result, nil
}

func (s *Store) Snapshot(ctx context.Context, runID string) (Snapshot, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		through, err := eventHighWatermark(ctx, conn, runID)
		if err != nil {
			return err
		}
		result, err = snapshotOnConnection(ctx, conn, runID, 0, through, -1)
		return err
	})
	return result, err
}

// ReadWindow returns one replay page and all of its runtime overlay from one
// SQLite read transaction. The caller may safely resume from
// ThroughEventOffset; HasMoreEvents means another page is already durable.
func (s *Store) ReadWindow(
	ctx context.Context,
	runID string,
	afterOffset int64,
	maxEvents int,
) (Window, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Window{}, err
	}
	if afterOffset < 0 || maxEvents < 1 || maxEvents > MaxWindowEvents {
		return Window{}, fail("INVALID_EVENT_WINDOW", nil)
	}
	var result Window
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		high, err := eventHighWatermark(ctx, conn, runID)
		if err != nil {
			return err
		}
		if afterOffset > high {
			return fail("INVALID_EVENT_OFFSET", nil)
		}
		snapshot, err := snapshotOnConnection(
			ctx,
			conn,
			runID,
			afterOffset,
			high,
			maxEvents+1,
		)
		if err != nil {
			return err
		}
		result.Snapshot = snapshot
		if len(result.Snapshot.Events) > maxEvents {
			result.Snapshot.Events = result.Snapshot.Events[:maxEvents]
			result.HasMoreEvents = true
			result.ThroughEventOffset =
				result.Snapshot.Events[len(result.Snapshot.Events)-1].Offset
		} else {
			result.ThroughEventOffset = high
		}
		result.Control, err = projectionOnConnection(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.Owner, result.OwnerPresent, err =
			currentOwnerOnConnection(ctx, conn, runID)
		return err
	})
	return result, err
}
