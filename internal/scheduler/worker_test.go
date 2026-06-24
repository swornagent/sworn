package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/swornagent/sworn/internal/board"
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
			ackPath := filepath.Join(workRoot, filepath.Dir(specPath), "approved-ack.md")
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
	// AC-3: redesign decision removes approved-ack.md before re-dispatching.
	tmpDir := t.TempDir()

	sid := "S01-redesign"
	d := filepath.Join(tmpDir, "docs", "release", "test-release", sid)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"failed_verification"}`), 0o644)
	os.WriteFile(filepath.Join(d, "approved-ack.md"), []byte("approved"), 0o644)

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
		t.Error("expected approved-ack.md to be removed after redesign decision")
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