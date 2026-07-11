package board

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
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

// baseFrontmatter is a LEGACY index.md frontmatter (still carrying the retired
// release/track worktree+state fields). The migration path no longer scrapes the
// release-level fields, and trackInfosToBoardTracks drops the track-level ones, so
// the migrated board.json is a pure plan.
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

	// The migrated BoardRecord is a pure plan: $schema + release + tracks only.
	if br.Release.Name != "test-release" {
		t.Errorf("release: got %q, want %q", br.Release.Name, "test-release")
	}
	if br.Schema != baton.BoardSchemaURI {
		t.Errorf("$schema: got %q, want %q", br.Schema, baton.BoardSchemaURI)
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

	t2 := br.Tracks[1]
	if t2.ID != "T2-aux" {
		t.Errorf("T2 id: got %q", t2.ID)
	}
	if len(t2.DependsOn) != 1 || t2.DependsOn[0] != "T1-core" {
		t.Errorf("T2 depends_on: got %v", t2.DependsOn)
	}

	// The written board.json is a pure plan: no worktree/state/schema_version keys,
	// and it validates against the strict vendored v0.10.0 board-v1.
	boardPath := filepath.Join(repoRoot, "docs", "release", "test-release", "board.json")
	data, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("board.json not written: %v", err)
	}
	for _, forbidden := range []string{"schema_version", "release_worktree_path", "release_worktree_branch", "worktree_path", "worktree_branch", "\"state\""} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("written board.json still carries retired field %q", forbidden)
		}
	}
	if err := baton.ValidateSchema("board-v1", data); err != nil {
		t.Errorf("migrated board.json does not conform to strict board-v1: %v", err)
	}
}

func TestReadBoard_ExistingBoardJSON(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	// Pre-write a board.json (pure plan) that differs from index.md frontmatter.
	br := &BoardRecord{
		Release: StringRelease("test-release"),
		Tracks: []BoardTrack{
			{
				ID:        "T1-core",
				Slices:    []string{"S01-alpha", "S02-beta", "S04-delta"},
				DependsOn: nil,
			},
		},
	}
	if err := WriteBoard(repoRoot, "test-release", br); err != nil {
		t.Fatalf("WriteBoard: %v", err)
	}

	// ReadBoard must read the existing board.json, not the index.md.
	got, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(got.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1 (board.json authoritative, not index.md)", len(got.Tracks))
	}
	if len(got.Tracks[0].Slices) != 3 {
		t.Errorf("T1 slices: got %d, want 3 (from board.json)", len(got.Tracks[0].Slices))
	}
}

// TestReadBoard_ToleratesLegacyOnDisk proves a legacy board.json still on disk
// (schema_version + release/track worktree+state — the shape of the un-migrated
// pre-spec-v1 releases) still loads as a pure plan AFTER the S11 normalise shim
// was removed by S12: BoardRecord carries only the pure-plan fields, so
// json.Unmarshal drops the retired keys by construction, no tolerance layer (sworn#90).
func TestReadBoard_ToleratesLegacyOnDisk(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())
	legacy := `{
		"$schema": "https://baton.sawy3r.net/schemas/board-v1.json",
		"schema_version": 1,
		"release": {"name": "test-release"},
		"release_worktree_path": "/tmp/test-release",
		"release_worktree_branch": "release-wt/test-release",
		"tracks": [{"id": "T1-core", "slices": ["S01-alpha"], "worktree_branch": "track/r/T1-core", "state": "merged"}]
	}`
	boardPath := filepath.Join(repoRoot, "docs", "release", "test-release", "board.json")
	if err := os.WriteFile(boardPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard(legacy on-disk): %v", err)
	}
	if len(got.Tracks) != 1 || got.Tracks[0].ID != "T1-core" || len(got.Tracks[0].Slices) != 1 {
		t.Errorf("legacy board did not load as a pure plan: %+v", got.Tracks)
	}
}

