package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/engine"
)

type EffectState string

const (
	EffectPending   EffectState = "pending"
	EffectRunning   EffectState = "running"
	EffectUnknown   EffectState = "unknown"
	EffectSucceeded EffectState = "succeeded"
	EffectFailed    EffectState = "failed"
)

var ErrNoPendingEffect = errors.New("no pending effect")

type Effect struct {
	ID            string          `json:"id"`
	DeliveryRunID string          `json:"delivery_run_id"`
	CommandID     string          `json:"command_id"`
	Ordinal       int64           `json:"ordinal"`
	Kind          string          `json:"kind"`
	Request       json.RawMessage `json:"request"`
	State         EffectState     `json:"state"`
	Attempt       int64           `json:"attempt"`
	OwnerID       string          `json:"owner_id,omitempty"`
	Receipt       json.RawMessage `json:"receipt,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	CreatedAtUS   int64           `json:"created_at_us"`
	StartedAtUS   int64           `json:"started_at_us,omitempty"`
	CompletedAtUS int64           `json:"completed_at_us,omitempty"`
}

// leaseIssuer binds an effect lease to the Store instance that minted it. A
// process restart therefore cannot reuse an in-memory lease: recovery must move
// the running effect to unknown and reconcile it first.
type leaseIssuer struct{ marker byte }

// EffectLease is an opaque, store-issued capability for exactly one claimed
// effect attempt. Completion consumes its effect ID, owner and attempt as one
// compare-and-swap boundary, closing stale-worker and same-owner ABA races.
type EffectLease struct {
	issuer *leaseIssuer
	effect Effect
}

func (lease EffectLease) EffectID() string      { return lease.effect.ID }
func (lease EffectLease) DeliveryRunID() string { return lease.effect.DeliveryRunID }
func (lease EffectLease) Attempt() int64        { return lease.effect.Attempt }
func (lease EffectLease) Kind() string          { return lease.effect.Kind }
func (lease EffectLease) Request() json.RawMessage {
	return append(json.RawMessage(nil), lease.effect.Request...)
}

// ProtocolRunID is the engine-owned invocation identity to use in Baton
// builder or producer records. Callers never select a separate run ID.
func (lease EffectLease) ProtocolRunID() string {
	return lease.effect.ID
}

func (s *Store) Effects(ctx context.Context, state EffectState) ([]Effect, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE state = ? ORDER BY created_at_us, effect_id`, state)
	if err != nil {
		return nil, fmt.Errorf("list %s effects: %w", state, err)
	}
	defer rows.Close() //nolint:errcheck
	var effects []Effect
	for rows.Next() {
		effect, err := scanEffect(rows)
		if err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate effects: %w", err)
	}
	return effects, nil
}

