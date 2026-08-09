package journal

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testRecoveryBinding(lane, cycle, turn, progress string) RecoveryBinding {
	return RecoveryBinding{
		LaneID:     lane,
		CycleID:    digest([]byte(cycle)),
		TurnID:     digest([]byte(turn)),
		ProgressID: digest([]byte(progress)),
	}
}

func testAttentionBinding(
	recovery RecoveryBinding,
	ordinal int64,
) AttentionBinding {
	return AttentionBinding{
		ID:       AttentionID(recovery, ordinal),
		Ordinal:  ordinal,
		Recovery: recovery,
	}
}

func testRecoveryStep(
	runID string,
	binding RecoveryBinding,
	ordinal int64,
	kind RecoveryStepKind,
) RecoveryStepCommand {
	return RecoveryStepCommand{
		RunID:   runID,
		ID:      RecoveryStepID(binding, ordinal),
		Binding: binding,
		Ordinal: ordinal,
		Kind:    kind,
	}
}

func TestHumanTurnAttentionBindingIsCompleteVersionedAndLegacyCompatible(
	t *testing.T,
) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding("T1", "human-cycle", "human-turn", "human-work")
	binding := testAttentionBinding(recovery, 1)
	binding.HumanTurn = &HumanTurnBinding{
		SchemaVersion:         HumanTurnBindingVersion,
		Kind:                  "human_confirmation",
		RunID:                 run.ID,
		Track:                 "T1",
		Slice:                 "S1",
		Role:                  "implementer",
		Responsibility:        "implementer_implementation",
		InvocationID:          run.ID + "/S1/implementer_implementation/1/1/1",
		BatonAttempt:          1,
		PlanAuthorityDigest:   digest([]byte("plan-authority")),
		TargetAuthorityDigest: digest([]byte("target-authority")),
		WorkIdentity:          recovery.ProgressID,
		CycleID:               recovery.CycleID,
		TurnID:                recovery.TurnID,
		Ordinal:               1,
		OpenGeneration:        1,
	}
	command := OpenAttentionCommand{
		RunID: run.ID, Attention: binding,
		ExpectedGeneration: 0,
		Question:           "Does this exactly match your intended meaning?",
	}
	opened, err := store.OpenAttention(ctx, owner, command, now)
	if err != nil || opened.Generation != 1 ||
		!equalAttentionBinding(opened.Attention, binding) {
		t.Fatalf("human open = %#v, %v", opened, err)
	}
	replayed, err := store.OpenAttention(
		ctx,
		OwnerLease{},
		command,
		now.Add(time.Hour),
	)
	if err != nil || replayed.Generation != opened.Generation ||
		replayed.State != opened.State ||
		!equalAttentionBinding(replayed.Attention, opened.Attention) {
		t.Fatalf("human replay = %#v, %v", replayed, err)
	}

	mutations := []struct {
		name   string
		mutate func(*HumanTurnBinding)
	}{
		{"schema_version", func(value *HumanTurnBinding) { value.SchemaVersion = "" }},
		{"kind", func(value *HumanTurnBinding) { value.Kind = "question" }},
		{"run_id", func(value *HumanTurnBinding) { value.RunID = "" }},
		{"track", func(value *HumanTurnBinding) { value.Track = "" }},
		{"slice", func(value *HumanTurnBinding) { value.Slice = "" }},
		{"role", func(value *HumanTurnBinding) { value.Role = "merge" }},
		{"responsibility", func(value *HumanTurnBinding) { value.Responsibility = "" }},
		{"invocation_id", func(value *HumanTurnBinding) { value.InvocationID = "" }},
		{"baton_attempt", func(value *HumanTurnBinding) { value.BatonAttempt = 0 }},
		{"plan_authority", func(value *HumanTurnBinding) { value.PlanAuthorityDigest = "" }},
		{"target_authority", func(value *HumanTurnBinding) { value.TargetAuthorityDigest = "" }},
		{"work_identity", func(value *HumanTurnBinding) { value.WorkIdentity = digest([]byte("wrong-work")) }},
		{"cycle_id", func(value *HumanTurnBinding) { value.CycleID = digest([]byte("wrong-cycle")) }},
		{"turn_id", func(value *HumanTurnBinding) { value.TurnID = digest([]byte("wrong-turn")) }},
		{"ordinal", func(value *HumanTurnBinding) { value.Ordinal++ }},
		{"open_generation", func(value *HumanTurnBinding) { value.OpenGeneration++ }},
	}
	for index, mutation := range mutations {
		t.Run("reject_"+mutation.name, func(t *testing.T) {
			invalid := testAttentionBinding(recovery, int64(index+2))
			invalidHuman := *binding.HumanTurn
			invalidHuman.WorkIdentity = invalid.Recovery.ProgressID
			invalidHuman.CycleID = invalid.Recovery.CycleID
			invalidHuman.TurnID = invalid.Recovery.TurnID
			invalidHuman.Ordinal = invalid.Ordinal
			mutation.mutate(&invalidHuman)
			invalid.HumanTurn = &invalidHuman
			if _, err := store.OpenAttention(
				ctx,
				owner,
				OpenAttentionCommand{
					RunID: run.ID, Attention: invalid,
					ExpectedGeneration: 0,
					Question:           "Reject incomplete human binding.",
				},
				now,
			); !IsCode(err, "INVALID_ATTENTION_BINDING") {
				t.Fatalf("mutation %s error = %v", mutation.name, err)
			}
		})
	}

	legacy := testAttentionBinding(recovery, 3)
	if _, err := store.OpenAttention(
		ctx,
		owner,
		OpenAttentionCommand{
			RunID: run.ID, Attention: legacy,
			ExpectedGeneration: 0, Question: "Legacy recovery question.",
		},
		now,
	); err != nil {
		t.Fatalf("legacy attention = %v", err)
	}
}

