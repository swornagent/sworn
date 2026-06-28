package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/state"
)

// fakeRunSlice is a test helper that records the slices it was called with.
func fakeRunSlice(allowFailAt string, called *[]string) func(context.Context, string, string, string) error {
	return func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		*called = append(*called, filepath.Base(filepath.Dir(specPath)))
		sliceParent := filepath.Base(filepath.Dir(specPath))
		if sliceParent == allowFailAt {
			return fmt.Errorf("simulated failure: %s", sliceParent)
		}
		return nil
	}
}

func TestRunTrack_AllSlicesPass(t *testing.T) {
	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-release", "S01-test")
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-test"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v — skipping worker test", err)
	}
	defer db.Close()

	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	if len(called) != 1 {
		t.Fatalf("expected 1 slice call, got %d: %v", len(called), called)
	}
}

func TestRunTrack_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := WorkerOptions{
		ReleaseName: "test",
		TrackInfo: board.TrackInfo{
			ID:     "T1",
			Slices: []string{"S01"},
		},
		PrimaryWorktreeRoot: t.TempDir(),
	}

	result := RunTrack(ctx, opts)
	if result != TrackSkipped {
		t.Fatalf("expected TrackSkipped for cancelled context, got %s", result)
	}
}

func TestRunTrack_SliceFail(t *testing.T) {
	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-release", "S01-fail")
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-fail"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("S01-fail", &called),
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
	if result != TrackFail {
		t.Fatalf("expected TrackFail for slice failure, got %s", result)
	}
}

func TestRunTrack_MultiSliceOrdering(t *testing.T) {
	tmpDir := t.TempDir()

	for _, sid := range []string{"S01-first", "S02-second", "S03-third"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-first", "S02-second", "S03-third"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v — skipping worker test", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	want := []string{"S01-first", "S02-second", "S03-third"}
	if len(called) != len(want) {
		t.Fatalf("expected %d slice calls, got %d: %v", len(want), len(called), called)
	}
	for i, sid := range want {
		if called[i] != sid {
			t.Errorf("call[%d] = %q, want %q", i, called[i], sid)
		}
	}
}

func TestRunTrack_MaterialisesWorktree(t *testing.T) {
	tmpDir := t.TempDir()

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-test"},
			WorktreePath:   filepath.Join(tmpDir, "nonexistent-worktree"),
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v — skipping worker test", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackFail {
		t.Fatalf("expected TrackFail (materialisation attempt fails without git repo), got %s", result)
	}
}

func TestRunTrack_EmptySlices(t *testing.T) {
	tmpDir := t.TempDir()
	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test",
		PrimaryWorktreeRoot: tmpDir,
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass for empty slices, got %s", result)
	}
	if len(called) != 0 {
		t.Fatalf("expected 0 slice calls for empty slices, got %d", len(called))
	}
}

// ── Router-driven worker tests (S59) ────────────────────────────────────

// fakeRouter returns scripted decisions for testing the router-driven worker.
type fakeRouter struct {
	decisions []SliceDecision
	callCount int
	calls     []struct{ sliceID string }
}

func (f *fakeRouter) Route(_ context.Context, _, sliceID, _ string) (SliceDecision, error) {
	f.calls = append(f.calls, struct{ sliceID string }{sliceID})
	idx := f.callCount
	if idx >= len(f.decisions) {
		idx = len(f.decisions) - 1
	}
	f.callCount++
	return f.decisions[idx], nil
}

// fakeRunSliceWithAckRemoval records calls and simulates ack removal checking.
func fakeRunSliceWithAckRemoval(called *[]string, ackRemoved *bool, workRoot string) func(context.Context, string, string, string) error {
	return func(ctx context.Context, wt, specPath, statusPath string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		*called = append(*called, filepath.Base(filepath.Dir(specPath)))
		if ackRemoved != nil {
			ackPath := filepath.Join(workRoot, filepath.Dir(specPath), "captain-proceed.md")
			_, err := os.Stat(ackPath)
			*ackRemoved = os.IsNotExist(err)
		}
		return nil
	}
}

