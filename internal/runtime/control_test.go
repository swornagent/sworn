package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func TestResumeReportsOwnerTransitionUntilExactReplayCanAcquire(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "run.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, manifestBody, _ := fixtureManifest(t)
	run := journal.Run{
		ID:             "run-1",
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     "/repository",
		Release:        "release-1",
		TargetRef:      "refs/heads/main",
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: run.ID, ReplayKey: "manifest", Kind: "start",
		Payload: manifestBody, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, run.ID, now, 30*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store, now: func() time.Time { return now }}
	command := journal.ControlCommand{
		RunID: run.ID, ID: "resume-1", Kind: journal.Resume, ExpectedGeneration: 0,
	}
	if _, err := service.Control(ctx, command); !IsCode(err, "OWNER_TRANSITION_PENDING") {
		t.Fatalf("active-owner resume = %v", err)
	}
	projection, err := store.ControlProjection(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Generation != 1 || projection.Desired != "running" {
		t.Fatalf("durable resume projection = %#v", projection)
	}
	if err := store.ReleaseOwner(ctx, owner, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, command, now.Add(2*time.Second)); err != nil {
		t.Fatalf("exact resume replay = %v", err)
	}
	if _, err := store.AcquireOwner(ctx, run.ID, now.Add(2*time.Second), 30*time.Second, false); err != nil {
		t.Fatalf("owner after exact replay = %v", err)
	}
}

func TestExactTakeoverReplayCanAcquireReleasedOwner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 2, 3, 4, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "run.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := journal.Run{
		ID:             "takeover-replay",
		ManifestDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Repository:     "/repository",
		Release:        "release-1",
		TargetRef:      "refs/heads/main",
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireOwner(
		ctx,
		run.ID,
		now,
		time.Second,
		false,
	); err != nil {
		t.Fatal(err)
	}
	command := journal.ControlCommand{
		RunID: run.ID,
		ID:    "takeover-1",
		Kind:  journal.Takeover,
	}
	takeoverAt := now.Add(2 * time.Second)
	if _, err := store.ApplyControl(ctx, command, takeoverAt); err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store}
	second, err := service.acquireControlOwner(ctx, command, takeoverAt)
	if err != nil || second.Generation != 2 {
		t.Fatalf("first takeover acquisition = %#v, %v", second, err)
	}
	if err := store.ReleaseOwner(ctx, second, takeoverAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	replayAt := takeoverAt.Add(2 * time.Second)
	if _, err := store.ApplyControl(ctx, command, replayAt); err != nil {
		t.Fatalf("exact takeover replay = %v", err)
	}
	replayed, err := service.acquireControlOwner(ctx, command, replayAt)
	if err != nil {
		t.Fatalf("replayed takeover acquisition = %v", err)
	}
	if replayed.Generation != 3 {
		t.Fatalf("replayed owner generation = %d, want 3", replayed.Generation)
	}
}

func TestAnswerAttentionReturnsSuccessAndContinuesDrivingInBackground(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:         true,
		yieldKind:      driver.YieldQuestion,
		expectedAnswer: "Use the exact approved fixture value.",
	}
	fixture := newProductionImplementationRecoveryFixture(t, fixtureDriver)
	defer fixture.service.Close()

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

	// Drive the slice until it parks on the human turn.
	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("human park = %v", err)
	}

	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}

	attentions, err := fixture.store.Attentions(fixture.ctx, fixture.owner.RunID)
	if err != nil || len(attentions) != 1 || attentions[0].State != journal.AttentionOpen {
		t.Fatalf("attentions = %#v, %v", attentions, err)
	}
	attention := attentions[0]

	// Release owner so AnswerAttention will acquire owner and start background drive.
	if err := fixture.store.ReleaseOwner(fixture.ctx, fixture.owner, fixture.now); err != nil {
		t.Fatal(err)
	}

	// AnswerAttention returns RunStatus immediately.
	status, err := fixture.service.AnswerAttention(fixture.ctx, AnswerAttentionCommand{
		RunID:              fixture.owner.RunID,
		AttentionID:        attention.Attention.ID,
		ExpectedGeneration: 1,
		Answer:             "Use the exact approved fixture value.",
	})
	if err != nil {
		t.Fatalf("AnswerAttention error = %v", err)
	}
	if status.RunID != fixture.owner.RunID {
		t.Fatalf("status.RunID = %q, want %q", status.RunID, fixture.owner.RunID)
	}
	if status.DesiredState != "running" {
		t.Fatalf("status.DesiredState = %q, want running", status.DesiredState)
	}

	// Wait for the background drive to reach its next effect.
	finalStatus, waitErr := fixture.service.Wait(fixture.ctx, fixture.owner.RunID)
	if waitErr != nil {
		t.Fatalf("Wait error = %v", waitErr)
	}
	if finalStatus.RunID != fixture.owner.RunID {
		t.Fatalf("finalStatus.RunID = %q", finalStatus.RunID)
	}

	fixtureDriver.mu.Lock()
	answerCalls := fixtureDriver.answerCalls
	fixtureDriver.mu.Unlock()
	if answerCalls != 1 {
		t.Fatalf("answerCalls = %d, want 1", answerCalls)
	}

	// Verify Baton state reached pass/merge stage.
	state, err := baton.ReadState(fixture.engine.git, fixture.manifest.value.Release, fixture.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	if !ok || slice.Pass == nil || slice.Outcome != "pass" {
		t.Fatalf("slice after background drive = %#v", slice)
	}
}

