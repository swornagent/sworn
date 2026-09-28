package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
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

	// Verify Protocol state reached pass/merge stage.
	state, err := protocol.ReadState(fixture.engine.git, fixture.manifest.value.Release, fixture.engine.inertness)
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
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest
	manifest.Scripts = []ScriptedAttempt{
		{Responsibility: driver.AssemblyVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.LeadReview, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerDesign, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Responsibility: driver.PlannerProposal, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.WorkVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
	}
	submission := func(
		slice string,
		responsibility driver.Responsibility,
		protocolAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			ProtocolAttempt: protocolAttempt, Epoch: 1, Try: 1}
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
	lead := submission("S1", driver.LeadReview, 1)
	lead.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest.Scripts[0].Submission = encodeSubmission(t, assembly)
	manifest.Scripts[1].Submission = encodeSubmission(t, lead)
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
	actions, err := protocol.NewActions(protocol.UseGitRepository(repoView), inertness, manifest.GitIdentity)
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

// TestHostCheckEnvironmentGateParksResolvesAndReparksWithDistinctOffsets is
// A4's exact required proof: the engine refuses to make progress while a
// declared check's command does not resolve on the host runner's own
// environment, naming the missing command before any dispatch; a later
// resume after the fix clears the park (Status shows no host_environment
// park); and breaking the identical command again writes a genuinely new,
// later-offset park record rather than being absorbed as an already-seen
// duplicate (the attempt-2 ordering defect).
func TestHostCheckEnvironmentGateParksResolvesAndReparksWithDistinctOffsets(t *testing.T) {
	const absentCommand = "sworn-genuinely-absent-command-xyz"
	check := absentCommand + " --version"
	fixture := newHostCheckFixture(t, []string{check})
	originalPath := os.Getenv("PATH")
	snapshot := func() journal.Snapshot {
		snap, err := fixture.store.Snapshot(fixture.ctx, fixture.owner.RunID)
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}
	parkEventOffsets := func(snap journal.Snapshot) []int64 {
		var offsets []int64
		for _, event := range snap.Events {
			if event.Kind != ParkEventKind {
				continue
			}
			parsed, err := ParseDegradationParkEvent(event.Body)
			if err != nil || parsed.Cause != ParkCauseHostEnvironment || parsed.Work != "" {
				continue
			}
			offsets = append(offsets, event.Offset)
		}
		return offsets
	}

	// Broken: the gate parks and names the missing command, before any
	// dispatch. hostChecksPlanBytes also declares a worker-only "worker
	// check" entry (never resolvable as a host command either), so the
	// detail is checked by substring, not exact equality.
	parked, err := fixture.service.driveHostCheckEnvironmentGate(
		fixture.ctx, fixture.engine, fixture.owner, snapshot(), fixture.state)
	if err != nil || !parked {
		t.Fatalf("gate() = (%v, %v), want (true, nil)", parked, err)
	}
	runParked, runDetail := hostCheckEnvironmentRunState(snapshot())
	if !runParked || !strings.Contains(runDetail, check) ||
		!strings.Contains(runDetail, absentCommand) {
		t.Fatalf("first park state = (%v, %q)", runParked, runDetail)
	}
	firstOffsets := parkEventOffsets(snapshot())
	if len(firstOffsets) != 1 {
		t.Fatalf("first park event offsets = %v, want exactly one", firstOffsets)
	}

	// A repeat gate call while still broken and unchanged must not write a
	// second, redundant event.
	if parked, err = fixture.service.driveHostCheckEnvironmentGate(
		fixture.ctx, fixture.engine, fixture.owner, snapshot(), fixture.state,
	); err != nil || !parked {
		t.Fatalf("repeat gate() = (%v, %v), want (true, nil)", parked, err)
	}
	if offsets := parkEventOffsets(snapshot()); len(offsets) != 1 {
		t.Fatalf("repeat park event offsets = %v, want still exactly one", offsets)
	}

	// Fix: put a resolvable script ahead of PATH under the exact missing
	// name (M10's own remedy), with the exact resolved shell as its own
	// interpreter (never a hardcoded /bin/sh, which this sandbox does
	// not have). hostChecksPlanBytes also declares a worker-only "worker
	// check" entry (never resolvable as a host command either, since
	// "worker" is not a real binary), so it gets an identical fake
	// script too - otherwise the gate could never reach fully resolved.
	// The gate must now clear.
	shell, shellErr := hostShell()
	if shellErr != nil {
		t.Fatal(shellErr)
	}
	fixDir := t.TempDir()
	for _, name := range []string{absentCommand, "worker"} {
		if err := os.WriteFile(
			filepath.Join(fixDir, name), []byte("#!"+shell+"\nexit 0\n"), 0o755,
		); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fixDir+string(os.PathListSeparator)+originalPath)

	if parked, err = fixture.service.driveHostCheckEnvironmentGate(
		fixture.ctx, fixture.engine, fixture.owner, snapshot(), fixture.state,
	); err != nil {
		t.Fatalf("gate() after fix error = %v", err)
	} else if parked {
		t.Fatal("gate() still parked after the fix")
	}
	if runParked, _ := hostCheckEnvironmentRunState(snapshot()); runParked {
		t.Fatal("host_environment park did not clear")
	}

	// Break again with the identical missing command: a fresh, later-offset
	// park record must be written - not absorbed as an already-recorded
	// duplicate of the first.
	t.Setenv("PATH", originalPath)
	if parked, err = fixture.service.driveHostCheckEnvironmentGate(
		fixture.ctx, fixture.engine, fixture.owner, snapshot(), fixture.state,
	); err != nil || !parked {
		t.Fatalf("repark gate() = (%v, %v), want (true, nil)", parked, err)
	}
	runParked, runDetail = hostCheckEnvironmentRunState(snapshot())
	if !runParked || !strings.Contains(runDetail, absentCommand) {
		t.Fatalf("repark state = (%v, %q)", runParked, runDetail)
	}
	secondOffsets := parkEventOffsets(snapshot())
	if len(secondOffsets) != 2 {
		t.Fatalf("repark event offsets = %v, want exactly two (not absorbed as a duplicate)", secondOffsets)
	}
	if secondOffsets[1] <= firstOffsets[0] {
		t.Fatalf("repark offset %d is not later than the first park offset %d", secondOffsets[1], firstOffsets[0])
	}
}

// TestHostEnvironmentSlicePathParksThroughStartStatusAndResume is A1/A2's
// exact required proof through the real entry points, never
// implementSlice, driveHostCheckEnvironmentGate, or any other internal
// helper: a slice-scoped host-environment crossing, created mid-run inside
// claimPreparedImplementation's own host-check step. The declared check's
// first word is a real, executable file the whole test long (a script
// with a bad shebang), so it resolves via `command -v` throughout - the
// run-start gate passes cleanly and never confuses this with the
// run-scoped refusal - and only the actual spawn's genuine exit 127
// (host_checks.go's post-run defense in depth) parks it, with no
// environment mutation needed mid-test. The run projects as parked -
// never running or uncertain - from Start() itself (captured while
// driveOwned still holds the owner lease), from a separate Status() call
// made after driveOwned has released it, and from Control(Resume) driven
// to settlement via Wait; the git.seal effect never advances past its
// first try.
func TestHostEnvironmentSlicePathParksThroughStartStatusAndResume(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	const scriptName = "sworn-slicepath-bad-interpreter"
	check := scriptName
	shell, shellErr := hostShell()
	if shellErr != nil {
		t.Fatal(shellErr)
	}
	fixDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(fixDir, scriptName),
		[]byte("#!/no/such/interpreter\nexit 0\n"), 0o755,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fixDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if failure, _ := classifyHostCheckExecution(shell, check); failure {
		t.Fatal("pre-spawn classification wrongly caught the bad-interpreter script")
	}

	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadata.Tracks[0].Slices[0].Checks = append(
		append([]string(nil), metadata.Tracks[0].Slices[0].Checks...), check,
	)
	metadata.Tracks[0].Slices[0].HostChecks = []string{check}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nHost-environment slice-path fixture.\n",
	)
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest

	submission := func(
		slice string,
		responsibility driver.Responsibility,
		protocolAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			ProtocolAttempt: protocolAttempt, Epoch: 1, Try: 1}
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
	lead := submission("S1", driver.LeadReview, 1)
	lead.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest.Scripts = []ScriptedAttempt{
		{Responsibility: driver.AssemblyVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.LeadReview, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerDesign, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Responsibility: driver.PlannerProposal, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.WorkVerification, ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
	}
	manifest.Scripts[0].Submission = encodeSubmission(t, assembly)
	manifest.Scripts[1].Submission = encodeSubmission(t, lead)
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
	targetP := runRuntimeGit(t, repository, "rev-parse", "refs/heads/main")
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := protocol.NewActions(protocol.UseGitRepository(repoView), inertness, manifest.GitIdentity)
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

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "slice-path.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 27, 5, 6, 7, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	requireParkedFacts := func(t *testing.T, label string, status RunStatus) {
		t.Helper()
		if status.State != "parked" {
			t.Fatalf("%s State = %q, want parked", label, status.State)
		}
		if status.Park == nil || status.Park.Cause != ParkCauseHostEnvironment {
			t.Fatalf("%s Park = %#v, want cause host_environment", label, status.Park)
		}
		if status.Park.FailureCode != "HOST_CHECK_ENVIRONMENT" {
			t.Fatalf("%s Park.FailureCode = %q", label, status.Park.FailureCode)
		}
		if !strings.Contains(status.Park.FailureDetail, check) {
			t.Fatalf("%s Park.FailureDetail = %q", label, status.Park.FailureDetail)
		}
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	requireParkedFacts(t, "Start()", status)
	if status.Park.Work == "" || !runtimeDigestPattern.MatchString(status.Park.Work) {
		t.Fatalf("Start() Park.Work = %q, want a work identity", status.Park.Work)
	}
	gitSealWork := status.Park.Work
	sealEffectID := journal.AttemptEffectID(gitSealWork, 1, 1)
	sealEffect, err := store.Effect(ctx, manifest.RunID, sealEffectID)
	if err != nil {
		t.Fatalf("git.seal try-1 effect lookup = %v", err)
	}
	if sealEffect.Kind != "git.seal" || sealEffect.State != journal.Claimed {
		t.Fatalf("git.seal try-1 effect = %#v", sealEffect)
	}
	for _, try := range []int64{2, 3} {
		if _, err := store.Effect(
			ctx, manifest.RunID, journal.AttemptEffectID(gitSealWork, 1, try),
		); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
			t.Fatalf("git.seal try-%d effect lookup = %v, want EFFECT_NOT_FOUND", try, err)
		}
	}

	// A4: the park event is written on the pass that parked - not a
	// later cycle or Resume. Count work-scoped host_environment park
	// events directly from the journal right after Start() returns.
	countHostEnvironmentParkEvents := func(t *testing.T) int {
		t.Helper()
		snap, err := store.Snapshot(ctx, manifest.RunID)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, event := range snap.Events {
			if event.Kind != ParkEventKind {
				continue
			}
			parsed, err := ParseDegradationParkEvent(event.Body)
			if err != nil || parsed.Cause != ParkCauseHostEnvironment || parsed.Work != gitSealWork {
				continue
			}
			count++
		}
		return count
	}
	if got := countHostEnvironmentParkEvents(t); got != 1 {
		t.Fatalf("host_environment park events after Start() = %d, want exactly 1", got)
	}

	// After Start() returns, driveOwned's ReleaseOwnerIfIdle has already
	// run (the captured Start() status came from Status() called while
	// the owner lease was still held); a separate Status() call now
	// proves the "after serve lets go" half of A1.
	afterRelease, err := service.Status(ctx, manifest.RunID)
	if err != nil {
		t.Fatalf("Status() after release error = %v", err)
	}
	requireParkedFacts(t, "Status() after release", afterRelease)
	if afterRelease.Park.Work != gitSealWork {
		t.Fatalf("Status() after release Park.Work = %q, want %q",
			afterRelease.Park.Work, gitSealWork)
	}

	// Resume, driven through Control and Wait (never driveOwned or
	// driveLoop directly): the check's first word still resolves the
	// whole test long, so the run-scoped A4 gate never fires on this
	// fresh drive pass either - Resume proves the identical lane-scoped
	// facts, including the exact same Park.Work.
	if _, err := service.Control(ctx, journal.ControlCommand{
		RunID: manifest.RunID, ID: "resume-1",
		Kind: journal.Resume, ExpectedGeneration: 0,
	}); err != nil {
		t.Fatalf("Control(Resume) error = %v", err)
	}
	resumed, err := service.Wait(ctx, manifest.RunID)
	if err != nil {
		t.Fatalf("Wait() after Resume error = %v", err)
	}
	requireParkedFacts(t, "Resume", resumed)
	if resumed.Park.Work != gitSealWork {
		t.Fatalf("Resume Park.Work = %q, want %q", resumed.Park.Work, gitSealWork)
	}
	// A4: appendParkEventOnce's content-addressing makes the identical,
	// unchanged crossing's repeat a no-op - Resume must add no further
	// host_environment park event for the same work.
	if got := countHostEnvironmentParkEvents(t); got != 1 {
		t.Fatalf("host_environment park events after Resume = %d, want still exactly 1", got)
	}
}

