package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	effectworker "github.com/swornagent/sworn/internal/effects"
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

// BuildRecoveryLease is a Store-issued capability for one exact unknown,
// unbound build attempt whose durable claimed witness has already validated.
// It authorizes external cleanup inspection, not a lifecycle transition by
// itself.
type BuildRecoveryLease struct {
	issuer    *leaseIssuer
	effect    Effect
	identity  engine.BuildAttemptIdentity
	challenge string
}

func (lease BuildRecoveryLease) Invocation() engine.JournalEffect {
	return journalEffect(lease.effect)
}

func (lease BuildRecoveryLease) Challenge() string { return lease.challenge }

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
		effect.Ordinal != lease.effect.Ordinal || effect.Kind != lease.effect.Kind ||
		!bytes.Equal(effect.Request, lease.effect.Request) {
		return fmt.Errorf("effect %q no longer matches its issued lease", lease.effect.ID)
	}
	return nil
}

// PrepareNativeBuildExecution reloads and validates the Store-issued lease and
// its durable attempt witness before the composition service may cross the
// external builder boundary. It returns journal bytes from current Store truth,
// never the claim-time lease projection.
func (s *Store) PrepareNativeBuildExecution(
	ctx context.Context,
	lease EffectLease,
) (engine.JournalEffect, error) {
	if s.readOnly {
		return engine.JournalEffect{}, errors.New("control store is read-only")
	}
	if err := s.validateEffectLease(lease); err != nil {
		return engine.JournalEffect{}, err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return engine.JournalEffect{}, fmt.Errorf("begin native build preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return engine.JournalEffect{}, err
	}
	if err := requireRunningLease(effect, lease); err != nil {
		return engine.JournalEffect{}, err
	}
	if engine.EffectKind(effect.Kind) != engine.EffectBuild || len(effect.Result) != 0 {
		return engine.JournalEffect{}, errors.New("native build execution requires an unbound running build")
	}
	if _, _, err := s.validateNativeBuildAttempt(ctx, transaction, effect); err != nil {
		return engine.JournalEffect{}, err
	}
	if err := transaction.Commit(); err != nil {
		return engine.JournalEffect{}, fmt.Errorf("finish native build preparation: %w", err)
	}
	return journalEffect(effect), nil
}

// SucceededEffect returns one immutable typed journal fact for dependent
// execution. It never treats an unbound artifact as an effect result.
func (s *Store) SucceededEffect(ctx context.Context, effectID string) (engine.JournalEffect, error) {
	return (journalResultResolver{query: s.db}).SucceededEffect(ctx, effectID)
}

// UnknownEffects exposes stopped journal facts for startup reconciliation. It
// does not make them claimable or grant a lifecycle transition.
func (s *Store) UnknownEffects(ctx context.Context) ([]engine.JournalEffect, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE state = 'unknown' ORDER BY created_at_us, effect_id`)
	if err != nil {
		return nil, fmt.Errorf("list unknown effects: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var unknown []engine.JournalEffect
	for rows.Next() {
		effect, err := scanEffect(rows)
		if err != nil {
			return nil, err
		}
		unknown = append(unknown, journalEffect(effect))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unknown effects: %w", err)
	}
	return unknown, nil
}

func journalEffect(effect Effect) engine.JournalEffect {
	return engine.JournalEffect{
		ID: effect.ID, DeliveryRunID: effect.DeliveryRunID, Kind: engine.EffectKind(effect.Kind),
		Attempt: effect.Attempt, Request: append(json.RawMessage(nil), effect.Request...),
		Result: append(json.RawMessage(nil), effect.Result...),
	}
}

func loadBuildAttemptIdentity(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) (engine.BuildAttemptIdentity, error) {
	var encoded []byte
	if err := query.QueryRowContext(ctx, `
		SELECT receipt_json FROM effect_observations
		WHERE effect_id = ? AND attempt = ? AND kind = 'claimed'`,
		effect.ID, effect.Attempt,
	).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return engine.BuildAttemptIdentity{}, errors.New("build attempt predates the durable attempt witness")
		}
		return engine.BuildAttemptIdentity{}, fmt.Errorf("load build attempt identity: %w", err)
	}
	if len(encoded) == 0 {
		return engine.BuildAttemptIdentity{}, errors.New("build attempt predates the durable attempt witness")
	}
	identity, err := engine.ParseBuildAttemptIdentity(encoded)
	if err != nil {
		return engine.BuildAttemptIdentity{}, fmt.Errorf("validate build attempt identity: %w", err)
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil || identity.EffectID != effect.ID || identity.EffectAttempt != effect.Attempt ||
		identity.BuilderDispatchDigest != request.BuilderDispatchDigest {
		return engine.BuildAttemptIdentity{}, errors.New("build attempt identity does not match its journal")
	}
	return identity, nil
}

// PrepareUnboundBuildRecovery validates journal authority before any caller is
// permitted to inspect or remove attempt-owned external residue. Legacy or
// corrupt claimed observations therefore stop without touching Git or disk.
func (s *Store) PrepareUnboundBuildRecovery(
	ctx context.Context,
	effectID string,
	expectedAttempt int64,
) (BuildRecoveryLease, error) {
	if s.readOnly {
		return BuildRecoveryLease{}, errors.New("control store is read-only")
	}
	if !engine.ValidID(effectID) || expectedAttempt < 1 {
		return BuildRecoveryLease{}, errors.New("valid build effect and attempt are required for recovery")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return BuildRecoveryLease{}, fmt.Errorf("begin build recovery preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return BuildRecoveryLease{}, err
	}
	if effect.State != EffectUnknown || effect.Attempt != expectedAttempt {
		return BuildRecoveryLease{}, fmt.Errorf(
			"effect %q is %s at attempt %d, want unknown at attempt %d",
			effectID, effect.State, effect.Attempt, expectedAttempt,
		)
	}
	identity, _, err := s.validateNativeBuildAttempt(ctx, transaction, effect)
	if err != nil {
		return BuildRecoveryLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return BuildRecoveryLease{}, fmt.Errorf("finish build recovery preparation: %w", err)
	}
	challenge, err := newBuildRecoveryChallenge()
	if err != nil {
		return BuildRecoveryLease{}, err
	}
	return BuildRecoveryLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect), identity: identity, challenge: challenge,
	}, nil
}

func newBuildRecoveryChallenge() (string, error) {
	var contents [32]byte
	if _, err := rand.Read(contents[:]); err != nil {
		return "", fmt.Errorf("generate build recovery challenge: %w", err)
	}
	return "recovery-" + hex.EncodeToString(contents[:]), nil
}

func (s *Store) validateNativeBuildAttempt(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) (engine.BuildAttemptIdentity, engine.State, error) {
	if engine.EffectKind(effect.Kind) != engine.EffectBuild || len(effect.Result) != 0 {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("native build attempt requires a build without a result")
	}
	identity, err := loadBuildAttemptIdentity(ctx, query, effect)
	if err != nil {
		return engine.BuildAttemptIdentity{}, engine.State{}, err
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil || request.SchemaVersion != engine.BuildEffectRequestSchemaVersion {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("native build attempt requires a native build request")
	}
	state, found, err := loadState(ctx, query, effect.DeliveryRunID)
	if err != nil || !found {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("native build attempt cannot resolve its delivery state")
	}
	matchedWork := false
	for _, work := range state.Work {
		matchedWork = matchedWork || work.ID == request.WorkID &&
			work.Attempt == request.WorkAttempt && work.State == engine.WorkActive
	}
	if s.builderDispatchDigest == "" || s.repository == nil || !matchedWork ||
		s.repository.Binding().RepositoryID != state.Repository ||
		request.DeliveryRunID != state.RunID || request.DeliveryID != state.DeliveryID ||
		request.BuilderDispatchDigest != s.builderDispatchDigest ||
		identity.BuilderDispatchDigest != s.builderDispatchDigest {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("native build attempt does not match its current journal and configuration")
	}
	return identity, state, nil
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
	claimReceipt, err := s.claimReceipt(effect)
	if err != nil {
		return EffectLease{}, err
	}
	if err := insertObservation(ctx, transaction, effect, "claimed", claimReceipt, "", now); err != nil {
		return EffectLease{}, err
	}
	if err := transaction.Commit(); err != nil {
		return EffectLease{}, fmt.Errorf("commit effect claim: %w", err)
	}
	return EffectLease{issuer: s.leaseIssuer, effect: cloneEffect(effect)}, nil
}

func (s *Store) claimReceipt(effect Effect) (json.RawMessage, error) {
	if engine.EffectKind(effect.Kind) != engine.EffectBuild {
		return nil, nil
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil {
		return nil, fmt.Errorf("parse claimed build request: %w", err)
	}
	if request.SchemaVersion == engine.LegacyBuildEffectRequestSchemaVersion {
		return nil, nil
	}
	if request.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		s.builderDispatchDigest == "" || request.BuilderDispatchDigest != s.builderDispatchDigest {
		return nil, errors.New("build effect does not match the configured builder dispatch")
	}
	identity, err := engine.BuildAttemptIdentityFor(
		effect.ID, effect.Attempt, request.BuilderDispatchDigest,
	)
	if err != nil {
		return nil, err
	}
	return engine.EncodeBuildAttemptIdentity(identity)
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
	if engine.EffectKind(effect.Kind) == engine.EffectBuild {
		request, err := engine.ParseBuildEffectRequest(effect.Request)
		if err != nil {
			return err
		}
		if request.SchemaVersion == engine.BuildEffectRequestSchemaVersion {
			if err := s.ensureBoundBuildPublished(ctx, transaction, effect); err != nil {
				return err
			}
		}
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

// RecoverBoundEffect closes an interrupted attempt only from the immutable
// typed result bound by that attempt. It never makes an unbound effect
// retryable or converts an operator assertion into external evidence. Effect
// ID and attempt form the replay identity; reconcilerID attributes only the
// process that wins the unknown-to-succeeded transition.
func (s *Store) RecoverBoundEffect(
	ctx context.Context,
	effectID string,
	expectedAttempt int64,
	reconcilerID string,
) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if !engine.ValidID(effectID) {
		return errors.New("valid effect id is required for recovery")
	}
	if !engine.ValidID(reconcilerID) {
		return errors.New("valid reconciler id is required")
	}
	if expectedAttempt < 1 {
		return errors.New("positive effect attempt is required for recovery")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin bound-effect recovery: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return err
	}
	if effect.Attempt != expectedAttempt {
		return fmt.Errorf(
			"effect %q is %s at attempt %d, want attempt %d",
			effectID, effect.State, effect.Attempt, expectedAttempt,
		)
	}
	if effect.State != EffectUnknown && effect.State != EffectSucceeded {
		return fmt.Errorf("effect %q is %s at attempt %d, want unknown or succeeded", effectID, effect.State, effect.Attempt)
	}
	if len(effect.Result) == 0 {
		return fmt.Errorf("effect %q has no result bound to unknown attempt %d", effectID, expectedAttempt)
	}
	if err := validateBoundEffectResult(ctx, journalResultResolver{query: transaction}, effect, effect.Result); err != nil {
		return err
	}
	if engine.EffectKind(effect.Kind) == engine.EffectBuild {
		if err := s.ensureBoundBuildPublished(ctx, transaction, effect); err != nil {
			return err
		}
	}
	if effect.State == EffectSucceeded {
		return nil
	}
	now := s.now().UTC().UnixMicro()
	result, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'succeeded', last_error = NULL, completed_at_us = ?
		WHERE effect_id = ? AND state = 'unknown' AND attempt = ?`, now, effectID, expectedAttempt)
	if err != nil {
		return fmt.Errorf("recover bound effect %q: %w", effectID, err)
	}
	if err := requireOneRow(result, "recover bound effect "+effectID); err != nil {
		return err
	}
	effect.OwnerID = reconcilerID
	if err := insertObservation(ctx, transaction, effect, "succeeded", effect.Result, "", now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit bound-effect recovery: %w", err)
	}
	return nil
}

