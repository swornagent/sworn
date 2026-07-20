// Package control joins Sworn's durable journal to its narrow effect workers.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

type builderJournal interface {
	PrepareAuthorizedBuildExecution(context.Context, store.AuthorizedBuildLease) (store.PreparedAuthorizedBuildLease, error)
	BindAuthorizedBuildResult(context.Context, store.PreparedAuthorizedBuildLease, json.RawMessage) error
	CompleteAuthorizedBuild(context.Context, store.PreparedAuthorizedBuildLease) error
	RecoverControlledInterruptedEffects(context.Context, *store.ControllerOwnership, string, string) (int, error)
	UnknownEffects(context.Context) ([]engine.JournalEffect, error)
	PrepareControlledBoundBuildCleanup(context.Context, *store.ControllerOwnership, string, string, int64) (store.BoundBuildCleanupLease, error)
	RecoverControlledBoundEffect(context.Context, *store.ControllerOwnership, string, string, int64) error
	PrepareControlledUnboundBuildRecovery(context.Context, *store.ControllerOwnership, string, string, int64) (store.BuildRecoveryLease, error)
	RecoverControlledUnboundBuildEffect(context.Context, *store.ControllerOwnership, string, store.BuildRecoveryLease, store.BuildRetryProof) error
}

type builderBoundary interface {
	Run(context.Context, store.PreparedAuthorizedBuildLease) (json.RawMessage, error)
	Cleanup(context.Context, store.BoundBuildCleanupLease) error
	ReconcileUnbound(context.Context, store.BuildRecoveryLease) (store.BuildRetryProof, error)
}

// BuilderService fixes the only safe order for builder side effects. Its
// effectful methods are package-private and receive only Store-issued
// controller capabilities; it does not claim work or own a loop.
type BuilderService struct {
	journal        builderJournal
	worker         builderBoundary
	dispatchDigest string
}

func NewBuilderService(journal *store.Store, worker effects.BuilderWorker) (BuilderService, error) {
	if journal == nil {
		return BuilderService{}, errors.New("builder service requires a control store")
	}
	boundControl, ok := worker.Control.(*store.Store)
	if !ok || boundControl != journal {
		return BuilderService{}, errors.New("builder service requires a worker bound to its exact Store")
	}
	dispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		return BuilderService{}, fmt.Errorf("configure builder service: %w", err)
	}
	if err := journal.RequireBuilderConfiguration(dispatchDigest, worker.Repository.Binding()); err != nil {
		return BuilderService{}, fmt.Errorf("configure builder service: %w", err)
	}
	return BuilderService{
		journal: journal, worker: worker, dispatchDigest: dispatchDigest,
	}, nil
}

// DispatchDigest identifies the immutable builder execution profile already
// cross-checked between Store and worker by NewBuilderService. BuilderController
// binds current authority to this exact profile before scheduling or running.
func (service BuilderService) DispatchDigest() string { return service.dispatchDigest }

// execute is intentionally package-private: only BuilderController can carry
// an AuthorizedBuildLease across this external-effect sequence.
func (service BuilderService) execute(ctx context.Context, lease store.AuthorizedBuildLease) error {
	if service.journal == nil || service.worker == nil {
		return errors.New("builder service is not initialized")
	}
	prepared, err := service.journal.PrepareAuthorizedBuildExecution(ctx, lease)
	if err != nil {
		return fmt.Errorf("prepare native builder execution: %w", err)
	}
	result, err := service.worker.Run(ctx, prepared)
	if err != nil {
		return fmt.Errorf("run prepared builder effect: %w", err)
	}
	if err := service.journal.BindAuthorizedBuildResult(ctx, prepared, result); err != nil {
		return fmt.Errorf("bind prepared builder result: %w", err)
	}
	if err := service.journal.CompleteAuthorizedBuild(ctx, prepared); err != nil {
		return fmt.Errorf("complete prepared builder effect: %w", err)
	}
	return nil
}

type RecoveryReport struct {
	Interrupted int
	Bound       int
	Retried     int
}

// reconcileAfterExclusiveOwnership is package-private and every Store mutation
// consumes the exact recovery-phase ownership capability supplied by Start.
func (service BuilderService) reconcileAfterExclusiveOwnership(
	ctx context.Context,
	ownership *store.ControllerOwnership,
	reason string,
	reconcilerID string,
) (RecoveryReport, error) {
	if service.journal == nil || service.worker == nil || ownership == nil {
		return RecoveryReport{}, errors.New("builder service is not initialized")
	}
	report := RecoveryReport{}
	interrupted, err := service.journal.RecoverControlledInterruptedEffects(
		ctx, ownership, reconcilerID, reason,
	)
	if err != nil {
		return report, err
	}
	report.Interrupted = interrupted
	unknown, err := service.journal.UnknownEffects(ctx)
	if err != nil {
		return report, err
	}
	for _, effect := range unknown {
		if len(effect.Result) != 0 {
			if effect.Kind == engine.EffectBuild {
				cleanup, err := service.journal.PrepareControlledBoundBuildCleanup(
					ctx, ownership, reconcilerID, effect.ID, effect.Attempt,
				)
				if err != nil {
					return report, fmt.Errorf("prepare bound builder cleanup %q: %w", effect.ID, err)
				}
				if err := service.worker.Cleanup(ctx, cleanup); err != nil {
					return report, fmt.Errorf("clean bound builder effect %q: %w", effect.ID, err)
				}
			}
			if err := service.journal.RecoverControlledBoundEffect(
				ctx, ownership, reconcilerID, effect.ID, effect.Attempt,
			); err != nil {
				return report, err
			}
			report.Bound++
			continue
		}
		if effect.Kind != engine.EffectBuild {
			return report, fmt.Errorf(
				"effect %q is an unbound %s attempt and has no retry proof", effect.ID, effect.Kind,
			)
		}
		lease, err := service.journal.PrepareControlledUnboundBuildRecovery(
			ctx, ownership, reconcilerID, effect.ID, effect.Attempt,
		)
		if err != nil {
			return report, err
		}
		proof, err := service.worker.ReconcileUnbound(ctx, lease)
		if err != nil {
			return report, fmt.Errorf("reconcile unbound builder effect %q: %w", effect.ID, err)
		}
		if err := service.journal.RecoverControlledUnboundBuildEffect(
			ctx, ownership, reconcilerID, lease, proof,
		); err != nil {
			return report, err
		}
		report.Retried++
	}
	return report, nil
}
