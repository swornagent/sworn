package control

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/store"
)

// ReviewableCommandIDs are the three durable idempotency identities consumed
// by one work attempt. They must be reused unchanged after restart.
type ReviewableCommandIDs struct {
	BuildDispatch string
	CheckDispatch string
	Admission     string
}

func (identities ReviewableCommandIDs) validate() error {
	values := []string{identities.BuildDispatch, identities.CheckDispatch, identities.Admission}
	for _, value := range values {
		if !engine.ValidID(value) {
			return errors.New("reviewable convergence requires three valid command ids")
		}
	}
	if values[0] == values[1] || values[0] == values[2] || values[1] == values[2] {
		return errors.New("reviewable convergence command ids must be distinct")
	}
	return nil
}

// ReviewableCommandIDsFor derives stable, domain-separated command IDs from
// existing delivery truth. It stores no cursor and creates no scheduler state.
func ReviewableCommandIDsFor(
	runID string,
	workID string,
	attempt int64,
) (ReviewableCommandIDs, error) {
	if !engine.ValidID(runID) || !engine.ValidID(workID) ||
		!protocol.ValidPositiveSafeInteger(attempt) {
		return ReviewableCommandIDs{}, errors.New("reviewable command ids require a valid run, work, and positive attempt")
	}
	identities := ReviewableCommandIDs{
		BuildDispatch: deliveryCommandID("build", runID, workID, attempt),
		CheckDispatch: deliveryCommandID("checks", runID, workID, attempt),
		Admission:     deliveryCommandID("admit", runID, workID, attempt),
	}
	return identities, identities.validate()
}

func deliveryCommandID(kind, runID, workID string, attempt int64) string {
	hasher := sha256.New()
	bind := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	bind("sworn-reviewable-command-v1")
	bind(kind)
	bind(runID)
	bind(workID)
	var encodedAttempt [8]byte
	binary.BigEndian.PutUint64(encodedAttempt[:], uint64(attempt))
	_, _ = hasher.Write(encodedAttempt[:])
	return "cmd-" + hex.EncodeToString(hasher.Sum(nil))
}

// ReviewableResult is the bounded operation's final committed truth and the
// exact effect identities used to reach it. Zero command results mean the work
// was already reviewable when the operation began.
type ReviewableResult struct {
	State          engine.State
	Build          store.ApplyResult
	Checks         store.ApplyResult
	Admission      store.ApplyResult
	BuildEffectID  string
	CheckEffectIDs []string
}

// AdvanceToReviewable derives stable command IDs for the selected current
// attempt and runs one bounded convergence. It never advances another work
// item, waits for new work, or obtains a verdict.
func (controller *Controller) AdvanceToReviewable(
	ctx context.Context,
	runID string,
	workID string,
) (ReviewableResult, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.requireReviewableService(); err != nil {
		return ReviewableResult{}, err
	}
	state, err := controller.journal.State(ctx, runID)
	if err != nil {
		return ReviewableResult{}, err
	}
	attempt, err := intendedReviewableAttempt(state, workID)
	if err != nil {
		return ReviewableResult{}, err
	}
	identities, err := ReviewableCommandIDsFor(runID, workID, attempt)
	if err != nil {
		return ReviewableResult{}, err
	}
	return advanceToReviewable(ctx, controllerDeliverySteps{controller}, runID, workID, identities)
}

// AdvanceToReviewableWithCommandIDs accepts caller-retained identities for a
// commit-ambiguity recovery path. Callers must reuse the same values for the
// same logical attempt; Store remains authoritative for exact replay.
func (controller *Controller) AdvanceToReviewableWithCommandIDs(
	ctx context.Context,
	runID string,
	workID string,
	identities ReviewableCommandIDs,
) (ReviewableResult, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if err := controller.requireReviewableService(); err != nil {
		return ReviewableResult{}, err
	}
	if err := identities.validate(); err != nil {
		return ReviewableResult{}, err
	}
	return advanceToReviewable(ctx, controllerDeliverySteps{controller}, runID, workID, identities)
}

func (controller *Controller) requireReviewableService() error {
	if err := controller.requireOwnership(); err != nil {
		return err
	}
	if !controller.checks.initializedFor(controller.journal) {
		return errors.New("controller has no check service bound to its exact Store")
	}
	return nil
}

type deliverySteps interface {
	state(context.Context, string) (engine.State, error)
	ensureBuild(context.Context, string, string, string) (store.ApplyResult, error)
	dispatchChecks(context.Context, string, string, string, string) (store.ApplyResult, error)
	executeChecks(context.Context, string, string, []string) error
	admitSubmission(context.Context, string, string, string) (store.ApplyResult, error)
}

type controllerDeliverySteps struct{ controller *Controller }

func (steps controllerDeliverySteps) state(ctx context.Context, runID string) (engine.State, error) {
	return steps.controller.journal.State(ctx, runID)
}

