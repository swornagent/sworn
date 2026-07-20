package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/repo"
)

const (
	buildCapabilityClaimed uint32 = iota + 1
	buildCapabilityPrepared
	buildCapabilityConsumed
	buildCapabilityProven
)

// buildCapabilityState is deliberately shared by every value copy of an
// opaque capability. Its atomic phase closes copied-value and concurrent-use
// races without pretending the process-local token survives a restart.
type buildCapabilityState struct {
	phase atomic.Uint32
}

func newBuildCapabilityState(phase uint32) *buildCapabilityState {
	state := new(buildCapabilityState)
	state.phase.Store(phase)
	return state
}

// RunBuilder is the process-local dispatch boundary for one prepared
// native build. It retains active ownership across the complete callback and
// permits exactly one value copy to enter it.
func (lease PreparedAuthorizedBuildLease) RunBuilder(
	run func(engine.JournalEffect) (json.RawMessage, error),
) (result json.RawMessage, resultErr error) {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer ||
		lease.ownership == nil || lease.effect.State != EffectRunning ||
		lease.effect.OwnerID != lease.permitRequest.ControllerID || run == nil {
		return nil, errors.New("builder execution requires a prepared Store capability")
	}
	resultErr = lease.ownership.withActiveBuildOperation(
		lease.control, lease.permitRequest.ControllerID, func() error {
			if !lease.capability.phase.CompareAndSwap(
				buildCapabilityPrepared, buildCapabilityConsumed,
			) {
				return errors.New("prepared builder execution capability was already consumed")
			}
			var err error
			result, err = run(journalEffect(lease.effect))
			return err
		},
	)
	if resultErr != nil {
		return nil, resultErr
	}
	return result, nil
}

// BoundBuildCleanupLease authorizes exactly one entry into builder cleanup for
// an exact unknown attempt whose typed result is already durable.
type BoundBuildCleanupLease struct {
	issuer     *leaseIssuer
	effect     Effect
	capability *buildCapabilityState
	control    *Store
	ownership  *ControllerOwnership
	ownerID    string
}

// RunBuilderCleanup retains recovery ownership while one exact bound invocation
// enters the builder cleanup implementation.
func (lease BoundBuildCleanupLease) RunBuilderCleanup(
	cleanup func(engine.JournalEffect) error,
) error {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer ||
		lease.ownership == nil || lease.effect.State != EffectUnknown ||
		len(lease.effect.Result) == 0 || cleanup == nil {
		return errors.New("builder cleanup requires a bound Store recovery capability")
	}
	return lease.ownership.withRecoveryBuildOperation(
		lease.control, lease.ownerID, func() error {
			if !lease.capability.phase.CompareAndSwap(
				buildCapabilityPrepared, buildCapabilityConsumed,
			) {
				return errors.New("builder cleanup capability was already consumed")
			}
			return cleanup(journalEffect(lease.effect))
		},
	)
}