// TestHostEnvironmentRunStartRefusalThroughStart is A1/A2's exact required
// proof for the run-start refusal path through the real entry point: a
// plan whose declared check is unresolvable on the host from the very
// first Start() call refuses to make any progress - Start() returns the
// parked status with no error, and no driver.dispatch effect is ever
// journaled for the run.
func TestHostEnvironmentRunStartRefusalThroughStart(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	const absentCommand = "sworn-genuinely-absent-command-runstart"
	check := absentCommand + " --version"

	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadata.Tracks[0].Slices[0].Checks = append(
		append([]string(nil), metadata.Tracks[0].Slices[0].Checks...), check,
	)
	metadata.Tracks[0].Slices[0].HostChecks = []string{check}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nHost-environment run-start refusal fixture.\n",
	)
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest
	// manifest.Scripts is left as fixtureManifest built it (referencing
	// the original plan) rather than cleared: the run-start gate refuses
	// before any dispatch, so no script is ever consulted, and an empty
	// Scripts list fails manifest admission independently of this test.
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repository, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	targetP := runRuntimeGit(t, repository, "rev-parse", "refs/heads/main")
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := protocol.NewActions(protocol.UseGitRepository(repoView), inertness, manifest.GitIdentity)
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

	// The command is never made resolvable: the dispatcher fails the test
	// outright if it is ever invoked, proving no driver.dispatch effect is
	// even attempted.
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		t.Fatalf("dispatcher invoked for %s, want no dispatch before the run-start refusal", invocation.Request.InvocationID)
		return driver.Observation{}, nil
	})

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "run-start-refusal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 27, 5, 6, 7, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.State != "parked" {
		t.Fatalf("Start() State = %q, want parked", status.State)
	}
	if status.Park == nil || status.Park.Cause != ParkCauseHostEnvironment {
		t.Fatalf("Start() Park = %#v, want cause host_environment", status.Park)
	}
	if status.Park.FailureCode != "HOST_CHECK_ENVIRONMENT" {
		t.Fatalf("Start() Park.FailureCode = %q", status.Park.FailureCode)
	}
	if !strings.Contains(status.Park.FailureDetail, check) ||
		!strings.Contains(status.Park.FailureDetail, absentCommand) {
		t.Fatalf("Start() Park.FailureDetail = %q", status.Park.FailureDetail)
	}
	if status.Park.Work != "" {
		t.Fatalf("Start() Park.Work = %q, want empty (run-scoped refusal)", status.Park.Work)
	}

	snapshot, err := store.Snapshot(ctx, manifest.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" {
			t.Fatalf("driver.dispatch effect exists: %#v", effect)
		}
	}
}

