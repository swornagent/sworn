package board

import "path/filepath"

// Derivation helpers for track/release worktree identity and track state
// (sworn#80, S11-baton-revendor). Baton board-v1 is a PURE PLAN at v0.9.0/v0.10.0:
// worktree branches/paths and track state are NEVER persisted in board.json —
// they are computed from (release, track-id) and git ref ancestry (track-mode
// invariant 5). These helpers are the single source of truth for that
// computation, replacing the fields the board writer used to emit.

// TrackWorktreeBranch derives a track's branch: track/<release>/<track-id>.
func TrackWorktreeBranch(release, trackID string) string {
	return "track/" + release + "/" + trackID
}

// ReleaseWorktreeBranch derives a release's assembly branch: release-wt/<release>.
func ReleaseWorktreeBranch(release string) string {
	return "release-wt/" + release
}

// ReleaseWorktreePathFrom derives the release worktree path as a sibling of the
// PRIMARY repo root: <dir(primaryRoot)>/<base(primaryRoot)>-worktrees/release-<release>.
// This is the release-level analogue of eval finding 3's track-path derivation:
// repo-local (a sibling of the actual repo), never the naive
// $HOME/projects/<repo>-worktrees convention that caused a consumer-repo run to
// materialise on sworn's worktree tree. Returns "" when primaryRoot is unknown
// (e.g. a content-only fake reader in tests), which callers treat as "path not
// derivable here" rather than emitting a wrong path.
func ReleaseWorktreePathFrom(primaryRoot, release string) string {
	if primaryRoot == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(primaryRoot), filepath.Base(primaryRoot)+"-worktrees", "release-"+release)
}

// TrackWorktreePathFrom derives the track worktree path as a sibling of the
// release worktree (eval finding 3, the logic internal/scheduler/worker.go's
// defaultTrackWorktreePath already carried): <dir(releaseWorktreePath)>/release-<release>-<track-id>.
// Returns "" when releaseWorktreePath is unknown.
func TrackWorktreePathFrom(releaseWorktreePath, release, trackID string) string {
	if releaseWorktreePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(releaseWorktreePath), "release-"+release+"-"+trackID)
}

// DeriveTrackInfos returns TrackInfos for a board's tracks with the worktree
// branch/path and state DERIVED (sworn#80) — the exported entry point for
// consumers that read a BoardRecord directly rather than going through the
// Oracle. branch = track/<release>/<id>; path = a sibling of the release worktree
// (itself a sibling of repoRoot); state is derived from git ancestry when ra is
// non-nil, else left "". A "" repoRoot yields empty paths (caller decides).
func DeriveTrackInfos(tracks []BoardTrack, repoRoot, release string, ra RefAncestry) []TrackInfo {
	releaseWTPath := ReleaseWorktreePathFrom(repoRoot, release)
	tis := make([]TrackInfo, len(tracks))
	for i, bt := range tracks {
		tis[i] = TrackInfo{
			ID:             bt.ID,
			Slices:         bt.Slices,
			DependsOn:      bt.DependsOn,
			WorktreeBranch: TrackWorktreeBranch(release, bt.ID),
			WorktreePath:   TrackWorktreePathFrom(releaseWTPath, release, bt.ID),
		}
		if ra != nil {
			if st, err := DeriveTrackState(ra, release, bt.ID); err == nil {
				tis[i].State = st
			}
		}
	}
	return tis
}

// RefAncestry is the git capability the track-state derivation needs: whether a
// ref exists, and whether one ref is an ancestor of another. gitRepoReader
// (production, backed by *git.Repo) implements it; a content-only fake reader
// does not, in which case DeriveTrackState is not invoked and state stays "".
type RefAncestry interface {
	RefExists(ref string) (bool, error)
	IsAncestor(ancestor, descendant string) (bool, error)
}

// DeriveTrackState computes a track's state from git refs alone (track-mode
// invariant 5): no such branch ref -> planned; the branch exists but is NOT an
// ancestor of release-wt/<release> -> in_progress; the branch IS an ancestor of
// release-wt/<release> -> merged (the same merge-base --is-ancestor check
// cmd/sworn/merge.go performs for its own gate). The board never stores this —
// /merge-track's merge commit is the only "merged" signal, read back here.
func DeriveTrackState(ra RefAncestry, release, trackID string) (string, error) {
	trackRef := "refs/heads/" + TrackWorktreeBranch(release, trackID)
	exists, err := ra.RefExists(trackRef)
	if err != nil {
		return "", err
	}
	if !exists {
		return "planned", nil
	}
	releaseRef := "refs/heads/" + ReleaseWorktreeBranch(release)
	anc, err := ra.IsAncestor(trackRef, releaseRef)
	if err != nil {
		return "", err
	}
	if anc {
		return "merged", nil
	}
	return "in_progress", nil
}
