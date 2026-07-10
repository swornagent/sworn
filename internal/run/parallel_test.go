package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	_ "modernc.org/sqlite"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/scheduler"
) // fakeRunSlicePass always returns nil (success).
func fakeRunSlicePass(_ context.Context, _, _, _ string) error {
	return nil
}

// fakeRunSliceFail always returns an error.
func fakeRunSliceFail(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("simulated slice failure")
}

// fakeRunSliceTrackFail fails for a specific track by examining the worktree
// root path for a track-specific marker.
func fakeRunSliceTrackFail(failTrackID string) func(context.Context, string, string, string) error {
	return func(_ context.Context, _, specPath, _ string) error {
		// Extract slice ID from the spec path: .../<slice-id>/spec.md
		sliceParent := filepath.Base(filepath.Dir(specPath))
		// If this slice belongs to the failing track (checked via filepath
		// markers), return error. The caller maps the slice-to-track relation.
		// We use a convention: slice IDs starting with the track prefix fail.
		if strings.HasPrefix(sliceParent, failTrackID) {
			return fmt.Errorf("simulated failure for track %s (slice %s)", failTrackID, sliceParent)
		}
		return nil
	}
}

// blockingRunSlice returns a RunSliceFn that signals on startCh when called
// and blocks on blockCh until instructed to continue.
func blockingRunSlice(startCh chan<- string, blockCh <-chan struct{}) func(context.Context, string, string, string) error {
	return func(ctx context.Context, _, specPath, _ string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		select {
		case startCh <- sliceID:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-blockCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func TestDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	if !dirExists(tmpDir) {
		t.Errorf("dirExists(%q) = false, want true", tmpDir)
	}

	nonExistent := filepath.Join(tmpDir, "nonexistent")
	if dirExists(nonExistent) {
		t.Errorf("dirExists(%q) = true, want false", nonExistent)
	}
}

func TestRunParallel_Basic(t *testing.T) {
	// Create a minimal fixture: index.md with 2 independent tracks.
	tmpDir := t.TempDir()

	// Create the release board directory.
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-parallel")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
title: Test Parallel
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: []
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: []
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts := ParallelOptions{
		ReleaseName:   "test-parallel",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    fakeRunSlicePass,
		ProjectDir:    "sworn",
	}

	err = RunParallel(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
}
func TestRunParallel_ReleaseWorktreePathMissing(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-missing")
	os.MkdirAll(releaseDir, 0o755)

	// No release_worktree_path in frontmatter.
	indexContent := `---
title: Test Missing
tracks: []
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	opts := ParallelOptions{
		ReleaseName:   "test-missing",
		WorkspaceRoot: tmpDir,
	}

	err := RunParallel(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing release_worktree_path, got nil")
	}
}

func TestRunParallel_NoTracks(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-notracks")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
title: Test No Tracks
release_worktree_path: ` + tmpDir + `
tracks: []
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	opts := ParallelOptions{
		ReleaseName:   "test-notracks",
		WorkspaceRoot: tmpDir,
	}

	err := RunParallel(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for no tracks, got nil")
	}
}

func TestRunParallel_MissingIndex(t *testing.T) {
	tmpDir := t.TempDir()

	opts := ParallelOptions{
		ReleaseName:   "test-nonexistent",
		WorkspaceRoot: tmpDir,
	}

	err := RunParallel(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing index.md, got nil")
	}
}

// TestRunParallel_FailureCascade exercises AC-3 semantic:
// T1 fails → T3 (depends_on T1) is skipped, T2 (independent) completes normally.
// Verifier Fix 3: wires fakeRunSliceFail into a real cascade test.
func TestRunParallel_FailureCascade(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-cascade")
	os.MkdirAll(releaseDir, 0o755)

	// Create slice dirs for T1, T2, T3 so worker can build paths.
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S01-t1-slice"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S01-t1-slice", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S01-t1-slice", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S02-t2-slice"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S02-t2-slice", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S02-t2-slice", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S03-t3-slice"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S03-t3-slice", "spec.md"), []byte("# t3"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-cascade", "S03-t3-slice", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := `---
title: Test Cascade
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: [S01-t1-slice]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: [S02-t2-slice]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
  - id: T3
    slices: [S03-t3-slice]
    depends_on: [T1]
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T3
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	// Use a RunSliceFn that fails for T1's slice only.
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		if sliceID == "S01-t1-slice" {
			return fmt.Errorf("simulated T1 failure")
		}
		return nil
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-cascade",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    runSliceFn,
		ProjectDir:    "sworn",
	}

	// T1 fails → RunParallel should return an error.
	err = RunParallel(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when T1 fails (cascade), got nil")
	}
	if !strings.Contains(err.Error(), "T1") {
		t.Errorf("error should mention T1, got: %v", err)
	}

	// ── S07 AC-03: no phase-wide cascade cancel ─────────────────────────
	// T1 and T2 share phase 1 (neither depends_on anything); T3 sits alone
	// in phase 2 (depends_on: [T1]). T1's failure must not cancel its
	// same-phase sibling T2 — RunTrack runs on the parent ctx, not
	// phaseCtx (#33) — while T3, the actual *dependent*, is skipped via the
	// phaseCtx.Err() check at phase-2 launch. The design.md draft claimed
	// this base test already asserted the T3-skip half; it does not (Captain
	// review pin 3) — both outcomes are asserted here, read from the durable
	// per-run loop log (RunParallel's "[<track>] result: <OUTCOME>" lines),
	// since RunParallel's return value only reports failedTracks by design.
	logPath := filepath.Join(tmpDir, ".sworn", "logs", "test-cascade", "loop.log")
	logBytes, logErr := os.ReadFile(logPath)
	if logErr != nil {
		t.Fatalf("read loop log %s: %v", logPath, logErr)
	}
	logContent := string(logBytes)
	if !strings.Contains(logContent, "[T2] result: PASS") {
		t.Errorf("expected T2 (independent same-phase sibling of failed T1) to reach TrackPass — no phase-wide cascade cancel; loop log:\n%s", logContent)
	}
	if !strings.Contains(logContent, "[T3] result: SKIPPED") {
		t.Errorf("expected T3 (depends_on: [T1], the actual dependent) to reach TrackSkipped; loop log:\n%s", logContent)
	}
}

// TestRunParallel_TimingConcurrency exercises AC-1:
// Two independent tracks start before either completes.
// Verifier Fix 4: uses blocking fake workers with channels to prove concurrency.
func TestRunParallel_TimingConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-concurrent")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S01-t1"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S01-t1", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S01-t1", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S02-t2"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S02-t2", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-concurrent", "S02-t2", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := `---
title: Test Concurrent
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: [S01-t1]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: [S02-t2]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	// Use blocking workers with channel synchronisation.
	startCh := make(chan string, 2)
	blockCh := make(chan struct{})

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-concurrent",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    blockingRunSlice(startCh, blockCh),
		ProjectDir:    "sworn",
	}

	// Run Parallel in a goroutine (it will block on the channel).
	var wg sync.WaitGroup
	wg.Add(1)
	var parallelErr error
	go func() {
		defer wg.Done()
		parallelErr = RunParallel(context.Background(), opts)
	}()

	// Wait for BOTH tracks to signal they've started their slice.
	// Use a timeout to avoid deadlocking if only one starts.
	seen := make(map[string]bool)
	timeout := time.After(5 * time.Second)

	for len(seen) < 2 {
		select {
		case id := <-startCh:
			seen[id] = true
		case <-timeout:
			t.Fatalf("timed out waiting for both tracks to start; seen %v", seen)
		}
	}

	// Now both have started — this proves concurrency (AC-1).
	// Release them to complete.
	close(blockCh)
	wg.Wait()

	if parallelErr != nil {
		t.Fatalf("RunParallel: %v", parallelErr)
	}
}

