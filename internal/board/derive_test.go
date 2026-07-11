package board

import "testing"

func TestTrackAndReleaseBranchDerivation(t *testing.T) {
	if got := TrackWorktreeBranch("r1", "T1-core"); got != "track/r1/T1-core" {
		t.Errorf("TrackWorktreeBranch = %q, want track/r1/T1-core", got)
	}
	if got := ReleaseWorktreeBranch("r1"); got != "release-wt/r1" {
		t.Errorf("ReleaseWorktreeBranch = %q, want release-wt/r1", got)
	}
}

// TestTrackWorktreePathFrom proves the track worktree path is a sibling of the
// release worktree — repo-local — so a run in any repo keeps its worktrees beside
// that repo, not in a shared ~/projects/<repo>-worktrees tree (eval finding 3).
// Ported from the deleted scheduler defaultTrackWorktreePath test.
func TestTrackWorktreePathFrom(t *testing.T) {
	got := TrackWorktreePathFrom("/home/x/projects/fired-worktrees/release-r1", "r1", "T1")
	if want := "/home/x/projects/fired-worktrees/release-r1-T1"; got != want {
		t.Errorf("repo-local path: got %q want %q", got, want)
	}
	// Unknown release worktree path -> "" (caller fails closed; no $HOME fallback).
	if got := TrackWorktreePathFrom("", "r1", "T1"); got != "" {
		t.Errorf("empty releaseWorktreePath should yield \"\", got %q", got)
	}
}

// TestReleaseWorktreePathFrom proves the release worktree path is derived as a
// sibling of the PRIMARY repo (Pin 1), the release-level analogue of the track
// derivation, never the naive $HOME convention.
func TestReleaseWorktreePathFrom(t *testing.T) {
	got := ReleaseWorktreePathFrom("/home/x/sworn", "2026-06-28-driver-contract")
	if want := "/home/x/sworn-worktrees/release-2026-06-28-driver-contract"; got != want {
		t.Errorf("release path: got %q want %q", got, want)
	}
	if got := ReleaseWorktreePathFrom("", "r1"); got != "" {
		t.Errorf("empty repoRoot should yield \"\", got %q", got)
	}
}

// fakeAncestry implements RefAncestry for DeriveTrackState.
type fakeAncestry struct {
	exists    map[string]bool
	ancestors map[string]bool // "ancestor|descendant"
}

func (f fakeAncestry) RefExists(ref string) (bool, error) { return f.exists[ref], nil }
func (f fakeAncestry) IsAncestor(a, d string) (bool, error) {
	return f.ancestors[a+"|"+d], nil
}

// TestDeriveTrackState covers the three git-ref-derived states (invariant 5):
// no branch -> planned; branch exists, not merged -> in_progress; ancestor of
// release-wt -> merged.
func TestDeriveTrackState(t *testing.T) {
	trackRef := "refs/heads/track/r1/T1"
	releaseRef := "refs/heads/release-wt/r1"

	// No branch ref -> planned.
	fa := fakeAncestry{exists: map[string]bool{}, ancestors: map[string]bool{}}
	if got, _ := DeriveTrackState(fa, "r1", "T1"); got != "planned" {
		t.Errorf("no branch: got %q, want planned", got)
	}

	// Branch exists, not an ancestor of release-wt -> in_progress.
	fa = fakeAncestry{exists: map[string]bool{trackRef: true}, ancestors: map[string]bool{}}
	if got, _ := DeriveTrackState(fa, "r1", "T1"); got != "in_progress" {
		t.Errorf("unmerged branch: got %q, want in_progress", got)
	}

	// Branch exists and is an ancestor of release-wt -> merged.
	fa = fakeAncestry{
		exists:    map[string]bool{trackRef: true},
		ancestors: map[string]bool{trackRef + "|" + releaseRef: true},
	}
	if got, _ := DeriveTrackState(fa, "r1", "T1"); got != "merged" {
		t.Errorf("merged branch: got %q, want merged", got)
	}
}
