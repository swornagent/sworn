package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