func TestAnswerAttentionCallerCloseLeavesNoClaimedUnexpiredLease(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:         true,
		yieldKind:      driver.YieldQuestion,
		expectedAnswer: "Use the exact approved fixture value.",
	}
	fixture := newProductionImplementationRecoveryFixture(t, fixtureDriver)
	defer fixture.workspace.Close()

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

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("human park = %v", err)
	}

	attentions, err := fixture.store.Attentions(fixture.ctx, fixture.owner.RunID)
	if err != nil || len(attentions) != 1 || attentions[0].State != journal.AttentionOpen {
		t.Fatalf("attentions = %#v, %v", attentions, err)
	}
	attention := attentions[0]

	if err := fixture.store.ReleaseOwner(fixture.ctx, fixture.owner, fixture.now); err != nil {
		t.Fatal(err)
	}

	status, err := fixture.service.AnswerAttention(fixture.ctx, AnswerAttentionCommand{
		RunID:              fixture.owner.RunID,
		AttentionID:        attention.Attention.ID,
		ExpectedGeneration: 1,
		Answer:             "Use the exact approved fixture value.",
	})
	if err != nil {
		t.Fatalf("AnswerAttention error = %v", err)
	}
	if status.RunID != fixture.owner.RunID {
		t.Fatalf("status.RunID = %q", status.RunID)
	}

	// Immediately close the caller service.
	if err := fixture.service.Close(); err != nil {
		t.Fatalf("service.Close() error = %v", err)
	}

	// Probe journal: verify no claimed unexpired owner lease remains.
	newStore, err := journal.Open(fixture.ctx, fixture.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer newStore.Close()

	owner, present, err := newStore.CurrentOwner(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatalf("owner lease remained after service.Close(): %#v", owner)
	}

	// A new caller can immediately acquire ownership without being blocked.
	newOwner, err := newStore.AcquireOwner(fixture.ctx, fixture.owner.RunID, fixture.now, time.Minute, false)
	if err != nil {
		t.Fatalf("AcquireOwner on cleanly closed run failed: %v", err)
	}
	if newOwner.RunID != fixture.owner.RunID {
		t.Fatalf("newOwner.RunID = %q", newOwner.RunID)
	}
}

func TestResumeAndTakeoverReturnPromptlyNamingLiveState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 4, 5, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "control-live.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	manifest1, manifestBody1, _ := fixtureManifest(t)
	run := journal.Run{
		ID:             manifest1.RunID,
		ManifestDigest: sha256Digest(manifestBody1),
		Repository:     manifest1.Repository,
		Release:        manifest1.Release,
		TargetRef:      manifest1.TargetRef,
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: run.ID, ReplayKey: "manifest", Kind: "start",
		Payload: manifestBody1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	service := &Service{journal: store, now: func() time.Time { return now }}
	defer service.Close()

	// Pause the run first.
	pauseCmd := journal.ControlCommand{
		RunID: run.ID, ID: "pause-1", Kind: journal.Pause, ExpectedGeneration: 0,
	}
	if _, err := service.Control(ctx, pauseCmd); err != nil {
		t.Fatalf("pause control = %v", err)
	}

	// Resume: returns RunStatus promptly with DesiredState: running and live state.
	resumeCmd := journal.ControlCommand{
		RunID: run.ID, ID: "resume-1", Kind: journal.Resume, ExpectedGeneration: 1,
	}
	status, err := service.Control(ctx, resumeCmd)
	if err != nil {
		t.Fatalf("resume control = %v", err)
	}
	if status.RunID != run.ID {
		t.Fatalf("status.RunID = %q, want %q", status.RunID, run.ID)
	}
	if status.DesiredState != "running" {
		t.Fatalf("status.DesiredState = %q, want running", status.DesiredState)
	}
	if status.ControlGeneration != 2 {
		t.Fatalf("status.ControlGeneration = %d, want 2", status.ControlGeneration)
	}

	// Takeover: simulate an expired owner on another run in a separate store.
	takeoverStore, err := journal.Open(ctx, filepath.Join(t.TempDir(), "takeover-live.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer takeoverStore.Close()

	takeoverManifest, takeoverBody, _ := fixtureManifest(t)
	takeoverRun := journal.Run{
		ID:             takeoverManifest.RunID,
		ManifestDigest: sha256Digest(takeoverBody),
		Repository:     takeoverManifest.Repository,
		Release:        takeoverManifest.Release,
		TargetRef:      takeoverManifest.TargetRef,
		CreatedAt:      now,
	}
	if err := takeoverStore.RegisterRun(ctx, takeoverRun); err != nil {
		t.Fatal(err)
	}
	if err := takeoverStore.RecordCommand(ctx, journal.Command{
		RunID: takeoverRun.ID, ReplayKey: "manifest", Kind: "start",
		Payload: takeoverBody, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	// Expired owner (lease 1 second, now+2 seconds)
	if _, err := takeoverStore.AcquireOwner(ctx, takeoverRun.ID, now, time.Second, false); err != nil {
		t.Fatal(err)
	}

	takeoverCmd := journal.ControlCommand{
		RunID: takeoverRun.ID, ID: "takeover-1", Kind: journal.Takeover, ExpectedGeneration: 0,
	}
	takeoverService := &Service{journal: takeoverStore, now: func() time.Time { return now.Add(2 * time.Second) }}
	defer takeoverService.Close()

	takeoverStatus, err := takeoverService.Control(ctx, takeoverCmd)
	if err != nil {
		t.Fatalf("takeover control = %v", err)
	}
	if takeoverStatus.RunID != takeoverRun.ID {
		t.Fatalf("takeoverStatus.RunID = %q", takeoverStatus.RunID)
	}
	if takeoverStatus.DesiredState != "running" {
		t.Fatalf("takeoverStatus.DesiredState = %q, want running", takeoverStatus.DesiredState)
	}
	if takeoverStatus.ControlGeneration != 1 {
		t.Fatalf("takeoverStatus.ControlGeneration = %d, want 1", takeoverStatus.ControlGeneration)
	}
}
