package board

import "testing"

// TestParseTracks_WorktreePathCommentPlaceholder proves the unfilled board
// placeholder (worktree_path: # set by first /implement-slice) parses as an
// empty path rather than yielding the comment text as a literal path that then
// reaches `git worktree add` (eval finding 2). The empty value is what the
// cold-start bootstrap relies on to know the worktree is not yet materialised.
func TestParseTracks_WorktreePathCommentPlaceholder(t *testing.T) {
	const body = `
title: Release board
tracks:
  - id: T1-cold
    slices: [S01-task]
    depends_on: null
    worktree_path: # set by first /implement-slice in this release
    worktree_branch: track/x/T1
    state: planned
  - id: T2-inline
    slices: [S02-task]
    depends_on: null
    worktree_path: /tmp/wt/T2  # inline note after a real path
    worktree_branch: track/x/T2
    state: planned
`
	tracks := ParseTracks(body)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].WorktreePath != "" {
		t.Errorf("comment-only placeholder should parse empty, got %q", tracks[0].WorktreePath)
	}
	if tracks[1].WorktreePath != "/tmp/wt/T2" {
		t.Errorf("inline comment not stripped: got %q want /tmp/wt/T2", tracks[1].WorktreePath)
	}
}
