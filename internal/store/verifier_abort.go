package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
)

// effectCapabilityAborted is a terminal process-local phase. It is deliberately
// distinct from consumed: abort proves that no verifier worker entry occurred.
const effectCapabilityAborted uint32 = 1 << 31

type verifierAbortCapability struct {
	issuer        *leaseIssuer
	effect        Effect
	capability    *effectCapabilityState
	control       *Store
	ownership     *ControllerOwnership
	permitRequest policy.VerifierExecutionPermitRequest
	expectedPhase uint32
}

// AbortAuthorizedVerifier returns one exact claimed verifier effect to pending
// before a worker capability is prepared. It is the narrow escape hatch for
// authority loss between claim and worker entry; it does not require current
// authority because it grants no execution or verdict capability.
func (s *Store) AbortAuthorizedVerifier(
	ctx context.Context,
	lease AuthorizedVerifierLease,
	detail string,
) error {
	return s.abortVerifierBeforeEntry(ctx, verifierAbortCapability{
		issuer: lease.issuer, effect: cloneEffect(lease.effect), capability: lease.capability,
		control: s, ownership: lease.ownership, permitRequest: lease.permitRequest,
		expectedPhase: effectCapabilityClaimed,
	}, detail)
}

// AbortPreparedAuthorizedVerifier returns an exact prepared verifier effect to
// pending when adapter setup fails before RunVerifier enters the worker. A
// copied RunVerifier and this abort compete on the same shared one-shot phase.
func (s *Store) AbortPreparedAuthorizedVerifier(
	ctx context.Context,
	lease PreparedAuthorizedVerifierLease,
	detail string,
) error {
	return s.abortVerifierBeforeEntry(ctx, verifierAbortCapability{
		issuer: lease.issuer, effect: cloneEffect(lease.effect), capability: lease.capability,
		control: lease.control, ownership: lease.ownership, permitRequest: lease.permitRequest,
		expectedPhase: effectCapabilityPrepared,
	}, detail)
}

func (s *Store) abortVerifierBeforeEntry(
	ctx context.Context,
	abort verifierAbortCapability,
	detail string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.readOnly {
		return errors.New("verifier abort requires a writable Store")
	}
	if strings.TrimSpace(detail) == "" {
		return errors.New("verifier abort requires an error detail")
	}
	if err := s.validateVerifierAbortCapability(abort); err != nil {
		return err
	}
	if err := abort.ownership.ValidateActive(s, abort.permitRequest.ControllerID); err != nil {
		return fmt.Errorf("validate verifier abort ownership: %w", err)
	}
	return abort.ownership.withActiveOperation(
		s, abort.permitRequest.ControllerID,
		func() error { return s.abortVerifierBeforeEntryTransaction(ctx, abort, detail) },
	)
}

func (s *Store) abortVerifierBeforeEntryTransaction(
	ctx context.Context,
	abort verifierAbortCapability,
	detail string,
) error {
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin verifier abort: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	effect, identity, err := s.validateAbortableVerifierLeaseTransaction(ctx, transaction, abort)
	if err != nil {
		return err
	}
	if !abort.capability.phase.CompareAndSwap(abort.expectedPhase, effectCapabilityAborted) {
		return errors.New("authorized verifier lease already entered or terminated")
	}
	restoreCapability := true
	defer func() {
		if restoreCapability {
			abort.capability.phase.CompareAndSwap(effectCapabilityAborted, abort.expectedPhase)
		}
	}()

	encodedIdentity, err := engine.EncodeVerifierAttemptIdentity(identity)
	if err != nil {
		return err
	}
	now := s.now().UTC().UnixMicro()
	if err := insertObservation(ctx, transaction, effect, "not_applied", encodedIdentity, detail, now); err != nil {
		return err
	}
	update, err := transaction.ExecContext(ctx, `
		UPDATE effects
		SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		    completed_at_us = NULL, receipt_json = NULL, last_error = NULL
		WHERE effect_id = ? AND state = 'running' AND owner_id = ? AND attempt = ?
		  AND receipt_json IS NULL`,
		effect.ID, effect.OwnerID, effect.Attempt,
	)
	if err != nil {
		return fmt.Errorf("return pre-entry verifier effect %q to pending: %w", effect.ID, err)
	}
	if err := requireOneRow(update, "return pre-entry verifier effect "+effect.ID+" to pending"); err != nil {
		return err
	}
	// A commit error is an ambiguous durable outcome. Keep the capability
	// terminal: reopening the Store will resolve the journal, while no copied
	// lease can enter a verifier turn in this process.
	restoreCapability = false
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit verifier abort: %w", err)
	}
	return nil
}

func (s *Store) validateAbortableVerifierLeaseTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	abort verifierAbortCapability,
) (Effect, engine.VerifierAttemptIdentity, error) {
	if err := s.validateVerifierAbortCapability(abort); err != nil {
		return Effect{}, engine.VerifierAttemptIdentity{}, err
	}
	request := abort.permitRequest
	closure, err := s.loadExactReviewClosure(ctx, transaction, request.RunID, request.WorkID)
	if err != nil {
		return Effect{}, engine.VerifierAttemptIdentity{}, err
	}
	if closure.state.Revision != request.StateRevision || closure.work.Attempt != request.WorkAttempt ||
		closure.contract.Digest() != request.Contract.Digest() ||
		closure.work.SubmissionID != request.SubmissionID ||
		closure.work.SubmissionDigest != request.SubmissionDigest ||
		closure.work.VerificationDispatchID != request.DispatchID ||
		request.DispatchID != request.VerifierEffectID {
		return Effect{}, engine.VerifierAttemptIdentity{},
			errors.New("verifier abort no longer matches current review truth")
	}
	effect, dispatchDigest, err := s.loadExactVerifierEffect(
		ctx, transaction, closure, request.VerifierEffectID, EffectRunning,
	)
	if err != nil {
		return Effect{}, engine.VerifierAttemptIdentity{}, err
	}
	if dispatchDigest != request.DispatchDigest ||
		request.VerifierProfileDigest != s.verifierProfileDigest ||
		!bytes.Equal(effect.Request, abort.effect.Request) {
		return Effect{}, engine.VerifierAttemptIdentity{},
			errors.New("verifier abort no longer matches its exact request")
	}
	if err := requireRunningLease(effect, EffectLease{issuer: abort.issuer, effect: abort.effect}); err != nil {
		return Effect{}, engine.VerifierAttemptIdentity{}, err
	}
	if len(effect.Result) != 0 {
		return Effect{}, engine.VerifierAttemptIdentity{},
			fmt.Errorf("verifier effect %q has a bound result and cannot be aborted", effect.ID)
	}
	identity, err := loadVerifierAttemptIdentity(ctx, transaction, effect)
	if err != nil {
		return Effect{}, engine.VerifierAttemptIdentity{}, err
	}
	return effect, identity, nil
}

func (s *Store) validateVerifierAbortCapability(abort verifierAbortCapability) error {
	if abort.issuer == nil || abort.issuer != s.leaseIssuer || abort.capability == nil ||
		abort.control != s || abort.ownership == nil ||
		(abort.expectedPhase != effectCapabilityClaimed && abort.expectedPhase != effectCapabilityPrepared) ||
		abort.capability.phase.Load() != abort.expectedPhase || abort.effect.State != EffectRunning ||
		abort.effect.Kind != string(engine.EffectVerifier) ||
		abort.effect.OwnerID != abort.permitRequest.ControllerID {
		return errors.New("verifier abort requires an exact pre-entry authorized lease")
	}
	return nil
}
