package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type turnRecoveryFixtureDriver struct {
	mu              sync.Mutex
	parkS1          bool
	failAnswer      bool
	successor       string
	automaticAnswer bool
	measured        bool
	onAnswer        func(driver.Invocation) error
	automationCalls int
	answerCalls     int
}

func completedTurnRecoveryObservation() driver.Observation {
	return driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
	}
}

func measuredTurnRecoveryObservation(
	duration, input, output int64,
) driver.Observation {
	observation := completedTurnRecoveryObservation()
	observation.DurationMillis = duration
	observation.Usage = driver.UsageReceipt{
		TokenStatus:  driver.UsageReported,
		InputTokens:  &input,
		OutputTokens: &output,
		CostStatus:   driver.UsageUnavailable,
	}
	return observation
}

func TestTurnRecoveryTotalsPreserveReportedUsageAndElapsed(
	t *testing.T,
) {
	var totals turnRecoveryTotals
	for _, observation := range []driver.Observation{
		measuredTurnRecoveryObservation(5, 7, 11),
		measuredTurnRecoveryObservation(31, 23, 29),
		measuredTurnRecoveryObservation(13, 17, 19),
	} {
		if err := totals.add(
			observation.DurationMillis,
			observation.Usage,
		); err != nil {
			t.Fatal(err)
		}
	}
	var result driver.Observation
	totals.apply(&result)
	if result.DurationMillis != 49 ||
		result.Usage.InputTokens == nil ||
		*result.Usage.InputTokens != 47 ||
		result.Usage.OutputTokens == nil ||
		*result.Usage.OutputTokens != 59 {
		t.Fatalf("recovery totals = %#v", result)
	}
}

func preparedTurnRecoveryFixture(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
) (preparedDriverDispatch, turnRecoveryCycle) {
	t.Helper()
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := turnRecoveryCycleForDispatch(
		fixture.manifest,
		prepared,
		fixture.coordinates,
		fixture.cycle.DispatchWork,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, cycle
}

func TestRecoveryContinuationOwnershipIsTotalOnSetupAndParkFailure(
	t *testing.T,
) {
	t.Run("revalidation failure", func(t *testing.T) {
		fixture := newProductionImplementationRecoveryFixture(t, nil)
		defer fixture.workspace.Close()
		prepared, cycle := preparedTurnRecoveryFixture(t, fixture)
		prepared.inputBody = []byte(`{"stale":true}`)
		pending := &retainedContinuation{
			handle: &driver.Continuation{},
		}
		_, retained, err := fixture.service.invokeRecoverableWorker(
			fixture.ctx,
			fixture.engine,
			fixture.workspace,
			prepared,
			fixture.cycle.Before,
			fixture.owner,
			&cycle,
			pending,
			driver.RecoverableTurnInput{
				SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
				Kind:          driver.RecoverableInputNudge,
			},
			true,
		)
		if !IsCode(err, "STALE_DISPATCH") ||
			retained != nil ||
			pending.handle != nil {
			t.Fatalf(
				"revalidation error=%v retained=%#v pending=%#v",
				err,
				retained,
				pending,
			)
		}
	})

	t.Run("journal park failure", func(t *testing.T) {
		fixture := newProductionImplementationRecoveryFixture(t, nil)
		defer fixture.workspace.Close()
		_, cycle := preparedTurnRecoveryFixture(t, fixture)
		prior := journal.AttentionBinding{
			Ordinal:  99,
			Recovery: cycle.binding,
		}
		prior.ID = journal.AttentionID(
			prior.Recovery,
			prior.Ordinal,
		)
		opened, err := fixture.store.OpenAttention(
			fixture.ctx,
			fixture.owner,
			journal.OpenAttentionCommand{
				RunID:              fixture.owner.RunID,
				Attention:          prior,
				ExpectedGeneration: 0,
				Question:           "Original recovery question.",
			},
			fixture.now,
		)
		if err != nil {
			t.Fatal(err)
		}
		staleAnswered := journal.AttentionProjection{
			Attention:  prior,
			Generation: opened.Generation + 1,
			State:      journal.AttentionAnswered,
			Question:   "Original recovery question.",
			Answer:     "Original recovery answer.",
		}
		pending := &retainedContinuation{
			handle: &driver.Continuation{},
		}
		err = fixture.service.parkTurnRecoveryReplacing(
			fixture.ctx,
			fixture.owner,
			&cycle,
			fixture.cycle.DispatchEffect,
			"Replacement recovery question.",
			pending,
			&staleAnswered,
			nil,
		)
		if !IsCode(err, "JOURNAL_WRITE_FAILED") ||
			pending.handle != nil {
			t.Fatalf(
				"park failure=%v pending=%#v",
				err,
				pending,
			)
		}
	})
}

func (fixture *turnRecoveryFixtureDriver) Invoke(
	_ context.Context,
	invocation driver.Invocation,
) (driver.Observation, error) {
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return driver.Observation{}, err
	}
	fixture.mu.Lock()
	park := fixture.parkS1 &&
		descriptor.Responsibility == driver.ImplementerImplementation &&
		strings.Contains(invocation.Request.InvocationID, "/S1/")
	measured := fixture.measured
	if park {
		fixture.parkS1 = false
	}
	fixture.mu.Unlock()
	if park {
		observation := completedTurnRecoveryObservation()
		if measured {
			observation = measuredTurnRecoveryObservation(5, 7, 11)
		}
		observation.Yield = &driver.Yield{
			SchemaVersion: driver.YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          driver.YieldQuestion,
			Message:       "Which exact approved value should I use?",
		}
		return observation, nil
	}
	return turnRecoveryFixtureHandoff(invocation, descriptor)
}