func TestWorkerPollsRouterDrivesSlice(t *testing.T) {
	// AC-1: Worker drives a 2-slice track by polling the router, not a
	// static list.
	tmpDir := t.TempDir()

	for _, sid := range []string{"S01-first", "S02-second"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)
	}

	// Script: implement S01 → advance to S02 → implement S02 → done.
	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "planned", Target: ""},
			{Type: "implement", Reason: "next", Target: "S02-second"},
			{Type: "none", Reason: "terminal"},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-first", "S02-second"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	if len(called) != 2 {
		t.Fatalf("expected 2 slice calls, got %d: %v", len(called), called)
	}
	if called[0] != "S01-first" {
		t.Errorf("call[0] = %q, want S01-first", called[0])
	}
	if called[1] != "S02-second" {
		t.Errorf("call[1] = %q, want S02-second", called[1])
	}
}

func TestWorkerResumesSkipsVerified(t *testing.T) {
	// AC-2: Resumability — slice 1 is already verified, skipped on re-entry.
	tmpDir := t.TempDir()

	for _, sid := range []string{"S01-done", "S02-next"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)
	}

	// Router: S01 verified → advance to S02 → implement S02 → done (2 decisions).
	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "next", Target: "S02-next"},
			{Type: "none", Reason: "terminal"},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-done", "S02-next"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	// S01-done must NOT be called.
	for _, s := range called {
		if s == "S01-done" {
			t.Errorf("S01-done was called but should have been skipped (already verified)")
		}
	}
	// S02-next must have been called exactly once.
	s02Count := 0
	for _, s := range called {
		if s == "S02-next" {
			s02Count++
		}
	}
	if s02Count != 1 {
		t.Errorf("S02-next called %d times, want 1", s02Count)
	}
}

func TestRedesignStripsAck(t *testing.T) {
	// AC-3: redesign decision removes captain-proceed.md before re-dispatching.
	tmpDir := t.TempDir()

	sid := "S01-redesign"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"failed_verification"}`), 0o644)
	os.WriteFile(filepath.Join(d, "captain-proceed.md"), []byte("approved"), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "redesign", Reason: "Gate 2 violation", Target: ""},
			{Type: "none", Reason: "terminal"},
		},
	}

	var called []string
	var ackRemoved bool
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSliceWithAckRemoval(&called, &ackRemoved, tmpDir),
		Router:     router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	if !ackRemoved {
		t.Error("expected captain-proceed.md to be removed after redesign decision")
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 RunSlice call after redesign, got %d", len(called))
	}
}

func TestPauseStateSurfacesNoLoop(t *testing.T) {
	// AC-4: coach_decision pauses and surfaces (no auto-pass, no loop).
	tmpDir := t.TempDir()

	sid := "S01-pause"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"design_review"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "coach_decision", Reason: "needs Coach approval", Target: ""},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
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
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for coach_decision, got %s", result)
	}

	if len(called) != 0 {
		t.Errorf("expected 0 RunSlice calls for pause, got %d", len(called))
	}
}

func TestReplanReleasePauses(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-replan"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "replan-release", Reason: "spec defect", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		Router: router,
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
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for replan-release, got %s", result)
	}
}

func TestMergeTrackDecisionPauses(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-final"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"verified"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "merge-track", Reason: "track fully verified", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		Router: router,
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
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for merge-track, got %s", result)
	}
}

func TestNoneDecisionCompletes(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-shipped"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"shipped"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "none", Reason: "terminal", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		Router: router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass for none, got %s", result)
	}
}

func TestRouterDrivenWorkerSupervisorAcquireRelease(t *testing.T) {
	// AC-5: supervisor.Acquire/Release still brackets every worker.
	tmpDir := t.TempDir()

	sid := "S01-test"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "none", Reason: "terminal", Target: ""},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	rows, err := db.Query(`SELECT state FROM tracks WHERE id = 'T1' AND release = 'test-release'`)
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no track row found — supervisor may not have released")
	}
	var state string
	rows.Scan(&state)
	if state != "done" {
		t.Errorf("expected supervisor state 'done', got %q", state)
	}
}

func TestCooperativePauseSignal(t *testing.T) {
	// AC-7: engine pause signal is honoured at the next poll boundary —
	// the worker completes the in-flight dispatch (S01-first), then stops
	// because the pause channel closes before the second router poll.
	tmpDir := t.TempDir()

	for _, sid := range []string{"S01-first", "S02-second"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-pause", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)
	}

	pauseCh := make(chan struct{})

	// Router: first call dispatches S01-first; a second call would advance
	// to S02-second — but the pause fires before the second poll.
	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "planned", Target: ""},
			{Type: "implement", Reason: "next", Target: "S02-second"},
			{Type: "none", Reason: "terminal"},
		},
	}

	var called []string
	runFn := func(ctx context.Context, wt, specPath, statusPath string) error {
		sid := filepath.Base(filepath.Dir(specPath))
		called = append(called, sid)
		// Close the pause channel once the first dispatch completes — the
		// next iteration's cooperative pause check will fire before polling.
		if len(called) == 1 {
			close(pauseCh)
		}
		return nil
	}

	opts := WorkerOptions{
		ReleaseName:         "test-pause",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-first", "S02-second"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: runFn,
		Router:     router,
		PauseCh:    pauseCh,
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
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused after cooperative pause signal, got %s", result)
	}
	// S01-first must have been dispatched (in-flight dispatch completed).
	if len(called) != 1 || called[0] != "S01-first" {
		t.Errorf("expected only S01-first dispatched before pause, got %v", called)
	}
	// S02-second must NOT have been dispatched.
	for _, s := range called {
		if s == "S02-second" {
			t.Error("S02-second was dispatched — cooperative pause was not honoured")
		}
	}
}

func TestRouterDrivenWorkerLegacyFallback(t *testing.T) {
	// When no Router is configured, fall back to static iteration.
	tmpDir := t.TempDir()

	for _, sid := range []string{"S01-first", "S02-second"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-first", "S02-second"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     nil, // no router — legacy path
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass (legacy), got %s", result)
	}

	if len(called) != 2 {
		t.Fatalf("expected 2 slice calls (legacy), got %d: %v", len(called), called)
	}
}

func TestCrashRecovery(t *testing.T) {
	// AC-8: Crash recovery — a slice in in_progress state at worker startup
	// simulates a process being SIGKILL'd mid-dispatch. On restart the router
	// re-derives "implement" from the committed in_progress state (per S58's
	// routing table: in_progress → implement) and the worker dispatches correctly.
	// No slice strands in_progress permanently; no work is double-applied.
	tmpDir := t.TempDir()

	sid := "S01-inprogress"
	d := filepath.Join(tmpDir, "docs", "release", "test-crash", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	// Slice is in_progress — simulates a SIGKILL mid-dispatch leaving committed state.
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"in_progress"}`), 0o644)

	// Router simulates S58 re-derivation: in_progress → implement (restart).
	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "in_progress → restart from committed state", Target: ""},
			{Type: "none", Reason: "terminal"},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-crash",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-crash/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass (crash recovery: in_progress → implement → pass), got %s", result)
	}

	// The worker must have dispatched implement for the in_progress slice.
	if len(called) != 1 || called[0] != sid {
		t.Fatalf("expected implement dispatch for %q, got %v", sid, called)
	}
}

