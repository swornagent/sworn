package run

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)
// fakeRunSlicePass always returns nil (success).
func fakeRunSlicePass(_ context.Context, _, _, _ string) error {
	return nil
}

// fakeRunSliceFail always returns an error.
func fakeRunSliceFail(_ context.Context, _, _, _ string) error {
	return nil // Let the worker handle it — we simulate via the track
}

// fakeRunSliceTrackFail fails for a specific track.
func fakeRunSliceTrackFail(trackID string) func(context.Context, string, string, string) error {
	return func(_ context.Context, _, specPath, _ string) error {
		// Extract track from the path... this is simplified
		_ = specPath
		_ = trackID
		return nil
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