func TestAttentionLifecycleIsPerAttentionCASReplaySafeAndContentBounded(
	t *testing.T,
) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding("T3-runtime", "cycle", "turn", "progress")
	first := testAttentionBinding(recovery, 1)
	second := testAttentionBinding(recovery, 2)
	open := OpenAttentionCommand{
		RunID: run.ID, Attention: first,
		ExpectedGeneration: 0, Question: "Which verified base should continue?",
	}
	opened, err := store.OpenAttention(ctx, owner, open, now)
	if err != nil || opened.Generation != 1 || opened.State != AttentionOpen {
		t.Fatalf("open = %#v, %v", opened, err)
	}
	replayed, err := store.OpenAttention(
		ctx,
		OwnerLease{},
		open,
		now.Add(time.Hour),
	)
	if err != nil || replayed != opened {
		t.Fatalf("owner-free exact replay = %#v, %v", replayed, err)
	}
	conflict := open
	conflict.Question = "A different question"
	if _, err := store.OpenAttention(
		ctx,
		owner,
		conflict,
		now,
	); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting open replay = %v", err)
	}

	answer := AnswerAttentionCommand{
		RunID: run.ID, Attention: first,
		ExpectedGeneration: 1, Answer: "Continue from the exact W3 PASS.",
	}
	answered, err := store.AnswerAttention(ctx, answer, now)
	if err != nil ||
		answered.Generation != 2 ||
		answered.State != AttentionAnswered {
		t.Fatalf("answer = %#v, %v", answered, err)
	}
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: second,
		ExpectedGeneration: 0, Question: "Independent lane question",
	}, now); err != nil {
		t.Fatalf("independent attention open: %v", err)
	}
	firstProjection, err := store.Attention(ctx, run.ID, first.ID)
	if err != nil ||
		firstProjection.Generation != 2 ||
		firstProjection.State != AttentionAnswered ||
		firstProjection.Question != open.Question ||
		firstProjection.Answer != answer.Answer {
		t.Fatalf("first projection = %#v, %v", firstProjection, err)
	}
	secondProjection, err := store.Attention(ctx, run.ID, second.ID)
	if err != nil ||
		secondProjection.Generation != 1 ||
		secondProjection.State != AttentionOpen {
		t.Fatalf("second projection = %#v, %v", secondProjection, err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: second,
		ExpectedGeneration: 1, Answer: "invalid\ranswer",
	}, now); !IsCode(err, "INVALID_ATTENTION") {
		t.Fatalf("carriage-return answer = %v", err)
	}
	secondProjection, err = store.Attention(ctx, run.ID, second.ID)
	if err != nil ||
		secondProjection.Generation != 1 ||
		secondProjection.State != AttentionOpen {
		t.Fatalf(
			"attention after rejected carriage return = %#v, %v",
			secondProjection,
			err,
		)
	}
	if _, err := store.ResolveAttention(ctx, owner, ResolveAttentionCommand{
		RunID: run.ID, Attention: first, ExpectedGeneration: 2,
	}, now); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var eventKinds []string
	for _, event := range snapshot.Events {
		if strings.HasPrefix(event.Kind, "attention_") {
			eventKinds = append(eventKinds, event.Kind)
			if strings.Contains(string(event.Body), open.Question) ||
				strings.Contains(string(event.Body), answer.Answer) {
				t.Fatalf("attention event leaked message: %s", event.Body)
			}
		}
	}
	wantKinds := []string{
		"attention_opened",
		"attention_answered",
		"attention_opened",
		"attention_resolved",
	}
	if strings.Join(eventKinds, ",") != strings.Join(wantKinds, ",") {
		t.Fatalf("attention event kinds = %v, want %v", eventKinds, wantKinds)
	}

	oversized := testAttentionBinding(
		testRecoveryBinding("T4-runtime", "other-cycle", "turn", "progress"),
		1,
	)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: oversized, ExpectedGeneration: 0,
		Question: strings.Repeat("q", MaxAttentionQuestionBytes+1),
	}, now); !IsCode(err, "INVALID_ATTENTION") {
		t.Fatalf("oversized question = %v", err)
	}
	invalidUTF8 := testAttentionBinding(
		testRecoveryBinding("T5-runtime", "utf8-cycle", "turn", "progress"),
		1,
	)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: invalidUTF8, ExpectedGeneration: 0,
		Question: string([]byte{0xff}),
	}, now); !IsCode(err, "INVALID_ATTENTION") {
		t.Fatalf("invalid UTF-8 question = %v", err)
	}
}