func (fixture *turnRecoveryFixtureDriver) InvokeRecoverableTurn(
	_ context.Context,
	invocation driver.Invocation,
	_ driver.ContinuationBinding,
	_ *driver.Continuation,
	input *driver.RecoverableTurnInput,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	if input == nil ||
		input.SchemaVersion != driver.RecoverableTurnInputSchemaVersion ||
		input.Kind != driver.RecoverableInputAnswer ||
		input.Answer != "Use the exact approved fixture value." {
		return driver.Observation{}, nil,
			driver.ContinuationResult{}, fmt.Errorf("unexpected recovery input")
	}
	fixture.mu.Lock()
	fixture.answerCalls++
	failAnswer := fixture.failAnswer
	successor := fixture.successor
	measured := fixture.measured
	onAnswer := fixture.onAnswer
	fixture.mu.Unlock()
	if failAnswer {
		return driver.Observation{}, nil,
			driver.ContinuationResult{},
			fmt.Errorf("fixture provider failure")
	}
	if successor != "" {
		observation := completedTurnRecoveryObservation()
		if measured {
			observation = measuredTurnRecoveryObservation(13, 17, 19)
		}
		observation.Yield = &driver.Yield{
			SchemaVersion: driver.YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          driver.YieldQuestion,
			Message:       successor,
		}
		return observation, nil, driver.ContinuationResult{
			Mode:   driver.ContinuationModeFreshRehydrate,
			Status: driver.ContinuationStatusCompleted,
		}, nil
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return driver.Observation{}, nil,
			driver.ContinuationResult{}, err
	}
	if onAnswer != nil {
		if err := onAnswer(invocation); err != nil {
			return driver.Observation{}, nil,
				driver.ContinuationResult{}, err
		}
	}
	observation, err := turnRecoveryFixtureHandoff(
		invocation,
		descriptor,
	)
	if measured {
		observation.DurationMillis = 13
		input, output := int64(17), int64(19)
		observation.Usage = driver.UsageReceipt{
			TokenStatus:  driver.UsageReported,
			InputTokens:  &input,
			OutputTokens: &output,
			CostStatus:   driver.UsageUnavailable,
		}
	}
	return observation, nil, driver.ContinuationResult{
		Mode:   driver.ContinuationModeFreshRehydrate,
		Status: driver.ContinuationStatusCompleted,
	}, err
}

