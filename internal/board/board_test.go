package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupReleaseDir creates a minimal release directory structure inside tmpdir
// with an index.md containing the given frontmatter body. Returns the repo root
// (tmpdir) and the release name.
func setupReleaseDir(t *testing.T, release, fmBody string) string {
	t.Helper()
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "docs", "release", release)
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	indexContent := "---\n" + fmBody + "\n---\n# rest of file"
	if err := os.WriteFile(filepath.Join(docsDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
	return tmp
}

func baseFrontmatter() string {
	return `release_benefit: test release
release_worktree_path: /tmp/test-release
release_worktree_branch: release-wt/test-release
tracks:
  - id: T1-core
    slices: [S01-alpha, S02-beta]
    depends_on: null
    worktree_path: /tmp/test-release-T1
    worktree_branch: track/r/T1-core
    state: in_progress
  - id: T2-aux
    slices: [S03-gamma]
    depends_on: T1-core
    worktree_path: /tmp/test-release-T2
    worktree_branch: track/r/T2-aux
    state: planned`
}

func TestReadBoard_LazyMigration(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	br, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	// Verify BoardRecord from lazy migration.
	if br.Release.Name != "test-release" {
		t.Errorf("release: got %q, want %q", br.Release.Name, "test-release")
	}
	if br.SchemaVersion != 1 {
		t.Errorf("schema_version: got %d, want 1", br.SchemaVersion)
	}
	if br.ReleaseWorktreePath != "/tmp/test-release" {
		t.Errorf("release_worktree_path: got %q", br.ReleaseWorktreePath)
	}
	if br.ReleaseWorktreeBranch != "release-wt/test-release" {
		t.Errorf("release_worktree_branch: got %q", br.ReleaseWorktreeBranch)
	}

	if len(br.Tracks) != 2 {
		t.Fatalf("tracks: got %d, want 2", len(br.Tracks))
	}

	t1 := br.Tracks[0]
	if t1.ID != "T1-core" {
		t.Errorf("T1 id: got %q", t1.ID)
	}
	if len(t1.Slices) != 2 || t1.Slices[0] != "S01-alpha" || t1.Slices[1] != "S02-beta" {
		t.Errorf("T1 slices: got %v", t1.Slices)
	}
	if t1.State != "in_progress" {
		t.Errorf("T1 state: got %q", t1.State)
	}
	if t1.WorktreeBranch != "track/r/T1-core" {
		t.Errorf("T1 worktree_branch: got %q", t1.WorktreeBranch)
	}

	t2 := br.Tracks[1]
	if t2.ID != "T2-aux" {
		t.Errorf("T2 id: got %q", t2.ID)
	}
	if len(t2.DependsOn) != 1 || t2.DependsOn[0] != "T1-core" {
		t.Errorf("T2 depends_on: got %v", t2.DependsOn)
	}
	if t2.State != "planned" {
		t.Errorf("T2 state: got %q", t2.State)
	}

	// Verify board.json was written to disk.
	boardPath := filepath.Join(repoRoot, "docs", "release", "test-release", "board.json")
	data, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("board.json not written: %v", err)
	}

	var br2 BoardRecord
	if err := json.Unmarshal(data, &br2); err != nil {
		t.Fatalf("parse board.json: %v", err)
	}
	if br2.SchemaVersion != 1 {
		t.Errorf("on-disk schema_version: got %d", br2.SchemaVersion)
	}
	if len(br2.Tracks) != 2 {
		t.Errorf("on-disk tracks: got %d", len(br2.Tracks))
	}
}

