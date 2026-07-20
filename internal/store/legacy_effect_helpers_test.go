package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/swornagent/sworn/internal/engine"
)

// These helpers preserve lower-level journal coverage without reopening the
// exported controller/authority gates which production callers must cross.
func (s *Store) claimNextEffectForStoreTest(ctx context.Context, ownerID string) (EffectLease, error) {
	return s.claimPendingEffect(ctx, ownerID, "", "", false, false)
}

func (s *Store) claimPendingBuildForStoreTest(
	ctx context.Context,
	runID string,
	ownerID string,
) (EffectLease, error) {
	if !engine.ValidID(runID) {
		return EffectLease{}, errors.New("valid delivery run id is required")
	}
	return s.claimPendingEffect(ctx, ownerID, runID, string(engine.EffectBuild), true, false)
}

func (s *Store) prepareNativeBuildExecutionForStoreTest(
	ctx context.Context,
	lease EffectLease,
) (engine.JournalEffect, error) {
	return s.prepareNativeBuildExecution(ctx, lease, nil)
}

func (s *Store) bindEffectResultForStoreTest(
	ctx context.Context,
	lease EffectLease,
	result json.RawMessage,
) error {
	if engine.EffectKind(lease.effect.Kind) == engine.EffectBuild {
		return s.bindEffectResult(ctx, lease, result, nil)
	}
	return s.BindEffectResult(ctx, lease, result)
}

func (s *Store) completeEffectForStoreTest(ctx context.Context, lease EffectLease) error {
	if engine.EffectKind(lease.effect.Kind) == engine.EffectBuild {
		return s.completeEffect(ctx, lease, nil)
	}
	return s.CompleteEffect(ctx, lease)
}

func (s *Store) failEffectForStoreTest(ctx context.Context, lease EffectLease, detail string) error {
	if engine.EffectKind(lease.effect.Kind) == engine.EffectBuild {
		return s.failEffect(ctx, lease, detail)
	}
	return s.FailEffect(ctx, lease, detail)
}

func (s *Store) recoverInterruptedEffectsForStoreTest(ctx context.Context, reason string) (int, error) {
	return s.recoverInterruptedEffects(ctx, nil, "", reason)
}

func (s *Store) recoverBoundEffectForStoreTest(
	ctx context.Context,
	effectID string,
	attempt int64,
	reconcilerID string,
) error {
	return s.recoverBoundEffect(ctx, nil, "", effectID, attempt, reconcilerID)
}

func (s *Store) prepareUnboundBuildRecoveryForStoreTest(
	ctx context.Context,
	effectID string,
	attempt int64,
) (BuildRecoveryLease, error) {
	return s.prepareUnboundBuildRecovery(ctx, nil, "", effectID, attempt)
}

func (s *Store) recoverUnboundBuildEffectForStoreTest(
	ctx context.Context,
	lease BuildRecoveryLease,
	reconcilerID string,
	proof BuildRetryProof,
) error {
	return s.recoverUnboundBuildEffect(ctx, nil, "", lease, reconcilerID, proof)
}
