package scheduler

import (
	"strings"
	"testing"
)

// TestDefaultTrackWorktreePath_RepoLocal proves the default track worktree is a
// sibling of the release worktree — repo-local — so a run in any repo keeps its
// worktrees beside that repo, not in a shared ~/projects/sworn-worktrees tree
// (eval finding 3).
func TestDefaultTrackWorktreePath_RepoLocal(t *testing.T) {
	got, err := defaultTrackWorktreePath(
		"/home/x/projects/fired-worktrees/release-r1", "fired", "r1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	want := "/home/x/projects/fired-worktrees/release-r1-T1"
	if got != want {
		t.Errorf("repo-local path: got %q want %q", got, want)
	}
}

// TestDefaultTrackWorktreePath_FallbackUsesProjectDir proves the ~/projects
// fallback (no release worktree path) uses the passed projectDir, NOT a
// hardcoded "sworn" — so even the fallback can't leak a fired run into sworn's
// worktree tree.
func TestDefaultTrackWorktreePath_FallbackUsesProjectDir(t *testing.T) {
	got, err := defaultTrackWorktreePath("", "fired", "r1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "fired-worktrees") {
		t.Errorf("fallback should use projectDir 'fired', got %q", got)
	}
	if strings.Contains(got, "sworn-worktrees") {
		t.Errorf("fallback leaked into sworn-worktrees: %q", got)
	}
}
