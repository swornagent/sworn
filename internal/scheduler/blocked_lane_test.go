package scheduler

// S14-blocked-terminal — AC-04 track-halt test (new file by design: AC-06
// forbids edits to existing test files, including worker_test.go).

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/orchestrator"
	"github.com/swornagent/sworn/internal/supervisor"
)

// TestLoopBlockedSliceHaltsTrack: a two-slice sequential track whose first
// slice returns a blocked-terminal error must halt immediately — the second
// slice is never dispatched — with the lane reported once through the
// RecordBlocked side-channel (blocker verbatim, route suffix trimmed) and
// the supervisor row persisted as StateFailed, never coerced to "done"
// (Captain review pin 1: releaseTrack("blocked") would be silently rewritten
// to StateDone by supervisor.Release).
func TestLoopBlockedSliceHaltsTrack(t *testing.T) {
	tmpDir := t.TempDir()
	sliceIDs := []string{"S01-blocked", "S02-never-runs"}
	for _, sid := range sliceIDs {
		d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	}

	const blocker = "spec defect: acceptance check contradicts out-of-scope — replan required"

	var called []string
	runSliceFn := func(_ context.Context, _, specPath, _ string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		called = append(called, sliceID)
		// The blocked-terminal error shape RunSlice emits (sentinel +
		// verbatim reason + route-directive suffix).
		return fmt.Errorf("%s %s%s", orchestrator.BlockedLaneSentinel, blocker, orchestrator.BlockedLaneRouteSuffix)
	}

	type blockedRec struct{ track, slice, reason string }
	var recorded []blockedRec

	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         sliceIDs,
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: runSliceFn,
		RecordBlocked: func(trackID, sliceID, reason string) {
			recorded = append(recorded, blockedRec{trackID, sliceID, reason})
		},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)

	// D3: the scheduler carrier stays TrackFail — the blocked/failed
	// distinction travels via RecordBlocked only.
	if result != TrackFail {
		t.Fatalf("expected TrackFail for a blocked lane, got %s", result)
	}

	// AC-04: the track halted at the blocked slice — S02 never dispatched.
	if len(called) != 1 || called[0] != "S01-blocked" {
		t.Fatalf("expected exactly one dispatch (S01-blocked), got %v", called)
	}

	// RecordBlocked invoked exactly once, blocker verbatim, suffix trimmed.
	if len(recorded) != 1 {
		t.Fatalf("RecordBlocked invoked %d times, want exactly 1", len(recorded))
	}
	if recorded[0].track != "T1" || recorded[0].slice != "S01-blocked" {
		t.Errorf("RecordBlocked(track=%q, slice=%q), want (T1, S01-blocked)", recorded[0].track, recorded[0].slice)
	}
	if recorded[0].reason != blocker {
		t.Errorf("RecordBlocked reason = %q, want the blocker verbatim with the route suffix trimmed", recorded[0].reason)
	}

	// Pin 1 anchor: the supervisor row is "failed" — a blocked lane recorded
	// as "done" would read as a completed track in the tracks DB.
	var supState string
	if err := db.QueryRow(`SELECT state FROM tracks WHERE id = ? AND release = ?`, "T1", "test-release").Scan(&supState); err != nil {
		t.Fatalf("read supervisor track row: %v", err)
	}
	if supState != supervisor.StateFailed {
		t.Errorf("supervisor track state = %q, want %q (never \"done\" for a blocked lane)", supState, supervisor.StateFailed)
	}
}
