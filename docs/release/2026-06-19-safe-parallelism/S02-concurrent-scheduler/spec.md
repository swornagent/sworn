---
title: 'S02-concurrent-scheduler — parallel track execution'
description: 'sworn run --parallel launches all independent tracks concurrently in isolated worktrees; dependent tracks start only after their dependency merges.'
---

# Slice: `S02-concurrent-scheduler`

## User outcome

A developer runs `sworn run --parallel` on a multi-track release and all independent
tracks start concurrently in their own worktrees, dependent tracks wait for their
dependency to merge, and a final summary reports the outcome of every track.

## Entry point

`sworn run --parallel` flag on `cmd/sworn/run.go`; or auto-detection: if the release
board contains more than one track with `state: planned`, parallel mode is implied.

## In scope

- `internal/scheduler/` package:
  - `scheduler.go`: reads `docs/release/<release>/index.md` frontmatter to discover
    tracks, their `depends_on` edges, and slice order; produces a dependency-ordered
    execution plan
  - `worker.go`: single-track worker — materialises the track worktree if absent (using
    the same logic as `/implement-slice`), then runs slices sequentially via `run.Run()`
  - Goroutine-per-track: one goroutine per independent track; dependent tracks block on
    a channel signal from the dependency track's goroutine
  - Failure semantics: a FAIL in T1 cancels T3 if `T3 depends_on T1`; T2 (independent)
    continues unaffected
  - DB integration: each worker calls `supervisor.Acquire(release, trackID)` on start,
    `supervisor.Release` on finish; updates track state in the events table
  - Progress: each worker logs to stderr prefixed with `[T1]`, `[T2]` etc.
  - Exit code: 0 only if all tracks complete with PASS verdict on all slices
- `internal/run/parallel.go`: entry point that takes a release name, constructs the
  scheduler, and runs it; called from `cmd/sworn/run.go` in parallel mode
- Worktree materialisation: if a track worktree does not exist, the worker creates it
  by branching from `release-wt/<release>` at the current HEAD (mirroring
  `/implement-slice` first-slice logic)

## Out of scope

- The process registry itself (S01 — this slice depends on it being done first)
- Verify-gate goroutine safety (S03)
- TUI display of concurrent progress (S04)
- Credits metering (S06)
- Notification on failure (S07)
- Replanning / replan-release flows

## Planned touchpoints

- `internal/scheduler/scheduler.go` (new)
- `internal/scheduler/scheduler_test.go` (new)
- `internal/scheduler/worker.go` (new)
- `internal/scheduler/worker_test.go` (new)
- `internal/run/parallel.go` (new)
- `internal/run/run.go` (touch — add parallel entry point; refactor single-slice path
  to be callable from the worker)
- `cmd/sworn/run.go` (touch — `--parallel` flag; release name flag; read board)

## Acceptance checks

- [ ] `sworn run --parallel --release 2026-06-19-safe-parallelism` on a 3-track release
  (T1 independent, T2 independent, T3 `depends_on T1`) starts T1 and T2 simultaneously
  (both log `[T1] starting` and `[T2] starting` before either completes) and T3 starts
  only after T1 logs `[T1] done`
- [ ] Each track runs in a distinct worktree path (confirmed by the worker logging the
  worktree path at start)
- [ ] A FAIL in T2 does not cancel T1 or T3 (T3 still waits on T1, not T2)
- [ ] A FAIL in T1 cancels T3 with a clear `[T3] skipped: depends_on T1 failed` log
  line and sets T3's state to `failed_verification` in the DB
- [ ] `sworn run --parallel` exit code is 1 if any track fails, 0 only if all succeed
- [ ] Supervisor `Acquire` is called per track; `Release` is called on normal exit and
  on failure (deferred); DB row reflects final state
- [ ] `go test -race ./internal/scheduler/...` passes with zero data race findings

## Required tests

- **Unit**: `internal/scheduler/scheduler_test.go`
  — `TestDependencyOrdering`: 3-track plan; assert T1+T2 start before T3; assert T3
    starts after T1 completes (use fake workers with controllable timing)
  — `TestFailurePropagation`: T1 fails; assert T3 is cancelled; T2 proceeds
  — `TestAllSucceed`: all tracks succeed; exit is success; all DB rows state=done
- **Unit**: `internal/scheduler/worker_test.go`
  — `TestWorkerMaterialisesWorktree`: worktree absent; worker creates it; runs
- **Integration**: `internal/run/parallel.go` covered by scheduler integration test
  using a fixture release with real (but tiny) spec files and mock `run.Run`
- **Reachability artefact**: smoke step — `sworn run --parallel --release <fixture>`
  on a 2-track fixture release; observe both `[T1]` and `[T2]` prefixes in stderr
  output before either completes; document in proof.md.

## Risks

- Worktree materialisation races: if two workers race to create the same base
  `release-wt/<release>` branch (the release worktree), they can conflict. Mitigation:
  the release worktree is created by the scheduler before launching any workers
  (sequential pre-flight step), not by individual workers.
- `run.Run()` refactor risk: the single-slice run loop is not currently designed to be
  called from multiple goroutines. The refactor in `run.go` must not introduce shared
  mutable state. The race detector in tests catches this.

## Deferrals allowed?

No. The concurrent scheduler is the R3 core deliverable. The benchmark (S05) and all
downstream slices require parallel execution to be working.