func TestAttentionAnswerPersistsWhilePausedAndCancelIsEffectiveTerminal(
	t *testing.T,
) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding("T3-runtime", "cycle", "turn", "progress")
	first := testAttentionBinding(recovery, 1)
	firstOpen := OpenAttentionCommand{
		RunID: run.ID, Attention: first,
		ExpectedGeneration: 0, Question: "Need a human answer",
	}
	if _, err := store.OpenAttention(
		ctx,
		OwnerLease{},
		firstOpen,
		now,
	); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("new open without owner = %v", err)
	}
	if _, err := store.OpenAttention(ctx, owner, firstOpen, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "pause-attention", Kind: Pause,
	}, now); err != nil {
		t.Fatal(err)
	}
	answer := AnswerAttentionCommand{
		RunID: run.ID, Attention: first,
		ExpectedGeneration: 1, Answer: "Approved while paused",
	}
	answered, err := store.AnswerAttention(ctx, answer, now)
	if err != nil || answered.State != AttentionAnswered {
		t.Fatalf("answer while paused = %#v, %v", answered, err)
	}
	if _, err := store.ResolveAttention(ctx, owner, ResolveAttentionCommand{
		RunID: run.ID, Attention: first, ExpectedGeneration: 2,
	}, now); !IsCode(err, "CONTROL_STOPPED") {
		t.Fatalf("resolve while paused = %v", err)
	}
	second := testAttentionBinding(recovery, 2)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: second,
		ExpectedGeneration: 0, Question: "Cannot open while paused",
	}, now); !IsCode(err, "CONTROL_STOPPED") {
		t.Fatalf("open while paused = %v", err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "resume-attention", Kind: Resume,
		ExpectedGeneration: 1,
	}, now); err != nil {
		t.Fatal(err)
	}
	resolve := ResolveAttentionCommand{
		RunID: run.ID, Attention: first, ExpectedGeneration: 2,
	}
	if _, err := store.ResolveAttention(
		ctx,
		OwnerLease{},
		resolve,
		now,
	); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("resolve without owner = %v", err)
	}
	if _, err := store.ResolveAttention(ctx, owner, resolve, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: second,
		ExpectedGeneration: 0, Question: "A second human answer",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "cancel-attention", Kind: Cancel,
		ExpectedGeneration: 2,
	}, now); err != nil {
		t.Fatal(err)
	}
	projected, err := store.Attention(ctx, run.ID, second.ID)
	if err != nil ||
		projected.Generation != 1 ||
		projected.State != AttentionCancelled {
		t.Fatalf("cancelled projection = %#v, %v", projected, err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: second,
		ExpectedGeneration: 1, Answer: "Too late",
	}, now); !IsCode(err, "CONTROL_STOPPED") {
		t.Fatalf("new answer after cancel = %v", err)
	}
	replay, err := store.AnswerAttention(ctx, answer, now.Add(time.Hour))
	if err != nil || replay != answered {
		t.Fatalf("accepted answer replay after cancel = %#v, %v", replay, err)
	}
}