// PrepareControlledBoundBuildCleanup validates an exact unknown native build
// with a durable typed result before the recovery owner may remove attempt
// residue.
func (s *Store) PrepareControlledBoundBuildCleanup(
	ctx context.Context,
	ownership *ControllerOwnership,
	ownerID string,
	effectID string,
	expectedAttempt int64,
) (BoundBuildCleanupLease, error) {
	if s == nil || s.readOnly {
		return BoundBuildCleanupLease{}, errors.New("control store is read-only")
	}
	if ownership == nil {
		return BoundBuildCleanupLease{}, ErrInvalidControllerOwnership
	}
	if !engine.ValidID(effectID) || expectedAttempt < 1 {
		return BoundBuildCleanupLease{}, errors.New("valid bound build effect and attempt are required for cleanup")
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return BoundBuildCleanupLease{}, fmt.Errorf("validate bound build cleanup ownership: %w", err)
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return BoundBuildCleanupLease{}, fmt.Errorf("begin bound build cleanup preparation: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return BoundBuildCleanupLease{}, fmt.Errorf("validate bound build cleanup transaction ownership: %w", err)
	}
	effect, err := loadEffect(ctx, transaction, effectID)
	if err != nil {
		return BoundBuildCleanupLease{}, err
	}
	if effect.State != EffectUnknown || effect.Attempt != expectedAttempt || len(effect.Result) == 0 {
		return BoundBuildCleanupLease{}, fmt.Errorf(
			"effect %q is %s at attempt %d with bound=%t, want unknown bound attempt %d",
			effectID, effect.State, effect.Attempt, len(effect.Result) != 0, expectedAttempt,
		)
	}
	if _, _, err := s.validateBoundNativeBuildAttempt(ctx, transaction, effect); err != nil {
		return BoundBuildCleanupLease{}, err
	}
	if err := ownership.ValidateRecovery(s, ownerID); err != nil {
		return BoundBuildCleanupLease{}, fmt.Errorf("revalidate bound build cleanup ownership: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return BoundBuildCleanupLease{}, fmt.Errorf("finish bound build cleanup preparation: %w", err)
	}
	return BoundBuildCleanupLease{
		issuer: s.leaseIssuer, effect: cloneEffect(effect),
		capability: newBuildCapabilityState(buildCapabilityPrepared), control: s,
		ownership: ownership, ownerID: ownerID,
	}, nil
}

func (s *Store) validateBoundNativeBuildAttempt(
	ctx context.Context,
	query rowQuerier,
	effect Effect,
) (engine.BuildAttemptIdentity, engine.State, error) {
	if engine.EffectKind(effect.Kind) != engine.EffectBuild || len(effect.Result) == 0 {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("bound native build cleanup requires a build result")
	}
	identity, err := loadBuildAttemptIdentity(ctx, query, effect)
	if err != nil {
		return engine.BuildAttemptIdentity{}, engine.State{}, err
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil || request.SchemaVersion != engine.BuildEffectRequestSchemaVersion {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("bound native build cleanup requires a native build request")
	}
	state, found, err := loadState(ctx, query, effect.DeliveryRunID)
	if err != nil || !found {
		return engine.BuildAttemptIdentity{}, engine.State{},
			errors.New("bound native build cleanup cannot resolve its delivery state")
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
			errors.New("bound native build cleanup does not match its current journal and configuration")
	}
	if err := validateBoundEffectResult(
		ctx, journalResultResolver{query: query}, effect, effect.Result,
	); err != nil {
		return engine.BuildAttemptIdentity{}, engine.State{}, err
	}
	return identity, state, nil
}

// BuildRetryProof is opaque Store-owned evidence that the exact recovery
// capability was consumed and both lower-level not-applied proofs matched.
// It remains replayable so a journal transition can converge after commit
// ambiguity without repeating external cleanup.
type BuildRetryProof struct {
	issuer        *leaseIssuer
	capability    *buildCapabilityState
	effectID      string
	effectAttempt int64
	identity      engine.BuildAttemptIdentity
	repositoryID  string
	targetRef     string
	unpublished   repo.AttemptUnpublishedProof
	writable      executor.WritableCleanup
}

// ReconcileBuilder retains recovery ownership while exactly one callback
// proves absence of publication and cleans attempt-owned workspaces. It binds
// those opaque lower proofs to this issuance before releasing ownership.
func (lease BuildRecoveryLease) ReconcileBuilder(
	reconcile func(engine.JournalEffect) (
		repo.AttemptUnpublishedProof,
		executor.WritableCleanup,
		error,
	),
) (proof BuildRetryProof, resultErr error) {
	if lease.issuer == nil || lease.capability == nil || lease.control == nil ||
		lease.issuer != lease.control.leaseIssuer ||
		lease.ownership == nil || lease.effect.State != EffectUnknown ||
		len(lease.effect.Result) != 0 || reconcile == nil {
		return BuildRetryProof{}, errors.New("builder reconciliation requires an unbound Store recovery capability")
	}
	resultErr = lease.ownership.withRecoveryBuildOperation(
		lease.control, lease.ownerID, func() error {
			if !lease.capability.phase.CompareAndSwap(
				buildCapabilityPrepared, buildCapabilityConsumed,
			) {
				return errors.New("builder reconciliation capability was already consumed")
			}
			unpublished, writable, err := reconcile(journalEffect(lease.effect))
			if err != nil {
				return err
			}
			if unpublished.RepositoryID() != lease.repositoryID ||
				unpublished.AttemptID() != lease.identity.InvocationID ||
				writable.InvocationID() != lease.identity.InvocationID {
				return errors.New("build retry proof does not match its exact recovery capability")
			}
			if !lease.capability.phase.CompareAndSwap(
				buildCapabilityConsumed, buildCapabilityProven,
			) {
				return errors.New("build retry proof was already sealed")
			}
			proof = BuildRetryProof{
				issuer: lease.issuer, capability: lease.capability,
				effectID: lease.effect.ID, effectAttempt: lease.effect.Attempt,
				identity:     lease.identity,
				repositoryID: lease.repositoryID, targetRef: lease.targetRef,
				unpublished: unpublished, writable: writable,
			}
			return nil
		},
	)
	if resultErr != nil {
		return BuildRetryProof{}, resultErr
	}
	return proof, nil
}