func (fixture *turnRecoveryFixtureDriver) InvokeAutomation(
	_ context.Context,
	invocation driver.AutomationInvocation,
) (driver.AutomationObservation, error) {
	if invocation.Recovery == nil || invocation.Advisory != nil {
		return driver.AutomationObservation{},
			fmt.Errorf("unexpected automation invocation")
	}
	fixture.mu.Lock()
	fixture.automationCalls++
	automaticAnswer := fixture.automaticAnswer
	measured := fixture.measured
	fixture.mu.Unlock()
	observation := driver.AutomationObservation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Recovery: &driver.RecoveryDecision{
			SchemaVersion: driver.RecoveryDecisionSchemaVersion,
			InvocationID:  invocation.Recovery.InvocationID,
			Action:        driver.RecoveryPauseForHuman,
		},
	}
	if automaticAnswer {
		answer := "Use the exact approved fixture value."
		observation.Recovery.Action = driver.RecoveryResumeWorker
		observation.Recovery.Answer = &answer
	}
	if measured {
		input, output := int64(23), int64(29)
		observation.DurationMillis = 31
		observation.Usage = driver.UsageReceipt{
			TokenStatus:  driver.UsageReported,
			InputTokens:  &input,
			OutputTokens: &output,
			CostStatus:   driver.UsageUnavailable,
		}
	}
	return observation, nil
}

func turnRecoveryFixtureHandoff(
	invocation driver.Invocation,
	descriptor driver.PermissionDescriptor,
) (driver.Observation, error) {
	responsibility := descriptor.Responsibility
	if responsibility == driver.ImplementerImplementation {
		path := "one.txt"
		if strings.Contains(invocation.Request.InvocationID, "/S2/") {
			path = "two.txt"
		}
		if err := os.WriteFile(
			filepath.Join(invocation.HostWorkspace, path),
			[]byte("recovered fixture\n"),
			0o600,
		); err != nil {
			return driver.Observation{}, err
		}
	}
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocation.Request.InvocationID,
		Responsibility: responsibility,
		Summary:        "Exact recovery fixture handoff.",
		Detail:         "Bound to the current admitted responsibility.",
	}
	var err error
	switch responsibility {
	case driver.CaptainReview:
		submission.Decision, err = driver.NewDecision(
			driver.DecisionProceed,
		)
	case driver.ImplementerImplementation:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("implementation recovery fixture checks\n"),
		)
	case driver.WorkVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("work recovery fixture checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(
				driver.DecisionPass,
			)
		}
	case driver.AssemblyVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("assembly recovery fixture checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(
				driver.DecisionPass,
			)
		}
	}
	if err != nil {
		return driver.Observation{}, err
	}
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		return driver.Observation{}, err
	}
	observation := completedTurnRecoveryObservation()
	observation.Handoff = &driver.SealedHandoff{
		SubmissionBytes:  body,
		SubmissionDigest: driver.Digest(body),
	}
	return observation, nil
}