func TestReleaseOwnerIfIdleRetainsAnsweredWakeAtomically(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding(
		"T-owner-wake",
		"owner-wake-cycle",
		"owner-wake-turn",
		"owner-wake-progress",
	)
	attention := testAttentionBinding(recovery, 1)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 0, Question: "Continue this lane?",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 1, Answer: "Continue.",
	}, now); err != nil {
		t.Fatal(err)
	}

	released, err := store.ReleaseOwnerIfIdle(ctx, owner, now)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("owner released while an answered wake was pending")
	}
	current, present, err := store.CurrentOwner(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !present || current.Token != owner.Token {
		t.Fatalf("owner after retained wake = %#v, present=%t", current, present)
	}

	if _, err := store.ResolveAttention(ctx, owner, ResolveAttentionCommand{
		RunID: run.ID, Attention: attention, ExpectedGeneration: 2,
	}, now); err != nil {
		t.Fatal(err)
	}
	released, err = store.ReleaseOwnerIfIdle(ctx, owner, now)
	if err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("idle owner was not released")
	}
	if current, present, err = store.CurrentOwner(ctx, run.ID); err != nil {
		t.Fatal(err)
	} else if present {
		t.Fatalf("owner remained after idle release = %#v", current)
	}
}

func TestReleaseOwnerIfIdleLetsPauseOutrankAnsweredWake(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding(
		"T-paused-wake",
		"paused-wake-cycle",
		"paused-wake-turn",
		"paused-wake-progress",
	)
	attention := testAttentionBinding(recovery, 1)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 0, Question: "Continue after pause?",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 1, Answer: "Continue when resumed.",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "pause-with-wake", Kind: Pause,
	}, now); err != nil {
		t.Fatal(err)
	}

	released, err := store.ReleaseOwnerIfIdle(ctx, owner, now)
	if err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("pause did not outrank answered wake")
	}
	projected, err := store.Attention(ctx, run.ID, attention.ID)
	if err != nil || projected.State != AttentionAnswered {
		t.Fatalf("paused answer = %#v, %v", projected, err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "resume-with-wake", Kind: Resume,
		ExpectedGeneration: 1,
	}, now); err != nil {
		t.Fatal(err)
	}
	resumedOwner, err := store.AcquireOwner(
		ctx,
		run.ID,
		now,
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	released, err = store.ReleaseOwnerIfIdle(ctx, resumedOwner, now)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("resumed answered wake did not retain its owner")
	}
	if _, err := store.ResolveAttention(ctx, resumedOwner, ResolveAttentionCommand{
		RunID: run.ID, Attention: attention, ExpectedGeneration: 2,
	}, now); err != nil {
		t.Fatal(err)
	}
	released, err = store.ReleaseOwnerIfIdle(ctx, resumedOwner, now)
	if err != nil || !released {
		t.Fatalf("release after resumed wake consumption = %t, %v", released, err)
	}
}

