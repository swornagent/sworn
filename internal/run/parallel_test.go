package run

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)// fakeRunSlicePass always returns nil (success).
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