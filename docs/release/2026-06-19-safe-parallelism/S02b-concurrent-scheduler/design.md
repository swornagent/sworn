# Design TL;DR — S02b-concurrent-scheduler

## §1. User-visible change

A developer can run `sworn run --parallel --release 2026-06-19-safe-parallelism` and the
binary reads the release board, discovers all tracks, topologically sorts them by their
`depends_on` edges, and launches independent tracks as concurrent goroutines. Dependent
tracks wait for their dependencies to complete. Each track runs in its own worktree
(materialised on-the-fly by the worker goroutine). The process prints per-track progress
on stderr (`[T1] starting`, `[T1] running slice …`, `[T1] done`) and exits 0 only if
every track passes all its slices. A track whose dependency fails is skipped with
`[T3] skipped: depends_on T1 failed`.

## §2. Design decisions not in spec (max 5)

1. **Board track parsing in `internal/board/` not `internal/scheduler/`.** The spec's
   Risk section says to reuse `internal/board/index.go` if it provides the tracks struct.
   It currently only validates structure. I'll add `ParseTracks(body string) []TrackInfo`
   to the board package so it is reusable by any consumer (scheduler, TUI, MCP). The
   scheduler calls this after reading `index.md` from disk.

2. **One worktree per track, materialised by the worker goroutine.** Each worker goroutine
   runs `git worktree add` if the track's worktree is absent. The release worktree is
   materialised as a **pre-flight step** in `RunParallel()` before any goroutines launch,
   avoiding the race condition described in the spec's Risks section. The pre-flight step
   also reaps stale supervisor rows.

3. **Failure cascade via context cancellation (direct dependents only).** When a track
   fails, we cancel the contexts of tracks that directly `depends_on` it (one level, not
   transitive). This matches AC-3 literally: "FAIL in T1 causes T3 to log skipped." The
   downstream track's worker checks `ctx.Err()` before each slice and skips cleanly.

4. **Dependency signalling uses a `sync.Map`-backed outcome store, not channels per edge.**
   Each completed track writes its outcome (pass/fail/skipped) into an outcome map. When
   all dependencies for a phase are done, the next phase starts. This avoids N×M channel
   plumbing and makes the summary collection trivial.

5. **RunParallel lives in `internal/run/parallel.go`, not in `internal/scheduler/`.** The
   scheduler is pure data: `ReadTracks()`, `BuildPlan()` → `ExecutionPlan` (phases of
   concurrent track sets). The goroutine orchestration (sync.WaitGroup, context trees,
   outcome map) lives in `internal/run/parallel.go` alongside `Run()` and `RunSlice()`.
   This keeps the scheduler testable without spawning real goroutines.

## §3. Files I'll touch grouped by purpose

- **New: board track parsing** — `internal/board/track.go` + `internal/board/track_test.go`
  TrackInfo type, ParseTracks() from frontmatter string, ParseTrackID() helper.
- **New: scheduler core** — `internal/scheduler/scheduler.go` (ExecutionPlan, BuildPlan,
  RunTrackOptions, TrackInfo re-exported), `internal/scheduler/scheduler_test.go`
  (3 test functions matching spec ACs).
- **New: worker** — `internal/scheduler/worker.go` (RunTrack — acquire supervisor,
  materialise worktree, call RunSlice per slice, release supervisor),
  `internal/scheduler/worker_test.go` (mock RunSlice, assert per-slice order).
- **New: parallel entry point** — `internal/run/parallel.go` (RunParallel — pre-flight
  release worktree, read board, build plan, fan-out goroutines per phase, collect
  outcomes, print summary, return exit code).
- **Touch: CLI flag wiring** — `cmd/sworn/run.go` (add `--parallel` and `--release`
  flags; when --parallel is set, call RunParallel instead of Run).

## §4. Things I'm NOT doing

- **TUI progress display.** S04b owns that. In S02b, progress is simple `[T1] starting`
  lines on stderr. No interactive display, no polling.
- **Credits metering.** S06b owns that. Not tracked here.
- **Failure notifications (webhooks, paging).** S07 owns that. Workers just log and
  propagate failure via context cancellation.
- **S03 — verify-under-concurrency.** That slice tests the verifier gate under N>1
  concurrent verify calls. S02b's acceptance checks use fake/scripted verifiers that
  return immediately; no concurrent real verifier calls happen in this slice.
- **Worktree deletion or cleanup.** Workers materialise worktrees. No cleanup logic
  (GitHub orphan worktrees from crashed runs are reaped by S01's stale-PID detection;
  actual directory cleanup is a future concern).
- **`depends_on` list validation beyond topological sort.** If a track declares a
  dep on a non-existent track, BuildPlan returns an error. No attempt to validate
  slice IDs against the spec or existence.

## §5. Reachability plan

The reachability artefact is a smoke step in proof.md:

```bash
# Build a 2-track fixture:
#   docs/release/S02b-fixture-2track/index.md
#   With T1 (independent), T2 (independent)
# Run sworn with test-only entry point or run.go patched for fixture path
# Observe stderr shows [T1] starting and [T2] starting before either done
```

In practice, the scheduler unit tests prove reachability via controlled
fake workers. The smoke step in proof.md documents a manual or integration
verification path. The test file `scheduler_test.go` includes a
`TestDependencyOrdering` that covers AC-1/AC-2 semantics with blocking
fake workers and timing assertions.

## §6. Open questions for the Coach

- The release worktree path must be known to `RunParallel`. Currently the
  release worktree path is stored in `index.md` frontmatter
  (`release_worktree_path`). Is it acceptable for the scheduler to parse
  this from the frontmatter, or should the path be passed as a CLI flag?
  -> **Proposal**: parse from frontmatter (single source of truth). The CLI
  takes `--release <name>`; the path `<release-worktree>/docs/release/<name>/index.md`
  is derived by convention (release worktree is at
  `~/.local/share/<project>/worktrees/release-<name>` per planner convention
  or stored in frontmatter). Currently the frontmatter stores
  `release_worktree_path`, so we read it from there.
- Where should the test fixture index.md live for scheduler tests? Temp dirs
  are fine for unit tests, but AC-1 and AC-2 describe an integration test
  that creates a real fixture index.md. Should I embed a fixture string in
  `scheduler_test.go` or write a fixture file?
  -> **Proposal**: embed fixture YAML strings in test files (no external
  fixture files). This keeps tests self-contained and avoids coupling test
  pass/fail to the state of any real index.md.