// TestHostEnvironmentAssemblyPathParksThroughStartStatusAndResume is A1/A2's
// exact required proof for the assembly path through the real entry
// points: two independent slices declare a check whose first word ("test")
// is always resolvable - the run-start gate passes cleanly the whole test
// long, with no environment mutation needed - and whose body execs an
// absent absolute path only when both slices' files are present in the
// workspace: never true for either slice's own single-file candidate, only
// true for the assembled tree, which composes both. So each slice's own
// implement-stage host check passes normally, and prepareAssembly's own
// fresh check.host effect (the reuse rule cannot apply: the composed tree
// differs from either slice's own tree) hits a genuine exit 127 - the
// post-run defense in depth, not the pre-spawn classification. The run
// projects as parked - never running or uncertain - from Start() (owner
// still held), from a separate Status() (owner released), and from
// Control(Resume) driven to settlement via Wait; the protocol.
// prepare_assembly effect never advances past its first try.
func TestHostEnvironmentAssemblyPathParksThroughStartStatusAndResume(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	check := "test ! -f one.txt || test ! -f two.txt || exec /no/such/absolute/path-sworn-assembly-xyz"
	shell, shellErr := hostShell()
	if shellErr != nil {
		t.Fatal(shellErr)
	}
	if failure, _ := classifyHostCheckExecution(shell, check); failure {
		t.Fatal("pre-spawn classification wrongly caught the composed-tree-only check")
	}

	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	metadata := plan.Metadata()
	for trackIndex := range metadata.Tracks {
		for sliceIndex := range metadata.Tracks[trackIndex].Slices {
			slice := &metadata.Tracks[trackIndex].Slices[sliceIndex]
			slice.Checks = append(append([]string(nil), slice.Checks...), check)
			slice.HostChecks = []string{check}
		}
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```protocol-plan-v2\n" + string(metadataBody) +
			"\n```\n\nHost-environment assembly-path fixture.\n",
	)
	plan, err = protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest

	submission := func(
		slice string,
		responsibility driver.Responsibility,
		protocolAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			ProtocolAttempt: protocolAttempt, Epoch: 1, Try: 1}
		return driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID(manifest.RunID, script),
			Responsibility: responsibility,
			Summary:        "Exact " + string(responsibility) + " for " + slice + ".",
			Detail:         "Bounded fixture detail.",
		}
	}
	planner := submission("", driver.PlannerProposal, 1)
	planner.Plan, _ = driver.NewPlanBytes(planBytes)
	manifest.Scripts = []ScriptedAttempt{
		{Responsibility: driver.PlannerProposal, ProtocolAttempt: 1, Epoch: 1, Try: 1,
			Behavior: "submit", Submission: encodeSubmission(t, planner)},
	}
	sliceFile := map[string]string{"S1": "one.txt", "S2": "two.txt"}
	for _, sliceID := range []string{"S1", "S2"} {
		design := submission(sliceID, driver.ImplementerDesign, 1)
		lead := submission(sliceID, driver.LeadReview, 1)
		lead.Decision, _ = driver.NewDecision(driver.DecisionProceed)
		implementation := submission(sliceID, driver.ImplementerImplementation, 1)
		implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
		work := submission(sliceID, driver.WorkVerification, 1)
		work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
		work.Decision, _ = driver.NewDecision(driver.DecisionPass)
		manifest.Scripts = append(manifest.Scripts,
			ScriptedAttempt{Slice: sliceID, Responsibility: driver.ImplementerDesign,
				ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit",
				Submission: encodeSubmission(t, design)},
			ScriptedAttempt{Slice: sliceID, Responsibility: driver.LeadReview,
				ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit",
				Submission: encodeSubmission(t, lead)},
			ScriptedAttempt{Slice: sliceID, Responsibility: driver.ImplementerImplementation,
				ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit",
				Submission: encodeSubmission(t, implementation)},
			ScriptedAttempt{Slice: sliceID, Responsibility: driver.WorkVerification,
				ProtocolAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit",
				Submission: encodeSubmission(t, work)},
		)
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repository, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	targetP := runRuntimeGit(t, repository, "rev-parse", "refs/heads/main")
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := protocol.NewActions(protocol.UseGitRepository(repoView), inertness, manifest.GitIdentity)
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
			for sliceID, file := range sliceFile {
				want := invocationID(manifest.RunID, ScriptedAttempt{
					Slice: sliceID, Responsibility: driver.ImplementerImplementation,
					ProtocolAttempt: 1, Epoch: 1, Try: 1,
				})
				if invocation.Request.InvocationID != want {
					continue
				}
				if err := os.WriteFile(
					filepath.Join(invocation.HostWorkspace, file),
					[]byte("implemented "+sliceID+"\n"), 0o600,
				); err != nil {
					return driver.Observation{}, err
				}
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

	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "assembly-path.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 27, 6, 7, 8, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	requireParkedFacts := func(t *testing.T, label string, status RunStatus) {
		t.Helper()
		if status.State != "parked" {
			t.Fatalf("%s State = %q, want parked", label, status.State)
		}
		if status.Park == nil || status.Park.Cause != ParkCauseHostEnvironment {
			t.Fatalf("%s Park = %#v, want cause host_environment", label, status.Park)
		}
		if status.Park.FailureCode != "HOST_CHECK_ENVIRONMENT" {
			t.Fatalf("%s Park.FailureCode = %q", label, status.Park.FailureCode)
		}
		if !strings.Contains(status.Park.FailureDetail, check) {
			t.Fatalf("%s Park.FailureDetail = %q", label, status.Park.FailureDetail)
		}
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	requireParkedFacts(t, "Start()", status)
	if status.Park.Work == "" || !runtimeDigestPattern.MatchString(status.Park.Work) {
		t.Fatalf("Start() Park.Work = %q, want a work identity", status.Park.Work)
	}
	prepareWork := status.Park.Work
	prepareEffectID := journal.AttemptEffectID(prepareWork, 1, 1)
	prepareEffect, err := store.Effect(ctx, manifest.RunID, prepareEffectID)
	if err != nil {
		t.Fatalf("prepare_assembly try-1 effect lookup = %v", err)
	}
	if prepareEffect.Kind != "protocol.prepare_assembly" || prepareEffect.State != journal.Claimed {
		t.Fatalf("prepare_assembly try-1 effect = %#v", prepareEffect)
	}
	for _, try := range []int64{2, 3} {
		if _, err := store.Effect(
			ctx, manifest.RunID, journal.AttemptEffectID(prepareWork, 1, try),
		); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
			t.Fatalf("prepare_assembly try-%d effect lookup = %v, want EFFECT_NOT_FOUND", try, err)
		}
	}

	afterRelease, err := service.Status(ctx, manifest.RunID)
	if err != nil {
		t.Fatalf("Status() after release error = %v", err)
	}
	requireParkedFacts(t, "Status() after release", afterRelease)
	if afterRelease.Park.Work != prepareWork {
		t.Fatalf("Status() after release Park.Work = %q, want %q",
			afterRelease.Park.Work, prepareWork)
	}

	if _, err := service.Control(ctx, journal.ControlCommand{
		RunID: manifest.RunID, ID: "resume-1",
		Kind: journal.Resume, ExpectedGeneration: 0,
	}); err != nil {
		t.Fatalf("Control(Resume) error = %v", err)
	}
	resumed, err := service.Wait(ctx, manifest.RunID)
	if err != nil {
		t.Fatalf("Wait() after Resume error = %v", err)
	}
	requireParkedFacts(t, "Resume", resumed)
	if resumed.Park.Work != prepareWork {
		t.Fatalf("Resume Park.Work = %q, want %q", resumed.Park.Work, prepareWork)
	}
}

