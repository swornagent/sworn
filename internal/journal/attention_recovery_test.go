package journal

import (
	"context"
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

func TestRecoveryBudgetScopesAndHardLimits(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("correction_per_turn", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-correction", "correction-cycle", "turn-1", "progress",
		)
		for ordinal := int64(1); ordinal <= 2; ordinal++ {
			if _, err := store.ReserveRecoveryStep(
				ctx,
				owner,
				testRecoveryStep(
					run.ID,
					binding,
					ordinal,
					RecoveryMalformedCorrection,
				),
				now,
			); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				binding,
				3,
				RecoveryMalformedCorrection,
			),
			now,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("third same-turn correction = %v", err)
		}
		nextTurn := binding
		nextTurn.TurnID = digest([]byte("turn-2"))
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				nextTurn,
				3,
				RecoveryMalformedCorrection,
			),
			now,
		); err != nil {
			t.Fatalf("first correction in next turn: %v", err)
		}
	})

	t.Run("nudge_per_turn", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-nudge", "nudge-cycle", "turn", "progress",
		)
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				binding,
				1,
				RecoveryProseNudge,
			),
			now,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				binding,
				2,
				RecoveryProseNudge,
			),
			now,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("second same-turn nudge = %v", err)
		}
	})

	t.Run("advisory_per_cycle", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-advisory", "advisory-cycle", "turn", "progress-1",
		)
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				binding,
				1,
				RecoveryAskCaptain,
			),
			now,
		); err != nil {
			t.Fatal(err)
		}
		nextProgress := binding
		nextProgress.ProgressID = digest([]byte("progress-2"))
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				nextProgress,
				2,
				RecoveryAskCaptain,
			),
			now,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("second cycle advisory = %v", err)
		}
	})

	t.Run("decisions_per_progress", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-decision", "decision-cycle", "turn", "progress-1",
		)
		for ordinal, kind := range []RecoveryStepKind{
			RecoveryResumeWorker,
			RecoveryRetryOperationally,
		} {
			if _, err := store.ReserveRecoveryStep(
				ctx,
				owner,
				testRecoveryStep(
					run.ID,
					binding,
					int64(ordinal+1),
					kind,
				),
				now,
			); err != nil {
				t.Fatal(err)
			}
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
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("third same-progress decision = %v", err)
		}
		nextProgress := binding
		nextProgress.ProgressID = digest([]byte("progress-2"))
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				nextProgress,
				3,
				RecoveryResumeWorker,
			),
			now,
		); err != nil {
			t.Fatalf("first next-progress decision: %v", err)
		}
	})

	t.Run("automatic_per_cycle", func(t *testing.T) {
		binding := testRecoveryBinding(
			"T-total", "total-cycle", "turn-1", "progress-1",
		)
		kinds := []RecoveryStepKind{
			RecoveryMalformedCorrection,
			RecoveryMalformedCorrection,
			RecoveryProseNudge,
			RecoveryResumeWorker,
		}
		for index, kind := range kinds {
			if _, err := store.ReserveRecoveryStep(
				ctx,
				owner,
				testRecoveryStep(
					run.ID,
					binding,
					int64(index+1),
					kind,
				),
				now,
			); err != nil {
				t.Fatal(err)
			}
		}
		changed := binding
		changed.TurnID = digest([]byte("turn-2"))
		changed.ProgressID = digest([]byte("progress-2"))
		if _, err := store.ReserveRecoveryStep(
			ctx,
			owner,
			testRecoveryStep(
				run.ID,
				changed,
				5,
				RecoveryResumeWorker,
			),
			now,
		); !IsCode(err, "RECOVERY_BUDGET_EXHAUSTED") {
			t.Fatalf("fifth automatic action = %v", err)
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
	if err != nil || replayed != reserved {
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
	if err != nil || replayedPark != parked {
		t.Fatalf("park replay = %#v, %v", replayedPark, err)
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
	if err != nil || !projection.Parked || projection.NextOrdinal != 3 {
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
		case "turn_recovery_parked":
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