// TestRunParallel_DependentTrackRunsAfterSuccess exercises AC-2 success path:
// T1 (phase 0) passes → T2 (depends_on T1, phase 1) must run and pass.
// Verifier Fix 2: prior tests only covered the failure cascade; this proves
// the success path where a dependent track actually RUNS after its dependency.
func TestRunParallel_DependentTrackRunsAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-dep-success")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S01-t1-slice"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S01-t1-slice", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S01-t1-slice", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S02-t2-slice"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S02-t2-slice", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-dep-success", "S02-t2-slice", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := `---
title: Test Dep Success
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: [S01-t1-slice]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: [S02-t2-slice]
    depends_on: [T1]
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	// Track which tracks had RunSliceFn called.
	var mu sync.Mutex
	called := make(map[string]bool)

	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		mu.Lock()
		called[sliceID] = true
		mu.Unlock()
		return nil
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-dep-success",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    runSliceFn,
		ProjectDir:    "sworn",
	}

	err = RunParallel(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunParallel: expected nil (all pass), got: %v", err)
	}

	// Assert T2's slice was actually called (not skipped).
	if !called["S02-t2-slice"] {
		t.Error("T2's slice (S02-t2-slice) was not called — dependent track was skipped despite T1 passing")
	}
	if !called["S01-t1-slice"] {
		t.Error("T1's slice (S01-t1-slice) was not called")
	}
}

