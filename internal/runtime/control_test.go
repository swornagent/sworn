package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
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
	_, ctrlErr := service.Control(ctx, command)
	t.Logf("ctrlErr = %#v, owner = %#v", ctrlErr, owner)
	if !IsCode(ctrlErr, "OWNER_TRANSITION_PENDING") {
		t.Fatalf("active-owner resume = %v", ctrlErr)
	}
	expiry, ok := OwnerLeaseExpiry(ctrlErr)
	if !ok || !expiry.Equal(owner.ExpiresAt) {
		t.Fatalf("OwnerLeaseExpiry = %v, %t; want %v", expiry, ok, owner.ExpiresAt)
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

func TestTakeoverReportsOwnerTransitionWithExpiryUntilOwnerExpires(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "takeover-trans.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manifest, manifestBody, _ := fixtureManifest(t)
	run := journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
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
		RunID: run.ID, ID: "takeover-1", Kind: journal.Takeover, ExpectedGeneration: 0,
	}
	_, ctrlErr := service.Control(ctx, command)
	if !IsCode(ctrlErr, "OWNER_TRANSITION_PENDING") {
		t.Fatalf("active-owner takeover = %v", ctrlErr)
	}
	expiry, ok := OwnerLeaseExpiry(ctrlErr)
	if !ok || !expiry.Equal(owner.ExpiresAt) {
		t.Fatalf("OwnerLeaseExpiry = %v, %t; want %v", expiry, ok, owner.ExpiresAt)
	}

	// Advance time past owner lease expiry and verify exact command replay acquires the owner cleanly.
	expiredAt := now.Add(31 * time.Second)
	expiredService := &Service{journal: store, now: func() time.Time { return expiredAt }}
	status, err := expiredService.Control(ctx, command)
	if err != nil {
		t.Fatalf("expired-owner takeover = %v", err)
	}
	if status.DesiredState != "running" {
		t.Fatalf("status.DesiredState = %q, want running", status.DesiredState)
	}
}

