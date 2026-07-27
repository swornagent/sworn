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
	var currentClaim, resultDigest, errorCode sql.NullString
	var result []byte
	err := conn.QueryRowContext(
		ctx,
		`SELECT run_id, effect_id, replay_key, kind, state,
		        before_digest, expected_digest, current_claim,
		        result_digest, result, error_code, updated_at
		 FROM effects WHERE run_id = ? AND effect_id = ?`,
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
	if value.ResultDigest != "" && digest(value.Result) != value.ResultDigest {
		return Effect{}, fail("CORRUPT_JOURNAL", nil)
	}
	return value, nil
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

func (s *Store) Snapshot(ctx context.Context, runID string) (Snapshot, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Snapshot{}, err
	}
	if s == nil {
		return Snapshot{}, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return Snapshot{}, fail("CLOSED", nil)
	}
	var result Snapshot
	var runAt string
	err := s.conn.QueryRowContext(
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

	commandRows, err := s.conn.QueryContext(
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

	effectRows, err := s.conn.QueryContext(
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
		effect, err := effectOnConnection(ctx, s.conn, runID, effectID)
		if err != nil {
			return Snapshot{}, err
		}
		result.Effects = append(result.Effects, effect)
	}

	eventRows, err := s.conn.QueryContext(
		ctx,
		`SELECT event_offset, kind, body_digest, body, created_at
		 FROM events WHERE run_id = ? ORDER BY event_offset`,
		runID,
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