func TestWriteBoard_Validation(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	// A track with no slices is invalid board-v1 (slices is required).
	br := &BoardRecord{
		Release: StringRelease("test-release"),
		Tracks:  []BoardTrack{{ID: "T-bad"}},
	}
	err := WriteBoard(repoRoot, "test-release", br)
	if err == nil {
		t.Fatal("expected validation error for a track missing slices, got nil")
	}
	if !strings.Contains(err.Error(), "validate board.json") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

// TestWriteBoard_RoundTrip is the AC-06 round-trip: a freshly-written board.json
// is a pure plan ($schema/release/tracks only, no worktree/state/schema_version)
// and validates against the strict vendored v0.10.0 board-v1.
func TestWriteBoard_RoundTrip(t *testing.T) {
	repoRoot := setupReleaseDir(t, "test-release", baseFrontmatter())

	br := &BoardRecord{
		Release: StringRelease("test-release"),
		Tracks: []BoardTrack{
			{ID: "T1-core", Slices: []string{"S01-alpha"}},
		},
	}

	if err := WriteBoard(repoRoot, "test-release", br); err != nil {
		t.Fatalf("WriteBoard: %v", err)
	}

	boardPath := filepath.Join(repoRoot, "docs", "release", "test-release", "board.json")
	data, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	// Strict v0.10.0 board-v1 conformance — the load-bearing AC-06 assertion.
	if err := baton.ValidateSchema("board-v1", data); err != nil {
		t.Errorf("freshly-written board.json does not conform to strict board-v1: %v", err)
	}
	// Top-level keys are exactly $schema, release, tracks.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	for k := range top {
		if k != "$schema" && k != "release" && k != "tracks" {
			t.Errorf("unexpected top-level key %q in a pure-plan board.json", k)
		}
	}

	got, err := ReadBoard(repoRoot, "test-release")
	if err != nil {
		t.Fatalf("ReadBoard after write: %v", err)
	}
	if got.Schema != baton.BoardSchemaURI {
		t.Errorf("$schema: got %q", got.Schema)
	}
	if len(got.Tracks) != 1 || got.Tracks[0].ID != "T1-core" {
		t.Errorf("tracks round-trip: got %+v", got.Tracks)
	}
}

func TestOracleReadBoard_BoardJSONFirst(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	// A LEGACY board.json on the release ref — the retired keys must be ignored
	// on read (struct removal drops unknown fields; no normalise shim), and the
	// track branch/state DERIVED.
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
				"state": "merged"
			}
		]
	}`
	fr.setContent(rwtRef, "docs/release/test-release/board.json", boardContent)

	// A different index.md — the oracle must ignore it.
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
---`)

	fr.setContent("refs/heads/track/test-release/T1-core", "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	// The DERIVED branch is track/<release>/<id>; make it exist but not merged → in_progress.
	fr.setRef("refs/heads/track/test-release/T1-core")

	o := NewOracle(fr)
	board, err := o.ReadBoard(context.Background(), fr, rwtRef, release)
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(board.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1 (board.json authoritative)", len(board.Tracks))
	}
	if board.Tracks[0].ID != "T1-core" {
		t.Errorf("track id: got %q", board.Tracks[0].ID)
	}
	// Branch is DERIVED as track/<release>/<id>, not read from the persisted field.
	if board.Tracks[0].WorktreeBranch != "track/test-release/T1-core" {
		t.Errorf("derived branch: got %q, want track/test-release/T1-core", board.Tracks[0].WorktreeBranch)
	}
	// State is DERIVED: the branch exists and is not an ancestor of release-wt → in_progress.
	if board.Tracks[0].State != "in_progress" {
		t.Errorf("derived state: got %q, want in_progress", board.Tracks[0].State)
	}
}

func TestOracleReadBoard_FallbackToIndex(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	// No board.json — must fall back to index.md (legacy frontmatter state).
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
	board, err := o.ReadBoard(context.Background(), fr, rwtRef, release)
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(board.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1", len(board.Tracks))
	}
	if board.Tracks[0].ID != "T1-core" {
		t.Errorf("track id: got %q", board.Tracks[0].ID)
	}
	// The legacy index.md fallback keeps the frontmatter-parsed state.
	if board.Tracks[0].State != "in_progress" {
		t.Errorf("track state: got %q", board.Tracks[0].State)
	}
}

func TestBoardTracksToTrackInfos_Derivation(t *testing.T) {
	original := []BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-alpha", "S02-beta"}, DependsOn: []string{}},
		{ID: "T2-aux", Slices: []string{"S03-gamma"}, DependsOn: []string{"T1-core"}},
	}

	// BoardTrack -> TrackInfo derives the branch; -> BoardTrack drops the derived
	// fields so the persisted form stays a pure plan.
	tis := boardTracksToTrackInfos(original, "r")
	if tis[0].WorktreeBranch != "track/r/T1-core" {
		t.Errorf("derived branch: got %q, want track/r/T1-core", tis[0].WorktreeBranch)
	}
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
	}
}