// A2/A4 (S2-pause-safe-host-checks): a pause that lands after the model has
// already answered and the candidate is already git-sealed - at the
// git.seal.prepared claim, the #357 production journey's exact window -
// stops driver.dispatch as RUN_STOPPED without marking it or the outer
// git.seal effect operationally failed, and spends no try. Resuming crosses
// a real process boundary: the paused owner is released, a fresh owner and
// a fresh Service - built with a driver that fails the test if invoked
// again - take over the same journal file, and the same try completes
// without re-invoking the model.
func TestPauseAtGitSealPreparedClaimResumesOnTheSameTryWithAFreshOwner(t *testing.T) {
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 4, 5, 6, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, manifest.value.RunID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	var invocations atomic.Int64
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		invocations.Add(1)
		if err := os.WriteFile(
			filepath.Join(invocation.HostWorkspace, "one.txt"),
			[]byte("paused production implementation\n"), 0o600,
		); err != nil {
			t.Fatal(err)
		}
		submission := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.ImplementerImplementation,
			Summary:        "Paused production candidate.",
			Detail:         "Sealed before the pause lands on git.seal.prepared.",
		}
		submission.Checks, _ = driver.NewCheckBytes(
			[]byte("production implementation checks\n"),
		)
		submissionBody, encodeErr := driver.EncodeSubmission(submission)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody, encodeErr := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     submission.InvocationID,
			SubmissionDigest: driver.Digest(submissionBody),
			Accepted:         true,
			Code:             "accepted",
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody = append(sealBody, '\n')
		// The pause lands exactly where #357's production journey found
		// it: after the model has answered (this invocation is already
		// returning its sealed handoff) and before the candidate's host
		// boundary - host checks or, as declared by this fixture's plan,
		// straight to the git.seal.prepared claim - ever starts.
		if _, controlErr := store.ApplyControl(ctx, journal.ControlCommand{
			RunID: manifest.value.RunID, ID: "pause-1", Kind: journal.Pause,
			ExpectedGeneration: 0,
		}, now); controlErr != nil {
			t.Fatal(controlErr)
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submissionBody,
				SubmissionDigest: driver.Digest(submissionBody),
				SealBytes:        sealBody,
				SealDigest:       driver.Digest(sealBody),
			},
		}, nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planBytes, _ := runtimePlan(
		t, manifest.value.Release, manifest.value.Authority.Project,
		manifest.value.TargetRef, "approval-release-1-v1",
	)
	if _, err := engine.actions.RecordPlanRevision(
		protocol.RecordPlanRevisionInput{
			PlanBytes: planBytes,
			Summary:   "Install the exact production test plan.",
			Detail:    []byte("Pause-at-git.seal.prepared fixture."),
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "implementer",
		Result: "designed", Summary: "Design the pause fixture.",
		Detail: []byte("Exact design."),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "lead",
		Result: "proceed", Summary: "Proceed with the pause fixture.",
		Detail: []byte("Exact review."),
	}); err != nil {
		t.Fatal(err)
	}
	state, err := protocol.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !ok || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" || slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{
		GitIdentity: runtimeTestGitIdentity,
		Release:     state.Release, Slice: "S1",
		Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
		TargetHead: state.Refs.Target.Head, Track: track.ID,
		TrackRef: track.Ref, TrackHead: track.Head,
		DispatchWork: workIdentity(outerID, "driver.dispatch"),
		PreparedWork: workIdentity(outerID, "git.seal.prepared"),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(cycle.DispatchWork, 1, 1)
	cycle.PreparedEffect = journal.AttemptEffectID(cycle.PreparedWork, 1, 1)
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now,
		},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := store.ClaimOwned(ctx, owner, outerID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	outer := journal.Effect{
		RunID: owner.RunID, ID: outerID, Kind: "git.seal",
		State: journal.Claimed, CurrentClaim: outerClaim.Token,
	}
	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: state.Release, Track: track.ID},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: slice.Attempt, Epoch: 1, Try: 1,
	}

	_, _, dispatchErr := service.runProductionImplementationDispatch(
		ctx, engine, owner, workspace, cycle, coordinates,
	)
	if !IsCode(dispatchErr, "RUN_STOPPED") {
		t.Fatalf("paused dispatch = %v, want RUN_STOPPED", dispatchErr)
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}

	dispatch, err := store.Effect(ctx, owner.RunID, cycle.DispatchEffect)
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.State != journal.Claimed {
		t.Fatalf("driver.dispatch after pause = %#v, want Claimed", dispatch)
	}
	// EnsureAttempt itself does not consult desired state (only ClaimOwned
	// does), so git.seal.prepared is admitted Pending; the pause is caught
	// at the claim that follows, and recoverImplementationCycle's
	// EFFECT_NOT_FOUND branch is not the one this pause resumes through -
	// its preparedErr == nil branch (a claim attempt below) is.
	if preparedAfterPause, err := store.Effect(ctx, owner.RunID, cycle.PreparedEffect); err != nil {
		t.Fatalf("git.seal.prepared after pause: %v", err)
	} else if preparedAfterPause.State != journal.Pending {
		t.Fatalf("git.seal.prepared after pause = %#v, want Pending", preparedAfterPause)
	}
	if outerAfterPause, err := store.Effect(ctx, owner.RunID, outerID); err != nil {
		t.Fatal(err)
	} else if outerAfterPause.State != journal.Claimed {
		t.Fatalf("outer git.seal after pause = %#v, want still Claimed", outerAfterPause)
	}
	if invocations.Load() != 1 {
		t.Fatalf("invocations after pause = %d, want 1", invocations.Load())
	}
	checkpoint, err := store.Effect(ctx, owner.RunID, pausedHandoffCheckpointID(cycle.DispatchEffect))
	if err != nil {
		t.Fatalf("paused-handoff checkpoint missing: %v", err)
	}
	if checkpoint.Kind != "driver.handoff" || checkpoint.State != journal.Succeeded {
		t.Fatalf("paused-handoff checkpoint = %#v", checkpoint)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Resume through a real process boundary: release the paused owner,
	// apply a control Resume, and let a fresh owner and a fresh Service -
	// whose driver fails the test if invoked - take over the same journal.
	if err := store.ReleaseOwner(ctx, owner, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, journal.ControlCommand{
		RunID: manifest.value.RunID, ID: "resume-1", Kind: journal.Resume,
		ExpectedGeneration: 1,
	}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	freshOwner, err := store.AcquireOwner(
		ctx, manifest.value.RunID, now.Add(2*time.Second), time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if freshOwner.Token == owner.Token {
		t.Fatal("resume must acquire a genuinely new owner token")
	}
	restartedProduction, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal: store,
		dispatcher: fixtureDriver(func(
			context.Context, driver.Invocation,
		) (driver.Observation, error) {
			t.Fatal("resume invoked the production driver again")
			return driver.Observation{}, nil
		}),
		production: restartedProduction, gitExecutable: gitExecutable,
		now: func() time.Time { return now.Add(2 * time.Second) },
	}
	restartedEngine, err := restarted.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()

	pending, err := restarted.driverRecoveryPending(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("driverRecoveryPending fenced a Claimed dispatch that has a durable paused-handoff checkpoint")
	}

	key := gitx.TrackKey{Release: state.Release, Track: track.ID}
	recovered, retry, err := restarted.recoverImplementationCycle(
		ctx, restartedEngine, freshOwner, cycle, outer, key, coordinates,
	)
	if err != nil {
		t.Fatalf("resume recovery = %v", err)
	}
	if retry {
		t.Fatal("resume recovery asked for another try instead of completing this one")
	}
	if invocations.Load() != 1 {
		t.Fatalf("invocations after resume = %d, want 1 (no re-invocation)", invocations.Load())
	}
	if !sealedRecordMatchesCycle(recovered, cycle) {
		t.Fatalf("recovered record = %#v", recovered)
	}
	for _, effectID := range []string{cycle.DispatchEffect, cycle.PreparedEffect, outerID} {
		effect, err := store.Effect(ctx, freshOwner.RunID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("effect %s after resume = %#v, want Succeeded", effectID, effect)
		}
	}
	if runRuntimeGit(t, repository, "rev-parse", track.Ref) != recovered.Candidate {
		t.Fatalf("track ref after resume = %q, want %q",
			runRuntimeGit(t, repository, "rev-parse", track.Ref), recovered.Candidate)
	}
}

// markerGate lets a test deterministically land a pause or a context
// cancellation exactly while a declared host check's shell process is
// running, without a bare-sleep race: the check itself blocks on a file the
// test creates only after the stop is durably applied, so every ordering
// below is a real happens-before, not a timing guess
// (S7-pause-safe-host-checks-repair A1-A4).
type markerGate struct {
	dir string
}

func newMarkerGate(t *testing.T) *markerGate {
	t.Helper()
	return &markerGate{dir: t.TempDir()}
}

// check returns a shell command that announces name has started (a
// "running" marker file) and then blocks until the test releases it (a
// "continue" marker file).
func (g *markerGate) check(name string) string {
	running := filepath.Join(g.dir, name+".running")
	cont := filepath.Join(g.dir, name+".continue")
	return "touch " + shellQuote(running) +
		" && while [ ! -f " + shellQuote(cont) + " ]; do sleep 0.01; done"
}

func (g *markerGate) waitRunning(t *testing.T, name string) {
	t.Helper()
	running := filepath.Join(g.dir, name+".running")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(running); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for gated check %q to start", name)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (g *markerGate) release(t *testing.T, name string) {
	t.Helper()
	cont := filepath.Join(g.dir, name+".continue")
	if err := os.WriteFile(cont, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// pauseSafeHostCheckFixture is newProductionImplementationRecoveryFixture's
// sibling for S7-pause-safe-host-checks-repair: the same production
// dispatch scaffolding, but installed with a plan that declares real host
// checks on S1 (hostChecksPlanBytes), so a pause or cancel can land at a
// genuine check boundary or the git.seal.prepared claim through the real
// production path, not a fixture-level shortcut.
type pauseSafeHostCheckFixture struct {
	ctx           context.Context
	repository    string
	config        driver.LoadedDriverConfig
	manifest      admittedManifest
	store         *journal.Store
	owner         journal.OwnerLease
	now           time.Time
	gitExecutable string
	service       *Service
	engine        *engine
	state         protocol.State
	slice         *protocol.SliceState
	track         *protocol.TrackState
	cycle         implementationCycle
	outerID       string
	outer         journal.Effect
	workspace     *gitx.WorkspaceLease
	coordinates   dispatchCoordinates
}

func newPauseSafeHostCheckFixture(
	t *testing.T,
	hostChecks []string,
	dispatcher driver.Driver,
) *pauseSafeHostCheckFixture {
	t.Helper()
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 5, 6, 7, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, manifest.value.RunID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	planBytes := hostChecksPlanBytes(t, manifest, hostChecks)
	if _, err := engine.actions.RecordPlanRevision(protocol.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install the exact pause-safe host-check test plan.",
		Detail:    []byte("S7-pause-safe-host-checks-repair fixture."),
	}); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []protocol.AppendReceiptInput{
		{Release: manifest.value.Release, Slice: "S1", Role: "implementer",
			Result: "designed", Summary: "Design the pause-safe fixture.",
			Detail: []byte("Exact design.")},
		{Release: manifest.value.Release, Slice: "S1", Role: "lead",
			Result: "proceed", Summary: "Proceed with the pause-safe fixture.",
			Detail: []byte("Exact review.")},
	} {
		if _, err := engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	state, err := protocol.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, sliceOK := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !sliceOK || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" || slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{
		GitIdentity: runtimeTestGitIdentity,
		Release:     state.Release, Slice: "S1",
		Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
		TargetHead: state.Refs.Target.Head, Track: track.ID,
		TrackRef: track.Ref, TrackHead: track.Head,
		DispatchWork: workIdentity(outerID, "driver.dispatch"),
		PreparedWork: workIdentity(outerID, "git.seal.prepared"),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(cycle.DispatchWork, 1, 1)
	cycle.PreparedEffect = journal.AttemptEffectID(cycle.PreparedWork, 1, 1)
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(ctx,
		journal.Command{RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now},
		journal.Effect{RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := store.ClaimOwned(ctx, owner, outerID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: state.Release, Track: track.ID},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &pauseSafeHostCheckFixture{
		ctx: ctx, repository: repository, config: config,
		manifest: manifest, store: store, owner: owner, now: now,
		gitExecutable: gitExecutable,
		service:       service, engine: engine, state: state,
		slice: slice, track: track, cycle: cycle,
		outerID: outerID,
		outer: journal.Effect{
			RunID: owner.RunID, ID: outerID, Kind: "git.seal",
			State: journal.Claimed, CurrentClaim: outerClaim.Token,
		},
		workspace: workspace,
		coordinates: dispatchCoordinates{
			Slice: "S1", Responsibility: driver.ImplementerImplementation,
			ProtocolAttempt: slice.Attempt, Epoch: 1, Try: 1,
		},
	}
}

func (f *pauseSafeHostCheckFixture) trackKey() gitx.TrackKey {
	return gitx.TrackKey{Release: f.state.Release, Track: f.track.ID}
}

// pauseSafeModelDispatcher is the one production model mock every test below
// shares: it writes the plan's scoped file and returns an accepted sealed
// handoff, applying no pause or cancel of its own (each test lands its stop
// from outside, deterministically, through a markerGate on a declared host
// check).
func pauseSafeModelDispatcher(t *testing.T, invocations *atomic.Int64) fixtureDriver {
	return fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		invocations.Add(1)
		if err := os.WriteFile(
			filepath.Join(invocation.HostWorkspace, "one.txt"),
			[]byte("pause-safe host-check production implementation\n"), 0o600,
		); err != nil {
			t.Fatal(err)
		}
		submission := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.ImplementerImplementation,
			Summary:        "Pause-safe host-check production candidate.",
			Detail:         "Sealed before a pause or cancel lands on a check boundary or the seal claim.",
		}
		submission.Checks, _ = driver.NewCheckBytes(
			[]byte("pause-safe production implementation checks\n"),
		)
		submissionBody, encodeErr := driver.EncodeSubmission(submission)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody, encodeErr := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     submission.InvocationID,
			SubmissionDigest: driver.Digest(submissionBody),
			Accepted:         true,
			Code:             "accepted",
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody = append(sealBody, '\n')
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submissionBody,
				SubmissionDigest: driver.Digest(submissionBody),
				SealBytes:        sealBody,
				SealDigest:       driver.Digest(sealBody),
			},
		}, nil
	})
}