func TestExternalStatusServiceObservingActiveOwnerRunReturnsRunning(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	repoPath := productionRepository(t)
	manifest, _, _ := fixtureManifest(t)
	manifest.Repository = repoPath
	manifestBody, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(t.TempDir(), "status-active.sqlite")
	store, err := journal.Open(ctx, storePath)
	if err != nil {
		t.Fatal(err)
	}
	run := journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
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
	if _, err := store.AcquireOwner(ctx, run.ID, now, 30*time.Second, false); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	readStore, err := journal.OpenReadOnly(ctx, storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer readStore.Close()
	statusService := &Service{journal: readStore, gitExecutable: gitExecutable, now: func() time.Time { return now }}
	status, err := statusService.Status(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "running" {
		t.Fatalf("status.State = %q, want running", status.State)
	}
}

func TestStatusDerivesTakeoverRequiredWhenOwnerLeaseExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	repoPath := productionRepository(t)
	manifest, _, _ := fixtureManifest(t)
	manifest.Repository = repoPath
	manifestBody, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "status-expired.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
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
	if _, err := store.AcquireOwner(ctx, run.ID, now, time.Second, false); err != nil {
		t.Fatal(err)
	}

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	statusService := &Service{journal: store, gitExecutable: gitExecutable, now: func() time.Time { return now.Add(2 * time.Second) }}
	status, err := statusService.Status(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "takeover_required" {
		t.Fatalf("status.State = %q, want takeover_required", status.State)
	}
}

func TestStatusDerivesRunningForAnsweredWithoutOwner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	repoPath := productionRepository(t)
	manifest, _, _ := fixtureManifest(t)
	manifest.Repository = repoPath
	manifestBody, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "status-no-owner.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run := journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
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

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	statusService := &Service{journal: store, gitExecutable: gitExecutable, now: func() time.Time { return now }}
	status, err := statusService.Status(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "running" {
		t.Fatalf("status.State = %q, want running", status.State)
	}
}

func TestStatusPreservesCompleteForCompletedRunsWhenOwnerExpired(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err = baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest
	manifest.Scripts = []ScriptedAttempt{
		{Responsibility: driver.AssemblyVerification, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.CaptainReview, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerDesign, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Responsibility: driver.PlannerProposal, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.WorkVerification, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
	}
	submission := func(
		slice string,
		responsibility driver.Responsibility,
		batonAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			BatonAttempt: batonAttempt, Epoch: 1, Try: 1}
		return driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID(manifest.RunID, script),
			Responsibility: responsibility,
			Summary:        "Exact " + string(responsibility) + ".",
			Detail:         "Bounded fixture detail.",
		}
	}
	planner := submission("", driver.PlannerProposal, 1)
	planner.Plan, _ = driver.NewPlanBytes(planBytes)
	design := submission("S1", driver.ImplementerDesign, 1)
	captain := submission("S1", driver.CaptainReview, 1)
	captain.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest.Scripts[0].Submission = encodeSubmission(t, assembly)
	manifest.Scripts[1].Submission = encodeSubmission(t, captain)
	manifest.Scripts[2].Submission = encodeSubmission(t, design)
	manifest.Scripts[3].Submission = encodeSubmission(t, implementation)
	manifest.Scripts[4].Submission = encodeSubmission(t, planner)
	manifest.Scripts[5].Submission = encodeSubmission(t, work)
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repository, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	// Pre-install the approved plan at commit P.
	targetP := runRuntimeGit(t, repository, "rev-parse", "refs/heads/main")
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := baton.NewActions(baton.UseGitRepository(repoView), inertness, manifest.GitIdentity)
	if err != nil {
		t.Fatal(err)
	}
	installer := newAuthorityInstaller(actions)
	admission := approvalAdmission{
		planBytes:  plan.Bytes(),
		planDigest: plan.Digest(),
		reference:  plan.Metadata().ApprovalRef,
	}
	if _, err := installer.install(admission, targetP); err != nil {
		t.Fatal(err)
	}

	submissions := make(map[string][]byte, len(manifest.Scripts))
	for _, script := range manifest.Scripts {
		encoded, decodeErr := base64.StdEncoding.DecodeString(script.Submission)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		submissions[invocationID(manifest.RunID, script)] = encoded
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		if invocation.Request.Role == driver.RoleImplementer &&
			invocation.Request.Workspace.Access == driver.ReadWrite {
			if err := os.WriteFile(
				filepath.Join(invocation.HostWorkspace, "one.txt"),
				[]byte("implemented one\n"),
				0o600,
			); err != nil {
				return driver.Observation{}, err
			}
		}
		sub := submissions[invocation.Request.InvocationID]
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  sub,
				SubmissionDigest: driver.Digest(sub),
			},
		}, nil
	})

	path := filepath.Join(t.TempDir(), "complete-expired.sqlite")
	store, err := journal.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	status, err := service.Start(ctx, body)
	if err != nil || status.State != "complete" {
		t.Fatalf("start error = %v, state = %s", err, status.State)
	}

	// Acquire owner with 1s lease, then query status 2s later (expired owner).
	if _, err := store.AcquireOwner(ctx, manifest.RunID, now, time.Second, false); err != nil {
		t.Fatal(err)
	}

	statusService := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now.Add(2 * time.Second) },
	}
	expiredStatus, err := statusService.Status(ctx, manifest.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if expiredStatus.State != "complete" {
		t.Fatalf("expiredStatus.State = %q, want complete", expiredStatus.State)
	}
}

func TestStatusPreservesAwaitingApprovalForProposedPlansWhenOwnerExpired(t *testing.T) {
	ctx := context.Background()
	fixture := newApprovalRecoveryFixture(t)
	store, err := journal.Open(ctx, fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Acquire owner with 1s lease
	if _, err := store.AcquireOwner(ctx, fixture.runID, fixture.now, time.Second, false); err != nil {
		t.Fatal(err)
	}

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	statusService := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return fixture.now.Add(2 * time.Second) },
	}
	status, err := statusService.Status(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "awaiting_approval" {
		t.Fatalf("status.State = %q, want awaiting_approval", status.State)
	}
}

