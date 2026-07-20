package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/store"
)

// BuilderController is the sole control-layer holder of Store-issued active
// ownership. Store revalidates that ownership and one current build permit at
// dispatch, claim, and the logical build-start preparation point; result
// binding and completion consume the exact prepared attempt capability so an
// authorized long build does not become unrecordable when its short permit
// ages. The controller deliberately owns no poller, scheduler, or loop.
type BuilderController struct {
	mu        sync.Mutex
	ownership *store.ControllerOwnership
	ownerID   string
	journal   *store.Store
	authority *policy.Authority
	builder   BuilderService
	closed    bool
}

// StartBuilderController acquires Store-issued recovery ownership, completes
// the mandatory native-builder recovery barrier, and only then activates the
// handle. No active command, claim, or external builder boundary can accept a
// recovery-phase handle.
func StartBuilderController(
	ctx context.Context,
	ownerID string,
	journal *store.Store,
	authority *policy.Authority,
	builder BuilderService,
) (_ *BuilderController, _ RecoveryReport, resultErr error) {
	if journal == nil || authority == nil {
		return nil, RecoveryReport{}, errors.New("builder controller requires a Store and authority service")
	}
	boundStore, ok := builder.journal.(*store.Store)
	if !ok || boundStore != journal {
		return nil, RecoveryReport{}, errors.New("builder controller requires a builder bound to its exact Store")
	}
	if !engine.ValidDigest(builder.DispatchDigest()) {
		return nil, RecoveryReport{}, errors.New("builder controller requires an exact builder dispatch")
	}
	if err := authority.RequireLedger(journal); err != nil {
		return nil, RecoveryReport{}, fmt.Errorf("bind controller authority ledger: %w", err)
	}
	ownership, err := journal.AcquireControllerOwnership(ownerID)
	if err != nil {
		return nil, RecoveryReport{}, err
	}
	ownershipTransferred := false
	defer func() {
		if !ownershipTransferred {
			resultErr = errors.Join(resultErr, ownership.Close())
		}
	}()
	if err := ownership.ValidateRecovery(journal, ownerID); err != nil {
		return nil, RecoveryReport{}, err
	}
	report, err := builder.reconcileAfterExclusiveOwnership(
		ctx, ownership, "controller acquired exclusive ownership", ownerID,
	)
	if err != nil {
		return nil, report, fmt.Errorf("complete controller recovery barrier: %w", err)
	}
	if err := ownership.ValidateRecovery(journal, ownerID); err != nil {
		return nil, report, err
	}
	if err := ownership.Activate(ctx, journal, ownerID); err != nil {
		return nil, report, fmt.Errorf("activate recovered controller ownership: %w", err)
	}
	if err := ownership.ValidateActive(journal, ownerID); err != nil {
		return nil, report, err
	}
	controller := &BuilderController{
		ownership: ownership, ownerID: ownerID, journal: journal,
		authority: authority, builder: builder,
	}
	ownershipTransferred = true
	return controller, report, nil
}

// DispatchBuild freshly authenticates the exact plan and next work attempt.
// Store then rejoins the active owner, permit, durable source head, current
// state, command, exact contract, and configured builder inside one transaction.
func (controller *BuilderController) DispatchBuild(
	ctx context.Context,
	runID string,
	workID string,
	commandID string,
) (store.ApplyResult, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.requireOwnership(); err != nil {
		return store.ApplyResult{}, err
	}
	state, plan, request, err := controller.buildPermitRequest(ctx, runID, workID, engine.WorkReady)
	if err != nil {
		return store.ApplyResult{}, err
	}
	permit, err := controller.authorizeBuild(ctx, plan, request)
	if err != nil {
		return store.ApplyResult{}, fmt.Errorf("authorize build dispatch: %w", err)
	}
	payload, err := json.Marshal(engine.DispatchBuildPayload{
		WorkID: workID, DispatchDigest: request.Contract.Digest(),
		BuilderDispatchDigest: request.BuilderDispatchDigest,
	})
	if err != nil {
		return store.ApplyResult{}, fmt.Errorf("encode authorized build dispatch: %w", err)
	}
	result, err := controller.journal.ApplyControlledBuild(
		ctx, controller.ownership, controller.authority, permit, request, engine.Command{
			ID: commandID, RunID: runID, Kind: engine.CommandDispatchBuild,
			ExpectedRevision: state.Revision, Payload: payload,
		},
	)
	if err != nil {
		return store.ApplyResult{}, err
	}
	if result.Outcome == store.OutcomeApplied && len(result.EffectIDs) != 1 {
		return result, errors.New("authorized build dispatch did not create exactly one effect")
	}
	return result, nil
}