func (s *Store) ClaimNextEffect(ctx context.Context, ownerID string) (EffectLease, error) {
	if s.readOnly {
		return EffectLease{}, errors.New("control store is read-only")
	}
	if !engine.ValidID(ownerID) {
		return EffectLease{}, errors.New("valid effect owner id is required")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return EffectLease{}, fmt.Errorf("begin effect claim: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	var effectID string
	err = transaction.QueryRowContext(ctx, `
		SELECT effect_id FROM effects
		WHERE state = 'pending'
		ORDER BY created_at_us, effect_id LIMIT 1`).Scan(&effectID)
	if errors.Is(err, sql.ErrNoRows) {
		return EffectLease{}, ErrNoPendingEffect
	}
	if err != nil {
		return EffectLease{}, fmt.Errorf("select pending effect: %w", err)
	}
	now := s.now().UTC().UnixMicro()
	result, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'running', attempt = attempt + 1, owner_id = ?,
		    started_at_us = ?, completed_at_us = NULL, receipt_json = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'pending'`, ownerID, now, effectID)
	if err != nil {
		return EffectLease{}, fmt.Errorf("claim effect %q: %w", effectID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return EffectLease{}, fmt.Errorf("claim effect %q lost ownership race", effectID)
	}
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return EffectLease{}, err
	}
	if err := insertObservation(ctx, transaction, effect, "claimed", nil, "", now); err != nil {
		return EffectLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return EffectLease{}, fmt.Errorf("commit effect claim: %w", err)
	}
	return EffectLease{issuer: s.leaseIssuer, effect: cloneEffect(effect)}, nil
}

func (s *Store) CompleteEffect(
	ctx context.Context,
	lease EffectLease,
	succeeded bool,
	receipt json.RawMessage,
	detail string,
) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if lease.issuer == nil || lease.issuer != s.leaseIssuer ||
		lease.effect.State != EffectRunning || !engine.ValidID(lease.effect.ID) ||
		!engine.ValidID(lease.effect.OwnerID) || lease.effect.Attempt < 1 {
		return errors.New("effect completion requires a current store-issued lease")
	}
	if !json.Valid(receipt) {
		return errors.New("effect completion requires a JSON receipt")
	}
	if !succeeded && strings.TrimSpace(detail) == "" {
		return errors.New("failed effect requires an error detail")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin effect completion: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return err
	}
	if effect.State != EffectRunning || effect.OwnerID != lease.effect.OwnerID || effect.Attempt != lease.effect.Attempt {
		return fmt.Errorf(
			"effect %q is not running for lease owner %q at attempt %d",
			lease.effect.ID, lease.effect.OwnerID, lease.effect.Attempt,
		)
	}
	if effect.DeliveryRunID != lease.effect.DeliveryRunID || effect.CommandID != lease.effect.CommandID ||
		effect.Kind != lease.effect.Kind || !bytes.Equal(effect.Request, lease.effect.Request) {
		return fmt.Errorf("effect %q no longer matches its issued lease", lease.effect.ID)
	}
	now := s.now().UTC().UnixMicro()
	state, observation := EffectFailed, "failed"
	var receiptValue any = []byte(receipt)
	var errorValue any = detail
	if succeeded {
		state, observation = EffectSucceeded, "succeeded"
		errorValue = nil
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = ?, receipt_json = ?, last_error = ?, completed_at_us = ?
		WHERE effect_id = ? AND state = 'running' AND owner_id = ? AND attempt = ?`,
		state, receiptValue, errorValue, now,
		lease.effect.ID, lease.effect.OwnerID, lease.effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("complete effect %q: %w", lease.effect.ID, err)
	}
	if err := requireOneRow(result, "complete effect "+lease.effect.ID); err != nil {
		return err
	}
	effect.State, effect.Receipt, effect.LastError, effect.CompletedAtUS = state, receipt, detail, now
	if err := insertObservation(ctx, transaction, effect, observation, receipt, detail, now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit effect completion: %w", err)
	}
	return nil
}

// RecoverInterruptedEffects must be called only after the command service has
// established exclusive process ownership. It never makes an effect retryable.
func (s *Store) RecoverInterruptedEffects(ctx context.Context, reason string) (int, error) {
	if s.readOnly {
		return 0, errors.New("control store is read-only")
	}
	if strings.TrimSpace(reason) == "" {
		return 0, errors.New("interruption reason is required")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin interrupted-effect recovery: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	rows, err := transaction.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE state = 'running' ORDER BY effect_id`)
	if err != nil {
		return 0, fmt.Errorf("list interrupted effects: %w", err)
	}
	var interrupted []Effect
	for rows.Next() {
		effect, err := scanEffect(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		interrupted = append(interrupted, effect)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close interrupted effects: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate interrupted effects: %w", err)
	}
	now := s.now().UTC().UnixMicro()
	for _, effect := range interrupted {
		result, err := transaction.ExecContext(ctx, `
			UPDATE effects SET state = 'unknown', last_error = ?
			WHERE effect_id = ? AND state = 'running'`, reason, effect.ID)
		if err != nil {
			return 0, fmt.Errorf("mark effect %q unknown: %w", effect.ID, err)
		}
		if err := requireOneRow(result, "mark effect "+effect.ID+" unknown"); err != nil {
			return 0, err
		}
		effect.State, effect.LastError = EffectUnknown, reason
		if err := insertObservation(ctx, transaction, effect, "unknown", nil, reason, now); err != nil {
			return 0, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("commit interrupted-effect recovery: %w", err)
	}
	return len(interrupted), nil
}

type Reconciliation string

const (
	ReconcileNotApplied Reconciliation = "not_applied"
	ReconcileSucceeded  Reconciliation = "succeeded"
	ReconcileFailed     Reconciliation = "failed"
)

func (s *Store) ReconcileUnknownEffect(
	ctx context.Context,
	effectID string,
	expectedAttempt int64,
	reconcilerID string,
	resolution Reconciliation,
	receipt json.RawMessage,
	detail string,
) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if !engine.ValidID(reconcilerID) {
		return errors.New("valid reconciler id is required")
	}
	if expectedAttempt < 1 {
		return errors.New("positive effect attempt is required for reconciliation")
	}
	if !json.Valid(receipt) {
		return errors.New("effect reconciliation requires a JSON receipt")
	}
	if resolution == ReconcileFailed && strings.TrimSpace(detail) == "" {
		return errors.New("failed reconciliation requires a detail")
	}
	if resolution != ReconcileNotApplied && resolution != ReconcileSucceeded && resolution != ReconcileFailed {
		return fmt.Errorf("unsupported reconciliation %q", resolution)
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin effect reconciliation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return err
	}
	if effect.State != EffectUnknown || effect.Attempt != expectedAttempt {
		return fmt.Errorf(
			"effect %q is %s at attempt %d, want unknown at attempt %d",
			effectID, effect.State, effect.Attempt, expectedAttempt,
		)
	}
	now := s.now().UTC().UnixMicro()
	var result sql.Result
	switch resolution {
	case ReconcileNotApplied:
		result, err = transaction.ExecContext(ctx, `
			UPDATE effects
			SET state = 'pending', owner_id = NULL, started_at_us = NULL,
			    receipt_json = NULL, last_error = NULL, completed_at_us = NULL
			WHERE effect_id = ? AND state = 'unknown' AND attempt = ?`, effectID, expectedAttempt)
	case ReconcileSucceeded:
		result, err = transaction.ExecContext(ctx, `
			UPDATE effects
			SET state = 'succeeded', receipt_json = ?, last_error = NULL, completed_at_us = ?
			WHERE effect_id = ? AND state = 'unknown' AND attempt = ?`, []byte(receipt), now, effectID, expectedAttempt)
	case ReconcileFailed:
		result, err = transaction.ExecContext(ctx, `
			UPDATE effects
			SET state = 'failed', receipt_json = NULL, last_error = ?, completed_at_us = ?
			WHERE effect_id = ? AND state = 'unknown' AND attempt = ?`, detail, now, effectID, expectedAttempt)
	}
	if err != nil {
		return fmt.Errorf("reconcile effect %q: %w", effectID, err)
	}
	if err := requireOneRow(result, "reconcile effect "+effectID); err != nil {
		return err
	}
	effect.OwnerID = reconcilerID
	if err := insertObservation(ctx, transaction, effect, string(resolution), receipt, detail, now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit effect reconciliation: %w", err)
	}
	return nil
}

func loadEffect(ctx context.Context, query rowQuerier, effectID string) (Effect, error) {
	row := query.QueryRowContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE effect_id = ?`, effectID)
	effect, err := scanEffect(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Effect{}, fmt.Errorf("effect %q: %w", effectID, sql.ErrNoRows)
	}
	return effect, err
}

func scanEffect(row rowScanner) (Effect, error) {
	var effect Effect
	var owner, receipt, lastError sql.NullString
	var started, completed sql.NullInt64
	if err := row.Scan(
		&effect.ID, &effect.DeliveryRunID, &effect.CommandID, &effect.Ordinal,
		&effect.Kind, &effect.Request, &effect.State, &effect.Attempt,
		&owner, &receipt, &lastError, &effect.CreatedAtUS, &started, &completed,
	); err != nil {
		return Effect{}, err
	}
	if owner.Valid {
		effect.OwnerID = owner.String
	}
	if receipt.Valid {
		effect.Receipt = json.RawMessage(receipt.String)
	}
	if lastError.Valid {
		effect.LastError = lastError.String
	}
	if started.Valid {
		effect.StartedAtUS = started.Int64
	}
	if completed.Valid {
		effect.CompletedAtUS = completed.Int64
	}
	return effect, nil
}

func cloneEffect(effect Effect) Effect {
	effect.Request = append(json.RawMessage(nil), effect.Request...)
	effect.Receipt = append(json.RawMessage(nil), effect.Receipt...)
	return effect
}

func insertObservation(
	ctx context.Context,
	transaction *sql.Tx,
	effect Effect,
	kind string,
	receipt json.RawMessage,
	detail string,
	now int64,
) error {
	var receiptValue, detailValue any
	if len(receipt) > 0 {
		receiptValue = []byte(receipt)
	}
	if detail != "" {
		detailValue = detail
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, detail, recorded_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		effect.ID, effect.Attempt, kind, nullableString(effect.OwnerID),
		receiptValue, detailValue, now,
	); err != nil {
		return fmt.Errorf("record %s observation for effect %q: %w", kind, effect.ID, err)
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func requireOneRow(result sql.Result, operation string) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: read affected rows: %w", operation, err)
	}
	if changed != 1 {
		return fmt.Errorf("%s: concurrent state change", operation)
	}
	return nil
}