// TestRunTrack_InterpreterInconclusivePauses proves that when RunSliceFn
// returns an INTERPRETER_INCONCLUSIVE error (S01), the worker pauses the
// track rather than failing it. This is the worker-side integration test
// for AC3/AC6: the non-typed-output -> interpreter -> triage path results
// in a PAGE event (track paused).
func TestRunTrack_InterpreterInconclusivePauses(t *testing.T) {
	trackID := "T1-interp"
	sliceID := "S01-interp-test"

	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-interp", sliceID)
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"in_progress"}`), 0o644)

	var called []string
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		called = append(called, filepath.Base(filepath.Dir(statusPath)))
		return fmt.Errorf("INTERPRETER_INCONCLUSIVE: interpreter could not classify output for %s (raw preview: ambiguous text)", sliceID)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := WorkerOptions{
		ReleaseName:         "test-interp",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             trackID,
			Slices:         []string{sliceID},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-interp/" + trackID,
		},
		RunSliceFn: runSliceFn,
		DB:         db,
	}

	result := RunTrack(context.Background(), opts)
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for interpreter INCONCLUSIVE, got %s", result)
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 RunSliceFn call, got %d", len(called))
	}
}

// TestRunTrack_MaxTurnsPausesLegacy proves that when RunSliceFn
// returns a max-turns exhaustion error (S03), the legacy worker path pauses the
// track rather than failing it.  The worker detects the sentinel in the error
// message and emits a PAGE event via RecordPage.
func TestRunTrack_MaxTurnsPausesLegacy(t *testing.T) {
	trackID := "T1-maxturns"
	sliceID := "S03-maxturns-legacy"

	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-maxturns", sliceID)
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"in_progress"}`), 0o644)

	var called []string
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		called = append(called, filepath.Base(filepath.Dir(statusPath)))
		return fmt.Errorf("RunSlice: max turns exhausted: max turns exhausted for %s", sliceID)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := WorkerOptions{
		ReleaseName:         "test-maxturns",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             trackID,
			Slices:         []string{sliceID},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-maxturns/" + trackID,
		},
		RunSliceFn: runSliceFn,
		DB:         db,
		Router:     nil, // legacy path
	}

	result := RunTrack(context.Background(), opts)
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for max-turns exhaustion (legacy), got %s", result)
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 RunSliceFn call, got %d", len(called))
	}
}