func TestRecoveryReservationIsOwnerBoundAndControlRunning(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-authority", "authority-cycle", "turn", "progress",
	)
	step := testRecoveryStep(
		run.ID,
		binding,
		1,
		RecoveryResumeWorker,
	)
	if _, err := store.ReserveRecoveryStep(
		ctx,
		OwnerLease{},
		step,
		now,
	); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("new reservation without owner = %v", err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "pause-recovery", Kind: Pause,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		step,
		now,
	); !IsCode(err, "CONTROL_STOPPED") {
		t.Fatalf("reservation while paused = %v", err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "resume-recovery", Kind: Resume,
		ExpectedGeneration: 1,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		step,
		now,
	); err != nil {
		t.Fatal(err)
	}
}

func TestParkRecoveryAttentionIsAtomicAndReplaySafe(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-atomic-park", "atomic-cycle", "turn", "progress",
	)
	step := testRecoveryStep(run.ID, binding, 1, RecoveryParkTrack)
	inputTokens, outputTokens := int64(7), int64(11)
	step.Accounting = &RecoveryAccounting{
		DurationMillis: 5,
		TokenStatus:    "reported",
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
		CostStatus:     "unavailable",
	}
	attention := testAttentionBinding(binding, 1)
	command := ParkRecoveryAttentionCommand{
		Step: step,
		Attention: OpenAttentionCommand{
			RunID: run.ID, Attention: attention,
			ExpectedGeneration: 0, Question: "Which exact base should continue?",
		},
	}
	parked, err := store.ParkRecoveryAttention(ctx, owner, command, now)
	if err != nil ||
		!parked.Step.Parked ||
		parked.Attention.State != AttentionOpen {
		t.Fatalf("atomic park = %#v, %v", parked, err)
	}
	replayed, err := store.ParkRecoveryAttention(
		ctx,
		OwnerLease{},
		command,
		now.Add(time.Hour),
	)
	if err != nil || !reflect.DeepEqual(replayed, parked) {
		t.Fatalf("owner-free replay = %#v, %v", replayed, err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	parkEvents := 0
	for _, event := range snapshot.Events {
		if event.Kind == RecoveryParkedEvent {
			parkEvents++
		}
	}
	if parkEvents != 1 {
		t.Fatalf("replayed park events = %d, want 1", parkEvents)
	}
	budget, err := store.RecoveryBudget(ctx, run.ID, binding)
	if err != nil ||
		!reflect.DeepEqual(budget.Accounting, step.Accounting) {
		t.Fatalf(
			"parked accounting = %#v, want %#v, error=%v",
			budget.Accounting,
			step.Accounting,
			err,
		)
	}
	changedAccounting := command
	changedAccounting.Step.Accounting = cloneRecoveryAccounting(
		command.Step.Accounting,
	)
	changedAccounting.Step.Accounting.DurationMillis++
	if _, err := store.ParkRecoveryAttention(
		ctx,
		OwnerLease{},
		changedAccounting,
		now,
	); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("changed accounting replay = %v", err)
	}

	rollbackBinding := testRecoveryBinding(
		"T-atomic-rollback", "rollback-cycle", "turn", "progress",
	)
	rollbackAttention := testAttentionBinding(rollbackBinding, 1)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: rollbackAttention,
		ExpectedGeneration: 0, Question: "Existing question",
	}, now); err != nil {
		t.Fatal(err)
	}
	conflict := ParkRecoveryAttentionCommand{
		Step: testRecoveryStep(
			run.ID,
			rollbackBinding,
			1,
			RecoveryParkTrack,
		),
		Attention: OpenAttentionCommand{
			RunID: run.ID, Attention: rollbackAttention,
			ExpectedGeneration: 0, Question: "Conflicting question",
		},
	}
	if _, err := store.ParkRecoveryAttention(
		ctx,
		owner,
		conflict,
		now,
	); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting attention = %v", err)
	}
	budget, err = store.RecoveryBudget(ctx, run.ID, rollbackBinding)
	if err != nil || budget.Parked || budget.NextOrdinal != 1 {
		t.Fatalf("rolled-back recovery step = %#v, %v", budget, err)
	}
}