func (steps controllerDeliverySteps) ensureBuild(
	ctx context.Context,
	runID string,
	workID string,
	commandID string,
) (store.ApplyResult, error) {
	result, err := steps.controller.dispatchBuild(ctx, runID, workID, commandID)
	if err != nil {
		return store.ApplyResult{}, err
	}
	if result.Outcome != store.OutcomeApplied || len(result.EffectIDs) != 1 {
		return store.ApplyResult{}, errors.New("build convergence lacks one applied effect")
	}
	effectID := result.EffectIDs[0]
	if _, err := steps.controller.journal.SucceededEffect(ctx, effectID); err != nil {
		if err := steps.controller.executePendingBuild(ctx, runID, workID); err != nil {
			return store.ApplyResult{}, err
		}
		if _, err := steps.controller.journal.SucceededEffect(ctx, effectID); err != nil {
			return store.ApplyResult{}, fmt.Errorf("validate converged builder result: %w", err)
		}
	}
	return result, nil
}

func (steps controllerDeliverySteps) dispatchChecks(
	ctx context.Context,
	runID string,
	workID string,
	builderEffectID string,
	commandID string,
) (store.ApplyResult, error) {
	return steps.controller.checks.dispatchChecks(ctx, runID, workID, builderEffectID, commandID)
}

func (steps controllerDeliverySteps) executeChecks(
	ctx context.Context,
	runID string,
	workID string,
	effectIDs []string,
) error {
	return steps.controller.executeChecks(ctx, runID, workID, slices.Clone(effectIDs))
}

func (steps controllerDeliverySteps) admitSubmission(
	ctx context.Context,
	runID string,
	workID string,
	commandID string,
) (store.ApplyResult, error) {
	return steps.controller.checks.admitSubmission(ctx, runID, workID, commandID)
}

func advanceToReviewable(
	ctx context.Context,
	steps deliverySteps,
	runID string,
	workID string,
	identities ReviewableCommandIDs,
) (ReviewableResult, error) {
	if steps == nil {
		return ReviewableResult{}, errors.New("reviewable convergence requires delivery steps")
	}
	if err := identities.validate(); err != nil {
		return ReviewableResult{}, err
	}
	state, err := steps.state(ctx, runID)
	if err != nil {
		return ReviewableResult{}, err
	}
	selected, err := reviewableWork(state, runID, workID)
	if err != nil {
		return ReviewableResult{}, err
	}
	if selected.State == engine.WorkReviewable {
		return ReviewableResult{State: state}, nil
	}

	build, err := steps.ensureBuild(ctx, runID, workID, identities.BuildDispatch)
	if err != nil {
		return ReviewableResult{}, fmt.Errorf("converge builder: %w", err)
	}
	if build.Outcome != store.OutcomeApplied || len(build.EffectIDs) != 1 {
		return ReviewableResult{}, errors.New("converged builder did not expose one applied effect")
	}
	result := ReviewableResult{Build: build, BuildEffectID: build.EffectIDs[0]}

	checks, err := steps.dispatchChecks(
		ctx, runID, workID, result.BuildEffectID, identities.CheckDispatch,
	)
	if err != nil {
		return result, fmt.Errorf("converge checks dispatch: %w", err)
	}
	if checks.Outcome != store.OutcomeApplied || len(checks.EffectIDs) == 0 {
		return result, errors.New("converged checks dispatch did not expose an applied batch")
	}
	result.Checks = checks
	result.CheckEffectIDs = slices.Clone(checks.EffectIDs)
	if err := steps.executeChecks(ctx, runID, workID, result.CheckEffectIDs); err != nil {
		return result, fmt.Errorf("execute exact checks: %w", err)
	}

	admission, err := steps.admitSubmission(ctx, runID, workID, identities.Admission)
	if err != nil {
		return result, fmt.Errorf("converge submission admission: %w", err)
	}
	if admission.Outcome != store.OutcomeApplied || len(admission.EffectIDs) != 0 {
		return result, errors.New("submission admission did not commit one effect-free transition")
	}
	result.Admission = admission
	result.State, err = steps.state(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("reload admitted delivery: %w", err)
	}
	selected, err = reviewableWork(result.State, runID, workID)
	if err != nil {
		return result, err
	}
	if selected.State != engine.WorkReviewable {
		return result, errors.New("submission admission did not expose reviewable work")
	}
	return result, nil
}

func intendedReviewableAttempt(state engine.State, workID string) (int64, error) {
	work, err := reviewableWork(state, state.RunID, workID)
	if err != nil {
		return 0, err
	}
	attempt := work.Attempt
	if work.State == engine.WorkReady {
		attempt++
	}
	if !protocol.ValidPositiveSafeInteger(attempt) {
		return 0, errors.New("reviewable convergence requires a positive current attempt")
	}
	return attempt, nil
}

func reviewableWork(state engine.State, runID, workID string) (engine.Work, error) {
	if err := state.Validate(); err != nil {
		return engine.Work{}, fmt.Errorf("validate delivery for reviewable convergence: %w", err)
	}
	if state.Phase != engine.PhaseActive || state.RunID != runID {
		return engine.Work{}, errors.New("reviewable convergence requires an active delivery")
	}
	for _, work := range state.Work {
		if work.ID != workID {
			continue
		}
		switch work.State {
		case engine.WorkReady, engine.WorkActive, engine.WorkChecking, engine.WorkReviewable:
			return work, nil
		default:
			return engine.Work{}, fmt.Errorf("work %q is %s and cannot converge to reviewable", workID, work.State)
		}
	}
	return engine.Work{}, fmt.Errorf("work %q is absent from the delivery", workID)
}
