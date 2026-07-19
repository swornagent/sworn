package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
)

func (s *Store) State(ctx context.Context, runID string) (engine.State, error) {
	state, found, err := loadState(ctx, s.db, runID)
	if err != nil {
		return engine.State{}, err
	}
	if !found {
		return engine.State{}, fmt.Errorf("run %q: %w", runID, sql.ErrNoRows)
	}
	return state, nil
}

func (s *Store) States(ctx context.Context) ([]engine.State, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, delivery_id, repository_id, target_ref, plan_digest, revision, phase, state_json
		FROM runs ORDER BY created_at_us, run_id`)
	if err != nil {
		return nil, fmt.Errorf("list run states: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var states []engine.State
	for rows.Next() {
		state, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run states: %w", err)
	}
	return states, nil
}

type rowScanner interface {
	Scan(...any) error
}

func loadState(ctx context.Context, query rowQuerier, runID string) (engine.State, bool, error) {
	row := query.QueryRowContext(ctx, `
		SELECT run_id, delivery_id, repository_id, target_ref, plan_digest, revision, phase, state_json
		FROM runs WHERE run_id = ?`, runID)
	state, err := scanState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return engine.State{}, false, nil
	}
	if err != nil {
		return engine.State{}, false, err
	}
	return state, true, nil
}

func scanState(row rowScanner) (engine.State, error) {
	var runID, deliveryID, repository, targetRef, planDigest, phase string
	var revision int64
	var encoded []byte
	if err := row.Scan(&runID, &deliveryID, &repository, &targetRef, &planDigest, &revision, &phase, &encoded); err != nil {
		return engine.State{}, err
	}
	var state engine.State
	if err := json.Unmarshal(encoded, &state); err != nil {
		return engine.State{}, fmt.Errorf("decode run %q state: %w", runID, err)
	}
	if err := state.Validate(); err != nil {
		return engine.State{}, fmt.Errorf("validate run %q state: %w", runID, err)
	}
	if state.RunID != runID || state.DeliveryID != deliveryID || state.Repository != repository ||
		state.TargetRef != targetRef || state.PlanDigest != planDigest || state.Revision != revision || string(state.Phase) != phase {
		return engine.State{}, fmt.Errorf("run %q snapshot disagrees with indexed columns", runID)
	}
	return state, nil
}