func TestRecoveryDecisionReservationsEmitClosedActionEvents(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-events", "event-cycle", "turn", "progress",
	)
	for ordinal, kind := range []RecoveryStepKind{
		RecoveryResumeWorker,
		RecoveryAskCaptain,
	} {
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(run.ID, binding, int64(ordinal+1), kind),
			now,
		); err != nil {
			t.Fatal(err)
		}
	}
	changedProgress := binding
	changedProgress.ProgressID = digest([]byte("next-progress"))
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		testRecoveryStep(
			run.ID,
			changedProgress,
			3,
			RecoveryRetryOperationally,
		),
		now,
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, event := range snapshot.Events {
		switch event.Kind {
		case RecoveryResumeWorkerEvent,
			RecoveryAskCaptainEvent,
			RecoveryRetryOperationalEvent:
			got = append(got, event.Kind)
		}
	}
	want := []string{
		RecoveryResumeWorkerEvent,
		RecoveryAskCaptainEvent,
		RecoveryRetryOperationalEvent,
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("recovery action events = %v, want %v", got, want)
	}
}

func TestRecoveryBudgetScopesAndHardLimits(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	reserve := func(
		binding RecoveryBinding,
		ordinal int64,
		kind RecoveryStepKind,
	) error {
		_, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(run.ID, binding, ordinal, kind),
			now,
		)
		return err
	}

	t.Run("correction_per_turn", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-correction", "correction-cycle", "turn-1", "progress",
		)
		// Corrections must flow far past the old two-per-turn allowance;
		// walking to the full runaway guard is a multi-minute journal
		// exercise, so depth is asserted at resilience scale instead.
		const correctionDepth = int64(25)
		for ordinal := int64(1); ordinal <= correctionDepth; ordinal++ {
			if err := reserve(
				binding, ordinal, RecoveryMalformedCorrection,
			); err != nil {
				t.Fatalf("correction %d = %v", ordinal, err)
			}
		}
		nextTurn := binding
		nextTurn.TurnID = digest([]byte("turn-2"))
		if err := reserve(
			nextTurn,
			correctionDepth+1,
			RecoveryMalformedCorrection,
		); err != nil {
			t.Fatalf("first correction in next turn: %v", err)
		}
	})

	t.Run("nudge_per_turn", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-nudge", "nudge-cycle", "turn", "progress",
		)
		const nudgeDepth = int64(25)
		for ordinal := int64(1); ordinal <= nudgeDepth; ordinal++ {
			if err := reserve(binding, ordinal, RecoveryProseNudge); err != nil {
				t.Fatalf("nudge %d = %v", ordinal, err)
			}
		}
	})

	t.Run("advisory_per_cycle", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-advisory", "advisory-cycle", "turn", "progress-0",
		)
		for ordinal := int64(1); ordinal <= MaxRecoveryAdvisoriesPerCycle; ordinal++ {
			next := binding
			next.ProgressID = digest([]byte(fmt.Sprintf("progress-%d", ordinal)))
			if err := reserve(next, ordinal, RecoveryAskCaptain); err != nil {
				t.Fatalf("advisory %d = %v", ordinal, err)
			}
		}
		over := binding
		over.ProgressID = digest([]byte("progress-over"))
		if err := reserve(
			over,
			MaxRecoveryAdvisoriesPerCycle+1,
			RecoveryAskCaptain,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("over-budget cycle advisory = %v", err)
		}
	})

	t.Run("decisions_per_progress", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-decision", "decision-cycle", "turn", "progress-1",
		)
		kinds := []RecoveryStepKind{
			RecoveryResumeWorker,
			RecoveryRetryOperationally,
		}
		for ordinal := int64(1); ordinal <= MaxRecoveryDecisionsPerProgress; ordinal++ {
			if err := reserve(
				binding, ordinal, kinds[int(ordinal)%len(kinds)],
			); err != nil {
				t.Fatalf("decision %d = %v", ordinal, err)
			}
		}
		if err := reserve(
			binding,
			MaxRecoveryDecisionsPerProgress+1,
			RecoveryResumeWorker,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("over-budget same-progress decision = %v", err)
		}
		nextProgress := binding
		nextProgress.ProgressID = digest([]byte("progress-2"))
		if err := reserve(
			nextProgress,
			MaxRecoveryDecisionsPerProgress+1,
			RecoveryResumeWorker,
		); err != nil {
			t.Fatalf("first next-progress decision: %v", err)
		}
	})

	t.Run("automatic_per_cycle", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-total", "total-cycle", "turn-0", "progress-0",
		)
		// The per-cycle guard sits at runaway scale; assert mixed automatic
		// actions keep flowing well past the old four-per-cycle allowance.
		const automaticDepth = int64(25)
		kinds := []RecoveryStepKind{
			RecoveryMalformedCorrection,
			RecoveryProseNudge,
		}
		for ordinal := int64(1); ordinal <= automaticDepth; ordinal++ {
			step := binding
			step.TurnID = digest([]byte(fmt.Sprintf("turn-%d", ordinal)))
			if err := reserve(
				step, ordinal, kinds[int(ordinal)%len(kinds)],
			); err != nil {
				t.Fatalf("automatic action %d = %v", ordinal, err)
			}
		}
	})
}

