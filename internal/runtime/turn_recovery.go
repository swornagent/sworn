package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func crashHumanTurnBarrier(name string) {
	if testHumanTurnCrash == name ||
		testHumanTurnCrash == "environment" &&
			os.Getenv("SWORN_TEST_HUMAN_TURN_CRASH") == name {
		os.Exit(86)
	}
}

const (
	turnRecoveryRecoveredEvent = "turn_recovery.outcome.recovered"
	humanParkCheckpointVersion = "sworn.human-park-checkpoint/v1"
)

type humanParkCheckpoint struct {
	SchemaVersion string                               `json:"schema_version"`
	ParentEffect  string                               `json:"parent_effect"`
	Command       journal.ParkRecoveryAttentionCommand `json:"command"`
}

func humanParkCheckpointID(parentEffect string) string {
	return parentEffect + "/human-park"
}

func expectedHumanParkCheckpoint(
	parentEffect string,
	command journal.ParkRecoveryAttentionCommand,
) (humanParkCheckpoint, error) {
	if parentEffect == "" || command.Attention.Attention.HumanTurn == nil ||
		command.Attention.RunID == "" ||
		command.Attention.ExpectedGeneration != 0 {
		return humanParkCheckpoint{}, runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	return humanParkCheckpoint{
		SchemaVersion: humanParkCheckpointVersion,
		ParentEffect:  parentEffect,
		Command:       command,
	}, nil
}

type turnRecoveryCycle struct {
	binding    journal.RecoveryBinding
	automation driver.AutomationBinding
}

type turnRecoveryTotals struct {
	initialized bool
	duration    int64
	tokenKnown  bool
	input       int64
	output      int64
	costKnown   bool
	cost        int64
	currency    string
	source      string
}

func addTurnRecoveryValue(left, right int64) (int64, error) {
	if left < 0 || right < 0 ||
		left > driver.MaxSafeInteger-right {
		return 0, runtimeFail("INVALID_TURN_RECOVERY", nil)
	}
	return left + right, nil
}

func (total *turnRecoveryTotals) add(
	duration int64,
	usage driver.UsageReceipt,
) error {
	nextDuration, err := addTurnRecoveryValue(total.duration, duration)
	if err != nil {
		return err
	}
	total.duration = nextDuration
	tokenReported := usage.TokenStatus == driver.UsageReported &&
		usage.InputTokens != nil && usage.OutputTokens != nil
	costReported := usage.CostStatus == driver.UsageReported &&
		usage.CostMicroUnits != nil && usage.Currency != nil &&
		usage.Source != nil
	if !total.initialized {
		total.initialized = true
		total.tokenKnown = tokenReported
		total.costKnown = costReported
		if costReported {
			total.currency = *usage.Currency
			total.source = *usage.Source
		}
	}
	if total.tokenKnown {
		if !tokenReported {
			total.tokenKnown = false
		} else {
			total.input, err = addTurnRecoveryValue(
				total.input,
				*usage.InputTokens,
			)
			if err == nil {
				total.output, err = addTurnRecoveryValue(
					total.output,
					*usage.OutputTokens,
				)
			}
			if err != nil {
				return err
			}
		}
	}
	if total.costKnown {
		if !costReported ||
			total.currency != *usage.Currency ||
			total.source != *usage.Source {
			total.costKnown = false
		} else {
			total.cost, err = addTurnRecoveryValue(
				total.cost,
				*usage.CostMicroUnits,
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (total turnRecoveryTotals) apply(
	observation *driver.Observation,
) {
	if observation == nil || !total.initialized {
		return
	}
	observation.DurationMillis = total.duration
	usage := driver.UsageReceipt{
		TokenStatus: driver.UsageUnavailable,
		CostStatus:  driver.UsageUnavailable,
	}
	if total.tokenKnown {
		input, output := total.input, total.output
		usage.TokenStatus = driver.UsageReported
		usage.InputTokens = &input
		usage.OutputTokens = &output
	}
	if total.costKnown {
		cost := total.cost
		currency, source := total.currency, total.source
		usage.CostStatus = driver.UsageReported
		usage.CostMicroUnits = &cost
		usage.Currency = &currency
		usage.Source = &source
	}
	observation.Usage = usage
}

func (total turnRecoveryTotals) accounting() *journal.RecoveryAccounting {
	if !total.initialized {
		return nil
	}
	result := &journal.RecoveryAccounting{
		DurationMillis: total.duration,
		TokenStatus:    string(driver.UsageUnavailable),
		CostStatus:     string(driver.UsageUnavailable),
	}
	if total.tokenKnown {
		input, output := total.input, total.output
		result.TokenStatus = string(driver.UsageReported)
		result.InputTokens = &input
		result.OutputTokens = &output
	}
	if total.costKnown {
		cost := total.cost
		currency, source := total.currency, total.source
		result.CostStatus = string(driver.UsageReported)
		result.CostMicroUnits = &cost
		result.Currency = &currency
		result.Source = &source
	}
	return result
}

func turnRecoveryTotalsFromAccounting(
	accounting *journal.RecoveryAccounting,
) (turnRecoveryTotals, error) {
	if accounting == nil {
		return turnRecoveryTotals{}, nil
	}
	total := turnRecoveryTotals{
		initialized: true,
		duration:    accounting.DurationMillis,
		tokenKnown:  accounting.TokenStatus == string(driver.UsageReported),
		costKnown:   accounting.CostStatus == string(driver.UsageReported),
	}
	if total.tokenKnown {
		if accounting.InputTokens == nil ||
			accounting.OutputTokens == nil {
			return turnRecoveryTotals{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		total.input = *accounting.InputTokens
		total.output = *accounting.OutputTokens
	}
	if total.costKnown {
		if accounting.CostMicroUnits == nil ||
			accounting.Currency == nil ||
			accounting.Source == nil {
			return turnRecoveryTotals{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		total.cost = *accounting.CostMicroUnits
		total.currency = *accounting.Currency
		total.source = *accounting.Source
	}
	return total, nil
}

type recoveryPlanAuthority struct {
	ManifestDigest string `json:"manifest_digest"`
	PlanOID        string `json:"plan_oid,omitempty"`
	PlanDigest     string `json:"plan_digest,omitempty"`
	PlanRevision   int64  `json:"plan_revision,omitempty"`
}

type recoveryTargetAuthority struct {
	Before       string                      `json:"before"`
	TargetRef    string                      `json:"target_ref"`
	TargetHead   string                      `json:"target_head,omitempty"`
	ReleaseHead  string                      `json:"release_head,omitempty"`
	TrackRef     string                      `json:"track_ref,omitempty"`
	TrackHead    string                      `json:"track_head,omitempty"`
	PreparedBase string                      `json:"prepared_base,omitempty"`
	Evidence     []productionEvidenceBinding `json:"evidence,omitempty"`
}

type recoveryCycleIdentity struct {
	SchemaVersion         string                `json:"schema_version"`
	RunID                 string                `json:"run_id"`
	LaneID                string                `json:"lane_id"`
	Slice                 string                `json:"slice"`
	Responsibility        driver.Responsibility `json:"responsibility"`
	BatonAttempt          int64                 `json:"baton_attempt"`
	WorkIdentity          string                `json:"work_identity"`
	PlanAuthorityDigest   string                `json:"plan_authority_digest"`
	TargetAuthorityDigest string                `json:"target_authority_digest"`
}

func recoveryAuthorityDigests(
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	before string,
) (string, string) {
	return recoveryAuthorityDigestsForContext(
		manifest,
		prepared.productionContext,
		before,
	)
}

func recoveryAuthorityDigestsForContext(
	manifest admittedManifest,
	work *productionWorkContext,
	before string,
) (string, string) {
	plan := recoveryPlanAuthority{ManifestDigest: manifest.digest}
	target := recoveryTargetAuthority{
		Before:    before,
		TargetRef: manifest.value.TargetRef,
	}
	if work != nil {
		if work.Plan != nil {
			plan.PlanOID = work.Plan.OID
			plan.PlanDigest = work.Plan.Digest
			plan.PlanRevision = work.Plan.Revision
		}
		target.TargetRef = work.Authority.TargetRef
		target.TargetHead = work.Authority.TargetHead
		if work.Slice == "" {
			target.ReleaseHead = work.Authority.ReleaseHead
		}
		target.TrackRef = work.Authority.TrackRef
		target.TrackHead = work.Authority.TrackHead
		target.PreparedBase = work.PreparedBase
		target.Evidence = append(
			[]productionEvidenceBinding(nil),
			work.Evidence...,
		)
	}
	return driver.Digest(mustJSON(plan)), driver.Digest(mustJSON(target))
}

func humanTurnLane(work productionWorkContext) (string, string) {
	track, slice := work.Track, work.Slice
	if track == "" {
		track = "release"
	}
	if slice == "" {
		slice = "release"
	}
	return track, slice
}

func humanTurnBindingForContext(
	manifest admittedManifest,
	work productionWorkContext,
	cycle *turnRecoveryCycle,
	kind driver.YieldKind,
	ordinal int64,
) (journal.HumanTurnBinding, error) {
	role, validRole := roleForResponsibility(work.Responsibility)
	track, slice := humanTurnLane(work)
	planDigest, targetDigest := recoveryAuthorityDigestsForContext(
		manifest,
		&work,
		work.Before,
	)
	if cycle == nil || !validRole || role != work.Role ||
		(kind != driver.YieldHumanChoice &&
			kind != driver.YieldHumanConfirmation) ||
		work.RunID != manifest.value.RunID ||
		work.InvocationID == "" || work.Attempt < 1 ||
		cycle.automation.RunID != work.RunID ||
		cycle.automation.TrackID != track ||
		cycle.automation.Slice != slice ||
		cycle.automation.BatonAttempt != work.Attempt ||
		cycle.automation.PlanAuthorityDigest != planDigest ||
		cycle.automation.TargetAuthorityDigest != targetDigest ||
		cycle.binding.CycleID == "" || cycle.binding.TurnID == "" ||
		ordinal < 1 {
		return journal.HumanTurnBinding{},
			runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	return journal.HumanTurnBinding{
		SchemaVersion:         journal.HumanTurnBindingVersion,
		Kind:                  string(kind),
		RunID:                 work.RunID,
		Track:                 track,
		Slice:                 slice,
		Role:                  string(work.Role),
		Responsibility:        string(work.Responsibility),
		InvocationID:          work.InvocationID,
		BatonAttempt:          work.Attempt,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
		WorkIdentity:          cycle.automation.WorkIdentity,
		CycleID:               cycle.binding.CycleID,
		TurnID:                cycle.binding.TurnID,
		Ordinal:               ordinal,
		OpenGeneration:        1,
	}, nil
}

func humanTurnBindingForPrepared(
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	cycle *turnRecoveryCycle,
	kind driver.YieldKind,
	ordinal int64,
) (journal.HumanTurnBinding, error) {
	if prepared.productionContext == nil {
		return journal.HumanTurnBinding{},
			runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	descriptor, err := prepared.permission.Describe()
	work := *prepared.productionContext
	role, validRole := roleForResponsibility(work.Responsibility)
	if err != nil || !validRole || role != work.Role ||
		descriptor.Role != work.Role ||
		descriptor.Responsibility != work.Responsibility ||
		descriptor.InvocationID != work.InvocationID ||
		prepared.request.Role != work.Role ||
		prepared.request.InvocationID != work.InvocationID {
		return journal.HumanTurnBinding{},
			runtimeFail("INVALID_HUMAN_TURN", err)
	}
	return humanTurnBindingForContext(
		manifest,
		work,
		cycle,
		kind,
		ordinal,
	)
}

func validateHumanTurn(
	actual *journal.HumanTurnBinding,
	expected journal.HumanTurnBinding,
) error {
	if actual == nil || *actual != expected {
		return runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	return nil
}

func validateHumanTurnAnswerAdmission(
	snapshot journal.Snapshot,
	manifest admittedManifest,
	attention journal.AttentionProjection,
	answer string,
) error {
	human := attention.Attention.HumanTurn
	if human == nil {
		return nil
	}
	exactReplay := attention.State == journal.AttentionAnswered &&
		attention.Generation == human.OpenGeneration+1 &&
		attention.Answer == answer
	if (attention.State != journal.AttentionOpen ||
		attention.Generation != human.OpenGeneration) && !exactReplay ||
		human.OpenGeneration != 1 ||
		human.RunID != snapshot.Run.ID ||
		human.RunID != manifest.value.RunID {
		return runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	matches := 0
	var expected journal.HumanTurnBinding
	var parentEffect string
	for _, command := range snapshot.Commands {
		if command.Kind != "driver.dispatch" ||
			!strings.HasPrefix(
				command.ReplayKey,
				"attempt/"+
					strings.TrimPrefix(human.WorkIdentity, "sha256:")+
					"/e",
			) {
			continue
		}
		effect, found := effects[command.ReplayKey]
		if !found || command.RunID != human.RunID ||
			effect.RunID != human.RunID ||
			effect.ID != command.ReplayKey ||
			effect.ReplayKey != command.ReplayKey ||
			effect.Kind != command.Kind ||
			effect.State != journal.Claimed ||
			effect.CurrentClaim == "" {
			continue
		}
		persisted, err := parseProductionDispatchCommand(
			manifest,
			command.Payload,
		)
		if err != nil {
			return err
		}
		work := persisted.Context
		if command.ReplayKey != journal.AttemptEffectID(
			human.WorkIdentity,
			work.Epoch,
			work.Try,
		) || work.InvocationID != human.InvocationID ||
			string(work.Role) != human.Role ||
			string(work.Responsibility) != human.Responsibility ||
			work.Attempt != human.BatonAttempt {
			continue
		}
		track, slice := humanTurnLane(work)
		if track != human.Track || slice != human.Slice {
			continue
		}
		cycle := turnRecoveryCycle{
			binding: attention.Attention.Recovery,
			automation: driver.AutomationBinding{
				RunID:                 work.RunID,
				TrackID:               track,
				Slice:                 slice,
				BatonAttempt:          work.Attempt,
				PlanAuthorityDigest:   human.PlanAuthorityDigest,
				TargetAuthorityDigest: human.TargetAuthorityDigest,
				WorkIdentity:          human.WorkIdentity,
				ProgressIdentity:      human.WorkIdentity,
			},
		}
		expected, err = humanTurnBindingForContext(
			manifest,
			work,
			&cycle,
			driver.YieldKind(human.Kind),
			attention.Attention.Ordinal,
		)
		if err != nil {
			return err
		}
		parentEffect = command.ReplayKey
		matches++
	}
	if matches != 1 {
		return runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	if err := validateHumanTurn(human, expected); err != nil {
		return err
	}
	checkpoint, found, err := humanParkCheckpointForEffect(
		manifest,
		snapshot,
		parentEffect,
	)
	if err != nil || !found {
		return runtimeFail("INVALID_HUMAN_TURN", err)
	}
	return validateHumanParkAttention(checkpoint, attention)
}

func turnRecoveryCycleForDispatch(
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	coordinates dispatchCoordinates,
	workIdentity string,
	before string,
) (turnRecoveryCycle, error) {
	if _, enabled := manifest.value.recoverySelection(); !enabled ||
		!runtimeDigestPattern.MatchString(workIdentity) {
		return turnRecoveryCycle{},
			runtimeFail("TURN_RECOVERY_DISABLED", nil)
	}
	lane, slice := "release", "release"
	if prepared.productionContext != nil {
		if prepared.productionContext.Track != "" {
			lane = prepared.productionContext.Track
		}
		if prepared.productionContext.Slice != "" {
			slice = prepared.productionContext.Slice
		}
	} else if coordinates.Slice != "" {
		lane, slice = coordinates.Slice, coordinates.Slice
	}
	planDigest, targetDigest := recoveryAuthorityDigests(
		manifest,
		prepared,
		before,
	)
	cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
		SchemaVersion:         "sworn.turn-recovery-cycle/v1",
		RunID:                 manifest.value.RunID,
		LaneID:                lane,
		Slice:                 slice,
		Responsibility:        coordinates.Responsibility,
		BatonAttempt:          coordinates.BatonAttempt,
		WorkIdentity:          workIdentity,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
	}))
	return turnRecoveryCycle{
		binding: journal.RecoveryBinding{
			LaneID:     lane,
			CycleID:    cycleID,
			TurnID:     recoveryTurnID(cycleID, 0),
			ProgressID: workIdentity,
		},
		automation: driver.AutomationBinding{
			RunID:                 manifest.value.RunID,
			TrackID:               lane,
			Slice:                 slice,
			BatonAttempt:          coordinates.BatonAttempt,
			PlanAuthorityDigest:   planDigest,
			TargetAuthorityDigest: targetDigest,
			WorkIdentity:          workIdentity,
			ProgressIdentity:      workIdentity,
		},
	}, nil
}

func recoveryTurnID(cycleID string, ordinal int64) string {
	return driver.Digest(mustJSON(struct {
		SchemaVersion string `json:"schema_version"`
		CycleID       string `json:"cycle_id"`
		Ordinal       int64  `json:"ordinal"`
	}{
		SchemaVersion: "sworn.turn-recovery-turn/v1",
		CycleID:       cycleID,
		Ordinal:       ordinal,
	}))
}

func recoverableContinuationBinding(
	prepared preparedDriverDispatch,
	cycle turnRecoveryCycle,
) (driver.ContinuationBinding, string, error) {
	descriptor, err := prepared.permission.Describe()
	if err != nil {
		return driver.ContinuationBinding{}, "",
			runtimeFail("INVALID_CONTINUATION", err)
	}
	release := "release"
	if prepared.productionContext != nil {
		release = prepared.productionContext.Release
	}
	selectionDigest := driver.Digest(mustJSON(
		continuationSelectionAuthority{
			Profile: prepared.selected.Profile,
			Adapter: prepared.selected.Adapter,
			Model:   prepared.selected.Model,
		},
	))
	return driver.ContinuationBinding{
		RunID:                 cycle.automation.RunID,
		Release:               release,
		Slice:                 cycle.automation.Slice,
		Attempt:               cycle.automation.BatonAttempt,
		PlanAuthorityDigest:   cycle.automation.PlanAuthorityDigest,
		TargetAuthorityDigest: cycle.automation.TargetAuthorityDigest,
		ToolContractDigest:    driver.Digest(mustJSON(descriptor)),
	}, selectionDigest, nil
}

func retainedRecoverableContinuation(
	handle *driver.Continuation,
	binding driver.ContinuationBinding,
	selectionDigest string,
	before string,
) *retainedContinuation {
	if handle == nil {
		return nil
	}
	return &retainedContinuation{
		handle:          handle,
		binding:         binding,
		selectionDigest: selectionDigest,
		before:          before,
	}
}

func (s *Service) nextTurnRecoveryStep(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	kind journal.RecoveryStepKind,
) (journal.RecoveryStepCommand, error) {
	if cycle == nil {
		return journal.RecoveryStepCommand{},
			runtimeFail("INVALID_TURN_RECOVERY", nil)
	}
	budget, err := s.journal.RecoveryBudget(
		ctx,
		owner.RunID,
		cycle.binding,
	)
	if err != nil {
		return journal.RecoveryStepCommand{},
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	step := journal.RecoveryStepCommand{
		RunID:   owner.RunID,
		Binding: cycle.binding,
		Ordinal: budget.NextOrdinal,
		Kind:    kind,
	}
	step.ID = journal.RecoveryStepID(step.Binding, step.Ordinal)
	return step, nil
}

func (s *Service) reserveTurnRecoveryStep(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	kind journal.RecoveryStepKind,
) (journal.RecoveryStepReceipt, error) {
	return s.reserveTurnRecoveryStepAccounting(
		ctx,
		owner,
		cycle,
		kind,
		nil,
	)
}

func (s *Service) reserveTurnRecoveryStepAccounting(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	kind journal.RecoveryStepKind,
	accounting *journal.RecoveryAccounting,
) (journal.RecoveryStepReceipt, error) {
	step, err := s.nextTurnRecoveryStep(ctx, owner, cycle, kind)
	if err != nil {
		return journal.RecoveryStepReceipt{}, err
	}
	step.Accounting = accounting
	receipt, err := s.journal.ReserveRecoveryStep(
		context.WithoutCancel(ctx),
		owner,
		step,
		s.now().UTC(),
	)
	if err != nil {
		return journal.RecoveryStepReceipt{}, err
	}
	if kind == journal.RecoveryResumeWorker {
		cycle.binding.TurnID = recoveryTurnID(
			cycle.binding.CycleID,
			receipt.Step.Ordinal,
		)
	}
	return receipt, nil
}

func (s *Service) reserveAnsweredResume(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	attention journal.AttentionProjection,
) (journal.RecoveryStepReceipt, error) {
	if cycle == nil ||
		attention.State != journal.AttentionAnswered ||
		attention.Generation != 2 ||
		attention.Attention.Recovery.CycleID != cycle.binding.CycleID ||
		attention.Attention.Recovery.LaneID != cycle.binding.LaneID ||
		attention.Attention.Recovery.ProgressID !=
			cycle.binding.ProgressID {
		return journal.RecoveryStepReceipt{},
			runtimeFail("INVALID_ATTENTION_BINDING", nil)
	}
	cycle.binding = attention.Attention.Recovery
	step := journal.RecoveryStepCommand{
		RunID:   owner.RunID,
		Binding: cycle.binding,
		Ordinal: attention.Attention.Ordinal + 1,
		Kind:    journal.RecoveryResumeWorker,
	}
	step.ID = journal.RecoveryStepID(step.Binding, step.Ordinal)
	receipt, err := s.journal.ReserveRecoveryStep(
		context.WithoutCancel(ctx),
		owner,
		step,
		s.now().UTC(),
	)
	if err != nil {
		return journal.RecoveryStepReceipt{}, err
	}
	cycle.binding.TurnID = recoveryTurnID(
		cycle.binding.CycleID,
		receipt.Step.Ordinal,
	)
	return receipt, nil
}

func (s *Service) turnRecoveryStepHook(
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
) driver.RecoveryStepHook {
	return func(ctx context.Context, kind driver.RecoveryStepKind) error {
		var durable journal.RecoveryStepKind
		switch kind {
		case driver.RecoveryStepSubmissionCorrection:
			durable = journal.RecoveryMalformedCorrection
		case driver.RecoveryStepProseNudge:
			durable = journal.RecoveryProseNudge
		default:
			return runtimeFail("INVALID_TURN_RECOVERY", nil)
		}
		_, err := s.reserveTurnRecoveryStep(ctx, owner, cycle, durable)
		return err
	}
}

func automationInvocationID(
	cycle turnRecoveryCycle,
	operation string,
	ordinal int64,
) string {
	value := workIdentity(
		"sworn.automation-invocation/v1",
		cycle.binding.CycleID,
		operation,
		ordinal,
	)
	return operation + "-" + strings.TrimPrefix(value, "sha256:")
}

func (s *Service) selectedAutomationProfile(
	ctx context.Context,
	engine *engine,
	selection driver.ModelSelection,
) (driver.SelectedProfile, error) {
	selected, err := engine.registry.ResolveSelection(selection)
	if err != nil {
		return driver.SelectedProfile{},
			runtimeFail("DRIVER_SELECTION_FAILED", err)
	}
	if engine.configured != nil {
		if err := engine.configured.validateSelected(selected); err != nil {
			return driver.SelectedProfile{}, err
		}
	}
	return selected, nil
}

func (s *Service) invokeRecoveryAutomation(
	ctx context.Context,
	engine *engine,
	cycle turnRecoveryCycle,
	yield driver.Yield,
) (driver.AutomationObservation, error) {
	selection, enabled := engine.manifest.value.recoverySelection()
	if !enabled {
		return driver.AutomationObservation{},
			runtimeFail("TURN_RECOVERY_DISABLED", nil)
	}
	selected, err := s.selectedAutomationProfile(ctx, engine, selection)
	if err != nil {
		return driver.AutomationObservation{}, err
	}
	budget, err := s.journal.RecoveryBudget(
		ctx,
		engine.manifest.value.RunID,
		cycle.binding,
	)
	if err != nil {
		return driver.AutomationObservation{},
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	automation, ok := s.dispatcher.(driver.AutomationDriver)
	if !ok {
		return driver.AutomationObservation{},
			runtimeFail("TURN_RECOVERY_UNSUPPORTED", nil)
	}
	invocation := driver.RecoveryInvocation{
		SchemaVersion: driver.RecoveryInvocationSchemaVersion,
		InvocationID: automationInvocationID(
			cycle,
			"recovery",
			budget.NextOrdinal,
		),
		Binding:   cycle.automation,
		Selection: selection,
		Facts: []driver.AutomationFact{
			{
				Name:  driver.FactWorkerTerminal,
				Value: string(yield.Kind),
			},
			{
				Name:  driver.FactWorkerMessage,
				Value: yield.Message,
			},
			{
				Name:  driver.FactCurrentStatus,
				Value: "no sealed submission",
			},
		},
	}
	snapshot := invocation
	snapshot.Facts = append(
		[]driver.AutomationFact(nil),
		invocation.Facts...,
	)
	automationInvocation := driver.AutomationInvocation{
		Selected: selected,
		Recovery: &invocation,
	}
	observation, err := automation.InvokeAutomation(ctx, automationInvocation)
	if err != nil {
		return driver.AutomationObservation{}, err
	}
	if err := driver.ValidateAutomationObservation(
		driver.AutomationInvocation{
			Selected: selected,
			Recovery: &snapshot,
		},
		observation,
	); err != nil {
		return driver.AutomationObservation{}, err
	}
	if observation.Recovery.Action == driver.RecoveryResumeWorker {
		answer, err := driver.RecoveryAnswerForInvocation(
			snapshot,
			*observation.Recovery,
		)
		if err != nil {
			return driver.AutomationObservation{}, err
		}
		observation.Recovery.Answer = &answer
	}
	return observation, nil
}

func (s *Service) invokeCaptainAdvisory(
	ctx context.Context,
	engine *engine,
	cycle turnRecoveryCycle,
	yield driver.Yield,
) (driver.AutomationObservation, error) {
	selection := driver.ModelSelection(engine.manifest.value.Roles.Captain)
	selected, err := s.selectedAutomationProfile(ctx, engine, selection)
	if err != nil {
		return driver.AutomationObservation{}, err
	}
	budget, err := s.journal.RecoveryBudget(
		ctx,
		engine.manifest.value.RunID,
		cycle.binding,
	)
	if err != nil {
		return driver.AutomationObservation{},
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	automation, ok := s.dispatcher.(driver.AutomationDriver)
	if !ok {
		return driver.AutomationObservation{},
			runtimeFail("TURN_RECOVERY_UNSUPPORTED", nil)
	}
	invocation := driver.AdvisoryInvocation{
		SchemaVersion: driver.AdvisoryInvocationSchemaVersion,
		InvocationID: automationInvocationID(
			cycle,
			"advisory",
			budget.NextOrdinal,
		),
		Binding:   cycle.automation,
		Selection: selection,
		Question:  yield.Message,
		Facts: []driver.AutomationFact{
			{
				Name:  driver.FactWorkerTerminal,
				Value: string(yield.Kind),
			},
			{
				Name:  driver.FactCurrentStatus,
				Value: "non-gate advice requested",
			},
		},
	}
	snapshot := invocation
	snapshot.Facts = append(
		[]driver.AutomationFact(nil),
		invocation.Facts...,
	)
	automationInvocation := driver.AutomationInvocation{
		Selected: selected,
		Advisory: &invocation,
	}
	observation, err := automation.InvokeAutomation(ctx, automationInvocation)
	if err != nil {
		return driver.AutomationObservation{}, err
	}
	if err := driver.ValidateAutomationObservation(
		driver.AutomationInvocation{
			Selected: selected,
			Advisory: &snapshot,
		},
		observation,
	); err != nil {
		return driver.AutomationObservation{}, err
	}
	return observation, nil
}

func recoverableAuthorityMatches(
	entry *retainedContinuation,
	fresh driver.ContinuationBinding,
	selectionDigest string,
	before string,
) bool {
	return sameStableContinuationAuthority(
		entry,
		fresh,
		selectionDigest,
	) &&
		entry.before == before &&
		entry.binding.Attempt == fresh.Attempt
}

func (s *Service) invokeRecoverableWorker(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	prepared preparedDriverDispatch,
	before string,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	entry *retainedContinuation,
	input driver.RecoverableTurnInput,
	allowFallback bool,
) (
	observation driver.Observation,
	retained *retainedContinuation,
	resultErr error,
) {
	defer func() {
		resultErr = errors.Join(
			resultErr,
			closeRetainedContinuation(entry),
		)
	}()
	if cycle == nil || workspace == nil {
		return driver.Observation{}, nil,
			runtimeFail("INVALID_TURN_RECOVERY", nil)
	}
	if prepared.productionContext != nil {
		if err := revalidatePreparedProductionDispatch(
			ctx,
			engine,
			dispatchCoordinates{
				Slice:          prepared.productionContext.Slice,
				Responsibility: prepared.productionContext.Responsibility,
				BatonAttempt:   prepared.productionContext.Attempt,
				Epoch:          prepared.productionContext.Epoch,
				Try:            prepared.productionContext.Try,
			},
			before,
			prepared,
		); err != nil {
			return driver.Observation{}, nil, err
		}
	}
	freshBinding, selectionDigest, err :=
		recoverableContinuationBinding(prepared, *cycle)
	if err != nil {
		return driver.Observation{}, nil, err
	}
	promotableDesign := entry != nil &&
		prepared.productionContext != nil &&
		prepared.productionContext.Responsibility ==
			driver.ImplementerDesign &&
		prepared.productionContext.Receipt != nil &&
		entry.sourceReceipt ==
			prepared.productionContext.Receipt.OID
	promotableVerifier := entry != nil &&
		prepared.productionContext != nil &&
		prepared.productionContext.Responsibility ==
			driver.WorkVerification &&
		prepared.productionContext.Receipt != nil &&
		entry.binding.Attempt ==
			prepared.productionContext.Attempt &&
		entry.verifierFailReceipt == "" &&
		entry.sourceReceipt ==
			prepared.productionContext.Receipt.OID
	promotableTerminal := promotableDesign || promotableVerifier
	if promotableTerminal {
		freshBinding, selectionDigest, err =
			continuationBindingForDispatch(
				prepared,
				dispatchCoordinates{
					Slice: prepared.productionContext.Slice,
					Responsibility: prepared.productionContext.
						Responsibility,
					BatonAttempt: prepared.productionContext.Attempt,
				},
			)
		if err != nil {
			return driver.Observation{}, nil, err
		}
	}
	if !recoverableAuthorityMatches(
		entry,
		freshBinding,
		selectionDigest,
		before,
	) {
		if cleanupErr := closeRetainedContinuation(entry); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		entry = nil
	}
	binding := freshBinding
	var source *driver.Continuation
	if entry != nil {
		binding = entry.binding
		source = entry.handle
		entry.handle = nil
	}
	request, permission := prepared.request, prepared.permission
	if entry != nil &&
		(promotableVerifier ||
			entry.binding.ToolContractDigest !=
				freshBinding.ToolContractDigest) &&
		prepared.resumeRequest != nil &&
		prepared.resumePermission != nil {
		request, permission =
			*prepared.resumeRequest, *prepared.resumePermission
	}
	invocation := preparedInvocation(
		prepared,
		workspace,
		request,
		permission,
	)
	invocation.RecoveryStepHook =
		s.turnRecoveryStepHook(owner, cycle)
	var targetBinding *driver.ContinuationBinding
	if entry != nil && promotableTerminal {
		target := entry.binding
		targetBinding = &target
		input.TargetBinding = &target
	}
	recoverable, supported :=
		s.dispatcher.(driver.RecoverableTurnDriver)
	if !supported {
		_ = closeRecoveryContinuation(
			&retainedContinuation{handle: source},
		)
		return driver.Observation{}, nil,
			runtimeFail("TURN_RECOVERY_UNSUPPORTED", nil)
	}
	observation, next, result, invokeErr :=
		recoverable.InvokeRecoverableTurn(
			ctx,
			invocation,
			binding,
			source,
			&input,
		)
	sourceCloseErr := closeRetainedContinuation(
		&retainedContinuation{handle: source},
	)
	invokeErr = runtimeContinuationError(invokeErr)
	if sourceCloseErr != nil {
		if next != nil {
			sourceCloseErr = errors.Join(sourceCloseErr, next.Close())
		}
		return driver.Observation{}, nil, sourceCloseErr
	}
	if source != nil &&
		requestsFreshRehydrate(observation, result, invokeErr) &&
		allowFallback {
		if next != nil {
			if cleanupErr := next.Close(); cleanupErr != nil {
				return driver.Observation{}, nil,
					runtimeFail(
						"CONTINUATION_CLEANUP_FAILED",
						cleanupErr,
					)
			}
			return driver.Observation{}, nil,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
		return s.invokeRecoverableWorker(
			ctx,
			engine,
			workspace,
			prepared,
			before,
			owner,
			cycle,
			nil,
			input,
			false,
		)
	}
	if invokeErr != nil {
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: next},
		); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		return observation, nil, invokeErr
	}
	if observation.Yield != nil {
		switch {
		case next != nil &&
			result.Status == driver.ContinuationStatusSuspended &&
			validRetainedContinuationMode(result.Mode):
			retained := retainedRecoverableContinuation(
				next,
				binding,
				selectionDigest,
				before,
			)
			if entry != nil {
				retained.sourceReceipt = entry.sourceReceipt
				retained.verifierFailReceipt =
					entry.verifierFailReceipt
			}
			return observation, retained, nil
		case next == nil && validFreshContinuationStart(next, result):
			return observation, nil, nil
		default:
			if cleanupErr := closeRetainedContinuation(
				&retainedContinuation{handle: next},
			); cleanupErr != nil {
				return driver.Observation{}, nil, cleanupErr
			}
			return driver.Observation{}, nil,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
	}
	if targetBinding != nil {
		if observation.Handoff != nil &&
			next != nil &&
			result.Status == driver.ContinuationStatusSuspended &&
			validRetainedContinuationMode(result.Mode) {
			return observation, &retainedContinuation{
				handle:              next,
				binding:             *targetBinding,
				selectionDigest:     selectionDigest,
				before:              before,
				sourceReceipt:       entry.sourceReceipt,
				verifierFailReceipt: entry.verifierFailReceipt,
			}, nil
		}
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: next},
		); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		return driver.Observation{}, nil,
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	validTerminal := observation.Handoff != nil && next == nil
	if source == nil {
		validTerminal = validTerminal &&
			validFreshContinuationStart(next, result)
	} else {
		validTerminal = validTerminal &&
			validRetainedContinuationResume(next, result)
	}
	if !validTerminal {
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: next},
		); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		return driver.Observation{}, nil,
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	return observation, nil, nil
}

func (s *Service) continueYieldedWorker(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	prepared preparedDriverDispatch,
	before string,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	observation driver.Observation,
	pending *retainedContinuation,
) (
	driver.Observation,
	*retainedContinuation,
	bool,
	error,
) {
	return s.continueYieldedWorkerReplacing(
		ctx,
		engine,
		workspace,
		prepared,
		before,
		owner,
		cycle,
		effectID,
		observation,
		pending,
		nil,
		nil,
	)
}

func (s *Service) continueYieldedWorkerReplacing(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	prepared preparedDriverDispatch,
	before string,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	observation driver.Observation,
	pending *retainedContinuation,
	replaced *journal.AttentionProjection,
	carried *turnRecoveryTotals,
) (
	driver.Observation,
	*retainedContinuation,
	bool,
	error,
) {
	recovered := false
	var totals turnRecoveryTotals
	if carried != nil {
		totals = *carried
	}
	if err := totals.add(
		observation.DurationMillis,
		observation.Usage,
	); err != nil {
		return driver.Observation{}, pending, false, err
	}
	for observation.Yield != nil {
		if observation.Yield.Kind == driver.YieldHumanChoice ||
			observation.Yield.Kind == driver.YieldHumanConfirmation {
			parkErr := s.parkHumanTurnRecoveryReplacing(
				ctx,
				engine.manifest,
				prepared,
				owner,
				cycle,
				effectID,
				*observation.Yield,
				pending,
				replaced,
				&totals,
			)
			return driver.Observation{}, nil, recovered, parkErr
		}
		budget, err := s.journal.RecoveryBudget(
			ctx,
			owner.RunID,
			cycle.binding,
		)
		if err != nil {
			return driver.Observation{}, pending, recovered,
				runtimeFail("JOURNAL_READ_FAILED", err)
		}
		if budget.AutomaticActions >=
			journal.MaxRecoveryAutomaticPerCycle ||
			budget.SameProgress >=
				journal.MaxRecoveryDecisionsPerProgress {
			parkErr := s.parkTurnRecoveryReplacing(
				ctx,
				owner,
				cycle,
				effectID,
				observation.Yield.Message,
				pending,
				replaced,
				&totals,
			)
			return driver.Observation{}, nil, recovered, parkErr
		}
		automation, err := s.invokeRecoveryAutomation(
			ctx,
			engine,
			*cycle,
			*observation.Yield,
		)
		if err == nil {
			err = totals.add(
				automation.DurationMillis,
				automation.Usage,
			)
		}
		if err != nil || automation.Recovery == nil {
			parkErr := s.parkTurnRecoveryReplacing(
				ctx,
				owner,
				cycle,
				effectID,
				observation.Yield.Message,
				pending,
				replaced,
				&totals,
			)
			return driver.Observation{}, nil, recovered,
				errors.Join(parkErr, err)
		}
		decision := automation.Recovery
		var answer string
		switch decision.Action {
		case driver.RecoveryResumeWorker:
			if decision.Answer == nil {
				return driver.Observation{}, pending, recovered,
					runtimeFail("INVALID_TURN_RECOVERY", nil)
			}
			answer = *decision.Answer
			if _, err := s.reserveTurnRecoveryStepAccounting(
				ctx,
				owner,
				cycle,
				journal.RecoveryResumeWorker,
				totals.accounting(),
			); err != nil {
				parkErr := s.parkTurnRecoveryReplacing(
					ctx,
					owner,
					cycle,
					effectID,
					observation.Yield.Message,
					pending,
					replaced,
					&totals,
				)
				return driver.Observation{}, nil, recovered,
					errors.Join(parkErr, err)
			}
		case driver.RecoveryAskCaptain:
			if _, err := s.reserveTurnRecoveryStepAccounting(
				ctx,
				owner,
				cycle,
				journal.RecoveryAskCaptain,
				totals.accounting(),
			); err != nil {
				parkErr := s.parkTurnRecoveryReplacing(
					ctx,
					owner,
					cycle,
					effectID,
					observation.Yield.Message,
					pending,
					replaced,
					&totals,
				)
				return driver.Observation{}, nil, recovered,
					errors.Join(parkErr, err)
			}
			advisory, advisoryErr := s.invokeCaptainAdvisory(
				ctx,
				engine,
				*cycle,
				*observation.Yield,
			)
			if advisoryErr == nil {
				advisoryErr = totals.add(
					advisory.DurationMillis,
					advisory.Usage,
				)
			}
			if advisoryErr != nil || advisory.Advisory == nil ||
				advisory.Advisory.Outcome ==
					driver.AdvisoryCannotAnswer {
				parkErr := s.parkTurnRecoveryReplacing(
					ctx,
					owner,
					cycle,
					effectID,
					observation.Yield.Message,
					pending,
					replaced,
					&totals,
				)
				return driver.Observation{}, nil, recovered,
					errors.Join(parkErr, advisoryErr)
			}
			answer = *advisory.Advisory.Answer
			if _, err := s.reserveTurnRecoveryStepAccounting(
				ctx,
				owner,
				cycle,
				journal.RecoveryResumeWorker,
				totals.accounting(),
			); err != nil {
				parkErr := s.parkTurnRecoveryReplacing(
					ctx,
					owner,
					cycle,
					effectID,
					observation.Yield.Message,
					pending,
					replaced,
					&totals,
				)
				return driver.Observation{}, nil, recovered,
					errors.Join(parkErr, err)
			}
		case driver.RecoveryRetryOperationally:
			_, reserveErr := s.reserveTurnRecoveryStepAccounting(
				ctx,
				owner,
				cycle,
				journal.RecoveryRetryOperationally,
				totals.accounting(),
			)
			cleanupErr := closeRecoveryContinuation(pending)
			if reserveErr != nil {
				parkErr := s.parkTurnRecoveryReplacing(
					ctx,
					owner,
					cycle,
					effectID,
					observation.Yield.Message,
					nil,
					replaced,
					&totals,
				)
				return driver.Observation{}, nil, recovered,
					errors.Join(parkErr, reserveErr, cleanupErr)
			}
			return observation, nil, recovered,
				recoveryFailure(
					runtimeFail(
						"RECOVERY_RETRY_OPERATIONALLY",
						nil,
					),
					cleanupErr,
				)
		case driver.RecoveryPauseForHuman:
			parkErr := s.parkTurnRecoveryReplacing(
				ctx,
				owner,
				cycle,
				effectID,
				observation.Yield.Message,
				pending,
				replaced,
				&totals,
			)
			return driver.Observation{}, nil, recovered, parkErr
		default:
			return driver.Observation{}, pending, recovered,
				runtimeFail("INVALID_TURN_RECOVERY", nil)
		}

		next, retained, err := s.invokeRecoverableWorker(
			ctx,
			engine,
			workspace,
			prepared,
			before,
			owner,
			cycle,
			pending,
			driver.RecoverableTurnInput{
				SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
				Kind:          driver.RecoverableInputAnswer,
				Answer:        answer,
			},
			true,
		)
		pending = retained
		if err != nil {
			return next, pending, recovered, err
		}
		if err := totals.add(
			next.DurationMillis,
			next.Usage,
		); err != nil {
			return driver.Observation{}, pending, recovered, err
		}
		recovered = true
		observation = next
		if observation.Handoff != nil {
			totals.apply(&observation)
			return observation, pending, recovered, nil
		}
	}
	return observation, pending, recovered,
		runtimeFail("INVALID_TURN_RECOVERY", nil)
}

func (s *Service) resumeAnsweredWorker(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	prepared preparedDriverDispatch,
	before string,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	attention journal.AttentionProjection,
) (
	driver.Observation,
	*retainedContinuation,
	bool,
	error,
) {
	if err := s.validateHumanTurnResume(
		ctx,
		engine.manifest,
		prepared,
		cycle,
		effectID,
		attention,
	); err != nil {
		return driver.Observation{}, nil, false, err
	}
	budget, err := s.journal.RecoveryBudget(
		ctx,
		owner.RunID,
		attention.Attention.Recovery,
	)
	if err != nil {
		return driver.Observation{}, nil, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	totals, err := turnRecoveryTotalsFromAccounting(
		budget.Accounting,
	)
	if err != nil {
		return driver.Observation{}, nil, false, err
	}
	if _, err := s.reserveAnsweredResume(
		ctx,
		owner,
		cycle,
		attention,
	); err != nil {
		return driver.Observation{}, nil, false, err
	}
	pending := s.takeRecoverableContinuation(owner.RunID, effectID)
	if attention.Attention.HumanTurn != nil {
		crashHumanTurnBarrier("after_continuation_rehydration")
	}
	observation, retained, invokeErr := s.invokeRecoverableWorker(
		ctx,
		engine,
		workspace,
		prepared,
		before,
		owner,
		cycle,
		pending,
		driver.RecoverableTurnInput{
			SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
			Kind:          driver.RecoverableInputAnswer,
			Answer:        attention.Answer,
		},
		true,
	)
	if invokeErr != nil {
		return observation, retained, false, invokeErr
	}
	if observation.Handoff != nil {
		if err := totals.add(
			observation.DurationMillis,
			observation.Usage,
		); err != nil {
			return driver.Observation{}, retained, false, err
		}
		totals.apply(&observation)
		return observation, retained, true, nil
	}
	if observation.Yield == nil {
		cleanupErr := closeRecoveryContinuation(retained)
		return driver.Observation{}, nil, false,
			recoveryFailure(
				runtimeFail("INVALID_TURN_RECOVERY", nil),
				cleanupErr,
			)
	}
	return s.continueYieldedWorkerReplacing(
		ctx,
		engine,
		workspace,
		prepared,
		before,
		owner,
		cycle,
		effectID,
		observation,
		retained,
		&attention,
		&totals,
	)
}

func (s *Service) validateHumanTurnResume(
	ctx context.Context,
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	cycle *turnRecoveryCycle,
	effectID string,
	attention journal.AttentionProjection,
) error {
	human := attention.Attention.HumanTurn
	if human == nil {
		return nil
	}
	expected, err := humanTurnBindingForPrepared(
		manifest,
		prepared,
		cycle,
		driver.YieldKind(human.Kind),
		attention.Attention.Ordinal,
	)
	if err != nil {
		return err
	}
	if err := validateHumanTurn(human, expected); err != nil {
		return err
	}
	snapshot, err := s.journal.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	checkpoint, found, err := humanParkCheckpointForEffect(
		manifest,
		snapshot,
		effectID,
	)
	if err != nil || !found {
		return runtimeFail("INVALID_HUMAN_TURN", err)
	}
	return validateHumanParkAttention(checkpoint, attention)
}

func activeAttentionForWork(
	attentions []journal.AttentionProjection,
	workIdentity string,
) (journal.AttentionProjection, bool, error) {
	var found journal.AttentionProjection
	for _, attention := range attentions {
		if attention.State != journal.AttentionOpen &&
			attention.State != journal.AttentionAnswered {
			continue
		}
		if attention.Attention.Recovery.ProgressID != workIdentity {
			continue
		}
		if found.Attention.ID != "" {
			return journal.AttentionProjection{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		found = attention
	}
	return found, found.Attention.ID != "", nil
}

func activeAttentionWork(
	attentions []journal.AttentionProjection,
) (map[string]journal.AttentionProjection, error) {
	result := make(
		map[string]journal.AttentionProjection,
		len(attentions),
	)
	for _, attention := range attentions {
		if attention.State != journal.AttentionOpen &&
			attention.State != journal.AttentionAnswered {
			continue
		}
		work := attention.Attention.Recovery.ProgressID
		if _, duplicate := result[work]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		result[work] = attention
	}
	return result, nil
}

func (s *Service) attentionForWork(
	ctx context.Context,
	runID string,
	workIdentity string,
) (journal.AttentionProjection, bool, error) {
	attentions, err := s.journal.Attentions(ctx, runID)
	if err != nil {
		return journal.AttentionProjection{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	return activeAttentionForWork(attentions, workIdentity)
}

func (s *Service) resolveAnsweredAttention(
	ctx context.Context,
	owner journal.OwnerLease,
	attention journal.AttentionProjection,
) error {
	if attention.State != journal.AttentionAnswered ||
		attention.Generation != 2 {
		return runtimeFail("INVALID_ATTENTION_BINDING", nil)
	}
	if _, err := s.journal.ResolveAttention(
		context.WithoutCancel(ctx),
		owner,
		journal.ResolveAttentionCommand{
			RunID:              owner.RunID,
			Attention:          attention.Attention,
			ExpectedGeneration: attention.Generation,
		},
		s.now().UTC(),
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) persistHumanParkCheckpoint(
	ctx context.Context,
	owner journal.OwnerLease,
	parentEffect string,
	command journal.ParkRecoveryAttentionCommand,
) error {
	checkpoint, err := expectedHumanParkCheckpoint(parentEffect, command)
	if err != nil {
		return err
	}
	body := mustJSON(checkpoint)
	id := humanParkCheckpointID(parentEffect)
	now := s.now().UTC()
	if err := s.journal.RecordCommandEffect(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: id,
			Kind: "runtime.human_park", Payload: body, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: id, ReplayKey: id,
			Kind:           "runtime.human_park",
			BeforeDigest:   sha256Digest([]byte(parentEffect)),
			ExpectedDigest: sha256Digest(body), UpdatedAt: now,
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, owner.RunID, id)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		if effect.ResultDigest != sha256Digest(effect.Result) ||
			!bytes.Equal(effect.Result, body) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return nil
	}
	if effect.State != journal.Pending {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	claim, err := s.journal.ClaimOwned(
		ctx, owner, id, now, effectLease,
	)
	if err != nil {
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	if err := s.journal.CompleteOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: id, Token: claim.Token,
			State: journal.Succeeded, Result: body,
			EventKind: "human_turn.park_checkpointed",
			EventBody: []byte(command.Attention.Attention.ID), At: now,
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func validateHumanParkCheckpoint(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	command journal.Command,
	effect journal.Effect,
) (humanParkCheckpoint, error) {
	if command.RunID != snapshot.Run.ID ||
		command.Kind != "runtime.human_park" ||
		effect.RunID != snapshot.Run.ID || effect.ID != command.ReplayKey ||
		effect.ReplayKey != command.ReplayKey || effect.Kind != command.Kind ||
		effect.State != journal.Succeeded ||
		effect.BeforeDigest == "" ||
		effect.ExpectedDigest != sha256Digest(command.Payload) ||
		effect.ResultDigest != sha256Digest(effect.Result) ||
		!bytes.Equal(command.Payload, effect.Result) {
		return humanParkCheckpoint{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var checkpoint humanParkCheckpoint
	if json.Unmarshal(effect.Result, &checkpoint) != nil ||
		!bytes.Equal(effect.Result, mustJSON(checkpoint)) ||
		checkpoint.SchemaVersion != humanParkCheckpointVersion ||
		command.ReplayKey != humanParkCheckpointID(checkpoint.ParentEffect) ||
		effect.BeforeDigest != sha256Digest([]byte(checkpoint.ParentEffect)) {
		return humanParkCheckpoint{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	expected, err := expectedHumanParkCheckpoint(
		checkpoint.ParentEffect,
		checkpoint.Command,
	)
	if err != nil || !bytes.Equal(mustJSON(checkpoint), mustJSON(expected)) ||
		checkpoint.Command.Attention.RunID != manifest.value.RunID ||
		checkpoint.Command.Step.RunID != manifest.value.RunID {
		return humanParkCheckpoint{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return checkpoint, nil
}

func humanParkCheckpointForEffect(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	parentEffect string,
) (humanParkCheckpoint, bool, error) {
	id := humanParkCheckpointID(parentEffect)
	var command *journal.Command
	for index := range snapshot.Commands {
		candidate := &snapshot.Commands[index]
		if candidate.ReplayKey != id {
			continue
		}
		if command != nil {
			return humanParkCheckpoint{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command = candidate
	}
	var effect *journal.Effect
	for index := range snapshot.Effects {
		candidate := &snapshot.Effects[index]
		if candidate.ID != id {
			continue
		}
		if effect != nil {
			return humanParkCheckpoint{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effect = candidate
	}
	if command == nil && effect == nil {
		return humanParkCheckpoint{}, false, nil
	}
	if command == nil || effect == nil {
		return humanParkCheckpoint{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	checkpoint, err := validateHumanParkCheckpoint(
		manifest,
		snapshot,
		*command,
		*effect,
	)
	if err != nil || checkpoint.ParentEffect != parentEffect {
		return humanParkCheckpoint{}, false,
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	return checkpoint, true, nil
}

func validateHumanParkAttention(
	checkpoint humanParkCheckpoint,
	attention journal.AttentionProjection,
) error {
	if checkpoint.Command.Attention.RunID == "" ||
		!bytes.Equal(
			mustJSON(checkpoint.Command.Attention.Attention),
			mustJSON(attention.Attention),
		) {
		return runtimeFail("INVALID_HUMAN_TURN", nil)
	}
	return nil
}

func (s *Service) recoverHumanParkCheckpoint(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	snapshot journal.Snapshot,
) (bool, error) {
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	attentions, err := s.journal.Attentions(ctx, owner.RunID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	active, err := activeAttentionWork(attentions)
	if err != nil {
		return false, err
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "runtime.human_park" {
			continue
		}
		effect, found := effects[command.ReplayKey]
		if !found {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		checkpoint, err := validateHumanParkCheckpoint(
			engine.manifest, snapshot, command, effect,
		)
		if err != nil {
			return false, err
		}
		parentCommand, commandFound := commands[checkpoint.ParentEffect]
		parent, parentFound := effects[checkpoint.ParentEffect]
		if !commandFound || !parentFound ||
			parentCommand.Kind != "driver.dispatch" ||
			parent.Kind != parentCommand.Kind ||
			parent.ID != parentCommand.ReplayKey ||
			parent.ReplayKey != parentCommand.ReplayKey {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if parent.State != journal.Claimed {
			if parent.State == journal.Succeeded ||
				parent.State == journal.OperationalFailed {
				continue
			}
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		human := checkpoint.Command.Attention.Attention.HumanTurn
		if parked, found := active[human.WorkIdentity]; found {
			if parked.Attention.ID != checkpoint.Command.Attention.Attention.ID ||
				validateHumanTurn(parked.Attention.HumanTurn, *human) != nil {
				return false, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			continue
		}
		projection := journal.AttentionProjection{
			Attention:  checkpoint.Command.Attention.Attention,
			Generation: 1,
			State:      journal.AttentionOpen,
		}
		if err := validateHumanTurnAnswerAdmission(
			snapshot, engine.manifest, projection, "",
		); err != nil {
			return false, err
		}
		dispatch, err := validateDriverRecoveryCommand(
			engine.manifest, parentCommand, parent,
		)
		if err != nil || dispatch.production == nil {
			return false, runtimeFail("CORRUPT_JOURNAL", err)
		}
		if err := validateCurrentProductionDispatchContext(
			ctx, engine, dispatch,
		); err != nil {
			return false, err
		}
		if _, err := s.journal.ParkRecoveryAttention(
			context.WithoutCancel(ctx),
			owner,
			checkpoint.Command,
			s.now().UTC(),
		); err != nil {
			return false, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) parkTurnRecovery(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	question string,
	pending *retainedContinuation,
) error {
	return s.parkTurnRecoveryReplacing(
		ctx,
		owner,
		cycle,
		effectID,
		question,
		pending,
		nil,
		nil,
	)
}

func (s *Service) parkTurnRecoveryReplacing(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	question string,
	pending *retainedContinuation,
	replaced *journal.AttentionProjection,
	accounting *turnRecoveryTotals,
) (resultErr error) {
	return s.parkTurnRecoveryReplacingBound(
		ctx,
		owner,
		cycle,
		effectID,
		question,
		pending,
		replaced,
		accounting,
		nil,
	)
}

type humanTurnParkBinding struct {
	manifest admittedManifest
	prepared preparedDriverDispatch
	kind     driver.YieldKind
}

func (s *Service) parkHumanTurnRecoveryReplacing(
	ctx context.Context,
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	yield driver.Yield,
	pending *retainedContinuation,
	replaced *journal.AttentionProjection,
	accounting *turnRecoveryTotals,
) error {
	return s.parkTurnRecoveryReplacingBound(
		ctx,
		owner,
		cycle,
		effectID,
		yield.Message,
		pending,
		replaced,
		accounting,
		&humanTurnParkBinding{
			manifest: manifest,
			prepared: prepared,
			kind:     yield.Kind,
		},
	)
}

func (s *Service) parkTurnRecoveryReplacingBound(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle *turnRecoveryCycle,
	effectID string,
	question string,
	pending *retainedContinuation,
	replaced *journal.AttentionProjection,
	accounting *turnRecoveryTotals,
	human *humanTurnParkBinding,
) (resultErr error) {
	defer func() {
		resultErr = errors.Join(
			resultErr,
			closeRecoveryContinuation(pending),
		)
	}()
	step, err := s.nextTurnRecoveryStep(
		ctx,
		owner,
		cycle,
		journal.RecoveryParkTrack,
	)
	if err != nil {
		return err
	}
	if accounting != nil {
		step.Accounting = accounting.accounting()
	}
	binding := journal.AttentionBinding{
		Ordinal:  step.Ordinal,
		Recovery: step.Binding,
	}
	binding.ID = journal.AttentionID(
		binding.Recovery,
		binding.Ordinal,
	)
	if human != nil {
		humanBinding, bindErr := humanTurnBindingForPrepared(
			human.manifest,
			human.prepared,
			cycle,
			human.kind,
			step.Ordinal,
		)
		if bindErr != nil {
			return bindErr
		}
		binding.HumanTurn = &humanBinding
	}
	if strings.TrimSpace(question) == "" {
		question = "Automatic recovery stopped. What should the worker do next?"
	}
	command := journal.ParkRecoveryAttentionCommand{
		Step: step,
		Attention: journal.OpenAttentionCommand{
			RunID:              owner.RunID,
			Attention:          binding,
			ExpectedGeneration: 0,
			Question:           question,
		},
	}
	if replaced != nil {
		command.Resolve = &journal.ResolveAttentionCommand{
			RunID:              owner.RunID,
			Attention:          replaced.Attention,
			ExpectedGeneration: replaced.Generation,
		}
	}
	if human != nil {
		if err := s.persistHumanParkCheckpoint(
			context.WithoutCancel(ctx),
			owner,
			effectID,
			command,
		); err != nil {
			return err
		}
		crashHumanTurnBarrier("before_park_commit")
	}
	if _, parkErr := s.journal.ParkRecoveryAttention(
		context.WithoutCancel(ctx),
		owner,
		command,
		s.now().UTC(),
	); parkErr != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", parkErr)
	}
	if human != nil {
		crashHumanTurnBarrier("after_park_commit")
	}
	if pending != nil && pending.handle != nil {
		if storeErr := s.storeRecoverableContinuation(
			owner.RunID,
			effectID,
			pending,
		); storeErr != nil {
			return storeErr
		}
		pending = nil
	}
	return runtimeFail("EFFECT_PARKED", nil)
}

func closeRecoveryContinuation(entry *retainedContinuation) error {
	return closeRetainedContinuation(entry)
}

func recoveryFailure(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	return errors.Join(
		primary,
		runtimeFail("CONTINUATION_CLEANUP_FAILED", cleanup),
	)
}
