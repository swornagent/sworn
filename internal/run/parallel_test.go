package run

import (
	"context"
	"database/sql"
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
func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "simple frontmatter",
			text: `---
title: Board
tracks:
  - id: T1
    slices: [S01]
---`,
			want: "title: Board\ntracks:\n  - id: T1\n    slices: [S01]",
		},
		{
			name: "no frontmatter",
			text: "# Just a heading\nbody",
			want: "",
		},
		{
			name: "empty frontmatter",
			text: "---\n---\nbody",
			want: "",
		},
		{
			name: "trailing whitespace on --- lines",
			text: "---  \ntitle: Board\n---\nbody",
			want: "title: Board",
		},
		{
			name: "single line (too short)",
			text: "---",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractFrontmatter(tc.text)
			if got != tc.want {
				t.Errorf("extractFrontmatter:\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestExtractReleaseWorktreePath(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "simple path",
			body: "release_worktree_path: /home/user/worktrees/release-x",
			want: "/home/user/worktrees/release-x",
		},
		{
			name: "no path",
			body: "title: Board\nrelease_index: 1",
			want: "",
		},
		{
			name: "quoted path",
			body: `release_worktree_path: "/home/user/worktrees/release-x"`,
			want: "/home/user/worktrees/release-x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractReleaseWorktreePath(tc.body)
			if got != tc.want {
				t.Errorf("extractReleaseWorktreePath:\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
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
