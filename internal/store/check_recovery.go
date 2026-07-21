package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
)

// CheckRecoveryLease is a Store-issued capability for one exact unknown,
// unbound local-check attempt. It authorizes quiescence and cleanup inspection,
// not a journal transition by itself.
type CheckRecoveryLease struct {
	issuer     *leaseIssuer
	effect     Effect
	identity   engine.CheckAttemptIdentity
	capability *effectCapabilityState
	control    *Store
	ownership  *ControllerOwnership
	ownerID    string
}

// CheckRetryProof is opaque Store-owned evidence that the exact recovery
// capability was consumed and its content-bound executor/local workspace was
// reconciled. Orphan CAS writes are harmless; no typed journal result was
// applied, so the exact attempt may be retried.
type CheckRetryProof struct {
	issuer        *leaseIssuer
	capability    *effectCapabilityState
	effectID      string
	effectAttempt int64
	identity      engine.CheckAttemptIdentity
	cleanup       executor.ContentBoundCleanup
}

// ReconcileCheck retains recovery ownership across the complete worker cleanup
// and seals the lower-level opaque executor proof to this exact issuance.
func (lease CheckRecoveryLease) ReconcileCheck(
	reconcile func(engine.JournalEffect) (executor.ContentBoundCleanup, error),
) (proof CheckRetryProof, resultErr error) {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer || lease.ownership == nil ||
		lease.effect.State != EffectUnknown || len(lease.effect.Result) != 0 || reconcile == nil {
		return CheckRetryProof{}, errors.New("check reconciliation requires an unbound Store recovery capability")
	}
	resultErr = lease.ownership.withRecoveryOperation(
		lease.control, lease.ownerID, func() error {
			if !lease.capability.phase.CompareAndSwap(
				effectCapabilityPrepared, effectCapabilityConsumed,
			) {
				return errors.New("check reconciliation capability was already consumed")
			}
			cleanup, err := reconcile(journalEffect(lease.effect))
			if err != nil {
				return err
			}
			if cleanup.InvocationID() != lease.identity.InvocationID {
				return errors.New("check retry cleanup does not match its exact recovery capability")
			}
			if !lease.capability.phase.CompareAndSwap(
				effectCapabilityConsumed, effectCapabilityProven,
			) {
				return errors.New("check retry proof was already sealed")
			}
			proof = CheckRetryProof{
				issuer: lease.issuer, capability: lease.capability,
				effectID: lease.effect.ID, effectAttempt: lease.effect.Attempt,
				identity: lease.identity, cleanup: cleanup,
			}
			return nil
		},
	)
	if resultErr != nil {
		return CheckRetryProof{}, resultErr
	}
	return proof, nil
}

// PrepareControlledUnboundCheckRecovery validates an exact unknown native
// check attempt before the recovery owner may inspect or remove its residue.
func (s *Store) PrepareControlledUnboundCheckRecovery(
	ctx context.Context,
	ownership *ControllerOwnership,
	ownerID string,
	effectID string,
	expectedAttempt int64,
) (CheckRecoveryLease, error) {
	if s == nil || s.readOnly {
		return CheckRecoveryLease{}, errors.New("control store is read-only")
	}
	if ownership == nil {
		return CheckRecoveryLease{}, ErrInvalidControllerOwnership
	}
	if !engine.ValidID(effectID) || expectedAttempt < 1 {
		return CheckRecoveryLease{}, errors.New("valid check effect and attempt are required for recovery")
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return CheckRecoveryLease{}, fmt.Errorf("validate check recovery ownership: %w", err)
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CheckRecoveryLease{}, fmt.Errorf("begin check recovery preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return CheckRecoveryLease{}, fmt.Errorf("validate check recovery transaction ownership: %w", err)
	}
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return CheckRecoveryLease{}, err
	}
	if effect.State != EffectUnknown || effect.Attempt != expectedAttempt || len(effect.Result) != 0 {
		return CheckRecoveryLease{}, fmt.Errorf(
			"effect %q is %s at attempt %d with bound=%t, want unknown unbound attempt %d",
			effectID, effect.State, effect.Attempt, len(effect.Result) != 0, expectedAttempt,
		)
	}
	identity, err := s.validateNativeCheckAttempt(ctx, transaction, effect)
	if err != nil {
		return CheckRecoveryLease{}, err
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return CheckRecoveryLease{}, fmt.Errorf("revalidate check recovery ownership: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return CheckRecoveryLease{}, fmt.Errorf("finish check recovery preparation: %w", err)
	}
	return CheckRecoveryLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect), identity: identity,
		capability: newEffectCapabilityState(effectCapabilityPrepared), control: s,
		ownership: ownership, ownerID: ownerID,
	}, nil
}

