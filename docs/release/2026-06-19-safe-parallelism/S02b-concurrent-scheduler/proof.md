# Proof bundle — S02b-concurrent-scheduler

## Scope

`sworn run --parallel --release <name>` reads the release board, discovers all tracks, topologically sorts them by their `depends_on` edges, and launches independent tracks as concurrent goroutines. Dependent tracks wait for their dependencies to complete. Each track runs in its own worktree (materialised on-the-fly by the worker goroutine). Exit 0 only if every track passes all its slices.

## Files changed (from start_commit 821edf2)

All files in the diff range `821edf2..HEAD` (19 files):

```
cmd/sworn/run.go
docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/journal.md
docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/proof.md
docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/status.json
docs/release/2026-06-19-safe-parallelism/S26-telemetry/spec.md
docs/release/2026-06-19-safe-parallelism/S26-telemetry/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/board/track.go
internal/board/track_test.go
internal/prompt/captain.md
internal/prompt/implementer.md
internal/run/parallel.go
internal/run/parallel_test.go
internal/scheduler/scheduler.go
internal/scheduler/scheduler_test.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
sworn
```

The diff range spans 14 commits from the original implementation (`5bb3666') through round-1 and round-2 verifier fixes. All planned touchpoints and test additions are in the diff.

Files from other slices appearing due to forward-merges of `release-wt/2026-06-19-safe-parallelism`: `docs/release/2026-06-19-safe-parallelism/index.md`, `S26-telemetry/`, `internal/prompt/`. The `sworn` binary is tracked in the repo (pre-existing issue). These are documented in Divergence from plan.

## Test results

```
$ go test -race ./internal/board/ ./internal/scheduler/ ./internal/run/ ./cmd/sworn/
ok  github.com/swornagent/sworn/internal/board	1.056s
ok  github.com/swornagent/sworn/internal/scheduler	(cached)
ok  github.com/swornagent/sworn/internal/run	2.538s
ok  github.com/swornagent/sworn/cmd/sworn	1.247s

$ go test -race ./internal/...
ok  github.com/swornagent/sworn/internal/adopt	(cached)
ok  github.com/swornagent/sworn/internal/agent	(cached)
ok  github.com/swornagent/sworn/internal/bench	1.650s
ok  github.com/swornagent/sworn/internal/board	(cached)
ok  github.com/swornagent/sworn/internal/config	(cached)
ok  github.com/swornagent/sworn/internal/db	(cached)
ok  github.com/swornagent/sworn/internal/designaudit	(cached)
ok  github.com/swornagent/sworn/internal/designfit	(cached)
ok  github.com/swornagent/sworn/internal/ears	(cached)
ok  github.com/swornagent/sworn/internal/git	(cached)
ok  github.com/swornagent/sworn/internal/implement	1.329s
ok  github.com/swornagent/sworn/internal/journey	(cached)
ok  github.com/swornagent/sworn/internal/model	(cached)
ok  github.com/swornagent/sworn/internal/prompt	1.017s
ok  github.com/swornagent/sworn/internal/reqvalidate	(cached)
ok  github.com/swornagent/sworn/internal/reqverify	(cached)
ok  github.com/swornagent/sworn/internal/rtm	(cached)
ok  github.com/swornagent/sworn/internal/run	(cached)
ok  github.com/swornagent/sworn/internal/scheduler	(cached)
ok  github.com/swornagent/sworn/internal/specquality	(cached)
ok  github.com/swornagent/sworn/internal/state	(cached)
ok  github.com/swornagent/sworn/internal/supervisor	(cached)
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  github.com/swornagent/sworn/internal/verify	1.024s
```

All packages pass with zero data race findings.

## Reachability artefact — concurrency proof

The following unit test proves that two independent tracks (T1, T2) start concurrently: both print `starting` before either completes. This is the AC-1 semantic (`[T1] starting` and `[T2] starting` appear before either `done`).

### Test: TestRunParallel_TimingConcurrency

```
sworn run --parallel: loaded 2 tracks in 1 phases
[T1] starting
[T2] starting
[T1] running slice S01-t1
[T2] running slice S02-t2
[T1] done
[T2] done
[T1] result: PASS
[T2] result: PASS
RunParallel: all 2 tracks PASS (skipped: 0)
```

Both T1 and T2 `starting` lines appear before either `done` — proving concurrent launch (AC-1).

### Test: TestRunParallel_FailureCascade

```
sworn run --parallel: loaded 3 tracks in 2 phases
[T1] starting
[T2] starting
[T1] running slice S01-t1-slice
[T1] slice S01-t1-slice failed: simulated T1 failure
[T2] running slice S02-t2-slice
[T2] done
[T3] skipped: depends_on failed (phase barrier)
[T1] result: FAIL
[T2] result: PASS
[T3] result: SKIPPED
```

This proves: AC-3 (T1 fails → T3 skipped, T2 completes normally), AC-4 (error returned when any track fails).

### Test: TestRunTrack_MaterialisesWorktree

```
[T1] starting
[T1] materialising worktree at /tmp/TestRunTrack_MaterialisesWorktree*/nonexistent-worktree
[T1] worktree materialisation failed: exit status 128
  fatal: not a git repository (or any of the parent directories): .git
```

Proves the worktree materialisation branch (line 94-121 in worker.go) is exercised. The git command fails because the temp dir has no repo — the code path is proven entered.

## Delivered

- `internal/board/track.go`: `TrackInfo` struct and `ParseTracks()` function for extracting structured track data from release-board index.md frontmatter. Supports inline slice lists, block-style slice lists, single-string `depends_on`, inline list `depends_on`, block-style `depends_on`, `worktree_path`, `worktree_branch`, and `state`. Tested via `track_test.go` (6 test functions).
- `internal/scheduler/scheduler.go`: `ExecutionPlan` and `BuildPlan()` using Kahn's algorithm for topological sort into concurrent phases. Supports cycle detection and non-existent dependency validation. Tested via `scheduler_test.go` (7 test functions including dependency ordering, failure propagation, all succeed, non-existent dep, cycle detection, multi-dependency, empty input).
- `internal/scheduler/worker.go`: `RunTrack()` — single-track goroutine that acquires supervisor ownership, materialises the track worktree if absent (via `git worktree add` from the release branch), runs each slice sequentially via `RunSliceFn()`, releases supervisor on completion (both pass and fail paths), and pushes the track branch on success. Tested via `worker_test.go` (6 test functions):
  - `TestRunTrack_AllSlicesPass` — single slice passes
  - `TestRunTrack_MultiSliceOrdering` — 3 slices called in correct order (Verifier Fix 2)
  - `TestRunTrack_MaterialisesWorktree` — exercises materialisation branch (Verifier Fix 1)
  - `TestRunTrack_ContextCancelled` — cancelled context → TrackSkipped
  - `TestRunTrack_SliceFail` — failing slice → TrackFail
  - `TestRunTrack_EmptySlices` — no slices → TrackPass
- `internal/run/parallel.go`: `RunParallel()` — reads release board frontmatter, parses tracks, builds execution plan via `BuildPlan()`, ensures release worktree exists (pre-flight), fans out goroutines per phase with context cancellation on failure, collects outcomes, reports per-track PASS/FAIL/SKIPPED, exits with non-zero on any track failure. Tested via `parallel_test.go` (6 test functions):
  - `TestRunParallel_Basic` — 2-track fixture all pass
  - `TestRunParallel_FailureCascade` — T1 fail → T3 skipped, T2 passes (Verifier Fix 3)
  - `TestRunParallel_TimingConcurrency` — both tracks start before either completes (Verifier Fix 4)
  - `TestRunParallel_ReleaseWorktreePathMissing` — error when path absent
  - `TestRunParallel_NoTracks` — error when no tracks
  - `TestRunParallel_MissingIndex` — error when index.md missing
- `cmd/sworn/run.go`: Added `--parallel` and `--release` flags. In parallel mode, opens the database, creates a `RunSliceFn` closure wrapping `RunSlice()`, and calls `RunParallel()`. Single-slice mode unchanged.
- Various frontmatter helper functions: `extractFrontmatter`, `extractReleaseWorktreePath`, `dirExists` in `parallel.go`.

## Not delivered

(none — all planned scope delivered)

**Carried forward acked deferral**: Coach acknowledged via approved-ack.md (prior round): "orphan DB rows are reaped by supervisor.Reap(); orphan git-worktree directories left on disk (no cleanup in this slice)." **Acknowledged**: Coach, 2026-06-27.

## Divergence from plan

1. **Added `internal/board/track.go` + `track_test.go`** — not in original spec touchpoints. These files were added during implementation to house the `ParseTracks()` function, keeping track parsing in the `board` package (reusable by scheduler, TUI, MCP). No touchpoint-matrix collision (confirmed by Captain review).

2. **Spec's 'In scope' prescribes `sync.WaitGroup + channels` for dependency signalling** — implemented with `sync.Map` outcome store instead. All ACs pass; sync.Map avoids N×M channel fan-out while delivering equivalent ordering and failure-cascade semantics.

3. **Flag (a) worktree-cleanup attribution**: orphan DB rows are reaped by `supervisor.Reap()`; orphan git-worktree directories are left on disk (no cleanup in this slice — future concern).

4. **Added `internal/run/parallel_test.go`** — not in original spec touchpoints but required by the test-first approach. The spec's "Required tests" section describes integration-level tests for the scheduler, which drove the creation of parallel_test.go as the natural home for RunParallel-related tests (frontmatter extraction, fixture-driven execution). All tests are self-contained (no external fixture files).

5. **Added 4 new test functions beyond the original round** — addressing verifier violations:
   - `TestRunTrack_MultiSliceOrdering` (worker_test.go) — multi-slice ordering assertion
   - `TestRunTrack_MaterialisesWorktree` (worker_test.go) — exercises materialisation code path
   - `TestRunParallel_FailureCascade` (parallel_test.go) — AC-3 failure cascade
   - `TestRunParallel_TimingConcurrency` (parallel_test.go) — AC-1 concurrency assertion with channel synchronisation
   - (Plus `blockingRunSlice`, fixed `fakeRunSliceFail`, fixed `fakeRunSliceTrackFail`)

6. **Forward-merge artefacts in diff range** — The diff base `821edf2..HEAD` includes 6 files from other slices and track merges (`docs/release/2026-06-19-safe-parallelism/index.md`, `S26-telemetry/spec.md` and `status.json`, `internal/prompt/captain.md` and `implementer.md`, `docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/approved-ack.md`). These were pulled in during `git merge release-wt/2026-06-19-safe-parallelism` operations and are not part of this slice's scope. The `sworn` binary (tracked in repo) is also a pre-existing diff artefact — excluded by `.gitignore` on rebuild.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S02b-concurrent-scheduler 2026-06-19-safe-parallelism
PASS
```