func TestRecoveryStepReplayRestartAndTerminalParkAreDurable(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-restart", "restart-cycle", "turn", "progress",
	)
	first := testRecoveryStep(
		run.ID,
		binding,
		1,
		RecoveryMalformedCorrection,
	)
	reserved, err := store.ReserveRecoveryStep(ctx, owner, first, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var durableCommand, durableEffect, durableEvent bool
	for _, command := range snapshot.Commands {
		durableCommand = durableCommand ||
			command.Kind == "turn_recovery.step"
	}
	for _, effect := range snapshot.Effects {
		durableEffect = durableEffect ||
			(effect.Kind == "runtime.turn_recovery" &&
				effect.State == Succeeded)
	}
	for _, event := range snapshot.Events {
		durableEvent = durableEvent ||
			event.Kind == RecoveryStepReservedEvent
	}
	if !durableCommand || !durableEffect || !durableEvent {
		t.Fatalf(
			"reservation transaction = command:%t effect:%t event:%t",
			durableCommand,
			durableEffect,
			durableEvent,
		)
	}
	replayed, err := store.ReserveRecoveryStep(
		ctx,
		OwnerLease{},
		first,
		now.Add(time.Hour),
	)
	if err != nil || !reflect.DeepEqual(replayed, reserved) {
		t.Fatalf("owner-free step replay = %#v, %v", replayed, err)
	}
	conflict := first
	conflict.Kind = RecoveryProseNudge
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		conflict,
		now,
	); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("same ordinal conflict = %v", err)
	}
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		testRecoveryStep(
			run.ID,
			binding,
			3,
			RecoveryResumeWorker,
		),
		now,
	); !IsCode(err, "STALE_RECOVERY_ORDINAL") {
		t.Fatalf("skipped ordinal = %v", err)
	}

	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	afterRestart, err := store.RecoveryBudget(ctx, run.ID, binding)
	if err != nil ||
		afterRestart.AutomaticActions != 1 ||
		afterRestart.Corrections != 1 ||
		afterRestart.NextOrdinal != 2 {
		t.Fatalf("budget after restart = %#v, %v", afterRestart, err)
	}
	takeoverAt := owner.ExpiresAt.Add(time.Nanosecond)
	owner, err = store.AcquireOwner(
		ctx,
		run.ID,
		takeoverAt,
		time.Minute,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	park := testRecoveryStep(
		run.ID,
		binding,
		2,
		RecoveryParkTrack,
	)
	parkInput, parkOutput := int64(31), int64(37)
	park.Accounting = &RecoveryAccounting{
		DurationMillis: 23,
		TokenStatus:    "reported",
		InputTokens:    &parkInput,
		OutputTokens:   &parkOutput,
		CostStatus:     "unavailable",
	}
	parked, err := store.ReserveRecoveryStep(ctx, owner, park, takeoverAt)
	if err != nil || !parked.Parked {
		t.Fatalf("park = %#v, %v", parked, err)
	}
	replayedPark, err := store.ReserveRecoveryStep(
		ctx,
		OwnerLease{},
		park,
		takeoverAt.Add(time.Hour),
	)
	if err != nil || !reflect.DeepEqual(replayedPark, parked) {
		t.Fatalf("park replay = %#v, %v", replayedPark, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		testRecoveryStep(
			run.ID,
			binding,
			3,
			RecoveryResumeWorker,
		),
		takeoverAt,
	); !IsCode(err, "RECOVERY_PARKED") {
		t.Fatalf("automatic action after park = %v", err)
	}
	projection, err := store.RecoveryBudget(ctx, run.ID, binding)
	if err != nil || !projection.Parked || projection.NextOrdinal != 3 ||
		!reflect.DeepEqual(projection.Accounting, park.Accounting) {
		t.Fatalf("parked projection = %#v, %v", projection, err)
	}
	control, err := store.ControlProjection(ctx, run.ID)
	if err != nil || len(control.RetryEpochs) != 0 {
		t.Fatalf("recovery changed operator retry epochs = %#v, %v", control, err)
	}

	snapshot, err = store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var reservedEvents, parkedEvents int
	for _, event := range snapshot.Events {
		switch event.Kind {
		case "turn_recovery_step_reserved":
			reservedEvents++
		case RecoveryParkedEvent:
			parkedEvents++
		}
	}
	if reservedEvents != 1 || parkedEvents != 1 {
		t.Fatalf(
			"recovery events = reserved:%d parked:%d",
			reservedEvents,
			parkedEvents,
		)
	}
}

func TestAnsweredAttentionResumesExactlyOneParkedRecoveryLane(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-human", "human-cycle", "turn", "progress",
	)
	park := testRecoveryStep(run.ID, binding, 1, RecoveryParkTrack)
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		park,
		now,
	); err != nil {
		t.Fatal(err)
	}
	attention := testAttentionBinding(binding, 1)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 0, Question: "Which authority should continue?",
	}, now); err != nil {
		t.Fatal(err)
	}
	resume := testRecoveryStep(
		run.ID,
		binding,
		2,
		RecoveryResumeWorker,
	)
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		resume,
		now,
	); !IsCode(err, "RECOVERY_PARKED") {
		t.Fatalf("unanswered attention resumed = %v", err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 1, Answer: "Continue from the pinned base.",
	}, now); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		resume,
		now,
	)
	if err != nil || receipt.Parked {
		t.Fatalf("answered attention resume = %#v, %v", receipt, err)
	}
	projection, err := store.RecoveryBudget(ctx, run.ID, binding)
	if err != nil || projection.Parked || projection.NextOrdinal != 3 {
		t.Fatalf("resumed budget = %#v, %v", projection, err)
	}
}