func TestTurnRecoveryParksExactLaneWithoutFalseAcceptanceAndResumesAfterRestart(
	t *testing.T,
) {
	driverBeforeRestart := &turnRecoveryFixtureDriver{
		parkS1:   true,
		measured: true,
	}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		driverBeforeRestart,
	)
	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.ReconcileOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID:     fixture.owner.RunID,
			EffectID:  fixture.outer.ID,
			Token:     fixture.outer.CurrentClaim,
			EventKind: "test_outer_not_started",
			At:        fixture.now,
		},
		journal.RecoveryAllOld,
	); err != nil {
		t.Fatal(err)
	}

	before := fixture.state
	if err := fixture.service.advanceSlice(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		"S1",
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("initial recovery = %v", err)
	}
	afterPark, err := baton.ReadState(
		fixture.engine.git,
		fixture.manifest.value.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	beforeS1, _ := before.Slice("S1")
	parkedS1, _ := afterPark.Slice("S1")
	beforeT1, _ := before.Track("T1")
	parkedT1, _ := afterPark.Track("T1")
	if beforeS1.CurrentReceipt.OID != parkedS1.CurrentReceipt.OID ||
		beforeS1.Stage != parkedS1.Stage ||
		beforeT1.Head != parkedT1.Head {
		t.Fatalf("yield changed Baton authority: %#v", parkedS1)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen {
		t.Fatalf("open attention = %#v, %v", attentions, err)
	}
	for _, effectID := range []string{
		fixture.outer.ID,
		fixture.cycle.DispatchEffect,
	} {
		effect, effectErr := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			effectID,
		)
		if effectErr != nil || effect.State != journal.Claimed {
			t.Fatalf("parked effect %s = %#v, %v", effectID, effect, effectErr)
		}
	}
	status, err := fixture.service.Status(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || status.State != "parked" {
		t.Fatalf("parked status = %#v, %v", status, err)
	}
	pending, err := fixture.service.driverRecoveryPending(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || pending {
		t.Fatalf("parked lane fenced independent work: %t, %v", pending, err)
	}
	if err := fixture.service.recoverClaimedEffects(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.advanceSlice(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		"S2",
	); err != nil {
		t.Fatalf("independent lane = %v", err)
	}
	independent, err := baton.ReadState(
		fixture.engine.git,
		fixture.manifest.value.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := independent.Slice("S2")
	if s2.Stage != "design" || s2.NextRole != "captain" {
		t.Fatalf("independent lane did not progress: %#v", s2)
	}

	if err := fixture.store.ReleaseOwner(
		fixture.ctx,
		fixture.owner,
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}
	driverAfterRestart := &turnRecoveryFixtureDriver{measured: true}
	restartedProduction, err := newProductionDriverRuntime(
		fixture.config,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal:       fixture.store,
		dispatcher:    driverAfterRestart,
		production:    restartedProduction,
		gitExecutable: fixture.service.gitExecutable,
		now:           fixture.service.now,
	}
	if _, err := fixture.store.AnswerAttention(
		fixture.ctx,
		journal.AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          attentions[0].Attention,
			ExpectedGeneration: 1,
			Answer:             "Use the exact approved fixture value.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	status, err = restarted.Status(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || status.State != "takeover_required" {
		t.Fatalf("answered without owner = %#v, %v", status, err)
	}
	restartedOwner, err := fixture.store.AcquireOwner(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.now,
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.store.ReleaseOwner(
		context.Background(),
		restartedOwner,
		fixture.now,
	)
	restartedManifest, _, err := restarted.loadRun(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	restartedEngine, err := restarted.openEngine(restartedManifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
	probeWorkspace, err := restartedEngine.workspaces.OpenTrack(
		gitx.TrackKey{
			Release: fixture.state.Release,
			Track:   fixture.track.ID,
		},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	probePrepared, err := restarted.prepareDriverDispatch(
		fixture.ctx,
		restartedEngine,
		probeWorkspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatalf("lane-local dispatch preparation after T2 progress: %v", err)
	}
	persistedCommand, persisted, err := restarted.persistedDriverCommand(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || !persisted {
		t.Fatalf("persisted parked dispatch = %t, %v", persisted, err)
	}
	prior, err := parseProductionDispatchCommand(
		restartedManifest,
		persistedCommand.Payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	currentBody, err := currentPreparedProductionBody(
		fixture.ctx,
		restartedEngine,
		fixture.coordinates,
		fixture.cycle.Before,
		probePrepared,
	)
	if err != nil || !bytes.Equal(currentBody, mustJSON(prior.Context)) {
		t.Fatalf("lane-local persisted authority = %v, exact=%t", err,
			bytes.Equal(currentBody, mustJSON(prior.Context)))
	}
	probePrepared, err = restorePreparedProductionDispatch(
		restartedManifest,
		probePrepared,
		prior,
	)
	if err != nil {
		t.Fatalf("restore parked dispatch: %v", err)
	}
	if err := revalidatePreparedProductionDispatch(
		fixture.ctx,
		restartedEngine,
		fixture.coordinates,
		fixture.cycle.Before,
		probePrepared,
	); err != nil {
		t.Fatalf("revalidate parked dispatch: %v", err)
	}
	probeCycle, err := turnRecoveryCycleForDispatch(
		restartedManifest,
		probePrepared,
		fixture.coordinates,
		fixture.cycle.DispatchWork,
		fixture.cycle.Before,
	)
	if err != nil ||
		probeCycle.binding.CycleID !=
			attentions[0].Attention.Recovery.CycleID {
		t.Fatalf("lane-local recovery cycle = %v, exact=%t", err,
			probeCycle.binding.CycleID ==
				attentions[0].Attention.Recovery.CycleID)
	}
	if err := probeWorkspace.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.recoverImplementationClaims(
		fixture.ctx,
		restartedEngine,
		restartedOwner,
	)
	if err != nil || !recovered {
		if err == nil {
			err = fmt.Errorf("answered recovery was not selected")
		}
		t.Fatal(err)
	}
	recoveredState, err := baton.ReadState(
		restartedEngine.git,
		fixture.manifest.value.Release,
		restartedEngine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	recoveredS1, found := recoveredState.Slice("S1")
	if !found ||
		recoveredS1.Stage != "verify" ||
		recoveredS1.NextRole != "verifier" {
		t.Fatalf("restarted S1 recovery = %#v", recoveredS1)
	}
	driverAfterRestart.mu.Lock()
	answerCalls := driverAfterRestart.answerCalls
	driverAfterRestart.mu.Unlock()
	if answerCalls != 1 {
		dispatch, _ := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			fixture.cycle.DispatchEffect,
		)
		outer, _ := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			fixture.outer.ID,
		)
		currentAttention, _ := fixture.store.Attention(
			fixture.ctx,
			fixture.owner.RunID,
			attentions[0].Attention.ID,
		)
		t.Fatalf(
			"answer resumes = %d; dispatch=%s/%s outer=%s/%s attention=%s",
			answerCalls,
			dispatch.State,
			dispatch.ErrorCode,
			outer.State,
			outer.ErrorCode,
			currentAttention.State,
		)
	}
	attention, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attentions[0].Attention.ID,
	)
	if err != nil || attention.State != journal.AttentionResolved {
		t.Fatalf("resolved attention = %#v, %v", attention, err)
	}
	observation, err := fixture.store.ReadObservation(
		fixture.ctx,
		fixture.owner.RunID,
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	var usage driver.UsageReceipt
	dispatchAttempts := 0
	for _, attempt := range observation.Attempts {
		if attempt.EffectID != fixture.cycle.DispatchEffect {
			continue
		}
		dispatchAttempts++
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil {
			t.Fatal(err)
		}
	}
	if dispatchAttempts != 1 ||
		usage.InputTokens == nil || *usage.InputTokens != 47 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 59 {
		t.Fatalf(
			"restart-folded recovery attempts=%d usage=%#v",
			dispatchAttempts,
			usage,
		)
	}
	var parkedEvents, recoveredEvents, falseAcceptances int
	for _, event := range observation.Events {
		switch {
		case event.Kind == journal.RecoveryParkedEvent:
			parkedEvents++
		case strings.HasPrefix(
			event.Kind,
			turnRecoveryRecoveredEvent,
		):
			recoveredEvents++
		case event.Kind ==
			"turn_recovery.outcome.false_acceptance":
			falseAcceptances++
		}
	}
	if parkedEvents != 1 || recoveredEvents != 1 ||
		falseAcceptances != 0 {
		t.Fatalf(
			"recovery events park=%d recovered=%d false=%d",
			parkedEvents,
			recoveredEvents,
			falseAcceptances,
		)
	}
}

func TestAnswerBetweenDriveDecisionAndOwnerReleaseIsConsumed(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	if err := fixture.store.RecordCommand(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: "manifest",
			Kind:      "start",
			Payload:   fixture.manifest.raw,
			CreatedAt: fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.ReconcileOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID:     fixture.owner.RunID,
			EffectID:  fixture.outer.ID,
			Token:     fixture.outer.CurrentClaim,
			EventKind: "test_outer_not_started",
			At:        fixture.now,
		},
		journal.RecoveryAllOld,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.advanceSlice(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		"S1",
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("initial park = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen {
		t.Fatalf("open attention = %#v, %v", attentions, err)
	}
	attention := attentions[0]
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}

	var answerOnce sync.Once
	var answerErr error
	fixture.service.beforeOwnerRelease = func() {
		answerOnce.Do(func() {
			_, answerErr = fixture.store.AnswerAttention(
				fixture.ctx,
				journal.AnswerAttentionCommand{
					RunID:              fixture.owner.RunID,
					Attention:          attention.Attention,
					ExpectedGeneration: attention.Generation,
					Answer:             "Use the exact approved fixture value.",
				},
				fixture.now,
			)
		})
	}
	if _, err := fixture.service.driveOwned(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.owner,
	); err != nil {
		t.Fatalf("drive after release-race answer = %v", err)
	}
	if answerErr != nil {
		t.Fatalf("answer at release barrier = %v", answerErr)
	}
	current, err := fixture.store.Attention(
		fixture.ctx,
		fixture.owner.RunID,
		attention.Attention.ID,
	)
	if err != nil || current.State != journal.AttentionResolved {
		t.Fatalf("consumed attention = %#v, %v", current, err)
	}
	fixtureDriver.mu.Lock()
	answerCalls := fixtureDriver.answerCalls
	fixtureDriver.mu.Unlock()
	if answerCalls != 1 {
		t.Fatalf("answer dispatch calls = %d, want 1", answerCalls)
	}
	if owner, present, err := fixture.store.CurrentOwner(
		fixture.ctx,
		fixture.owner.RunID,
	); err != nil {
		t.Fatal(err)
	} else if present {
		t.Fatalf("owner remained after consuming release-race answer = %#v", owner)
	}
}

func TestAutomaticTurnRecoveryPersistsAggregateUsage(
	t *testing.T,
) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:          true,
		automaticAnswer: true,
		measured:        true,
	}
	fixture := newProductionImplementationRecoveryFixture(
		t,
		fixtureDriver,
	)
	defer fixture.workspace.Close()
	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); err != nil {
		t.Fatal(err)
	}
	observation, err := fixture.store.ReadObservation(
		fixture.ctx,
		fixture.owner.RunID,
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	var usage driver.UsageReceipt
	for _, attempt := range observation.Attempts {
		if attempt.EffectID != fixture.cycle.DispatchEffect {
			continue
		}
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil {
			t.Fatal(err)
		}
	}
	if usage.InputTokens == nil || *usage.InputTokens != 47 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 59 {
		t.Fatalf("persisted aggregate usage = %#v", usage)
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	recoveredEvents := 0
	for _, event := range snapshot.Events {
		if strings.HasPrefix(
			event.Kind,
			turnRecoveryRecoveredEvent,
		) {
			recoveredEvents++
		}
	}
	if recoveredEvents != 1 {
		t.Fatalf("durable recovered events = %d, want 1", recoveredEvents)
	}
}

func TestAnsweredRecoveryRemainsDurableUntilCompletionOrSuccessorPark(
	t *testing.T,
) {
	for _, test := range []struct {
		name      string
		failure   bool
		successor string
		drift     bool
		wantCode  string
	}{
		{
			name:     "provider failure retains answer",
			failure:  true,
			wantCode: "RECOVERY_UNCERTAIN",
		},
		{
			name:      "successor question replaces answer atomically",
			successor: "Which approved follow-up value should I use?",
			wantCode:  "EFFECT_PARKED",
		},
		{
			name:     "unsafe recovered output is rejected without false acceptance",
			drift:    true,
			wantCode: "RECOVERY_UNCERTAIN",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixtureDriver := &turnRecoveryFixtureDriver{parkS1: true}
			fixture := newProductionImplementationRecoveryFixture(
				t,
				fixtureDriver,
			)
			defer fixture.workspace.Close()

			if _, _, err := fixture.service.
				runProductionImplementationDispatch(
					fixture.ctx,
					fixture.engine,
					fixture.owner,
					fixture.workspace,
					fixture.cycle,
					fixture.coordinates,
				); !IsCode(err, "EFFECT_PARKED") {
				t.Fatalf("initial park = %v", err)
			}
			attentions, err := fixture.store.Attentions(
				fixture.ctx,
				fixture.owner.RunID,
			)
			if err != nil || len(attentions) != 1 ||
				attentions[0].State != journal.AttentionOpen {
				t.Fatalf("initial attention = %#v, %v", attentions, err)
			}
			original := attentions[0]
			if _, err := fixture.store.AnswerAttention(
				fixture.ctx,
				journal.AnswerAttentionCommand{
					RunID:              fixture.owner.RunID,
					Attention:          original.Attention,
					ExpectedGeneration: original.Generation,
					Answer:             "Use the exact approved fixture value.",
				},
				fixture.now,
			); err != nil {
				t.Fatal(err)
			}

			fixtureDriver.mu.Lock()
			fixtureDriver.failAnswer = test.failure
			fixtureDriver.successor = test.successor
			if test.drift {
				fixtureDriver.onAnswer = func(
					invocation driver.Invocation,
				) error {
					path := filepath.Join(
						invocation.HostWorkspace,
						"authority-drift.txt",
					)
					if err := os.WriteFile(
						path,
						[]byte("changed\n"),
						0o600,
					); err != nil {
						return err
					}
					runRuntimeGit(
						t,
						invocation.HostWorkspace,
						"add",
						"--",
						"authority-drift.txt",
					)
					runRuntimeGit(
						t,
						invocation.HostWorkspace,
						"-c",
						"user.name=Recovery Fixture",
						"-c",
						"user.email=recovery@example.invalid",
						"commit",
						"--quiet",
						"-m",
						"change recovery authority",
					)
					return nil
				}
			}
			fixtureDriver.mu.Unlock()
			_, _, err = fixture.service.runProductionImplementationDispatch(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
				fixture.workspace,
				fixture.cycle,
				fixture.coordinates,
			)
			if !IsCode(err, test.wantCode) {
				t.Fatalf("answered dispatch = %v", err)
			}

			attentions, err = fixture.store.Attentions(
				fixture.ctx,
				fixture.owner.RunID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if test.failure || test.drift {
				if len(attentions) != 1 ||
					attentions[0].State !=
						journal.AttentionAnswered ||
					attentions[0].Answer !=
						"Use the exact approved fixture value." {
					t.Fatalf(
						"retained answered attention = %#v",
						attentions,
					)
				}
			} else {
				var resolved, open bool
				for _, attention := range attentions {
					switch {
					case attention.Attention.ID ==
						original.Attention.ID:
						resolved = attention.State ==
							journal.AttentionResolved
					case attention.State ==
						journal.AttentionOpen &&
						attention.Question == test.successor:
						open = true
					}
				}
				if len(attentions) != 2 || !resolved || !open {
					t.Fatalf(
						"atomic successor attention = %#v",
						attentions,
					)
				}
			}
			for _, effectID := range []string{
				fixture.outer.ID,
				fixture.cycle.DispatchEffect,
			} {
				effect, effectErr := fixture.store.Effect(
					fixture.ctx,
					fixture.owner.RunID,
					effectID,
				)
				if effectErr != nil ||
					effect.State != journal.Claimed {
					t.Fatalf(
						"recoverable effect %s = %#v, %v",
						effectID,
						effect,
						effectErr,
					)
				}
			}
			if test.drift {
				state, stateErr := baton.ReadState(
					fixture.engine.git,
					fixture.manifest.value.Release,
					fixture.engine.inertness,
				)
				if stateErr != nil {
					t.Fatal(stateErr)
				}
				beforeSlice, _ := fixture.state.Slice("S1")
				afterSlice, _ := state.Slice("S1")
				beforeTrack, _ := fixture.state.Track("T1")
				afterTrack, _ := state.Track("T1")
				if beforeSlice.CurrentReceipt.OID !=
					afterSlice.CurrentReceipt.OID ||
					beforeSlice.Stage != afterSlice.Stage ||
					beforeSlice.NextRole != afterSlice.NextRole ||
					beforeTrack.Head != afterTrack.Head ||
					fixture.state.Refs.Release.Head !=
						state.Refs.Release.Head {
					t.Fatalf(
						"unsafe recovery advanced Baton: %#v",
						afterSlice,
					)
				}
				snapshot, snapshotErr := fixture.store.Snapshot(
					fixture.ctx,
					fixture.owner.RunID,
				)
				if snapshotErr != nil {
					t.Fatal(snapshotErr)
				}
				for _, event := range snapshot.Events {
					if event.Kind ==
						"turn_recovery.outcome.false_acceptance" {
						t.Fatalf(
							"rejected breach counted as false acceptance",
						)
					}
				}
			}
		})
	}
}