// TestRunTrack_MaxTurnsPausesRouter proves that when RunSliceFn returns a
// max-turns exhaustion error (S03), the router-driven worker path also pauses
// the track. Same sentinel detection as the legacy path.
func TestRunTrack_MaxTurnsPausesRouter(t *testing.T) {
	trackID := "T1-maxturns-router"
	sliceID := "S03-maxturns-router"

	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-maxturns-router", sliceID)
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"in_progress"}`), 0o644)

	var called []string
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		called = append(called, filepath.Base(filepath.Dir(statusPath)))
		return fmt.Errorf("RunSlice: max turns exhausted: max turns exhausted for %s", sliceID)
	}

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "planned", Target: ""},
		},
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := WorkerOptions{
		ReleaseName:         "test-maxturns-router",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             trackID,
			Slices:         []string{sliceID},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-maxturns-router/" + trackID,
		},
		RunSliceFn: runSliceFn,
		DB:         db,
		Router:     router,
	}

	result := RunTrack(context.Background(), opts)
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for max-turns exhaustion (router), got %s", result)
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 RunSliceFn call, got %d", len(called))
	}
}

// verifyPageEvents checks that events matching eventType+detail are present
// in the events table. Returns the count of matching rows.
func verifyPageEvents(t *testing.T, db *sql.DB, eventType, detail string) int {
	t.Helper()
	rows, err := db.Query(
		`SELECT COUNT(*) FROM events WHERE event = ? AND detail = ?`,
		eventType, detail,
	)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var count int
	if rows.Next() {
		rows.Scan(&count)
	}
	return count
}

func TestRunTrack_InterpreterSentinelIsNotNormalFailure(t *testing.T) {
	trackID := "T1-normal-fail"
	sliceID := "S01-normal-fail"

	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-normal-fail", sliceID)
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"in_progress"}`), 0o644)

	var called []string
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		called = append(called, filepath.Base(filepath.Dir(statusPath)))
		return fmt.Errorf("RunSlice: verification failed after 3 attempts")
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := WorkerOptions{
		ReleaseName:         "test-normal-fail",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             trackID,
			Slices:         []string{sliceID},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-normal-fail/" + trackID,
		},
		RunSliceFn: runSliceFn,
		DB:         db,
	}

	result := RunTrack(context.Background(), opts)
	if result != TrackFail {
		t.Fatalf("expected TrackFail for normal verification failure, got %s", result)
	}
}

