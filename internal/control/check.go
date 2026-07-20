package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/store"
)

type checkJournal interface {
	PrepareAuthorizedCheckExecution(context.Context, store.AuthorizedCheckLease) (store.PreparedAuthorizedCheckLease, error)
	BindAuthorizedCheckResult(context.Context, store.PreparedAuthorizedCheckLease, json.RawMessage) error
	CompleteAuthorizedCheck(context.Context, store.PreparedAuthorizedCheckLease) error
	PrepareControlledUnboundCheckRecovery(context.Context, *store.ControllerOwnership, string, string, int64) (store.CheckRecoveryLease, error)
	RecoverControlledUnboundCheckEffect(context.Context, *store.ControllerOwnership, string, store.CheckRecoveryLease, store.CheckRetryProof) error
}

type checkBoundary interface {
	Run(context.Context, store.PreparedAuthorizedCheckLease) (json.RawMessage, error)
	ReconcileUnbound(context.Context, store.CheckRecoveryLease) (store.CheckRetryProof, error)
}

// CheckService fixes the only safe order for local-check execution and
// recovery. Its effectful methods are package-private and accept only
// Store-issued controller capabilities; it owns no scheduler or state machine.
type CheckService struct {
	journal       checkJournal
	store         *store.Store
	worker        checkBoundary
	runtimeDigest string
}

func NewCheckService(journal *store.Store, worker effects.LocalCheckWorker) (CheckService, error) {
	if journal == nil {
		return CheckService{}, errors.New("check service requires a control store")
	}
	boundControl, ok := worker.Control.(*store.Store)
	if !ok || boundControl != journal {
		return CheckService{}, errors.New("check service requires a worker bound to its exact Store")
	}
	if err := worker.ValidateConfiguration(); err != nil {
		return CheckService{}, fmt.Errorf("configure check worker: %w", err)
	}
	if err := journal.RequireCheckConfiguration(
		worker.Runtime.Digest(), worker.Repository.Binding(),
	); err != nil {
		return CheckService{}, fmt.Errorf("configure check service: %w", err)
	}
	return CheckService{
		journal: journal, store: journal, worker: worker,
		runtimeDigest: worker.Runtime.Digest(),
	}, nil
}

func (service CheckService) initializedFor(journal *store.Store) bool {
	return service.journal != nil && service.store == journal && service.worker != nil &&
		engine.ValidDigest(service.runtimeDigest)
}

func (service CheckService) execute(ctx context.Context, lease store.AuthorizedCheckLease) error {
	if service.journal == nil || service.worker == nil {
		return errors.New("check service is not initialized")
	}
	prepared, err := service.journal.PrepareAuthorizedCheckExecution(ctx, lease)
	if err != nil {
		return fmt.Errorf("prepare local check execution: %w", err)
	}
	result, err := service.worker.Run(ctx, prepared)
	if err != nil {
		return fmt.Errorf("run prepared local check: %w", err)
	}
	if err := service.journal.BindAuthorizedCheckResult(ctx, prepared, result); err != nil {
		return fmt.Errorf("bind prepared local check result: %w", err)
	}
	if err := service.journal.CompleteAuthorizedCheck(ctx, prepared); err != nil {
		return fmt.Errorf("complete prepared local check: %w", err)
	}
	return nil
}

func (service CheckService) reconcileUnbound(
	ctx context.Context,
	ownership *store.ControllerOwnership,
	ownerID string,
	effect engine.JournalEffect,
) error {
	if service.journal == nil || service.worker == nil {
		return errors.New("check service is not initialized")
	}
	lease, err := service.journal.PrepareControlledUnboundCheckRecovery(
		ctx, ownership, ownerID, effect.ID, effect.Attempt,
	)
	if err != nil {
		return err
	}
	proof, err := service.worker.ReconcileUnbound(ctx, lease)
	if err != nil {
		return fmt.Errorf("reconcile unbound check effect %q: %w", effect.ID, err)
	}
	if err := service.journal.RecoverControlledUnboundCheckEffect(
		ctx, ownership, ownerID, lease, proof,
	); err != nil {
		return err
	}
	return nil
}