// failIfInvokedDispatcher fails the test if the production driver is
// invoked again: every resume below must re-enter through its durable
// paused-handoff checkpoint, never re-invoke the model.
func failIfInvokedDispatcher(t *testing.T, label string) fixtureDriver {
	return fixtureDriver(func(context.Context, driver.Invocation) (driver.Observation, error) {
		t.Fatalf("%s invoked the production driver again", label)
		return driver.Observation{}, nil
	})
}

// A1: a second or later pause of the same candidate that stops at a
// different point than the first (a check boundary, then the seal step)
// neither fails the implementer dispatch nor spends the try: both stops
// record the same checkpoint body under the same replay key, and a resume
// after both pauses seals the candidate on the same try it started with.
func TestSecondPauseAtADifferentStopPointReusesTheFirstCheckpointAndResumesOnTheSameTry(t *testing.T) {
	gate := newMarkerGate(t)
	firstCheck := gate.check("check1")
	secondCheck := gate.check("check2")
	var invocations atomic.Int64
	fixture := newPauseSafeHostCheckFixture(
		t, []string{firstCheck, secondCheck}, pauseSafeModelDispatcher(t, &invocations),
	)

	type dispatchResult struct {
		err error
	}

	// Pause 1: a check boundary - between the two declared host checks,
	// after the first has succeeded and before the second is admitted.
	done1 := make(chan dispatchResult, 1)
	go func() {
		_, _, err := fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace,
			fixture.cycle, fixture.coordinates,
		)
		done1 <- dispatchResult{err}
	}()
	gate.waitRunning(t, "check1")
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "pause-1", Kind: journal.Pause,
		ExpectedGeneration: 0,
	}, fixture.now); err != nil {
		t.Fatal(err)
	}
	gate.release(t, "check1")
	if r := <-done1; !IsCode(r.err, "RUN_STOPPED") {
		t.Fatalf("dispatch after pause 1 = %v, want RUN_STOPPED", r.err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}

	checkpoint1, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, pausedHandoffCheckpointID(fixture.cycle.DispatchEffect),
	)
	if err != nil {
		t.Fatalf("checkpoint after pause 1: %v", err)
	}
	if dispatchEffect, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, fixture.cycle.DispatchEffect,
	); err != nil || dispatchEffect.State != journal.Claimed {
		t.Fatalf("driver.dispatch after pause 1 = %#v, %v, want Claimed", dispatchEffect, err)
	}
	if _, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, fixture.cycle.PreparedEffect,
	); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
		t.Fatalf("git.seal.prepared admitted before any check.host effect exists: err=%v", err)
	}

	// Resume 1: release, apply Resume, take over with a fresh owner and a
	// Service whose driver fails the test if invoked.
	if err := fixture.store.ReleaseOwner(fixture.ctx, fixture.owner, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "resume-1", Kind: journal.Resume,
		ExpectedGeneration: 1,
	}, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	owner2, err := fixture.store.AcquireOwner(
		fixture.ctx, fixture.owner.RunID, fixture.now.Add(2*time.Second), time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	production2, err := newProductionDriverRuntime(fixture.config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	restarted1 := &Service{
		journal: fixture.store, dispatcher: failIfInvokedDispatcher(t, "resume 1"),
		production: production2, gitExecutable: fixture.gitExecutable,
		now: func() time.Time { return fixture.now.Add(2 * time.Second) },
	}
	engine2, err := restarted1.openEngine(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}

	// Pause 2: the seal step - after the second check has also succeeded on
	// resume, before the git.seal.prepared claim.
	type recoverResult struct {
		retry bool
		err   error
	}
	done2 := make(chan recoverResult, 1)
	go func() {
		_, retry, err := restarted1.recoverImplementationCycle(
			fixture.ctx, engine2, owner2, fixture.cycle, fixture.outer,
			fixture.trackKey(), fixture.coordinates,
		)
		done2 <- recoverResult{retry, err}
	}()
	gate.waitRunning(t, "check2")
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "pause-2", Kind: journal.Pause,
		ExpectedGeneration: 2,
	}, fixture.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	gate.release(t, "check2")
	if r := <-done2; !IsCode(r.err, "RUN_STOPPED") {
		t.Fatalf("recovery after pause 2 = %v, want RUN_STOPPED", r.err)
	}
	if err := engine2.Close(); err != nil {
		t.Fatal(err)
	}

	checkpoint2, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, pausedHandoffCheckpointID(fixture.cycle.DispatchEffect),
	)
	if err != nil {
		t.Fatalf("checkpoint after pause 2: %v", err)
	}
	if checkpoint2.ResultDigest != checkpoint1.ResultDigest {
		t.Fatalf(
			"checkpoint body changed across the second pause at a different stop point: %s != %s",
			checkpoint2.ResultDigest, checkpoint1.ResultDigest,
		)
	}
	if dispatchEffect, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, fixture.cycle.DispatchEffect,
	); err != nil || dispatchEffect.State != journal.Claimed {
		t.Fatalf("driver.dispatch after pause 2 = %#v, %v, want still Claimed", dispatchEffect, err)
	}
	if _, err := fixture.store.Effect(
		fixture.ctx, fixture.owner.RunID, journal.AttemptEffectID(fixture.cycle.DispatchWork, 1, 2),
	); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
		t.Fatalf("a second try's driver.dispatch effect exists after two pauses: err=%v", err)
	}

	// Resume 2: unobstructed - seals the candidate on the same try, with no
	// further model invocation.
	if err := fixture.store.ReleaseOwner(fixture.ctx, owner2, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "resume-2", Kind: journal.Resume,
		ExpectedGeneration: 3,
	}, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	owner3, err := fixture.store.AcquireOwner(
		fixture.ctx, fixture.owner.RunID, fixture.now.Add(3*time.Second), time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	production3, err := newProductionDriverRuntime(fixture.config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	restarted2 := &Service{
		journal: fixture.store, dispatcher: failIfInvokedDispatcher(t, "resume 2"),
		production: production3, gitExecutable: fixture.gitExecutable,
		now: func() time.Time { return fixture.now.Add(3 * time.Second) },
	}
	engine3, err := restarted2.openEngine(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine3.Close()

	recovered, retry, err := restarted2.recoverImplementationCycle(
		fixture.ctx, engine3, owner3, fixture.cycle, fixture.outer,
		fixture.trackKey(), fixture.coordinates,
	)
	if err != nil {
		t.Fatalf("final resume recovery = %v", err)
	}
	if retry {
		t.Fatal("final resume recovery asked for another try instead of completing this one")
	}
	if invocations.Load() != 1 {
		t.Fatalf("invocations after two pauses and two resumes = %d, want 1 (no re-invocation)", invocations.Load())
	}
	if !sealedRecordMatchesCycle(recovered, fixture.cycle) {
		t.Fatalf("recovered record = %#v", recovered)
	}
	for _, effectID := range []string{fixture.cycle.DispatchEffect, fixture.cycle.PreparedEffect, fixture.outerID} {
		effect, err := fixture.store.Effect(fixture.ctx, owner3.RunID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("effect %s after final resume = %#v, want Succeeded", effectID, effect)
		}
	}
}

// A2 (i): a production-mode pause that lands while a declared host check's
// process is actually running lets that check finish and records its
// result; the check after it is never admitted; resume runs the remaining
// check and seals the candidate.
func TestPauseWhileADeclaredHostCheckIsRunningFinishesItAndResumesTheNextCheck(t *testing.T) {
	gate := newMarkerGate(t)
	runningCheck := gate.check("running-check")
	secondCheck := "echo second check ok"
	var invocations atomic.Int64
	fixture := newPauseSafeHostCheckFixture(
		t, []string{runningCheck, secondCheck}, pauseSafeModelDispatcher(t, &invocations),
	)

	type dispatchResult struct{ err error }
	done := make(chan dispatchResult, 1)
	go func() {
		_, _, err := fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace,
			fixture.cycle, fixture.coordinates,
		)
		done <- dispatchResult{err}
	}()
	gate.waitRunning(t, "running-check")
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "pause-mid-check", Kind: journal.Pause,
		ExpectedGeneration: 0,
	}, fixture.now); err != nil {
		t.Fatal(err)
	}
	gate.release(t, "running-check")
	if r := <-done; !IsCode(r.err, "RUN_STOPPED") {
		t.Fatalf("dispatch while a check was running = %v, want RUN_STOPPED", r.err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	succeededHostChecks := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind != "check.host" {
			continue
		}
		succeededHostChecks++
		if effect.State != journal.Succeeded {
			t.Fatalf(
				"check.host effect %s across the pause = %#v, want Succeeded (a running check must finish)",
				effect.ID, effect,
			)
		}
	}
	if succeededHostChecks != 1 {
		t.Fatalf("succeeded check.host effects while paused = %d, want 1", succeededHostChecks)
	}

	if err := fixture.store.ReleaseOwner(fixture.ctx, fixture.owner, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		RunID: fixture.owner.RunID, ID: "resume-1", Kind: journal.Resume,
		ExpectedGeneration: 1,
	}, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	freshOwner, err := fixture.store.AcquireOwner(
		fixture.ctx, fixture.owner.RunID, fixture.now.Add(2*time.Second), time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	restartedProduction, err := newProductionDriverRuntime(fixture.config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal: fixture.store, dispatcher: failIfInvokedDispatcher(t, "resume"),
		production: restartedProduction, gitExecutable: fixture.gitExecutable,
		now: func() time.Time { return fixture.now.Add(2 * time.Second) },
	}
	restartedEngine, err := restarted.openEngine(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()

	recovered, retry, err := restarted.recoverImplementationCycle(
		fixture.ctx, restartedEngine, freshOwner, fixture.cycle, fixture.outer,
		fixture.trackKey(), fixture.coordinates,
	)
	if err != nil {
		t.Fatalf("resume recovery = %v", err)
	}
	if retry {
		t.Fatal("resume recovery asked for another try instead of completing this one")
	}
	if invocations.Load() != 1 {
		t.Fatalf("invocations after resume = %d, want 1 (no re-invocation)", invocations.Load())
	}
	if !sealedRecordMatchesCycle(recovered, fixture.cycle) {
		t.Fatalf("recovered record = %#v", recovered)
	}
	finalSnapshot, err := fixture.store.Snapshot(fixture.ctx, freshOwner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	finalSucceeded := 0
	for _, effect := range finalSnapshot.Effects {
		if effect.Kind == "check.host" && effect.State == journal.Succeeded {
			finalSucceeded++
		}
	}
	if finalSucceeded != 2 {
		t.Fatalf("succeeded check.host effects after resume = %d, want 2 (the second check ran on resume)", finalSucceeded)
	}
	for _, effectID := range []string{fixture.cycle.DispatchEffect, fixture.cycle.PreparedEffect, fixture.outerID} {
		effect, err := fixture.store.Effect(fixture.ctx, freshOwner.RunID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("effect %s after resume = %#v, want Succeeded", effectID, effect)
		}
	}
}

// A2 (ii): a context cancelled exactly at the git.seal.prepared claim (not a
// journal.Pause control command) is reported as RUN_STOPPED layered over
// OPERATION_CANCELLED, not as a failure of the work: the driver.dispatch,
// git.seal.prepared and outer git.seal effects stay Claimed/Pending, never
// OperationalFailed. No host checks are declared, so runHostChecks' own
// ctx.Err() boundary never intercepts the cancellation first.
func TestCancelledContextAtGitSealPreparedClaimReportsRunStoppedFromOperationCancelled(t *testing.T) {
	gate := newMarkerGate(t)
	onlyCheck := gate.check("only-check")
	var invocations atomic.Int64
	fixture := newPauseSafeHostCheckFixture(t, []string{onlyCheck}, pauseSafeModelDispatcher(t, &invocations))

	dispatchCtx, cancelDispatch := context.WithCancel(context.Background())
	defer cancelDispatch()

	type dispatchResult struct{ err error }
	done := make(chan dispatchResult, 1)
	go func() {
		_, _, err := fixture.service.runProductionImplementationDispatch(
			dispatchCtx, fixture.engine, fixture.owner, fixture.workspace,
			fixture.cycle, fixture.coordinates,
		)
		done <- dispatchResult{err}
	}()
	// The declared check is admitted and starts running on a live context;
	// only once it is confirmed running is the context cancelled, so the
	// cancellation lands after the check (and prepareHandoff's own earlier
	// reads) and before the git.seal.prepared claim that follows - not
	// inside runHostChecks' own ctx.Err() boundary, which is never
	// re-checked once the only declared check has been admitted.
	gate.waitRunning(t, "only-check")
	cancelDispatch()
	gate.release(t, "only-check")

	r := <-done
	if !IsCode(r.err, "RUN_STOPPED") {
		t.Fatalf("dispatch across a cancelled context = %v, want RUN_STOPPED", r.err)
	}
	if !journal.IsCode(r.err, "OPERATION_CANCELLED") {
		t.Fatalf(
			"dispatch error chain = %v, want RUN_STOPPED layered over OPERATION_CANCELLED (not CONTROL_STOPPED)",
			r.err,
		)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}

	// The check that was already running when the cancel landed still
	// finished and its result is recorded - exec.Command carries no
	// context, so the process itself is unaffected by the cancel.
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	succeededHostChecks := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "check.host" {
			succeededHostChecks++
			if effect.State != journal.Succeeded {
				t.Fatalf("check.host effect %s across the cancel = %#v, want Succeeded", effect.ID, effect)
			}
		}
	}
	if succeededHostChecks != 1 {
		t.Fatalf("succeeded check.host effects across the cancel = %d, want 1", succeededHostChecks)
	}

	dispatchEffect, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, fixture.cycle.DispatchEffect)
	if err != nil {
		t.Fatal(err)
	}
	if dispatchEffect.State != journal.Claimed {
		t.Fatalf("driver.dispatch across the cancel = %#v, want Claimed, never a failure of the work", dispatchEffect)
	}
	// The cancelled context reaches whichever of git.seal.prepared's own
	// EnsureAttempt or ClaimOwned runs first; either way the effect is
	// never completed as a failure of the work - it is either never
	// admitted at all, or admitted but never claimed.
	preparedEffect, preparedErr := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, fixture.cycle.PreparedEffect)
	switch {
	case journal.IsCode(preparedErr, "EFFECT_NOT_FOUND"):
		// Never admitted: EnsureAttempt itself observed the cancelled
		// context.
	case preparedErr == nil && preparedEffect.State == journal.Pending:
		// Admitted but never claimed: EnsureAttempt succeeded before the
		// cancellation, ClaimOwned observed it.
	default:
		t.Fatalf("git.seal.prepared across the cancel = %#v, %v, want absent or Pending, never a failure", preparedEffect, preparedErr)
	}
	outerEffect, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, fixture.outerID)
	if err != nil {
		t.Fatal(err)
	}
	if outerEffect.State != journal.Claimed {
		t.Fatalf("outer git.seal across the cancel = %#v, want still Claimed", outerEffect)
	}
	if invocations.Load() != 1 {
		t.Fatalf("invocations = %d, want 1", invocations.Load())
	}
}