// pausingRouter is a fake scheduler.SliceRouter that always returns
// coach_decision (a human-gated pause state) for use in parallel tests.
type pausingRouter struct{}

func (p *pausingRouter) Route(_ context.Context, _, _, _ string) (scheduler.SliceDecision, error) {
	return scheduler.SliceDecision{Type: "coach_decision", Reason: "needs Coach approval"}, nil
}

// TestRunParallel_TrackPaused exercises the TrackPaused path through RunParallel.
// AC-6: a paused track must yield non-zero exit (RunParallel must return error).
func TestRunParallel_TrackPaused(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-paused")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-paused", "S01-pause"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-paused", "S01-pause", "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-paused", "S01-pause", "status.json"), []byte(`{"state":"planned"}`), 0o644)

	indexContent := `---
title: Test Paused
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: [S01-pause]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-paused",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    fakeRunSlicePass,
		ProjectDir:    "sworn",
		Router:        &pausingRouter{},
	}

	err = RunParallel(context.Background(), opts)
	if err == nil {
		t.Fatal("expected non-zero error for paused track (AC-6), got nil")
	}
	if !strings.Contains(err.Error(), "paused") {
		t.Errorf("error should mention 'paused', got: %v", err)
	}
	if !strings.Contains(err.Error(), "T1") {
		t.Errorf("error should mention 'T1', got: %v", err)
	}
}

// ── Invariant-2 tests (S06) ─────────────────────────────────────────────────

// fakePlannedFilesFn returns a PlannedFilesFn that maps track IDs to their
// planned files. Missing tracks get empty slices (fail open).
func fakePlannedFilesFn(files map[string][]string) func(context.Context, string) ([]string, error) {
	return func(_ context.Context, trackID string) ([]string, error) {
		if f, ok := files[trackID]; ok {
			return f, nil
		}
		return nil, nil
	}
}

// TestInvariant2_OverlapBlocksSecondTrack exercises AC-1: two tracks with
// overlapping planned_files → the second track is blocked at dispatch time
// with the INVARIANT-2 message, then retried in the follow-up phase after
// the first track completes. The retry succeeds (no conflict), so RunParallel
// returns nil. The test captures stderr to verify the INVARIANT-2 message
// was emitted.
func TestInvariant2_OverlapBlocksSecondTrack(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S01-t1"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S01-t1", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S01-t1", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S02-t2"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S02-t2", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-overlap", "S02-t2", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := "---\n" +
		"title: Test Invariant-2 Overlap\n" +
		"release_worktree_path: " + tmpDir + "\n" +
		"tracks:\n" +
		"  - id: T1\n" +
		"    slices: [S01-t1]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T1\n" +
		"    state: planned\n" +
		"  - id: T2\n" +
		"    slices: [S02-t2]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T2\n" +
		"    state: planned\n" +
		"---\n\n" +
		"# Test\n"
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	// Capture stderr to verify the INVARIANT-2 message.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	opts := ParallelOptions{
		ReleaseName:   "test-inv2-overlap",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    fakeRunSlicePass,
		ProjectDir:    "sworn",
		PlannedFilesFn: fakePlannedFilesFn(map[string][]string{
			"T1": {"internal/run/parallel.go"},
			"T2": {"internal/run/parallel.go"},
		}),
	}

	err = RunParallel(context.Background(), opts)

	// Restore stderr and read captured output.
	w.Close()
	var stderrBuf strings.Builder
	io.Copy(&stderrBuf, r)
	os.Stderr = origStderr
	stderr := stderrBuf.String()

	// RunParallel should return nil — T1 runs, T2 is blocked initially,
	// then retried in the follow-up phase and succeeds.
	if err != nil {
		t.Fatalf("RunParallel: expected nil (T2 retried and passed), got: %v", err)
	}

	// AC-1: verify the INVARIANT-2 message was logged.
	if !strings.Contains(stderr, "INVARIANT-2") {
		t.Error("stderr should contain 'INVARIANT-2'")
	}
	// Captain pin 4: assert shared prefix through "both write" so message
	// and test can't drift.
	if !strings.Contains(stderr, "both write") {
		t.Errorf("stderr should contain shared prefix 'both write', got: %s", stderr)
	}
	if !strings.Contains(stderr, "T2") {
		t.Errorf("stderr should mention T2 (blocked track), got: %s", stderr)
	}
	if !strings.Contains(stderr, "internal/run/parallel.go") {
		t.Errorf("stderr should mention the overlapping file, got: %s", stderr)
	}
}

// TestInvariant2_NoOverlapBothRun exercises: disjoint planned_files → both
// tracks launch and pass.
func TestInvariant2_NoOverlapBothRun(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S01-t1"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S01-t1", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S01-t1", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S02-t2"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S02-t2", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-no-overlap", "S02-t2", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := "---\n" +
		"title: Test Invariant-2 No Overlap\n" +
		"release_worktree_path: " + tmpDir + "\n" +
		"tracks:\n" +
		"  - id: T1\n" +
		"    slices: [S01-t1]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T1\n" +
		"    state: planned\n" +
		"  - id: T2\n" +
		"    slices: [S02-t2]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T2\n" +
		"    state: planned\n" +
		"---\n\n" +
		"# Test\n"
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	var mu sync.Mutex
	ran := make(map[string]bool)
	runSliceFn := func(_ context.Context, _, specPath, _ string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		mu.Lock()
		ran[sliceID] = true
		mu.Unlock()
		return nil
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-inv2-no-overlap",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    runSliceFn,
		ProjectDir:    "sworn",
		PlannedFilesFn: fakePlannedFilesFn(map[string][]string{
			"T1": {"internal/run/parallel.go"},
			"T2": {"internal/run/slice.go"},
		}),
	}

	err = RunParallel(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunParallel: expected nil (disjoint files), got: %v", err)
	}

	if !ran["S01-t1"] {
		t.Error("T1's slice was not called")
	}
	if !ran["S02-t2"] {
		t.Error("T2's slice was not called")
	}
}

// TestInvariant2_DocumentedSharedExempt exercises AC-3: overlapping
// planned_files that are in the DOCUMENTED SHARED list → both tracks launch.
func TestInvariant2_DocumentedSharedExempt(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S01-t1"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S01-t1", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S01-t1", "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S02-t2"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S02-t2", "spec.md"), []byte("# t2"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-docshared", "S02-t2", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := "---\n" +
		"title: Test Invariant-2 Documented Shared\n" +
		"release_worktree_path: " + tmpDir + "\n" +
		"tracks:\n" +
		"  - id: T1\n" +
		"    slices: [S01-t1]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T1\n" +
		"    state: planned\n" +
		"  - id: T2\n" +
		"    slices: [S02-t2]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T2\n" +
		"    state: planned\n" +
		"---\n\n" +
		"# Test\n\n" +
		"## Touchpoint matrix\n\n" +
		"| File | T1 | T2 |\n" +
		"|------|----|----|\n" +
		"| " + "`" + "internal/model/oai.go" + "`" + " | ✓ (DOCUMENTED SHARED) | ✓ |\n"
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	var mu sync.Mutex
	ran := make(map[string]bool)
	runSliceFn := func(_ context.Context, _, specPath, _ string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		mu.Lock()
		ran[sliceID] = true
		mu.Unlock()
		return nil
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	opts := ParallelOptions{
		ReleaseName:   "test-inv2-docshared",
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    runSliceFn,
		ProjectDir:    "sworn",
		PlannedFilesFn: fakePlannedFilesFn(map[string][]string{
			"T1": {"internal/model/oai.go"},
			"T2": {"internal/model/oai.go"},
		}),
	}

	err = RunParallel(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunParallel: expected nil (documented shared exempt), got: %v", err)
	}

	if !ran["S01-t1"] {
		t.Error("T1's slice was not called")
	}
	if !ran["S02-t2"] {
		t.Error("T2's slice was not called")
	}
}

// TestInvariant2_DocumentedSharedFromRenderedBoard exercises S06 AC-03: a file
// declared as a touchpoint by two tracks is detected as documented-shared from a
// GENUINELY RENDERED index.md (built by board.Render from board.json + slice
// spec.json/status.json — NOT a hand-authored fixture), so the second track is
// NOT blocked by invariant-2. The rendered matrix marks the shared file with a
// ✓ under each owning track and carries NO explicit "(DOCUMENTED SHARED)"
// annotation — exactly the ≥2-checkmark case the deleted parseDocumentedSharedFiles
// silently dropped (it matched only the annotation). Delegating to
// router.ParseDocumentedShared closes the gap: the assertion is that no
// INVARIANT-2 block is emitted and both tracks run.
func TestInvariant2_DocumentedSharedFromRenderedBoard(t *testing.T) {
	tmpDir := t.TempDir()
	release := "test-inv2-rendered"
	relDir := filepath.Join(tmpDir, "docs", "release", release)
	os.MkdirAll(relDir, 0o755)

	sharedFile := "internal/shared/thing.go"

	// board.json: two independent tracks, one slice each.
	boardJSON := `{
  "$schema": "https://baton.sawy3r.net/schemas/board-v1.json",
  "schema_version": 1,
  "release": {"name": "` + release + `"},
  "release_worktree_path": "` + tmpDir + `",
  "tracks": [
    {"id": "T1-alpha", "slices": ["S01-alpha"], "worktree_path": "` + tmpDir + `", "worktree_branch": "track/test/T1-alpha", "state": "planned"},
    {"id": "T2-beta", "slices": ["S02-beta"], "worktree_path": "` + tmpDir + `", "worktree_branch": "track/test/T2-beta", "state": "planned"}
  ]
}`
	os.WriteFile(filepath.Join(relDir, "board.json"), []byte(boardJSON), 0o644)

	// Each slice declares the SAME touchpoint file — a genuine ≥2-track overlap.
	for _, s := range []struct{ id, track string }{
		{"S01-alpha", "T1-alpha"},
		{"S02-beta", "T2-beta"},
	} {
		sliceDir := filepath.Join(relDir, s.id)
		os.MkdirAll(sliceDir, 0o755)
		spec := `{"user_outcome": "outcome for ` + s.id + `", "touchpoints": ["` + sharedFile + `"], "effort_complexity": {"quadrant": "chore"}}`
		os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(spec), 0o644)
		os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte("# "+s.id), 0o644)
		os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(`{"state": "planned"}`), 0o644)
	}

	// Render index.md from the real renderer (NOT hand-authored markdown).
	if err := board.RenderToFile(tmpDir, release); err != nil {
		t.Fatalf("board.RenderToFile: %v", err)
	}
	// Guard: the rendered matrix must carry ≥2 checkmarks for the shared file and
	// NO explicit annotation — otherwise the test would pass via the old
	// explicit-marker path and prove nothing.
	rendered, _ := os.ReadFile(filepath.Join(relDir, "index.md"))
	if strings.Contains(string(rendered), "DOCUMENTED SHARED") {
		t.Fatalf("rendered index.md unexpectedly contains an explicit DOCUMENTED SHARED annotation; test would not exercise the ≥2-checkmark path")
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	// Capture stderr to prove NO invariant-2 block fires.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	var mu sync.Mutex
	ran := make(map[string]bool)
	runSliceFn := func(_ context.Context, _, specPath, _ string) error {
		sliceID := filepath.Base(filepath.Dir(specPath))
		mu.Lock()
		ran[sliceID] = true
		mu.Unlock()
		return nil
	}

	opts := ParallelOptions{
		ReleaseName:   release,
		WorkspaceRoot: tmpDir,
		DB:            db,
		RunSliceFn:    runSliceFn,
		ProjectDir:    "sworn",
		PlannedFilesFn: fakePlannedFilesFn(map[string][]string{
			"T1-alpha": {sharedFile},
			"T2-beta":  {sharedFile},
		}),
	}

	err = RunParallel(context.Background(), opts)

	w.Close()
	var stderrBuf strings.Builder
	io.Copy(&stderrBuf, r)
	os.Stderr = origStderr
	stderr := stderrBuf.String()

	if err != nil {
		t.Fatalf("RunParallel: expected nil (shared file documented via rendered ≥2-checkmark matrix), got: %v\nstderr:\n%s", err, stderr)
	}
	// AC-03: the shared file must NOT be silently dropped — no invariant-2 block.
	if strings.Contains(stderr, "INVARIANT-2") {
		t.Errorf("shared file %q was NOT recognised as documented-shared from the rendered matrix — invariant-2 wrongly blocked a track:\n%s", sharedFile, stderr)
	}
	if !ran["S01-alpha"] {
		t.Error("T1-alpha's slice was not called")
	}
	if !ran["S02-beta"] {
		t.Error("T2-beta's slice was not called")
	}
}

// TestInvariant2_OracleReadFailureFailsOpen exercises AC-4: when
// PlannedFilesFn returns an error, the track launches (fail open).
func TestInvariant2_OracleReadFailureFailsOpen(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-inv2-failopen")
	os.MkdirAll(releaseDir, 0o755)

	os.MkdirAll(filepath.Join(tmpDir, "docs", "release", "test-inv2-failopen", "S01-t1"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-failopen", "S01-t1", "spec.md"), []byte("# t1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "docs", "release", "test-inv2-failopen", "S01-t1", "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	indexContent := "---\n" +
		"title: Test Invariant-2 Fail Open\n" +
		"release_worktree_path: " + tmpDir + "\n" +
		"tracks:\n" +
		"  - id: T1\n" +
		"    slices: [S01-t1]\n" +
		"    depends_on: null\n" +
		"    worktree_path: " + tmpDir + "\n" +
		"    worktree_branch: track/test/T1\n" +
		"    state: planned\n" +
		"---\n\n" +
		"# Test\n"
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	errorPlannedFilesFn := func(_ context.Context, trackID string) ([]string, error) {
		return nil, fmt.Errorf("simulated oracle read failure")
	}

	opts := ParallelOptions{
		ReleaseName:    "test-inv2-failopen",
		WorkspaceRoot:  tmpDir,
		DB:             db,
		RunSliceFn:     fakeRunSlicePass,
		ProjectDir:     "sworn",
		PlannedFilesFn: errorPlannedFilesFn,
	}

	err = RunParallel(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunParallel: expected nil (fail open), got: %v", err)
	}
}

// TestRunParallel_AC06_RealReleaseBoardResolvesTracks is S06's AC-06
// reachability artefact (Coach-ratified option (a), see review.md): a Go-level
// run.RunParallel invocation against THIS repo's own multi-track current-format
// release board.json, proving the loop finds its tracks instead of erroring "no
// tracks found in release board" — the exact failure the old
// board.ParseTracks(extractFrontmatter(index.md)) path produced against any
// board.json-backed release.
//
// Isolation (no side effects on the real in-flight track worktrees, per the
// Coach decision): the live board.json is copied into a throwaway temp
// workspace with release_worktree_path redirected to that temp dir (so no real
// worktree is materialised or merged), a pausing router is injected (so no real
// /implement-slice dispatch runs and swornagent/sworn#46 is never reached), the
// planned-files reader is faked (no git), and the DB is in-memory.
func TestRunParallel_AC06_RealReleaseBoardResolvesTracks(t *testing.T) {
	const release = "2026-07-01-render-drift-reconciliation"
	liveBoard := filepath.Join("..", "..", "docs", "release", release, "board.json")
	raw, err := os.ReadFile(liveBoard)
	if os.IsNotExist(err) {
		t.Skipf("live board.json not found at %s — not running from the worktree", liveBoard)
	}
	if err != nil {
		t.Fatalf("read live board.json: %v", err)
	}

	// Copy the live board.json verbatim into an isolated temp workspace, then
	// redirect ONLY release_worktree_path to the temp dir (an existing directory)
	// so RunParallel skips worktree materialisation and never touches the real
	// release-wt. A generic map preserves every other field (the canonical release
	// object, nested per-track worktree records) exactly as committed.
	tmpDir := t.TempDir()
	relDir := filepath.Join(tmpDir, "docs", "release", release)
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal live board.json: %v", err)
	}
	doc["release_worktree_path"] = tmpDir
	// Redirect every track's worktree_path to the (existing) temp dir so the
	// pausing workers skip worktree materialisation entirely — no git op runs
	// against any real or temp path. This keeps the run purely about track
	// resolution (the AC-06 claim), not dispatch mechanics.
	if tracks, ok := doc["tracks"].([]interface{}); ok {
		for _, tr := range tracks {
			if tm, ok := tr.(map[string]interface{}); ok {
				tm["worktree_path"] = tmpDir
				delete(tm, "worktree")
			}
		}
	}
	patched, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal patched board.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(relDir, "board.json"), patched, 0o644); err != nil {
		t.Fatal(err)
	}
	// Copy the live index.md too (used by the documented-shared parse); absence
	// would only fail-open, but copying keeps the reachability run faithful.
	if idx, err := os.ReadFile(filepath.Join("..", "..", "docs", "release", release, "index.md")); err == nil {
		os.WriteFile(filepath.Join(relDir, "index.md"), idx, 0o644)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	opts := ParallelOptions{
		ReleaseName:    release,
		WorkspaceRoot:  tmpDir,
		DB:             db,
		RunSliceFn:     fakeRunSlicePass, // never reached — pausing router pauses first
		ProjectDir:     "sworn",
		Router:         &pausingRouter{},
		PlannedFilesFn: fakePlannedFilesFn(map[string][]string{}),
	}

	err = RunParallel(context.Background(), opts)

	w.Close()
	var stderrBuf strings.Builder
	io.Copy(&stderrBuf, r)
	os.Stderr = origStderr
	stderr := stderrBuf.String()

	// The whole point: the old frontmatter parser returned zero tracks here and
	// hard-errored "no tracks found in release board". The oracle read must load
	// all five tracks.
	if strings.Contains(stderr, "no tracks found") {
		t.Fatalf("RunParallel could not resolve tracks from the live board.json:\n%s", stderr)
	}
	if !strings.Contains(stderr, "loaded 5 tracks") {
		t.Errorf("expected 'loaded 5 tracks' from the live 5-track board.json; stderr:\n%s", stderr)
	}
	// A pausing router makes every track pause, so RunParallel returns a paused
	// error — that is the EXPECTED, side-effect-free outcome; track resolution
	// (the AC-06 claim) already happened before dispatch.
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Errorf("expected a 'paused' outcome from the injected pausing router, got: %v", err)
	}
	t.Logf("AC-06 reachability: live board.json resolved tracks — stderr:\n%s", stderr)
}

// TestProductionMergeTrack_LinkedWorktree proves the Rule 11 target assertion
// accepts a real `git worktree add` release worktree — whose .git is a FILE
// (gitdir pointer), not a directory — and that the merge actually engages.
// Regression for the dirExists(.git) guard inversion that silently no-op'd
// every production track auto-merge while finishTrack logged "auto-merged".
func TestProductionMergeTrack_LinkedWorktree(t *testing.T) {
	repo, _ := setupTestRepo(t)

	// Release worktree, bootstrapped the same way RunParallel does it.
	releasePath := filepath.Join(t.TempDir(), "release-wt")
	runCmd(t, repo, "git", "worktree", "add", "-b", "release-wt/r1", releasePath)

	// Track branch with one commit ahead of the release base.
	runCmd(t, repo, "git", "switch", "-c", "track/r1/T1")
	if err := os.WriteFile(filepath.Join(repo, "track.txt"), []byte("track work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, repo, "git", "add", "track.txt")
	runCmd(t, repo, "git", "commit", "-m", "track commit")
	trackHead := strings.TrimSpace(runCmd(t, repo, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, "git", "switch", "main")

	if err := ProductionMergeTrack(releasePath, "T1", "track/r1/T1"); err != nil {
		t.Fatalf("ProductionMergeTrack on linked worktree: %v", err)
	}

	// The track commit must now be an ancestor of the release worktree HEAD.
	cmd := exec.Command("git", "merge-base", "--is-ancestor", trackHead, "HEAD")
	cmd.Dir = releasePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("track commit %s not an ancestor of release HEAD: merge silently skipped", trackHead)
	}
}

// TestProductionMergeTrack_NonGitTargetErrors proves the target assertion
// fails closed: a merge target that is not a git worktree must return an
// error, never nil — a nil return is reported upstream as a successful merge.
func TestProductionMergeTrack_NonGitTargetErrors(t *testing.T) {
	if err := ProductionMergeTrack(t.TempDir(), "T1", "track/r1/T1"); err == nil {
		t.Fatal("expected error for non-git merge target, got nil")
	}
}
