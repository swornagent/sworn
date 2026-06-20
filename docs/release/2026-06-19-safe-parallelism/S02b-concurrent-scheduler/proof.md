# Proof bundle — S02b-concurrent-scheduler

## Scope

`sworn run --parallel --release <name>` reads the release board, discovers all tracks, topologically sorts them by their `depends_on` edges, and launches independent tracks as concurrent goroutines. Dependent tracks wait for their dependencies to complete. Each track runs in its own worktree (materialised on-the-fly by the worker goroutine). Exit 0 only if every track passes all its slices.

## Files changed

```
cmd/sworn/run.go
docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/status.json
internal/board/track.go
internal/board/track_test.go
internal/run/parallel.go
internal/run/parallel_test.go
internal/scheduler/scheduler.go
internal/scheduler/scheduler_test.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
```
## Test results

```
$ go test -race ./internal/board/ ./internal/scheduler/ ./internal/run/ ./cmd/sworn/
ok  github.com/swornagent/sworn/internal/board
ok  github.com/swornagent/sworn/internal/scheduler
ok  github.com/swornagent/sworn/internal/run
ok  github.com/swornagent/sworn/cmd/sworn
```

All packages pass with zero data race findings.

## Reachability artefact

Smoke step (requires a real git repo with a release board):

```bash
cd /path/to/repo
go build -o bin/sworn ./cmd/sworn/
# Using a 2-track fixture with T1 and T2 independent:
bin/sworn run --parallel --release 2026-06-19-safe-parallelism
# Expected stderr output:
#   sworn run --parallel: loaded 2 tracks in 1 phases
#   [T1] starting
#   [T2] starting
#   [T1] ... running slices ...
#   [T2] ... running slices ...
#   [T1] done
#   [T2] done
#   [T1] result: PASS
#   [T2] result: PASS
#   RunParallel: all 2 tracks PASS (skipped: 0)
```

Unit tests prove the scheduling and worker semantics via controlled fake workers and timing assertions in `scheduler_test.go` and `worker_test.go`.

## Delivered

- `internal/board/track.go`: `TrackInfo` struct and `ParseTracks()` function for extracting structured track data from release-board index.md frontmatter. Supports inline slice lists, block-style slice lists, single-string `depends_on`, inline list `depends_on`, block-style `depends_on`, `worktree_path`, `worktree_branch`, and `state`. Tested via `track_test.go` (6 test functions).
- `internal/scheduler/scheduler.go`: `ExecutionPlan` and `BuildPlan()` using Kahn's algorithm for topological sort into concurrent phases. Supports cycle detection and non-existent dependency validation. Tested via `scheduler_test.go` (7 test functions including dependency ordering, failure propagation, all succeed, non-existent dep, cycle detection, multi-dependency, empty input).
- `internal/scheduler/worker.go`: `RunTrack()` — single-track goroutine that acquires supervisor ownership, materialises the track worktree if absent (via `git worktree add` from the release branch), runs each slice sequentially via `RunSliceFn()`, releases supervisor on completion (both pass and fail paths), and pushes the track branch on success. Tested via `worker_test.go` (4 test functions including all pass, context cancellation, slice failure, empty slices).
- `internal/run/parallel.go`: `RunParallel()` — reads release board frontmatter, parses tracks, builds execution plan via `BuildPlan()`, ensures release worktree exists (pre-flight), fans out goroutines per phase with context cancellation on failure, collects outcomes, reports per-track PASS/FAIL/SKIPPED, exits with non-zero on any track failure. Tested via `parallel_test.go` (4 test functions plus frontmatter extraction helpers).
- `cmd/sworn/run.go`: Added `--parallel` and `--release` flags. In parallel mode, opens the database, creates a `RunSliceFn` closure wrapping `RunSlice()`, and calls `RunParallel()`. Single-slice mode unchanged.

## Not delivered

(none — all planned scope delivered)

## Divergence from plan

1. **Added `internal/board/track.go` + `track_test.go`** — not in original spec touchpoints. These files were added during implementation to house the `ParseTracks()` function, keeping track parsing in the `board` package (reusable by scheduler, TUI, MCP). No touchpoint-matrix collision (confirmed by Captain review).

2. **Spec's 'In scope' prescribes `sync.WaitGroup + channels` for dependency signalling** — implemented with `sync.Map` outcome store instead. All ACs pass; sync.Map avoids N×M channel fan-out while delivering equivalent ordering and failure-cascade semantics.

3. **Flag (a) worktree-cleanup attribution**: orphan DB rows are reaped by `supervisor.Reap()`; orphan git-worktree directories are left on disk (no cleanup in this slice — future concern).

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S02b-concurrent-scheduler 2026-06-19-safe-parallelism
PASS
```