package board

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fakeGitReader implements gitContentReader with a map of ref:path -> content.
type fakeGitReader struct {
	content    map[string]string // "ref|path" → content
	showCounts map[string]int    // "ref|path" → call count
}

func newFakeReader() *fakeGitReader {
	return &fakeGitReader{
		content:    make(map[string]string),
		showCounts: make(map[string]int),
	}
}

func (f *fakeGitReader) key(ref, path string) string {
	return ref + "|" + path
}

func (f *fakeGitReader) setContent(ref, path, content string) {
	f.content[f.key(ref, path)] = content
}

func (f *fakeGitReader) Show(ref, path string) (string, error) {
	k := f.key(ref, path)
	f.showCounts[k]++
	c, ok := f.content[k]
	if !ok {
		return "", fmt.Errorf("git show %s:%s: path not found", ref, path)
	}
	return c, nil
}

func (f *fakeGitReader) CatFileExists(ref, path string) (bool, error) {
	_, ok := f.content[f.key(ref, path)]
	return ok, nil
}

// oneShotEmptyReader wraps a gitContentReader and returns empty content on
// the first call to Show for a specific (ref, path) pair. Used to test
// transient-read retry.
type oneShotEmptyReader struct {
	inner      gitContentReader
	emptyRef   string
	emptyPath  string
	wasEmptied bool
}

func (o *oneShotEmptyReader) Show(ref, path string) (string, error) {
	if ref == o.emptyRef && path == o.emptyPath && !o.wasEmptied {
		o.wasEmptied = true
		return "", nil // one-shot empty
	}
	return o.inner.Show(ref, path)
}

func (o *oneShotEmptyReader) CatFileExists(ref, path string) (bool, error) {
	return o.inner.CatFileExists(ref, path)
}

func trackMapFromFM(fmBody string) map[string]TrackInfo {
	tracks := ParseTracks(fmBody)
	m := make(map[string]TrackInfo, len(tracks))
	for _, t := range tracks {
		m[t.ID] = t
	}
	return m
}

