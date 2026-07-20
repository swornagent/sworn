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
	Result        json.RawMessage `json:"result,omitempty"`
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

func (lease EffectLease) Invocation() engine.JournalEffect {
	return engine.JournalEffect{
		ID: lease.effect.ID, DeliveryRunID: lease.effect.DeliveryRunID,
		Kind: engine.EffectKind(lease.effect.Kind), Attempt: lease.effect.Attempt,
		Request: append(json.RawMessage(nil), lease.effect.Request...),
	}
}

func (s *Store) validateEffectLease(lease EffectLease) error {
	if lease.issuer == nil || lease.issuer != s.leaseIssuer ||
		lease.effect.State != EffectRunning || !engine.ValidID(lease.effect.ID) ||
		!engine.ValidID(lease.effect.OwnerID) || lease.effect.Attempt < 1 {
		return errors.New("effect operation requires a current store-issued lease")
	}
	return nil
}

func requireRunningLease(effect Effect, lease EffectLease) error {
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
	return nil
}

// SucceededEffect returns one immutable typed journal fact for dependent
// execution. It never treats an unbound artifact as an effect result.
func (s *Store) SucceededEffect(ctx context.Context, effectID string) (engine.JournalEffect, error) {
	return (journalResultResolver{query: s.db}).SucceededEffect(ctx, effectID)
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
		SELECT pending.effect_id FROM effects AS pending
		WHERE pending.state = 'pending'
		  AND NOT EXISTS (
			SELECT 1 FROM effects AS earlier
			WHERE earlier.command_id = pending.command_id
			  AND earlier.ordinal < pending.ordinal
			  AND earlier.state != 'succeeded'
		  )
		ORDER BY pending.created_at_us, pending.command_id, pending.ordinal, pending.effect_id
		LIMIT 1`).Scan(&effectID)
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

// BindEffectResult durably binds one canonical, kind-specific result to the
// exact running lease which observed it. Binding is separate from completion
// so recovery can finish an interrupted success without accepting caller JSON.
func (s *Store) BindEffectResult(ctx context.Context, lease EffectLease, result json.RawMessage) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if err := s.validateEffectLease(lease); err != nil {
		return err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin effect result binding: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return err
	}
	if err := requireRunningLease(effect, lease); err != nil {
		return err
	}
	if len(effect.Result) != 0 {
		if bytes.Equal(effect.Result, result) {
			return validateBoundEffectResult(
				ctx, journalResultResolver{query: transaction}, effect, effect.Result,
			)
		}
		return fmt.Errorf("effect %q is already bound to a different result", effect.ID)
	}
	if err := validateBoundEffectResult(ctx, journalResultResolver{query: transaction}, effect, result); err != nil {
		return err
	}
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects SET receipt_json = ?
		WHERE effect_id = ? AND state = 'running' AND owner_id = ? AND attempt = ? AND receipt_json IS NULL`,
		[]byte(result), effect.ID, effect.OwnerID, effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("bind result for effect %q: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "bind result for effect "+effect.ID); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit effect result binding: %w", err)
	}
	return nil
}

// CompleteEffect closes a running effect only from its already bound typed
// result. The caller cannot substitute different success bytes at completion.
func (s *Store) CompleteEffect(ctx context.Context, lease EffectLease) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if err := s.validateEffectLease(lease); err != nil {
		return err
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
	if err := requireRunningLease(effect, lease); err != nil {
		return err
	}
	if len(effect.Result) == 0 {
		return fmt.Errorf("effect %q has no result bound to attempt %d", effect.ID, effect.Attempt)
	}
	if err := validateBoundEffectResult(ctx, journalResultResolver{query: transaction}, effect, effect.Result); err != nil {
		return err
	}
	now := s.now().UTC().UnixMicro()
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'succeeded', last_error = NULL, completed_at_us = ?
		WHERE effect_id = ? AND state = 'running' AND owner_id = ? AND attempt = ?`,
		now, lease.effect.ID, lease.effect.OwnerID, lease.effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("complete effect %q: %w", lease.effect.ID, err)
	}
	if err := requireOneRow(update, "complete effect "+lease.effect.ID); err != nil {
		return err
	}
	effect.State, effect.LastError, effect.CompletedAtUS = EffectSucceeded, "", now
	if err := insertObservation(ctx, transaction, effect, "succeeded", effect.Result, "", now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit effect completion: %w", err)
	}
	return nil
}

// FailEffect records infrastructure failure only when no trustworthy typed
// result was bound. Known domain outcomes must complete successfully instead.
func (s *Store) FailEffect(ctx context.Context, lease EffectLease, detail string) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if err := s.validateEffectLease(lease); err != nil {
		return err
	}
	if strings.TrimSpace(detail) == "" {
		return errors.New("failed effect requires an error detail")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin effect failure: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return err
	}
	if err := requireRunningLease(effect, lease); err != nil {
		return err
	}
	if len(effect.Result) != 0 {
		return fmt.Errorf("effect %q has a bound result and cannot be failed", effect.ID)
	}
	now := s.now().UTC().UnixMicro()
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'failed', receipt_json = NULL, last_error = ?, completed_at_us = ?
		WHERE effect_id = ? AND state = 'running' AND owner_id = ? AND attempt = ?`,
		detail, now, lease.effect.ID, lease.effect.OwnerID, lease.effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("fail effect %q: %w", lease.effect.ID, err)
	}
	if err := requireOneRow(update, "fail effect "+lease.effect.ID); err != nil {
		return err
	}
	effect.State, effect.LastError, effect.CompletedAtUS = EffectFailed, detail, now
	if err := insertObservation(ctx, transaction, effect, "failed", nil, detail, now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit effect failure: %w", err)
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

// ReconcileUnknownEffect is an explicit manual recovery boundary. In
// particular, a nonempty detail is not machine proof of non-application; an
// autonomous caller must not select ReconcileNotApplied until its effect kind
// has attempt-bound external evidence.
func (s *Store) ReconcileUnknownEffect(
	ctx context.Context,
	effectID string,
	expectedAttempt int64,
	reconcilerID string,
	resolution Reconciliation,
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
	if resolution != ReconcileSucceeded && strings.TrimSpace(detail) == "" {
		return errors.New("non-success reconciliation requires a manual audit detail")
	}
	if resolution == ReconcileSucceeded && detail != "" {
		return errors.New("successful reconciliation cannot carry an error detail")
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
	if resolution == ReconcileSucceeded {
		if len(effect.Result) == 0 {
			return fmt.Errorf("effect %q has no result bound to unknown attempt %d", effectID, expectedAttempt)
		}
		if err := validateBoundEffectResult(ctx, journalResultResolver{query: transaction}, effect, effect.Result); err != nil {
			return err
		}
	} else if len(effect.Result) != 0 {
		return fmt.Errorf("effect %q has a bound result and cannot be reconciled as %s", effectID, resolution)
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
			SET state = 'succeeded', last_error = NULL, completed_at_us = ?
			WHERE effect_id = ? AND state = 'unknown' AND attempt = ?`, now, effectID, expectedAttempt)
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
	var observedResult json.RawMessage
	if resolution == ReconcileSucceeded {
		observedResult = effect.Result
	}
	if err := insertObservation(ctx, transaction, effect, string(resolution), observedResult, detail, now); err != nil {
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
		effect.Result = json.RawMessage(receipt.String)
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
	effect.Result = append(json.RawMessage(nil), effect.Result...)
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