func (s *Store) ensureBoundBuildPublished(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) error {
	if s.repository == nil {
		return errors.New("bound build success requires the immutable configured repository")
	}
	state, found, stateErr := loadState(ctx, query, effect.DeliveryRunID)
	request, requestErr := engine.ParseBuildEffectRequest(effect.Request)
	build, resultErr := engine.ParseBuildEffectResult(effect.Result)
	if stateErr != nil || requestErr != nil || resultErr != nil || !found ||
		s.repository.Binding().RepositoryID != state.Repository ||
		request.DeliveryRunID != state.RunID || request.DeliveryID != state.DeliveryID ||
		build.Candidate.RepositoryID != state.Repository || build.Candidate.TargetRef != state.TargetRef {
		return errors.New("bound build success does not match its configured delivery repository and target")
	}
	matchedAttempt := false
	for _, work := range state.Work {
		matchedAttempt = matchedAttempt || work.ID == request.WorkID &&
			work.Attempt == request.WorkAttempt &&
			(effect.State == EffectSucceeded || work.State == engine.WorkActive)
	}
	if !matchedAttempt {
		return errors.New("bound build success does not match its current work attempt")
	}
	if request.SchemaVersion == engine.LegacyBuildEffectRequestSchemaVersion {
		if err := s.repository.EnsureCandidate(ctx, build.Candidate); err != nil {
			return fmt.Errorf("repair legacy bound build candidate: %w", err)
		}
		return nil
	}
	if request.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		request.BuilderDispatchDigest != s.builderDispatchDigest {
		return errors.New("bound build success does not match its configured native dispatch")
	}
	plan, err := loadExactPlan(ctx, query, state.PlanDigest)
	if err != nil {
		return fmt.Errorf("load bound build plan: %w", err)
	}
	contract, exists := plan.Work(request.WorkID)
	if !exists || contract.Digest() != request.DispatchDigest {
		return errors.New("bound build success does not match its exact work contract")
	}
	attempt, err := engine.BuildAttemptIdentityFor(
		effect.ID, effect.Attempt, request.BuilderDispatchDigest,
	)
	if err != nil {
		return err
	}
	if err := s.repository.EnsureAttemptCandidate(ctx, attempt.InvocationID, build.Candidate); err != nil {
		return fmt.Errorf("publish bound build candidate: %w", err)
	}
	return nil
}