func TestReadBoard_ExistingBoardJSON(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	// Pre-write a board.json that differs from index.md frontmatter.
	br := &BoardRecord{
		SchemaVersion:         1,
		Release:               StringRelease("test-release"),
		ReleaseWorktreePath:   "/tmp/test-release",
		ReleaseWorktreeBranch: "release-wt/test-release",
		Tracks: []BoardTrack{
			{
				ID:             "T1-core",
				Slices:         []string{"S01-alpha", "S02-beta", "S04-delta"},
				DependsOn:      nil,
				WorktreePath:   "/tmp/test-release-T1",
				WorktreeBranch: "track/r/T1-core",
				State:          "merged",
			},
		},
	}
	data, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	boardPath := filepath.Join(repoRoot, "docs", "release", "test-release", "board.json")
	if err := os.WriteFile(boardPath, data, 0644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	// ReadBoard must read the existing board.json, not the index.md.
	got, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(got.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1 (board.json should be authoritative, not index.md)", len(got.Tracks))
	}
	if got.Tracks[0].State != "merged" {
		t.Errorf("T1 state: got %q, want merged (from board.json)", got.Tracks[0].State)
	}
	if len(got.Tracks[0].Slices) != 3 {
		t.Errorf("T1 slices: got %d, want 3 (from board.json)", len(got.Tracks[0].Slices))
	}
}

func TestWriteBoard_Validation(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	// Missing required fields should fail validation.
	br := &BoardRecord{
		Release: StringRelease("test-release"),
		Tracks: []BoardTrack{
			{
				ID:             "T-bad",
				WorktreeBranch: "",
				State:          "planned",
			},
		},
	}
	err := WriteBoard(repoRoot, "test-release", br)
	if err == nil {
		t.Fatal("expected validation error for missing worktree_branch, got nil")
	}
	if !strings.Contains(err.Error(), "validate board.json") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestWriteBoard_RoundTrip(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	br := &BoardRecord{
		Release:               StringRelease("test-release"),
		ReleaseWorktreePath:   "/tmp/test-release",
		ReleaseWorktreeBranch: "release-wt/test-release",
		Tracks: []BoardTrack{
			{
				ID:             "T1-core",
				Slices:         []string{"S01-alpha"},
				WorktreeBranch: "track/r/T1-core",
				State:          "in_progress",
			},
		},
	}

	if err := WriteBoard(repoRoot, "test-release", br); err != nil {
		t.Fatalf("WriteBoard: %v", err)
	}

	// Read it back.
	got, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard after write: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version: got %d", got.SchemaVersion)
	}
	if len(got.Tracks) != 1 {
		t.Errorf("tracks: got %d", len(got.Tracks))
	}
	if got.Tracks[0].ID != "T1-core" {
		t.Errorf("track id: got %q", got.Tracks[0].ID)
	}
}

func TestOracleReadBoard_BoardJSONFirst(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	// Set up board.json on the release ref — this is what the oracle should read.
	boardContent := `{
		"schema_version": 1,
		"release": {"name": "test-release"},
		"release_worktree_path": "/tmp/test-release",
		"release_worktree_branch": "release-wt/test-release",
		"tracks": [
			{
				"id": "T1-core",
				"slices": ["S01-alpha"],
				"worktree_branch": "track/r/T1-core",
				"state": "in_progress"
			}
		]
	}`
	fr.setContent(rwtRef, "docs/release/test-release/board.json", boardContent)

	// Set up a different index.md — the oracle must ignore it.
	fr.setContent(rwtRef, "docs/release/test-release/index.md",
		`---
release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: merged
    slices:
      - S01-alpha
      - S02-beta
  - id: T2-extra
    worktree_branch: track/r/T2-extra
    state: planned
    slices:
      - S03-gamma
---`)

	fr.setContent("refs/heads/track/r/T1-core", "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	o := NewOracle(fr)
	board, err := o.ReadBoard(nil, fr, rwtRef, release)
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	// Must have exactly 1 track from board.json, not 2 from index.md.
	if len(board.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1 (board.json authoritative)", len(board.Tracks))
	}
	if board.Tracks[0].ID != "T1-core" {
		t.Errorf("track id: got %q", board.Tracks[0].ID)
	}
	// State from board.json is "in_progress", not "merged" from index.md.
	if board.Tracks[0].State != "in_progress" {
		t.Errorf("track state: got %q, want in_progress (from board.json)", board.Tracks[0].State)
	}
}

func TestOracleReadBoard_FallbackToIndex(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	// No board.json — must fall back to index.md.
	fr.setContent(rwtRef, "docs/release/test-release/index.md",
		`---
release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha
---`)

	fr.setContent("refs/heads/track/r/T1-core", "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	o := NewOracle(fr)
	board, err := o.ReadBoard(nil, fr, rwtRef, release)
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(board.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1", len(board.Tracks))
	}
	if board.Tracks[0].ID != "T1-core" {
		t.Errorf("track id: got %q", board.Tracks[0].ID)
	}
	if board.Tracks[0].State != "in_progress" {
		t.Errorf("track state: got %q", board.Tracks[0].State)
	}
}

func TestBoardTracksToTrackInfos_RoundTrip(t *testing.T) {
	original := []BoardTrack{
		{
			ID:             "T1-core",
			Slices:         []string{"S01-alpha", "S02-beta"},
			DependsOn:      []string{},
			WorktreePath:   "/tmp/T1",
			WorktreeBranch: "track/r/T1-core",
			State:          "in_progress",
		},
		{
			ID:             "T2-aux",
			Slices:         []string{"S03-gamma"},
			DependsOn:      []string{"T1-core"},
			WorktreePath:   "/tmp/T2",
			WorktreeBranch: "track/r/T2-aux",
			State:          "planned",
		},
	}

	// Convert BoardTrack -> TrackInfo -> BoardTrack.
	tis := boardTracksToTrackInfos(original)
	back := trackInfosToBoardTracks(tis)

	if len(back) != len(original) {
		t.Fatalf("length: got %d, want %d", len(back), len(original))
	}
	for i := range original {
		if back[i].ID != original[i].ID {
			t.Errorf("[%d] id: got %q, want %q", i, back[i].ID, original[i].ID)
		}
		if len(back[i].Slices) != len(original[i].Slices) {
			t.Errorf("[%d] slices len: got %d, want %d", i, len(back[i].Slices), len(original[i].Slices))
		}
		if len(back[i].DependsOn) != len(original[i].DependsOn) {
			t.Errorf("[%d] depends_on len: got %d, want %d", i, len(back[i].DependsOn), len(original[i].DependsOn))
		}
		if back[i].State != original[i].State {
			t.Errorf("[%d] state: got %q, want %q", i, back[i].State, original[i].State)
		}
	}
}