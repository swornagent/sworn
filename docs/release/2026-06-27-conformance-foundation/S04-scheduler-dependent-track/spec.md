---
title: 'S04 — Scheduler dependent-track branch from dependency tip'
description: 'Dependent track worktrees branch from the dependency track''s merge tip (release-wt after finishTrack), not from the release-wt that may lack the dependency''s code.'
---

# Slice: `S04-scheduler-dependent-track`

## User outcome

When a release has a track T5 with `depends_on: T6`, the scheduler waits until T6 has merged to `release-wt/<release>`, then creates T5's worktree branching from the post-T6 `release-wt` tip — so T5's implementer starts with T6's code already present.

## Entry point

`sworn run --release <name>` → `internal/scheduler/` — the scheduler reads the `depends_on` fields from `index.md` tracks frontmatter and enforces the ordering.

## In scope

- `internal/scheduler/` (worker.go or a new `scheduler.go` at the package level): before starting a track's first slice, check whether the track has `depends_on` set; if yes, poll until the dependency track's state is `merged` in the oracle
- When the dependency is merged, create the track worktree branching from the current `release-wt` tip (not from the tip that existed when `sworn run` started)
- `internal/run/parallel.go` (or the track setup in worker.go): update `git worktree add` call to branch from `release-wt` tip at the moment of worktree creation, not at run-start time
- Auto-merge finishTrack: when a track's last slice reaches `verified`, the scheduler invokes `sworn merge-track` automatically (or the equivalent `internal/merge` call) so `release-wt` is updated before dependent tracks can start

## Out of scope

- Manual `sworn merge-track` CLI (that is S05)
- Circular dependency detection (surfaces as a planner error, not a runtime concern)
- Multiple levels of dependency (A→B→C) — the same mechanism handles transitively; each track waits for its direct dependency

## Planned touchpoints

- `internal/scheduler/worker.go` (add depends_on polling + auto-merge-track trigger)
- `internal/run/parallel.go` (update worktree branch point to release-wt tip at creation time)

## Acceptance checks

- [ ] WHEN a release has `T5 depends_on T6` in index.md frontmatter and `sworn run` starts, THE SYSTEM SHALL NOT start T5's first slice until T6's track state is `merged`
- [ ] WHEN T6 reaches `merged` state, THE SYSTEM SHALL create T5's worktree branching from the current `release-wt` HEAD (not from the pre-T6 release-wt tip)
- [ ] WHEN a track's last slice transitions to `verified`, THE SYSTEM SHALL automatically invoke merge-track for that track before the scheduler loop continues (finishTrack auto-merge)
- [ ] IF the dependency track never merges (stuck or paused), THE SYSTEM SHALL not start the dependent track but also not deadlock — it polls with a configurable interval (default 30s) and surfaces the stall via the TUI or log
- [ ] Test: run a two-track scenario (T_dep → T_main) in a test worktree; assert T_main's worktree branches from the post-T_dep release-wt tip

## Required tests

- **Unit**: table test in `internal/scheduler/` covering the depends_on ordering logic with a mock oracle
- **Integration**: end-to-end scenario with a real git repo: two tracks with depends_on, assert correct branch points
- **Reachability artefact**: `go test ./internal/scheduler/... -v -run TestDependentTrack` exits 0

## Risks

- The auto-merge-track call may require the merge gate (S05) to be complete first — if S05 is not yet merged when T1 ships, the auto-trigger can call the underlying merge logic directly (bypassing the CLI gate check); the CLI gate in S05 is a wrapper, not the only path

## Deferrals allowed?

No. If parallel.go (T1 file) and worker.go (T1 file) both need changes, that is expected — both are in T1.