// RecoverUnboundBuildEffect consumes the exact prevalidated Store lease and
// composite proof minted by the configured builder only after unpublished Git
// state and all attempt-owned cleanup have been established. They are valid
// only while the command service retains exclusive controller ownership.
func (s *Store) RecoverUnboundBuildEffect(
	ctx context.Context,
	lease BuildRecoveryLease,
	reconcilerID string,
	proof effectworker.BuildRetryProof,
) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	if lease.issuer == nil || lease.issuer != s.leaseIssuer ||
		lease.effect.State != EffectUnknown || !engine.ValidID(reconcilerID) {
		return errors.New("unbound build recovery requires a current Store-issued lease and reconciler")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin unbound build recovery: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return err
	}
	if effect.Attempt != lease.effect.Attempt ||
		(effect.State != EffectUnknown && effect.State != EffectPending) ||
		effect.DeliveryRunID != lease.effect.DeliveryRunID ||
		effect.CommandID != lease.effect.CommandID || effect.Ordinal != lease.effect.Ordinal ||
		effect.Kind != lease.effect.Kind || !bytes.Equal(effect.Request, lease.effect.Request) {
		return fmt.Errorf(
			"effect %q no longer matches its unknown recovery lease", lease.effect.ID,
		)
	}
	identity, state, err := s.validateNativeBuildAttempt(ctx, transaction, effect)
	if err != nil {
		return err
	}
	if identity != lease.identity || proof.EffectID() != effect.ID ||
		proof.EffectAttempt() != effect.Attempt || proof.InvocationID() != identity.InvocationID ||
		proof.RecoveryChallenge() != lease.challenge ||
		proof.BuilderDispatchDigest() != identity.BuilderDispatchDigest ||
		proof.RepositoryID() != state.Repository || proof.TargetRef() != state.TargetRef ||
		proof.WritableCleanup().InvocationID() != identity.InvocationID ||
		proof.Unpublished().RepositoryID() != state.Repository ||
		proof.Unpublished().AttemptID() != identity.InvocationID {
		return errors.New("unbound build recovery proof does not match its current journal and configuration")
	}
	encodedIdentity, err := engine.EncodeBuildAttemptIdentity(identity)
	if err != nil {
		return err
	}
	if effect.State == EffectPending {
		var receipt []byte
		if err := transaction.QueryRowContext(ctx, `
			SELECT receipt_json FROM effect_observations
			WHERE effect_id = ? AND attempt = ? AND kind = 'not_applied'`,
			effect.ID, effect.Attempt,
		).Scan(&receipt); err != nil || !bytes.Equal(receipt, encodedIdentity) {
			return errors.New("pending build retry lacks its exact not-applied witness")
		}
		return nil
	}
	now := s.now().UTC().UnixMicro()
	effect.OwnerID = reconcilerID
	if err := insertObservation(ctx, transaction, effect, "not_applied", encodedIdentity, "", now); err != nil {
		return err
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		    completed_at_us = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'unknown' AND attempt = ? AND receipt_json IS NULL`,
		effect.ID, effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("requeue reconciled build effect %q: %w", effect.ID, err)
	}
	if err := requireOneRow(result, "requeue reconciled build effect "+effect.ID); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit unbound build recovery: %w", err)
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