func (service CheckService) dispatchChecks(
	ctx context.Context,
	runID string,
	workID string,
	builderEffectID string,
	commandID string,
) (store.ApplyResult, error) {
	state, err := service.store.State(ctx, runID)
	if err != nil {
		return store.ApplyResult{}, err
	}
	work, err := reviewableWork(state, runID, workID)
	if err != nil {
		return store.ApplyResult{}, err
	}
	expectedRevision := state.Revision
	if work.State == engine.WorkChecking {
		expectedRevision--
	} else if work.State != engine.WorkActive {
		return store.ApplyResult{}, fmt.Errorf("work %q is %s, want active or checking", workID, work.State)
	}
	plan, err := service.store.Plan(ctx, state.PlanDigest)
	if err != nil {
		return store.ApplyResult{}, err
	}
	selection, err := protocol.ResolveExactLocalChecks(ctx, service.store, plan, workID)
	if err != nil {
		return store.ApplyResult{}, fmt.Errorf("resolve exact local checks: %w", err)
	}
	requirements := selection.Requirements()
	checks := make([]engine.CheckSelection, len(requirements))
	for index, requirement := range requirements {
		checks[index] = engine.CheckSelection{
			CheckID: requirement.CheckID, DefinitionDigest: requirement.Definition.Digest,
		}
	}
	payload, err := json.Marshal(engine.DispatchChecksPayload{
		WorkID: workID, BuilderEffectID: builderEffectID,
		RuntimeManifestDigest: service.runtimeDigest, Checks: checks,
	})
	if err != nil {
		return store.ApplyResult{}, err
	}
	return service.store.Apply(ctx, engine.Command{
		ID: commandID, RunID: runID, Kind: engine.CommandDispatchChecks,
		ExpectedRevision: expectedRevision, Payload: payload,
	})
}

func (service CheckService) admitSubmission(
	ctx context.Context,
	runID string,
	workID string,
	commandID string,
) (store.ApplyResult, error) {
	state, err := service.store.State(ctx, runID)
	if err != nil {
		return store.ApplyResult{}, err
	}
	work, err := reviewableWork(state, runID, workID)
	if err != nil {
		return store.ApplyResult{}, err
	}
	expectedRevision := state.Revision
	if work.State == engine.WorkReviewable {
		expectedRevision--
	} else if work.State != engine.WorkChecking {
		return store.ApplyResult{}, fmt.Errorf("work %q is %s, want checking or reviewable", workID, work.State)
	}
	payload, err := json.Marshal(engine.AdmitSubmissionPayload{WorkID: workID})
	if err != nil {
		return store.ApplyResult{}, err
	}
	return service.store.Apply(ctx, engine.Command{
		ID: commandID, RunID: runID, Kind: engine.CommandAdmitSubmission,
		ExpectedRevision: expectedRevision, Payload: payload,
	})
}

func (controller *Controller) executeChecks(
	ctx context.Context,
	runID string,
	workID string,
	effectIDs []string,
) error {
	for _, effectID := range effectIDs {
		if _, err := controller.journal.SucceededEffect(ctx, effectID); err == nil {
			continue
		}
		_, plan, request, err := controller.journal.PendingCheckPermitRequest(
			ctx, controller.ownership, controller.ownerID, runID, workID, effectID,
		)
		if err != nil {
			return err
		}
		permit, err := controller.authority.AuthorizeCheck(ctx, plan, request)
		if err != nil {
			return fmt.Errorf("authorize pending check execution: %w", err)
		}
		if err := controller.authority.ValidateCheckPermit(permit, request); err != nil {
			return fmt.Errorf("validate pending check permit: %w", err)
		}
		lease, err := controller.journal.ClaimControlledCheck(
			ctx, controller.ownership, controller.authority, permit, request,
		)
		if err != nil {
			return controller.stopForRecovery(err)
		}
		if err := controller.executeClaimedCheck(ctx, lease); err != nil {
			return err
		}
	}
	return nil
}

// executeClaimedCheck mirrors the builder termination barrier. A panic or
// runtime.Goexit in any worker/executor boundary still runs this defer, closes
// active ownership, and forces the next controller through startup recovery.
func (controller *Controller) executeClaimedCheck(
	ctx context.Context,
	lease store.AuthorizedCheckLease,
) (resultErr error) {
	completed := false
	defer func() {
		if !completed {
			resultErr = controller.stopForRecovery(resultErr)
		}
	}()
	if err := controller.checks.execute(ctx, lease); err != nil {
		return err
	}
	completed = true
	return nil
}

// Keep compile-time coupling narrow and explicit.
var _ policy.ApprovalLedger = (*store.Store)(nil)
