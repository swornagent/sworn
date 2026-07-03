# implement-slice's release-wt board.json write is an avoidable race

Date: 2026-07-03
Release: 2026-06-28-driver-contract (found during T1-contract merge + T2/T3 kickoff)

## Finding

Two `/implement-slice` sessions dispatched concurrently (`S02-claude-subprocess-driver`
on track T2-subprocess, `S04-inprocess-oai-driver` on track T3-inprocess) both
became the *first* dispatch for their respective tracks at the same time —
both unblocked by the same event (T1-contract merging into release-wt).

`implement-slice.md` Step 0 has exactly one write to the **release** worktree:
on first dispatch of a track, when `<worktree_path>` is null, it materialises
the track worktree, then updates `docs/release/<release>/board.json` on
`release-wt/<release>` to record `{worktree: {path, branch}, state:
"in_progress"}` for that track, and commits.

Both agents hit that step at effectively the same instant, in the *same*
shared release worktree's working tree and git index. One agent's commit
(`446ab82`) swept up the other's uncommitted edit to the same file. Content
came out correct by luck (the edits were to disjoint track entries in the
same JSON file, and Go map/array literal edits happened not to conflict at
the text level) — but this is a real race, not a design.

## Root cause

`internal/board.TrackInfo.WorktreeBranch`/`WorktreePath` and the track's
`state` field are modeled as **persisted data**, written once by the first
`/implement-slice` of a track (`internal/board/track.go:36-39,152-164`,
`internal/board/board.go:87-88`), even though `track-mode.md`'s "Naming,
locked" table (lines 100-110) already documents these as **fully
deterministic** from `(release, track-id)`:

- Track branch: `track/<release>/<track-id>` — always.
- Track worktree path: `$HOME/projects/<REPO_BASENAME>-worktrees/release-<release>-<track-id>` — always.

Nothing about these values is a planning decision or a runtime fact that
needs a human/agent to record — they're a pure function of two strings the
board already knows (`release`, `track.id`). Persisting them created a
write that (a) didn't need to exist and (b) is exactly the kind of
process-global mutation two concurrent sessions can race on (Baton Rule 11).

The oracle already treats `release-wt`'s copy of *slice* state as a cache,
not a source of truth — `Oracle.ReadSliceStatus` (oracle.go:280-363) reads
the owning track's own branch first and falls back to release-wt only if
that read is unavailable. Track *identity* (branch/path) and track *state*
(planned/in_progress/merged) don't get the same treatment today, but could:
track state is derivable via the exact `git merge-base --is-ancestor
<track-branch> release-wt/<release>` check `/merge-track`'s own idempotency
gate (merge-track.md Step 1.2) already performs.

## Proposed fix (not yet decided — Type-1, cross-repo)

1. **sworn (Go):** add pure helper functions implementing the "Naming,
   locked" table — `TrackWorktreeBranch(release, trackID) string`,
   `TrackWorktreePath(repoRoot, release, trackID) string`, and their
   release-level equivalents. Single source of truth; unit-test against the
   table's own example row.
2. **sworn (Go):** `Oracle.readTrackInfos` / `ReadSliceStatus` stop reading
   `worktree_branch`/`worktree_path` from board.json — compute them via (1).
   Derive track `state` from `merge-base --is-ancestor` against
   `release-wt/<release>` (no branch ref = `planned`; branch exists, not an
   ancestor = `in_progress`; ancestor = `merged`) instead of trusting a
   persisted string.
3. **baton (protocol):** drop `worktree_path` / `worktree_branch` / `state`
   as fields on `board-v1.json`'s `tracks[]` entries — keep only `id`,
   `slices`, `depends_on` (the actual planning-time facts). Versioned schema
   change; would ride a baton bump (the same kind of vehicle as the
   v0.7.1 pin T7-baton-revendor is already cutting for this release).
4. **baton (commands):** delete `implement-slice.md` Step 0's "record the
   worktree on the board" sub-step entirely — first dispatch runs `git
   worktree add` and proceeds; release-wt is never touched. Delete
   `merge-track.md` Step 5's "set track state to merged" (now derived);
   keep the merge commit itself (Step 4) as the one required release-wt
   write, plus the activity-log append if a non-git-log audit trail is
   still wanted (git's own merge-commit message already carries this
   narrative, so that append may also be redundant — separate call).

Net effect: `/implement-slice` never writes to `release-wt` under any
circumstance. `/merge-track` remains the sole write point, and its surface
shrinks to (at most) the merge commit + an optional activity-log append —
small enough that a plain retry-on-conflict git loop closes the residual
"two tracks verify and merge at the same moment" race, which exists today
in latent form (merge-track.md Step 5 is an unguarded read-modify-write,
just less frequently triggered than the one this capture documents).

## Status

Proposed by Brad 2026-07-03, in response to this finding. Not yet
implemented — needs a Type-1 decision (Rule 9): scope is "derive worktree
identity only" (narrower, fixes the observed race) vs. "derive worktree
identity + track state" (closes the merge-track residual race too, larger
diff touching more of oracle.go's state-resolution path). Tracking issue
not yet filed.