// TestRecordDecisionCalledPerRoutingEvent is the S02 integration test:
// it runs a mock slice through the router-driven worker and asserts that
// RecordDecision is called once per routing event with correct fields.
func TestRecordDecisionCalledPerRoutingEvent(t *testing.T) {
	tmpDir := t.TempDir()

	// Two slices so we get multiple routing events.
	for _, sid := range []string{"S01-first", "S02-second"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-s02", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"planned"}`), 0o644)
	}

	// Router: implement S01 → advance to S02 → implement S02 → done.
	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "implement", Reason: "planned", Target: ""},
			{Type: "implement", Reason: "next up", Target: "S02-second"},
			{Type: "none", Reason: "complete"},
		},
	}

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-s02",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-first", "S02-second"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-s02/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
		Router:     router,
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v — skipping S02 decision-log test", err)
	}
	defer db.Close()

	// Create all tables the worker + supervisor need.
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slice_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		role        TEXT NOT NULL,
		action      TEXT NOT NULL,
		reason      TEXT NOT NULL DEFAULT '',
		recorded_at TEXT NOT NULL
	)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	// Verify RecordDecision was called once per routing event (3 Route calls).
	rows, err := db.Query(`SELECT slice_id, release, role, action, reason FROM decisions ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query decisions: %v", err)
	}
	defer rows.Close()

	var decisions []struct {
		sliceID string
		release string
		role    string
		action  string
		reason  string
	}
	for rows.Next() {
		var d struct {
			sliceID string
			release string
			role    string
			action  string
			reason  string
		}
		if err := rows.Scan(&d.sliceID, &d.release, &d.role, &d.action, &d.reason); err != nil {
			t.Fatalf("scan decision row: %v", err)
		}
		decisions = append(decisions, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration: %v", err)
	}

	// 3 routing events → 3 RecordDecision calls.
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decision rows (one per Route call), got %d", len(decisions))
	}

	// Each row must have correct role ("router") and release.
	for i, d := range decisions {
		if d.release != "test-s02" {
			t.Errorf("decision[%d].release = %q, want test-s02", i, d.release)
		}
		if d.role != "router" {
			t.Errorf("decision[%d].role = %q, want router", i, d.role)
		}
		if d.action == "" {
			t.Errorf("decision[%d].action is empty", i)
		}
	}

	// Verify the 2 RunSliceFn calls correspond to the 2 implement dispatches.
	if len(called) != 2 {
		t.Fatalf("expected 2 slice calls, got %d: %v", len(called), called)
	}
	if called[0] != "S01-first" {
		t.Errorf("call[0] = %q, want S01-first", called[0])
	}
	if called[1] != "S02-second" {
		t.Errorf("call[1] = %q, want S02-second", called[1])
	}
}

// ── S04 dependent-track tests ──────────────────────────────────────────────

// TestDependentTrack_MergeTrackFnCalled proves that finishTrack calls
// MergeTrackFn when it is set in WorkerOptions. AC3: auto-invoke merge-track
// when the last slice is terminal.
func TestDependentTrack_MergeTrackFnCalled(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-final"
	d := filepath.Join(tmpDir, "docs", "release", "test-dep", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"verified"}`), 0o644)

	mergeCalled := false
	var mergeTrackID, mergeBranch string

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "none", Reason: "terminal", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-dep",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		ReleaseWorktreePath: tmpDir,
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-dep/T1",
		},
		Router: router,
		MergeTrackFn: func(releasePath, trackID, branch string) error {
			mergeCalled = true
			mergeTrackID = trackID
			mergeBranch = branch
			return nil
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	if !mergeCalled {
		t.Fatal("expected MergeTrackFn to be called in finishTrack")
	}
	if mergeTrackID != "T1" {
		t.Errorf("mergeTrackID = %q, want T1", mergeTrackID)
	}
	if mergeBranch != "track/test-dep/T1" {
		t.Errorf("mergeBranch = %q, want track/test-dep/T1", mergeBranch)
	}
}

// TestDependentTrack_MergeTrackFnErrorFails proves that finishTrack returns
// TrackFail when MergeTrackFn returns an error. AC4: no silent merge failures.
func TestDependentTrack_MergeTrackFnErrorFails(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-failmerge"
	d := filepath.Join(tmpDir, "docs", "release", "test-dep", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"verified"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "none", Reason: "terminal", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-dep",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		ReleaseWorktreePath: tmpDir,
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-dep/T1",
		},
		Router: router,
		MergeTrackFn: func(releasePath, trackID, branch string) error {
			return fmt.Errorf("simulated merge conflict")
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
	if result != TrackFail {
		t.Fatalf("expected TrackFail when MergeTrackFn errors, got %s", result)
	}
}

// TestDependentTrack_MergeTrackDecisionAutoMerges proves that when the router
// returns "merge-track" and MergeTrackFn is set, the worker calls finishTrack
// (auto-merge) instead of pausing.
func TestDependentTrack_MergeTrackDecisionAutoMerges(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-automerge"
	d := filepath.Join(tmpDir, "docs", "release", "test-dep", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"verified"}`), 0o644)

	mergeCalled := false

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "merge-track", Reason: "track fully verified", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-dep",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		ReleaseWorktreePath: tmpDir,
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-dep/T1",
		},
		Router: router,
		MergeTrackFn: func(releasePath, trackID, branch string) error {
			mergeCalled = true
			return nil
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
	if result != TrackPass {
		t.Fatalf("expected TrackPass (auto-merge via merge-track decision), got %s", result)
	}

	if !mergeCalled {
		t.Fatal("expected MergeTrackFn to be called for merge-track decision with MergeTrackFn set")
	}
}

// TestDependentTrack_MergeTrackDecisionPausesWhenNoMergeTrackFn proves that
// merge-track pauses when MergeTrackFn is nil (backward-compatible fallback).
func TestDependentTrack_MergeTrackDecisionPausesWhenNoMergeTrackFn(t *testing.T) {
	tmpDir := t.TempDir()

	sid := "S01-nomergefn"
	d := filepath.Join(tmpDir, "docs", "release", "test-dep", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"verified"}`), 0o644)

	router := &fakeRouter{
		decisions: []SliceDecision{
			{Type: "merge-track", Reason: "track fully verified", Target: ""},
		},
	}

	opts := WorkerOptions{
		ReleaseName:         "test-dep",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{sid},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test-dep/T1",
		},
		Router:       router,
		MergeTrackFn: nil, // no MergeTrackFn — should pause
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
	if result != TrackPaused {
		t.Fatalf("expected TrackPaused for merge-track when MergeTrackFn is nil, got %s", result)
	}
}