// RecoverControlledUnboundCheckEffect requeues only the exact attempt whose
// Store capability sealed a matching content-bound cleanup proof.
func (s *Store) RecoverControlledUnboundCheckEffect(
	ctx context.Context,
	ownership *ControllerOwnership,
	ownerID string,
	lease CheckRecoveryLease,
	proof CheckRetryProof,
) error {
	if ownership == nil || lease.ownership != ownership || lease.ownerID != ownerID {
		return ErrInvalidControllerOwnership
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return fmt.Errorf("validate unbound check recovery ownership: %w", err)
	}
	if lease.issuer == nil || lease.issuer != s.leaseIssuer ||
		lease.effect.State != EffectUnknown || !engine.ValidID(ownerID) {
		return errors.New("unbound check recovery requires a current Store-issued lease and owner")
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin unbound check recovery: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return fmt.Errorf("validate unbound check transaction ownership: %w", err)
	}
	effect, err := loadEffect(ctx, transaction, lease.effect.ID)
	if err != nil {
		return err
	}
	if effect.Attempt != lease.effect.Attempt ||
		(effect.State != EffectUnknown && effect.State != EffectPending) ||
		effect.DeliveryRunID != lease.effect.DeliveryRunID ||
		effect.CommandID != lease.effect.CommandID || effect.Ordinal != lease.effect.Ordinal ||
		effect.Kind != lease.effect.Kind || !bytes.Equal(effect.Request, lease.effect.Request) {
		return fmt.Errorf("effect %q no longer matches its unknown recovery lease", lease.effect.ID)
	}
	identity, err := s.validateNativeCheckAttempt(ctx, transaction, effect)
	if err != nil {
		return err
	}
	if identity != lease.identity || proof.issuer == nil || proof.issuer != s.leaseIssuer ||
		proof.capability == nil || proof.capability != lease.capability ||
		proof.capability.phase.Load() != effectCapabilityProven ||
		proof.effectID != effect.ID || proof.effectAttempt != effect.Attempt ||
		proof.identity != identity || proof.cleanup.InvocationID() != identity.InvocationID {
		return errors.New("unbound check recovery proof does not match its current journal and configuration")
	}
	encodedIdentity, err := engine.EncodeCheckAttemptIdentity(identity)
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
			return errors.New("pending check retry lacks its exact not-applied witness")
		}
		return nil
	}
	now := s.now().UTC().UnixMicro()
	effect.OwnerID = ownerID
	if err := insertObservation(ctx, transaction, effect, "not_applied", encodedIdentity, "", now); err != nil {
		return err
	}
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		    completed_at_us = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'unknown' AND attempt = ? AND receipt_json IS NULL`,
		effect.ID, effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("requeue reconciled check effect %q: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "requeue reconciled check effect "+effect.ID); err != nil {
		return err
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return fmt.Errorf("revalidate unbound check recovery ownership: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit unbound check recovery: %w", err)
	}
	return nil
}

func (s *Store) validateNativeCheckAttempt(
	ctx context.Context,
	transaction *sql.Tx,
	effect Effect,
) (engine.CheckAttemptIdentity, error) {
	if engine.EffectKind(effect.Kind) != engine.EffectLocalCheck || len(effect.Result) != 0 {
		return engine.CheckAttemptIdentity{}, errors.New("native check attempt requires a check without a result")
	}
	identity, err := loadCheckAttemptIdentity(ctx, transaction, effect)
	if err != nil {
		return engine.CheckAttemptIdentity{}, err
	}
	request, err := engine.ParseLocalCheckEffectRequest(effect.Request)
	if err != nil || request.RuntimeManifestDigest != s.localCheckRuntimeManifestDigest ||
		identity.RuntimeManifestDigest != s.localCheckRuntimeManifestDigest {
		return engine.CheckAttemptIdentity{}, errors.New("native check attempt does not match the configured runtime")
	}
	state, found, err := loadState(ctx, transaction, effect.DeliveryRunID)
	if err != nil || !found || state.Phase != engine.PhaseActive {
		return engine.CheckAttemptIdentity{}, errors.New("native check attempt cannot resolve its active delivery state")
	}
	attempt, err := loadExactAttempt(ctx, transaction, state, request.WorkID, engine.WorkChecking)
	if err != nil || attempt.number != request.WorkAttempt {
		return engine.CheckAttemptIdentity{}, errors.New("native check attempt does not match its checking work")
	}
	_, effects, err := loadExactCheckBatch(ctx, transaction, state, attempt)
	if err != nil {
		return engine.CheckAttemptIdentity{}, err
	}
	for _, exact := range effects {
		if exact.ID == effect.ID && exact.Attempt == effect.Attempt && exact.Kind == effect.Kind &&
			bytes.Equal(exact.Request, effect.Request) {
			return identity, nil
		}
	}
	return engine.CheckAttemptIdentity{}, errors.New("native check attempt is absent from its exact dispatch batch")
}

func loadCheckAttemptIdentity(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) (engine.CheckAttemptIdentity, error) {
	var encoded []byte
	if err := query.QueryRowContext(ctx, `
		SELECT receipt_json FROM effect_observations
		WHERE effect_id = ? AND attempt = ? AND kind = 'claimed'`,
		effect.ID, effect.Attempt,
	).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return engine.CheckAttemptIdentity{}, errors.New("check attempt predates the durable attempt witness")
		}
		return engine.CheckAttemptIdentity{}, fmt.Errorf("load check attempt identity: %w", err)
	}
	identity, err := engine.ParseCheckAttemptIdentity(encoded)
	if err != nil {
		return engine.CheckAttemptIdentity{}, fmt.Errorf("validate check attempt identity: %w", err)
	}
	request, requestErr := engine.ParseLocalCheckEffectRequest(effect.Request)
	if requestErr != nil || identity.EffectID != effect.ID ||
		identity.EffectAttempt != effect.Attempt ||
		identity.RuntimeManifestDigest != request.RuntimeManifestDigest {
		return engine.CheckAttemptIdentity{}, errors.New("check attempt identity does not match its journal")
	}
	return identity, nil
}