func TestSecondProcessAnswerReturnsRunningWhileResidentDriverHoldsOwner(t *testing.T) {
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

	// Resident driver continues holding an unexpired owner lease (fixture.owner).
	// A second process (service2) opens the store and answers the attention.
	store2, err := journal.Open(fixture.ctx, fixture.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	service2 := &Service{
		journal:       store2,
		gitExecutable: fixture.service.gitExecutable,
		now:           func() time.Time { return fixture.now },
	}

	status, err := service2.AnswerAttention(fixture.ctx, AnswerAttentionCommand{
		RunID:              fixture.owner.RunID,
		AttentionID:        attention.Attention.ID,
		ExpectedGeneration: 1,
		Answer:             "Use the exact approved fixture value.",
	})
	if err != nil {
		t.Fatalf("second process AnswerAttention error = %v", err)
	}
	if status.State != "running" {
		t.Fatalf("second process AnswerAttention status.State = %q, want running", status.State)
	}
}

func TestDriveOwnedReleasesExpiredCurrentOwnerOnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "drive-owned-expired-release.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	manifest, manifestBody, _ := fixtureManifest(t)
	run := journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: sha256Digest(manifestBody),
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
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

	leaseDuration := 500 * time.Millisecond
	owner, err := store.AcquireOwner(ctx, run.ID, now, leaseDuration, false)
	if err != nil {
		t.Fatal(err)
	}

	// Cancel context to force driveOwnedCycle to return an error (simulating cancelled background drive).
	cancel()

	// Advance time past owner lease expiry so driveOwned's error release branch exercises an expired-but-current lease.
	currentTime := owner.ExpiresAt.Add(time.Second)
	service := &Service{
		journal: store,
		now:     func() time.Time { return currentTime },
	}

	_, driveErr := service.driveOwned(ctx, run.ID, owner)
	if driveErr == nil {
		t.Fatal("driveOwned expected error from cancelled context, got nil")
	}
	// The drive error must reflect the cancellation, not OWNER_FENCED from failed release.
	if IsCode(driveErr, "OWNER_FENCED") {
		t.Fatalf("driveOwned returned OWNER_FENCED from release: %v", driveErr)
	}

	// Verify owner is no longer claimed (cleanly released).
	currentOwner, present, err := store.CurrentOwner(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatalf("owner remained claimed after driveOwned error release: %#v", currentOwner)
	}

	// Verify subsequent ordinary acquire succeeds without takeover required.
	nextOwner, err := store.AcquireOwner(context.Background(), run.ID, currentTime.Add(time.Second), time.Minute, false)
	if err != nil {
		t.Fatalf("ordinary AcquireOwner after expired drive release failed: %v", err)
	}
	if nextOwner.Generation != 2 {
		t.Fatalf("nextOwner.Generation = %d, want 2", nextOwner.Generation)
	}
}

// A4: an oversize answer surfaces as ATTENTION_REJECTED whose cause chain
// names ATTENTION_ANSWER_OVERSIZE with the bound, so the sworn#207
// unlabelled-refusal tail is closed on the service surface.
func TestAnswerAttentionOversizeRefusalCarriesItsCause(t *testing.T) {
	fixtureDriver := &turnRecoveryFixtureDriver{
		parkS1:    true,
		yieldKind: driver.YieldQuestion,
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
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen {
		t.Fatalf("attentions = %#v, %v", attentions, err)
	}
	attention := attentions[0]
	_, err = fixture.service.AnswerAttention(fixture.ctx, AnswerAttentionCommand{
		RunID:              fixture.owner.RunID,
		AttentionID:        attention.Attention.ID,
		ExpectedGeneration: 1,
		Answer:             strings.Repeat("x", journal.MaxAttentionAnswerBytes+1),
	})
	if !IsCode(err, "ATTENTION_REJECTED") {
		t.Fatalf("oversize answer = %v, want ATTENTION_REJECTED", err)
	}
	var journalErr *journal.Error
	if !errors.As(err, &journalErr) ||
		!journal.IsCode(journalErr, "ATTENTION_ANSWER_OVERSIZE") {
		t.Fatalf("oversize cause chain = %v", err)
	}
	if !strings.Contains(err.Error(), "16384") {
		t.Fatalf("oversize detail names the bound: %v", err)
	}
}