func baseIndexFM() string {
	return `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha
      - S02-beta
  - id: T2-aux
    worktree_branch: track/r/T2-aux
    state: planned
    slices:
      - S03-gamma
    depends_on:
      - T1-core`
}
func TestOwnerBranchWins(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"

	t1Ref := "refs/heads/track/r/T1-core"
	t2Ref := "refs/heads/track/r/T2-aux"

	// T1-core has planned, T2-aux has verified (stale ghost).
	fr.setContent(t1Ref, "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)
	fr.setContent(t2Ref, "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"verified","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha
  - id: T2-aux
    worktree_branch: track/r/T2-aux
    state: planned
    slices:
      - S03-gamma`
	tm := trackMapFromFM(fmBody)

	o := NewOracle(fr)
	ss, resFrom, err := o.ReadSliceStatus(context.Background(), fr,
		"refs/heads/track/r/T1-core", "refs/heads/release-wt/test-release",
		release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus: %v", err)
	}
	if ss.State != "planned" {
		t.Errorf("owner branch wins: want planned, got %s (resolved from %s)", ss.State, resFrom)
	}
	if resFrom != ResolvedByTrack {
		t.Errorf("resolved from: want track-branch, got %s", resFrom)
	}
}

func TestGhostCopyIgnored(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"

	t1Ref := "refs/heads/track/r/T1-core"
	fr.setContent(t1Ref, "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	fmBody := baseIndexFM()
	tm := trackMapFromFM(fmBody)

	o := NewOracle(fr)

	// Read S01-alpha from T1-core's perspective.
	ss, _, err := o.ReadSliceStatus(context.Background(), fr,
		"refs/heads/track/r/T1-core", "refs/heads/release-wt/test-release",
		release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus from owner: %v", err)
	}
	if ss.State != "in_progress" {
		t.Errorf("owner read: want in_progress, got %s", ss.State)
	}

	// S03-gamma owned by T2-aux — only on T2-aux's branch.
	t2Ref := "refs/heads/track/r/T2-aux"
	fr.setContent(t2Ref, "docs/release/test-release/S03-gamma/status.json",
		`{"slice_id":"S03-gamma","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T2-aux","verification":{"result":"pending"}}`)

	ss3, _, err := o.ReadSliceStatus(context.Background(), fr,
		"refs/heads/track/r/T2-aux", "refs/heads/release-wt/test-release",
		release, "S03-gamma", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus S03-gamma: %v", err)
	}
	if ss3.State != "planned" {
		t.Errorf("S03-gamma: want planned, got %s", ss3.State)
	}
	if ss3.Track != "T2-aux" {
		t.Errorf("S03-gamma track: want T2-aux, got %s", ss3.Track)
	}
}

func TestRefPriorityFallback(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"

	rwtRef := "refs/heads/release-wt/test-release"
	fr.setContent(rwtRef, "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"design_review","owner":"human","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: ""
    state: planned
    slices:
      - S01-alpha`
	tm := trackMapFromFM(fmBody)

	o := NewOracle(fr)
	ss, resFrom, err := o.ReadSliceStatus(context.Background(), fr,
		"", rwtRef, release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus: %v", err)
	}
	if ss.State != "design_review" {
		t.Errorf("fallback to release-wt: want design_review, got %s", ss.State)
	}
	if resFrom != ResolvedByReleaseWT {
		t.Errorf("resolved from: want release-wt, got %s", resFrom)
	}

	// Test fallback to HEAD.
	fr2 := newFakeReader()
	fr2.setContent("HEAD", "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	o2 := NewOracle(fr2)
	ss2, resFrom2, err := o2.ReadSliceStatus(context.Background(), fr2,
		"", rwtRef, release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus HEAD fallback: %v", err)
	}
	if ss2.State != "implemented" {
		t.Errorf("fallback to HEAD: want implemented, got %s", ss2.State)
	}
	if resFrom2 != ResolvedByWorkingTree {
		t.Errorf("resolved from: want working-tree, got %s", resFrom2)
	}
}

func TestDocsPrefixProbe(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"

	// Fumadocs-style prefix.
	rwtRef := "refs/heads/release-wt/test-release"
	fr.setContent(rwtRef, "apps/docs/content/docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha`
	tm := trackMapFromFM(fmBody)

	o := NewOracle(fr)
	ss, _, err := o.ReadSliceStatus(context.Background(), fr,
		"", rwtRef, release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus Fumadocs prefix: %v", err)
	}
	if ss.State != "planned" {
		t.Errorf("Fumadocs prefix: want planned, got %s", ss.State)
	}
}

func TestTransientReadRetry(t *testing.T) {
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"
	path := "docs/release/test-release/S01-alpha/status.json"

	// Base reader with content.
	fr := newFakeReader()
	fr.setContent(rwtRef, path,
		`{"slice_id":"S01-alpha","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)

	// Wrap with one-shot empty: first call returns "", second returns content.
	rr := &oneShotEmptyReader{
		inner:     fr,
		emptyRef:  rwtRef,
		emptyPath: path,
	}

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: ""
    state: planned
    slices:
      - S01-alpha`
	tm := trackMapFromFM(fmBody)

	o := NewOracle(rr)
	ss, _, err := o.ReadSliceStatus(context.Background(), rr,
		"", rwtRef, release, "S01-alpha", tm)
	if err != nil {
		t.Fatalf("ReadSliceStatus with one-shot empty: %v", err)
	}
	if ss.State != "planned" {
		t.Errorf("retry recovered: want planned, got %s", ss.State)
	}
}

func TestTransientReadRetry_EmptyTwice(t *testing.T) {
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"
	path := "docs/release/test-release/S01-alpha/status.json"

	// Content is empty — retry reads empty again, falls through to "missing".
	fr := newFakeReader()
	fr.setContent(rwtRef, path, "")

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: ""
    state: planned
    slices:
      - S01-alpha`
	tm := trackMapFromFM(fmBody)

	o := NewOracle(fr)
	_, _, err := o.ReadSliceStatus(context.Background(), fr,
		"", rwtRef, release, "S01-alpha", tm)
	if err == nil {
		t.Error("expected error for empty+retry-empty content")
	}
	if !strings.Contains(err.Error(), "not found on any ref") {
		t.Errorf("expected 'not found on any ref' error, got: %v", err)
	}
}

func TestParseStatusJSON_Blocked(t *testing.T) {
	raw := `{"slice_id":"S01-alpha","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"blocked","violations":["spec defect: missing acceptance check"],"routing":"needs_planner"}}`
	tm := map[string]TrackInfo{
		"T1-core": {ID: "T1-core", DependsOn: []string{}, Slices: []string{"S01-alpha"}},
	}

	ss, err := parseStatusJSON(raw, "S01-alpha", "T1-core", tm)
	if err != nil {
		t.Fatalf("parseStatusJSON: %v", err)
	}
	if !ss.Blocked {
		t.Error("expected blocked=true")
	}
	if ss.BlockedReason != "spec defect: missing acceptance check" {
		t.Errorf("blocked reason: got %q", ss.BlockedReason)
	}
	if ss.BlockedOwner != BlockedNeedsPlanner {
		t.Errorf("blocked owner: want needs_planner, got %s", ss.BlockedOwner)
	}
	if ss.State != "implemented" {
		t.Errorf("state: want implemented, got %s", ss.State)
	}
}

func TestParseStatusJSON_BlockedInferred(t *testing.T) {
	raw := `{"slice_id":"S01-alpha","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"blocked","violations":["something wrong"]}}`
	tm := map[string]TrackInfo{
		"T1-core": {ID: "T1-core", Slices: []string{"S01-alpha"}},
	}

	ss, err := parseStatusJSON(raw, "S01-alpha", "T1-core", tm)
	if err != nil {
		t.Fatalf("parseStatusJSON: %v", err)
	}
	if ss.BlockedOwner != BlockedNeedsPlanner {
		t.Errorf("inferred owner: want needs_planner, got %s", ss.BlockedOwner)
	}
}

func TestReadBoard_GhostFilter(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	indexContent := `---
release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha
  - id: T2-aux
    worktree_branch: track/r/T2-aux
    state: planned
    slices:
      - S02-beta
---`
	fr.setContent(rwtRef, "docs/release/test-release/index.md", indexContent)

	fr.setContent("refs/heads/track/r/T1-core", "docs/release/test-release/S01-alpha/status.json",
		`{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`)
	fr.setContent("refs/heads/track/r/T2-aux", "docs/release/test-release/S02-beta/status.json",
		`{"slice_id":"S02-beta","state":"planned","owner":"human","last_updated_at":"2026-01-01T00:00:00Z","track":"T2-aux","verification":{"result":"pending"}}`)

	o := NewOracle(fr)
	board, err := o.ReadBoard(context.Background(), fr, rwtRef, release)
	if err != nil {
		t.Fatalf("ReadBoard: %v", err)
	}

	if len(board.Tracks) != 2 {
		t.Fatalf("tracks: want 2, got %d", len(board.Tracks))
	}

	if len(board.Tracks[0].Slices) != 1 {
		t.Errorf("T1-core slices: want 1, got %d", len(board.Tracks[0].Slices))
	}
	if board.Tracks[0].Slices[0].State != "in_progress" {
		t.Errorf("S01-alpha state: want in_progress, got %s", board.Tracks[0].Slices[0].State)
	}

	if len(board.Tracks[1].Slices) != 1 {
		t.Errorf("T2-aux slices: want 1, got %d", len(board.Tracks[1].Slices))
	}
	if board.Tracks[1].Slices[0].State != "planned" {
		t.Errorf("S02-beta state: want planned, got %s", board.Tracks[1].Slices[0].State)
	}
}

// S05-board-canonical-emit claimed the strict Release reader makes a
// bare-string board fail closed "on read" — but every existing S05 test
// (board_release_test.go) calls json.Unmarshal directly on Release/BoardRecord
// in isolation, never through readTrackInfos/ReadBoard, the function
// cmd/sworn board actually calls. This is the Rule-1 reachability test that
// was missing: it must render through the real integration point.
func TestReadTrackInfos_BareStringRelease_FailsClosed(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	fr.setContent(rwtRef, "docs/release/test-release/board.json",
		`{"schema_version":1,"release":"test-release","tracks":[]}`)

	o := NewOracle(fr)
	if _, err := o.readTrackInfos(fr, rwtRef, release); err == nil {
		t.Fatal("want error reading a bare-string release through readTrackInfos, got nil")
	}
}

// A release that has migrated to board.json (a copy is committed on HEAD)
// must not silently fall back to the unvalidated legacy index.md parser just
// because releaseRef's copy is missing (e.g. release-wt hasn't absorbed the
// migration commit yet) — that would bypass the S05 strict reader entirely.
// Falling back to index.md is only safe for a release that never had a
// board.json anywhere.
func TestReadTrackInfos_MigratedOnHeadButMissingOnReleaseRef_FailsClosed(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	fr.setContent("HEAD", "docs/release/test-release/board.json",
		`{"schema_version":1,"release":{"name":"test-release"},"tracks":[]}`)
	// A stale legacy index.md still committed on releaseRef — the bait that
	// would previously have been silently accepted.
	fr.setContent(rwtRef, "docs/release/test-release/index.md", "---\ntracks: []\n---\n")

	o := NewOracle(fr)
	if _, err := o.readTrackInfos(fr, rwtRef, release); err == nil {
		t.Fatal("want error when board.json exists on HEAD but not releaseRef, got nil (silently used legacy index.md)")
	}
}

// A release that never had board.json anywhere (pre-ADR-0009) must still
// resolve via the legacy index.md parser — the hardening above must not
// regress genuinely-legacy releases.
func TestReadTrackInfos_NeverMigrated_UsesLegacyIndexMD(t *testing.T) {
	fr := newFakeReader()
	release := "test-release"
	rwtRef := "refs/heads/release-wt/test-release"

	fmBody := `release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/r/T1-core
    state: in_progress
    slices:
      - S01-alpha`
	fr.setContent(rwtRef, "docs/release/test-release/index.md", "---"+fmBody+"\n---\n")

	o := NewOracle(fr)
	tracks, err := o.readTrackInfos(fr, rwtRef, release)
	if err != nil {
		t.Fatalf("legacy index.md fallback: %v", err)
	}
	if len(tracks) != 1 || tracks[0].ID != "T1-core" {
		t.Fatalf("legacy index.md fallback: want 1 track T1-core, got %+v", tracks)
	}
}

func TestExtractFrontmatterBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"standard",
			"---\nkey: value\n---\nrest of file",
			"\nkey: value",
		},
		{
			"no frontmatter",
			"# just markdown",
			"# just markdown",
		},
		{
			"only opening",
			"---\nkey: value\nno closing",
			"---\nkey: value\nno closing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterBody(tt.in)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
