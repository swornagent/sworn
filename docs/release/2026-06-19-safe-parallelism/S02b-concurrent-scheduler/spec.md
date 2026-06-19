---
title: 'S02b-concurrent-scheduler — parallel track execution via goroutines'
description: 'sworn run --parallel reads the release board, launches one goroutine per independent track in its own worktree, serialises dependent tracks, and reports a combined outcome.'
---

# Slice: `S02b-concurrent-scheduler`

## User outcome

A developer runs `sworn run --parallel --release <name>` and all independent tracks
start concurrently in isolated worktrees; dependent tracks start only after their
dependency merges; a combined pass/fail summary is printed when all tracks complete.

## Entry point

`sworn run --parallel --release <release-name>` flag combination on `cmd/sworn/run.go`.

## In scope

- `internal/scheduler/scheduler.go`: reads `index.md` frontmatter to discover tracks
  and `depends_on` edges; topologically sorts into independent and dependent sets;
  returns an execution plan
- `internal/scheduler/worker.go`: single-track goroutine — materialises the track
  worktree if absent (branch from `release-wt/<release>` at HEAD), runs each
  planned slice sequentially via `run.RunSlice()`, calls
  `supervisor.Acquire/Release()` (S01), updates track state in the DB events table
- `internal/run/parallel.go`: entry point — constructs scheduler, fans out goroutines,
  uses `sync.WaitGroup` + channels for dependency signalling, collects outcomes
- `cmd/sworn/run.go`: `--parallel` flag; `--release` flag; in parallel mode reads board
  instead of accepting `--task`
- Failure semantics: FAIL in T1 cancels any track with `depends_on: T1` (via context
  cancellation); independent tracks continue
- Exit code: 0 only if all tracks complete PASS on all slices
- Progress: per-track stderr lines prefixed `[T1]`, `[T2]`, etc.

## Out of scope

- The RunSlice() function itself (S02a — prerequisite)
- TUI display of concurrent progress (S04b)
- Credits metering per track (S06b)
- Failure notifications (S07)

## Planned touchpoints

- `internal/scheduler/scheduler.go` (new)
- `internal/scheduler/scheduler_test.go` (new)
- `internal/scheduler/worker.go` (new)
- `internal/scheduler/worker_test.go` (new)
- `internal/run/parallel.go` (new)
- `cmd/sworn/run.go` (touch — `--parallel`, `--release` flags)

## Acceptance checks

- [ ] `sworn run --parallel --release <name>` on a 2-track fixture (T1 independent,
  T2 independent) shows `[T1] starting` and `[T2] starting` before either completes
- [ ] A 3-track fixture (T1 and T2 independent, T3 `depends_on T1`): T3 does not log
  `starting` until T1 logs `done`
- [ ] FAIL in T1 causes T3 to log `[T3] skipped: depends_on T1 failed` and appear in
  the summary as failed; T2 completes normally
- [ ] Exit code is 0 when all tracks pass; 1 when any track fails
- [ ] Each worker calls `supervisor.Acquire` on start and `supervisor.Release` on exit
  (both normal and error paths); DB row reflects final state
- [ ] `go test -race ./internal/scheduler/...` passes with zero data race findings

## Required tests

- **Unit**: `internal/scheduler/scheduler_test.go` — `TestDependencyOrdering`,
  `TestFailurePropagation`, `TestAllSucceed` (using fake workers with controllable
  timing channels)
- **Unit**: `internal/scheduler/worker_test.go` — `TestWorkerMaterialisesWorktree`,
  `TestWorkerCallsRunSlice` (mock RunSlice, assert called per slice in order)
- **Reachability artefact**: smoke step — `sworn run --parallel --release <fixture>`
  on a 2-track fixture; observe both `[T1]` and `[T2]` prefixes in stderr output.
  Document in proof.md.

## Risks

- Worktree materialisation race: if the release worktree (`release-wt/<release>`)
  doesn't exist, two workers may try to create it simultaneously. Mitigation: the
  parallel entry point checks and creates the release worktree as a sequential
  pre-flight step before launching any goroutines.
- Reading `index.md` frontmatter YAML is already done by existing tooling; reuse
  `internal/board/index.go` (R2) if it provides the tracks struct, rather than
  parsing YAML independently.

## Deferrals allowed?

No. This is the core deliverable of R3.