// A3: a pause or a cancel that lands during the start-of-cycle recovery
// sweep (recoverClaimedEffects, including recoverImplementationCycle's own
// resumePausedProductionDispatch continuation) is treated as a stop, exactly
// as the drive loop treats it: driveOwned - the primitive Start, Resume and
// Serve all share - does not return it as an error, and the run stays
// resumable on a later, unobstructed resume.
func TestPauseOrCancelDuringTheRecoverySweepIsAStopNotAnErrorAndTheRunStaysResumable(t *testing.T) {
	for _, kind := range []string{"pause", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			gate := newMarkerGate(t)
			boundaryCheck := gate.check("boundary")
			resumeCheck := gate.check("resume-sweep")
			var invocations atomic.Int64
			fixture := newPauseSafeHostCheckFixture(
				t, []string{boundaryCheck, resumeCheck}, pauseSafeModelDispatcher(t, &invocations),
			)
			// driveOwned (unlike a direct runProductionImplementationDispatch
			// or recoverImplementationCycle call) re-derives its manifest
			// from the journal, so the run's start command must be durably
			// recorded for it to find.
			if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
				RunID: fixture.owner.RunID, ReplayKey: "manifest", Kind: "start",
				Payload: fixture.manifest.raw, CreatedAt: fixture.now,
			}); err != nil {
				t.Fatal(err)
			}

			// Pause 1: an ordinary check-boundary stop. This leaves a
			// Claimed driver.dispatch with a durable paused-handoff
			// checkpoint - exactly the state a resume's recovery sweep
			// must continue from via resumePausedProductionDispatch.
			type dispatchResult struct{ err error }
			done1 := make(chan dispatchResult, 1)
			go func() {
				_, _, err := fixture.service.runProductionImplementationDispatch(
					fixture.ctx, fixture.engine, fixture.owner, fixture.workspace,
					fixture.cycle, fixture.coordinates,
				)
				done1 <- dispatchResult{err}
			}()
			gate.waitRunning(t, "boundary")
			if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
				RunID: fixture.owner.RunID, ID: "pause-1", Kind: journal.Pause,
				ExpectedGeneration: 0,
			}, fixture.now); err != nil {
				t.Fatal(err)
			}
			gate.release(t, "boundary")
			if r := <-done1; !IsCode(r.err, "RUN_STOPPED") {
				t.Fatalf("dispatch after pause 1 = %v, want RUN_STOPPED", r.err)
			}
			if err := fixture.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			if err := fixture.engine.Close(); err != nil {
				t.Fatal(err)
			}

			if err := fixture.store.ReleaseOwner(fixture.ctx, fixture.owner, fixture.now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
				RunID: fixture.owner.RunID, ID: "resume-1", Kind: journal.Resume,
				ExpectedGeneration: 1,
			}, fixture.now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			owner2, err := fixture.store.AcquireOwner(
				fixture.ctx, fixture.owner.RunID, fixture.now.Add(2*time.Second), time.Minute, false,
			)
			if err != nil {
				t.Fatal(err)
			}
			production2, err := newProductionDriverRuntime(fixture.config, driver.DriverFactoryOptions{})
			if err != nil {
				t.Fatal(err)
			}
			restarted := &Service{
				journal:    fixture.store,
				dispatcher: failIfInvokedDispatcher(t, "the recovery sweep's resume"),
				production: production2, gitExecutable: fixture.gitExecutable,
				now: func() time.Time { return fixture.now.Add(2 * time.Second) },
			}

			// The second stop lands inside driveOwned's own recovery sweep,
			// while ownedCtx is live and the resumed continuation is
			// blocked on the gated check - not through a direct
			// recoverClaimedEffects call.
			driveCtx, cancelDrive := context.WithCancel(context.Background())
			defer cancelDrive()
			type driveResult struct {
				status RunStatus
				err    error
			}
			done2 := make(chan driveResult, 1)
			go func() {
				status, err := restarted.driveOwned(driveCtx, fixture.owner.RunID, owner2)
				done2 <- driveResult{status, err}
			}()
			gate.waitRunning(t, "resume-sweep")
			switch kind {
			case "pause":
				if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
					RunID: fixture.owner.RunID, ID: "pause-2", Kind: journal.Pause,
					ExpectedGeneration: 2,
				}, fixture.now.Add(3*time.Second)); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancelDrive()
			}
			gate.release(t, "resume-sweep")
			result := <-done2
			if result.err != nil {
				t.Fatalf(
					"driveOwned across a %s landing inside the recovery sweep = %v, want nil (a stop, not a CLI error)",
					kind, result.err,
				)
			}

			// driveOwned released owner2 itself (ReleaseOwnerIfIdle) before
			// returning; a later, unobstructed resume proves the run stays
			// resumable and seals on the same try, with no further model
			// invocation.
			if kind == "pause" {
				if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
					RunID: fixture.owner.RunID, ID: "resume-2", Kind: journal.Resume,
					ExpectedGeneration: 3,
				}, fixture.now.Add(4*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			owner3, err := fixture.store.AcquireOwner(
				fixture.ctx, fixture.owner.RunID, fixture.now.Add(5*time.Second), time.Minute, false,
			)
			if err != nil {
				t.Fatal(err)
			}
			finalProduction, err := newProductionDriverRuntime(fixture.config, driver.DriverFactoryOptions{})
			if err != nil {
				t.Fatal(err)
			}
			final := &Service{
				journal:    fixture.store,
				dispatcher: failIfInvokedDispatcher(t, "the final resume"),
				production: finalProduction, gitExecutable: fixture.gitExecutable,
				now: func() time.Time { return fixture.now.Add(5 * time.Second) },
			}
			finalEngine, err := final.openEngine(fixture.manifest)
			if err != nil {
				t.Fatal(err)
			}
			defer finalEngine.Close()
			recovered, retry, err := final.recoverImplementationCycle(
				fixture.ctx, finalEngine, owner3, fixture.cycle, fixture.outer,
				fixture.trackKey(), fixture.coordinates,
			)
			if err != nil {
				t.Fatalf("final resume recovery = %v", err)
			}
			if retry {
				t.Fatal("final resume recovery asked for another try instead of completing this one")
			}
			if invocations.Load() != 1 {
				t.Fatalf("invocations after the %s and both resumes = %d, want 1", kind, invocations.Load())
			}
			if !sealedRecordMatchesCycle(recovered, fixture.cycle) {
				t.Fatalf("recovered record = %#v", recovered)
			}
			for _, effectID := range []string{fixture.cycle.DispatchEffect, fixture.cycle.PreparedEffect, fixture.outerID} {
				effect, err := fixture.store.Effect(fixture.ctx, owner3.RunID, effectID)
				if err != nil {
					t.Fatal(err)
				}
				if effect.State != journal.Succeeded {
					t.Fatalf("effect %s after final resume = %#v, want Succeeded", effectID, effect)
				}
			}
		})
	}
}