func TestHumanWakeDoesNotReplenishOrConsumeAutomaticBudget(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	binding := testRecoveryBinding(
		"T-human-bound", "human-bound-cycle", "turn", "progress",
	)
	for index, kind := range []RecoveryStepKind{
		RecoveryMalformedCorrection,
		RecoveryMalformedCorrection,
		RecoveryProseNudge,
		RecoveryResumeWorker,
	} {
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(run.ID, binding, int64(index+1), kind),
			now,
		); err != nil {
			t.Fatal(err)
		}
	}
	park := testRecoveryStep(run.ID, binding, 5, RecoveryParkTrack)
	if _, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		park,
		now,
	); err != nil {
		t.Fatal(err)
	}
	attention := testAttentionBinding(binding, 5)
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 0, Question: "Human input required.",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID: run.ID, Attention: attention,
		ExpectedGeneration: 1, Answer: "Continue once.",
	}, now); err != nil {
		t.Fatal(err)
	}
	resume := testRecoveryStep(
		run.ID,
		binding,
		6,
		RecoveryResumeWorker,
	)
	receipt, err := store.ReserveRecoveryStep(
		ctx,
		owner,
		resume,
		now,
	)
	// Four automatic steps were reserved before the park; the park and the
	// post-answer resume must neither consume nor replenish that count.
	const reservedAutomatic = int64(4)
	if err != nil ||
		receipt.AutomaticActions != reservedAutomatic ||
		receipt.SameProgress != 1 ||
		receipt.Parked {
		t.Fatalf("human wake receipt = %#v, %v", receipt, err)
	}
	projection, err := store.RecoveryBudget(ctx, run.ID, binding)
	if err != nil ||
		projection.AutomaticActions != reservedAutomatic ||
		projection.SameProgress != 1 ||
		projection.Parked {
		t.Fatalf("human wake replay = %#v, %v", projection, err)
	}
}
