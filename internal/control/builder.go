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
	PrepareNativeBuildExecution(context.Context, store.EffectLease) (engine.JournalEffect, error)
	BindEffectResult(context.Context, store.EffectLease, json.RawMessage) error
	CompleteEffect(context.Context, store.EffectLease) error
	RecoverInterruptedEffects(context.Context, string) (int, error)
	UnknownEffects(context.Context) ([]engine.JournalEffect, error)
	RecoverBoundEffect(context.Context, string, int64, string) error
	PrepareUnboundBuildRecovery(context.Context, string, int64) (store.BuildRecoveryLease, error)
	RecoverUnboundBuildEffect(context.Context, store.BuildRecoveryLease, string, effects.BuildRetryProof) error
}

type builderBoundary interface {
	Run(context.Context, engine.JournalEffect) (json.RawMessage, error)
	Cleanup(context.Context, engine.JournalEffect) error
	ReconcileUnbound(context.Context, engine.JournalEffect, string) (effects.BuildRetryProof, error)
}

// BuilderService fixes the only safe order for builder side effects. It does
// not claim work or own a loop; a later controller supplies those policies.
type BuilderService struct {
	journal builderJournal
	worker  builderBoundary
}

func NewBuilderService(journal *store.Store, worker effects.BuilderWorker) (BuilderService, error) {
	if journal == nil {
		return BuilderService{}, errors.New("builder service requires a control store")
	}
	dispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		return BuilderService{}, fmt.Errorf("configure builder service: %w", err)
	}
	if err := journal.RequireBuilderConfiguration(dispatchDigest, worker.Repository.Binding()); err != nil {
		return BuilderService{}, fmt.Errorf("configure builder service: %w", err)
	}
	return BuilderService{journal: journal, worker: worker}, nil
}

// Execute prepares an unpublished candidate, binds its exact result, then asks
// Store to publish its Git facts and complete the lease as one guarded success
// transition. Startup reconciliation resolves any remaining crash window.
func (service BuilderService) Execute(ctx context.Context, lease store.EffectLease) error {
	if service.journal == nil || service.worker == nil {
		return errors.New("builder service is not initialized")
	}
	effect, err := service.journal.PrepareNativeBuildExecution(ctx, lease)
	if err != nil {
		return fmt.Errorf("prepare native builder execution: %w", err)
	}
	if effect.Kind != engine.EffectBuild {
		return errors.New("builder service requires a build effect lease")
	}
	result, err := service.worker.Run(ctx, effect)
	if err != nil {
		return fmt.Errorf("run builder effect %q: %w", effect.ID, err)
	}
	if err := service.journal.BindEffectResult(ctx, lease, result); err != nil {
		return fmt.Errorf("bind builder effect %q: %w", effect.ID, err)
	}
	if err := service.journal.CompleteEffect(ctx, lease); err != nil {
		return fmt.Errorf("complete builder effect %q: %w", effect.ID, err)
	}
	return nil
}

type RecoveryReport struct {
	Interrupted int
	Bound       int
	Retried     int
}

// ReconcileAfterExclusiveOwnership is a startup barrier. The caller must hold
// exclusive controller ownership, and must not claim new work unless this
// method returns successfully.
func (service BuilderService) ReconcileAfterExclusiveOwnership(
	ctx context.Context,
	reason string,
	reconcilerID string,
) (RecoveryReport, error) {
	if service.journal == nil || service.worker == nil {
		return RecoveryReport{}, errors.New("builder service is not initialized")
	}
	report := RecoveryReport{}
	interrupted, err := service.journal.RecoverInterruptedEffects(ctx, reason)
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
				if err := service.worker.Cleanup(ctx, effect); err != nil {
					return report, fmt.Errorf("clean bound builder effect %q: %w", effect.ID, err)
				}
			}
			if err := service.journal.RecoverBoundEffect(
				ctx, effect.ID, effect.Attempt, reconcilerID,
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
		lease, err := service.journal.PrepareUnboundBuildRecovery(
			ctx, effect.ID, effect.Attempt,
		)
		if err != nil {
			return report, err
		}
		proof, err := service.worker.ReconcileUnbound(ctx, lease.Invocation(), lease.Challenge())
		if err != nil {
			return report, fmt.Errorf("reconcile unbound builder effect %q: %w", effect.ID, err)
		}
		if err := service.journal.RecoverUnboundBuildEffect(ctx, lease, reconcilerID, proof); err != nil {
			return report, err
		}
		report.Retried++
	}
	return report, nil
}