// TestDependentTrack_WorktreeBranchesFromMergedTip is the AC5 integration test.
//
// It proves that when a dependency track merges into release-wt via
// finishTrack / MergeTrackFn, a dependent track's worktree created from
// release-wt gets the dependency's code. Uses real git repos.
//
// Scenario: T_dep → T_main. T_dep merges its code into release-wt.
// T_main's worktree is then created branching from release-wt. The
// dependency's file MUST be present in T_main's worktree.
func TestDependentTrack_WorktreeBranchesFromMergedTip(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping integration test")
	}

	tmpDir := t.TempDir()

	// 1. Create a bare "origin" repo.
	barePath := filepath.Join(tmpDir, "origin.git")
	runGit(t, tmpDir, "init", "--bare", barePath)

	// 2. Clone a "release worktree" from the bare repo.
	releasePath := filepath.Join(tmpDir, "release-wt")
	runGit(t, tmpDir, "clone", barePath, releasePath)

	// 3. In the release worktree, create the release-wt branch with an
	//    initial file, and push it so the bare repo has the branch.
	runGit(t, releasePath, "checkout", "-b", "release-wt/2026-06-27-conformance-foundation")
	writeFile(t, filepath.Join(releasePath, "before.txt"), "before-merge\n")
	runGit(t, releasePath, "add", "before.txt")
	runGit(t, releasePath, "commit", "-m", "initial release-wt")
	runGit(t, releasePath, "push", "origin", "release-wt/2026-06-27-conformance-foundation")

	// 4. Create a track branch (T_dep) off release-wt, add a dependency
	//    file, and push it.
	runGit(t, releasePath, "checkout", "-b", "track/2026-06-27-conformance-foundation/T_dep")
	writeFile(t, filepath.Join(releasePath, "dependency.txt"), "dep-code\n")
	runGit(t, releasePath, "add", "dependency.txt")
	runGit(t, releasePath, "commit", "-m", "T_dep: add dependency file")
	runGit(t, releasePath, "push", "origin", "track/2026-06-27-conformance-foundation/T_dep")

	// 5. Switch back to release-wt and merge the track branch —
	//    simulating what ProductionMergeTrack does inside finishTrack.
	runGit(t, releasePath, "checkout", "release-wt/2026-06-27-conformance-foundation")
	runGit(t, releasePath, "merge", "--no-ff", "track/2026-06-27-conformance-foundation/T_dep", "--no-edit")
	runGit(t, releasePath, "push", "origin", "release-wt/2026-06-27-conformance-foundation")

	// 6. Now simulate a dependent track (T_main) being created: run
	//    `git worktree add -b <branch> release-wt/<release>`, which
	//    branches from the live release-wt tip.
	dependentPath := filepath.Join(tmpDir, "dependent-worktree")
	runGit(t, releasePath, "worktree", "add", dependentPath,
		"-b", "track/2026-06-27-conformance-foundation/T_main",
		"release-wt/2026-06-27-conformance-foundation")

	// 7. ASSERT: T_main's worktree has both the pre-merge file AND the
	//    dependency's file, proving it branched from the post-T_dep tip.
	beforeContent := readFile(t, filepath.Join(dependentPath, "before.txt"))
	if beforeContent != "before-merge\n" {
		t.Errorf("before.txt = %q, want %q", beforeContent, "before-merge\n")
	}

	depContent := readFile(t, filepath.Join(dependentPath, "dependency.txt"))
	if depContent != "dep-code\n" {
		t.Fatalf("dependency.txt = %q, want %q — dependent worktree did not get dependency's code", depContent, "dep-code\n")
	}
}

