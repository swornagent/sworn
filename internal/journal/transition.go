package journal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
)

// succeededTransition stores one reducer command, its deterministic result,
// and the public event emitted by accepting it. It intentionally uses the
// existing v2 command/effect/event tables.
type succeededTransition struct {
	runID        string
	replayKey    string
	commandKind  string
	effectKind   string
	body         []byte
	beforeDigest string
	result       []byte
	eventKind    string
	eventBody    []byte
	at           string
}

func replayedTransition(
	ctx context.Context,
	conn *sql.Conn,
	runID, replayKey, commandKind, effectKind string,
	body []byte,
) ([]byte, bool, error) {
	var observedKind, observedDigest string
	var observed []byte
	err := conn.QueryRowContext(
		ctx,
		`SELECT kind,payload_digest,payload FROM commands
		  WHERE run_id=? AND replay_key=?`,
		runID,
		replayKey,
	).Scan(&observedKind, &observedDigest, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, dbError(err)
	}
	if observedKind != commandKind ||
		observedDigest != digest(observed) {
		return nil, true, fail("CORRUPT_JOURNAL", nil)
	}
	if !bytes.Equal(observed, body) {
		return nil, true, fail("REPLAY_CONFLICT", nil)
	}
	var observedEffectKind, state, resultDigest string
	var result []byte
	if err := conn.QueryRowContext(
		ctx,
		`SELECT kind,state,result_digest,result FROM effects
		  WHERE run_id=? AND effect_id=?`,
		runID,
		replayKey,
	).Scan(
		&observedEffectKind,
		&state,
		&resultDigest,
		&result,
	); err != nil ||
		observedEffectKind != effectKind ||
		state != string(Succeeded) ||
		resultDigest != digest(result) {
		return nil, true, fail("CORRUPT_JOURNAL", nil)
	}
	return result, true, nil
}

func appendSucceededTransition(
	ctx context.Context,
	conn *sql.Conn,
	value succeededTransition,
) error {
	if _, err := conn.ExecContext(
		ctx,
		`INSERT INTO commands(
			run_id,replay_key,kind,payload_digest,payload,created_at
		) VALUES(?,?,?,?,?,?)`,
		value.runID,
		value.replayKey,
		value.commandKind,
		digest(value.body),
		value.body,
		value.at,
	); err != nil {
		return dbError(err)
	}
	if _, err := conn.ExecContext(
		ctx,
		`INSERT INTO effects(
			run_id,effect_id,replay_key,kind,state,before_digest,
			expected_digest,result_digest,result,updated_at
		) VALUES(?,?,?,?,'succeeded',?,?,?,?,?)`,
		value.runID,
		value.replayKey,
		value.replayKey,
		value.effectKind,
		value.beforeDigest,
		digest(value.body),
		digest(value.result),
		value.result,
		value.at,
	); err != nil {
		return dbError(err)
	}
	return appendEvent(
		ctx,
		conn,
		value.runID,
		value.eventKind,
		value.eventBody,
		value.at,
	)
}