// ExecutePendingBuild freshly authorizes the current active attempt, atomically
// claims only its exact pending effect, and passes the resulting opaque
// AuthorizedBuildLease through the Store-guarded builder sequence. A failed
// claim or any failure after claim releases ownership and requires startup
// reconciliation.
func (controller *BuilderController) ExecutePendingBuild(
	ctx context.Context,
	runID string,
	workID string,
) (resultErr error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.requireOwnership(); err != nil {
		return err
	}
	_, plan, request, err := controller.buildPermitRequest(ctx, runID, workID, engine.WorkActive)
	if err != nil {
		return err
	}
	permit, err := controller.authorizeBuild(ctx, plan, request)
	if err != nil {
		return fmt.Errorf("authorize pending build execution: %w", err)
	}
	claimHandedOff := false
	defer func() {
		if !claimHandedOff {
			resultErr = controller.stopForRecovery(resultErr)
		}
	}()
	lease, err := controller.journal.ClaimControlledBuild(
		ctx, controller.ownership, controller.authority, permit, request,
	)
	if err != nil {
		return err
	}
	claimHandedOff = true
	return controller.executeClaimedBuild(ctx, lease)
}

func (controller *BuilderController) executeClaimedBuild(
	ctx context.Context,
	lease store.AuthorizedBuildLease,
) (resultErr error) {
	completed := false
	defer func() {
		if !completed {
			resultErr = controller.stopForRecovery(resultErr)
		}
	}()
	if err := controller.builder.execute(ctx, lease); err != nil {
		return err
	}
	completed = true
	return nil
}

func (controller *BuilderController) authorizeBuild(
	ctx context.Context,
	plan protocol.ExactPlan,
	request policy.BuildPermitRequest,
) (policy.CurrentBuildPermit, error) {
	permit, err := controller.authority.AuthorizeBuild(ctx, plan, request)
	if err != nil {
		return policy.CurrentBuildPermit{}, err
	}
	if err := controller.authority.ValidateBuildPermit(permit, request); err != nil {
		return policy.CurrentBuildPermit{}, err
	}
	return permit, nil
}

func (controller *BuilderController) buildPermitRequest(
	ctx context.Context,
	runID string,
	workID string,
	want engine.WorkState,
) (engine.State, protocol.ExactPlan, policy.BuildPermitRequest, error) {
	state, err := controller.journal.State(ctx, runID)
	if err != nil {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, err
	}
	if state.Phase != engine.PhaseActive {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, errors.New("build requires an active delivery")
	}
	plan, err := controller.journal.Plan(ctx, state.PlanDigest)
	if err != nil {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, err
	}
	planRecord, target := plan.Record(), plan.Target()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion || planRecord.Digest != state.PlanDigest ||
		plan.DeliveryID() != state.DeliveryID || target.Repository != state.Repository || target.Ref != state.TargetRef ||
		!slices.Equal(plan.WorkIDs(), stateWorkIDs(state.Work)) {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, errors.New("build state does not match its exact plan")
	}
	var selected *engine.Work
	for index := range state.Work {
		if state.Work[index].ID == workID {
			selected = &state.Work[index]
			break
		}
	}
	if selected == nil {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, errors.New("build work is absent from delivery state")
	}
	if selected.State != want {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, fmt.Errorf("work %q is %s, want %s", workID, selected.State, want)
	}
	attempt := selected.Attempt
	if want == engine.WorkReady {
		attempt++
	}
	if !protocol.ValidPositiveSafeInteger(attempt) {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, errors.New("build attempt exceeds the interoperable integer ceiling")
	}
	contract, exists := plan.Work(workID)
	if !exists || contract.Digest() == "" || contract.View().ID != workID {
		return engine.State{}, protocol.ExactPlan{}, policy.BuildPermitRequest{}, errors.New("build work is absent from the exact plan")
	}
	return state, plan, policy.BuildPermitRequest{
		ControllerID: controller.ownerID, RunID: runID, StateRevision: state.Revision,
		WorkID: workID, WorkAttempt: attempt, Contract: contract,
		BuilderDispatchDigest: controller.builder.DispatchDigest(),
	}, nil
}

func stateWorkIDs(work []engine.Work) []string {
	ids := make([]string, len(work))
	for index := range work {
		ids[index] = work[index].ID
	}
	return ids
}

func (controller *BuilderController) requireOwnership() error {
	if controller == nil || controller.closed || controller.ownership == nil ||
		controller.journal == nil || controller.authority == nil {
		return errors.New("builder controller is closed or uninitialized")
	}
	return controller.ownership.ValidateActive(controller.journal, controller.ownerID)
}

func (controller *BuilderController) stopForRecovery(cause error) error {
	controller.closed = true
	return errors.Join(cause, controller.ownership.Close())
}

// Close releases Store ownership. A later controller must reacquire recovery-
// phase ownership and complete the full barrier before activation.
func (controller *BuilderController) Close() error {
	if controller == nil {
		return nil
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.closed {
		return nil
	}
	controller.closed = true
	if controller.ownership == nil {
		return nil
	}
	return controller.ownership.Close()
}