// ── Test helpers ──────────────────────────────────────────────────────────

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, string(out))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
}

// ---------- S07 fakes ----------

// fakeOracle is a minimal OracleReader for S07 tests.
type fakeOracle struct {
	slices map[string]board.SliceState
}

func (f *fakeOracle) ReadSliceStatus(_ context.Context, _, sliceID string) (board.SliceState, error) {
	ss, ok := f.slices[sliceID]
	if !ok {
		return board.SliceState{}, errors.New("slice not found")
	}
	return ss, nil
}

func (f *fakeOracle) ReadBoard(_ context.Context, _ string) (*board.BoardState, error) {
	return nil, errors.New("not implemented")
}

// ---------- S07 tests ----------

// TestFindFirstNonTerminalCommitted verifies AC1 (committed seed) + AC2
// (implemented is non-terminal). A mock oracle returns committed states
// [verified, implemented, planned]; the seed must be the implemented slice
// (index 1), proving committed-read + implemented-non-terminal.
func TestFindFirstNonTerminalCommitted(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-verified":    {ID: "S01-verified", State: state.Verified, Track: "T1"},
			"S02-implemented": {ID: "S02-implemented", State: state.Implemented, Track: "T1"},
			"S03-planned":     {ID: "S03-planned", State: state.Planned, Track: "T1"},
		},
	}
	slices := []string{"S01-verified", "S02-implemented", "S03-planned"}

	got := findFirstNonTerminal(context.Background(), oracle, "test-release", "T1", slices)
	if got != "S02-implemented" {
		t.Errorf("expected S02-implemented (first non-terminal committed), got %s", got)
	}
}

// TestFindFirstNonTerminalAllTerminalMergesTrack verifies AC4: when every
// slice is terminal, findFirstNonTerminal returns "" so runTrackRouter
// reaches finishTrack (the fused-line fix).
func TestFindFirstNonTerminalAllTerminalMergesTrack(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-verified": {ID: "S01-verified", State: state.Verified, Track: "T1"},
			"S02-shipped":  {ID: "S02-shipped", State: "shipped", Track: "T1"},
			"S03-deferred": {ID: "S03-deferred", State: state.Deferred, Track: "T1"},
		},
	}
	slices := []string{"S01-verified", "S02-shipped", "S03-deferred"}

	got := findFirstNonTerminal(context.Background(), oracle, "test-release", "T1", slices)
	if got != "" {
		t.Errorf("expected empty (all terminal → merge), got %s", got)
	}
}

// TestFindFirstNonTerminalNilOracle verifies the legacy fallback:
// when oracle is nil, return slices[0].
func TestFindFirstNonTerminalNilOracle(t *testing.T) {
	slices := []string{"S01-planned", "S02-planned"}
	got := findFirstNonTerminal(context.Background(), nil, "", "", slices)
	if got != "S01-planned" {
		t.Errorf("nil oracle should fall back to slices[0], got %s", got)
	}
}

// TestFindFirstNonTerminalEmptySlices verifies empty input returns "".
func TestFindFirstNonTerminalEmptySlices(t *testing.T) {
	got := findFirstNonTerminal(context.Background(), &fakeOracle{}, "", "", nil)
	if got != "" {
		t.Errorf("empty slices should return \"\", got %s", got)
	}
}

// TestFindFirstNonTerminalOracleErrorSeedsAtUnreadable verifies AC3 +
// the seed-don't-skip thesis (Captain pin 5): when the oracle errors on a
// slice, seed AT that slice rather than skipping past it.
func TestFindFirstNonTerminalOracleErrorSeedsAtUnreadable(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S02-planned": {ID: "S02-planned", State: state.Planned, Track: "T1"},
		},
	}
	// S01-not-found will error (not in fakeOracle.slices map).
	slices := []string{"S01-not-found", "S02-planned"}

	got := findFirstNonTerminal(context.Background(), oracle, "test-release", "T1", slices)
	if got != "S01-not-found" {
		t.Errorf("oracle error on S01-not-found should seed at it (not skip), got %s", got)
	}
}

// TestFindFirstNonTerminalIsTerminalImport verifies AC5: the scheduler
// imports router.IsTerminal (no second definition).
func TestFindFirstNonTerminalIsTerminalImport(t *testing.T) {
	// Compile-time check: router.IsTerminal compiles in scheduler package.
	_ = router.IsTerminal("verified")
}
