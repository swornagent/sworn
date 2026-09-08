package journal

import (
	"context"
	"testing"
	"time"
)

func TestJournalUnverifiedCheckpointPersistenceAndRecovery(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)

	cp1 := UnverifiedCheckpoint{
		Repository:     run.Repository,
		RunID:          run.ID,
		Release:        run.Release,
		Track:          "T1",
		Slice:          "S1",
		PlanOID:        "1111111111111111111111111111111111111111",
		PlanDigest:     "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		ContractPath:   "contracts/S1.json",
		ContractDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		PreparedBase:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DispatchWork:   "S1",
		Epoch:          1,
		Try:            1,
		CheckpointRef:  "refs/heads/checkpoints/rel/T1/S1-1-1",
		CommitOID:      "3333333333333333333333333333333333333333",
		TreeOID:        "4444444444444444444444444444444444444444",
		TreeDigest:     "sha256:5555555555555555555555555555555555555555555555555555555555555555",
		StagedBytes:    1024,
		FileCount:      2,
	}

	if err := store.RecordUnverifiedCheckpoint(ctx, cp1, now); err != nil {
		t.Fatalf("record checkpoint 1 failed: %v", err)
	}

	// Read latest for S1
	latest, err := store.LatestUnverifiedCheckpoint(ctx, run.ID, "S1")
	if err != nil || latest == nil {
		t.Fatalf("latest checkpoint failed: %v, %v", latest, err)
	}
	if latest.CommitOID != cp1.CommitOID || latest.TreeOID != cp1.TreeOID {
		t.Fatalf("latest mismatch: got %#v, want %#v", latest, cp1)
	}

	// Record try 2 for S1
	now2 := now.Add(time.Second)
	cp2 := cp1
	cp2.Try = 2
	cp2.CommitOID = "6666666666666666666666666666666666666666"
	cp2.TreeOID = "7777777777777777777777777777777777777777"
	if err := store.RecordUnverifiedCheckpoint(ctx, cp2, now2); err != nil {
		t.Fatalf("record checkpoint 2 failed: %v", err)
	}

	latest2, err := store.LatestUnverifiedCheckpoint(ctx, run.ID, "S1")
	if err != nil || latest2 == nil {
		t.Fatalf("latest checkpoint 2 failed: %v", err)
	}
	if latest2.CommitOID != cp2.CommitOID || latest2.Try != 2 {
		t.Fatalf("expected try 2, got: %#v", latest2)
	}

	// List checkpoints
	all, err := store.ListUnverifiedCheckpoints(ctx, run.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 checkpoints, got: %d (%v)", len(all), err)
	}
}

func TestJournalCheckpointRestoredPersistenceAndRecovery(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)

	restored := CheckpointRestored{
		RunID:         run.ID,
		Release:       run.Release,
		Track:         "T1",
		Slice:         "S1",
		CheckpointRef: "refs/heads/checkpoints/rel/T1/S1-1-1",
		TreeOID:       "4444444444444444444444444444444444444444",
	}

	if err := store.RecordCheckpointRestored(ctx, restored, now); err != nil {
		t.Fatalf("record checkpoint restored failed: %v", err)
	}

	list, err := store.ListRestoredCheckpoints(ctx, run.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 restored checkpoint, got: %d (%v)", len(list), err)
	}
	if list[0].CheckpointRef != restored.CheckpointRef || list[0].TreeOID != restored.TreeOID {
		t.Fatalf("restored checkpoint mismatch: got %#v, want %#v", list[0], restored)
	}
}
