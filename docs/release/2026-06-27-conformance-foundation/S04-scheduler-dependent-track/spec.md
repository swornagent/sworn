---
title: 'S04 — Scheduler dependent-track branch from dependency tip'
description: 'Dependent track worktrees branch from the dependency track''s merge tip (release-wt after finishTrack auto-merge), enforced by a topological phase barrier — not from a release-wt that may lack the dependency''s code.'
---

# Slice: `S04-scheduler-dependent-track`

## User outcome

When a release has a track T5 with `depends_on: T6`, the scheduler runs T6 in an
earlier topological phase than T5; T6's last verified slice auto-merges to
`release-wt/<release>` inside `finishTrack` before T6's goroutine returns, and the
phase barrier holds T5 until T6's phase completes. T5's worktree is then created
branching from the now-current `release-wt` tip — so T5's implementer starts with
T6's code already present.

## Entry point

`sworn run --release <name>` → `internal/run/parallel.go` (`RunParallel`) +
`internal/scheduler/` (`BuildPlan`, `RunTrack`/`finishTrack`) — the scheduler
reads `depends_on` from `index.md` tracks frontmatter, builds a topological phase
plan, and enforces the ordering via a per-phase barrier.

## Design mechanism (ratified — design-review Pin 1)

The original design proposed a `waitForDependencies` poll loop; the Captain
review (Pin 1) rejected it as deadlock-prone (the board oracle never transitions a
track to `merged` from a bare auto-merge, so a value-poll would spin forever and
the `ctx.Done()` anti-deadlock could not fire). The ratified mechanism is instead:

1. `scheduler.BuildPlan(tracks)` topologically orders tracks into phases; a track
   sits in a phase strictly later than every track it `depends_on`.
2. `RunParallel` runs each phase's tracks concurrently and calls `wg.Wait()` — the
   **phase barrier** — before starting the next phase.
3. `finishTrack` auto-merges a track's verified work to `release-wt` *before* the
   track goroutine returns `TrackPass`, so by the time the barrier releases, the
   dependency's code is on `release-wt`.
4. The dependent track's worktree is created with `git worktree add -b <branch>
   release-wt/<release>`, resolving to the live tip at creation time.

## In scope

- `internal/scheduler/` — `BuildPlan` topological phase ordering from `depends_on`;
  `finishTrack` auto-merges the track (via `MergeTrackFn`) before returning.
- `internal/run/parallel.go` — per-phase concurrency + `wg.Wait()` barrier between
  phases; dependent worktree branches from the `release-wt` tip at creation time;
  a failed dependency cancels (`failCancel`) so dependents in later phases are
  skipped rather than started on an unmerged tip.

## Out of scope

- Manual `sworn merge-track` CLI (that is S05).
- Circular dependency detection (a planner error surfaced by `BuildPlan`, not a
  runtime concern).
- Multiple dependency levels (A→B→C) — handled transitively by phase ordering.
- **Paused / stuck dependency handling** — deferred to S07 (see Deferrals).

## Acceptance checks (EARS)

- [ ] **AC1** — WHEN a release has `T5 depends_on T6` in index.md frontmatter and
  `sworn run` starts, THE scheduler SHALL place T5 in a later phase than T6 and
  SHALL NOT start T5's first slice until T6's phase has completed (`wg.Wait`
  returned for the prior phase).
- [ ] **AC2** — WHEN T5's phase begins, THE SYSTEM SHALL create T5's worktree
  branching from the current `release-wt` HEAD (the post-T6 tip), not from the
  pre-T6 release-wt tip captured at run start.
- [ ] **AC3** — WHEN a track's last slice transitions to `verified`, THE SYSTEM
  SHALL automatically merge that track to `release-wt` inside `finishTrack`
  (via `MergeTrackFn`) before the track goroutine returns, so `release-wt` carries
  the dependency's code before the barrier releases.
- [ ] **AC4** — WHEN a dependency track FAILS before merging, THE SYSTEM SHALL
  cancel (`failCancel`) so its dependent tracks in later phases are skipped and
  never started on an unmerged `release-wt` tip.
- [ ] **AC5** (test) — A two-track integration scenario (T_dep → T_main) over a
  real git repo SHALL assert that T_main's worktree is created from the
  post-T_dep `release-wt` tip (i.e. the dependency's merge commit is reachable
  from T_main's branch base) under the phase-barrier + finishTrack mechanism.

## Deferred (Rule 2)

- **Paused / stuck dependency handling → S07-pause-resume-committed.** WHEN a
  dependency track PAUSES (PAGE) or stalls without merging or failing, the
  dependent should be skipped, the stall surfaced, and the run must not deadlock.
  The current phase barrier handles the FAIL cascade (AC4) but not a *paused*
  dependency: a pause does not trigger `failCancel`, and `wg.Wait` can block on a
  stalled goroutine. **Why:** pause/resume + PAGE semantics are S07's domain
  (S07-pause-resume-committed owns committed-state pause/resume); wiring
  pause-aware dependent-skip + stall surfacing belongs with that machinery, not
  duplicated here. **Tracking:** S07-pause-resume-committed. **Acknowledged:**
  Brad, 2026-06-28 (this replan).

## Required tests

- **Unit**: `internal/scheduler/` table tests covering `BuildPlan` phase ordering
  from `depends_on` (incl. transitive A→B→C and the fail-cascade skip).
- **Integration**: end-to-end two-track scenario over a real git repo asserting
  the dependent worktree's branch point is the post-dependency `release-wt` tip
  (AC5).
- **Reachability artefact**: `go test ./internal/scheduler/... -v -run TestDependentTrack` exits 0.

## Risks

- The auto-merge inside `finishTrack` invokes the merge logic directly (via
  `MergeTrackFn`), not the S05 CLI gate wrapper — acceptable, since the CLI gate is
  a wrapper over the same merge path, and S05 is not a hard dependency of S04.
