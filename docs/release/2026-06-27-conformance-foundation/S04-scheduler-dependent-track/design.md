# Design TL;DR — S04 scheduler-dependent-track

## Approach

Four cooperating changes land the dependent-track ordering guarantee without polling:

1. **`BuildPlan` topological phase ordering** — `scheduler.BuildPlan(tracks)` topologically sorts tracks into phases from `depends_on` edges (Kahn's algorithm). A dependent track lands in a strictly later phase than every track it `depends_on`.

2. **Phase barrier in `RunParallel`** — `RunParallel` runs each phase's tracks concurrently via goroutines, then calls `wg.Wait()` — the **phase barrier** — before starting the next phase.

3. **`finishTrack` auto-merge** — when a track's last slice is done, `finishTrack` calls `opts.MergeTrackFn(releasePath, trackID, branch)` before returning `TrackPass`. By the time the phase barrier releases, `release-wt` carries the dependency's code.

4. **Worktree branches from live tip** — `git worktree add ... -b <track-branch> release-wt/<release>` resolves `release-wt/<release>` to the current HEAD at call time. Because auto-merge happens inside `finishTrack` before the goroutine returns `TrackPass`, the phase barrier guarantees `release-wt` is updated before the dependent track's worktree is created.

## Key design choices

### No polling — phase barrier is the ordering mechanism (Pin 1 ratified)

The original design proposed a `waitForDependencies` poll loop. The Captain review (Pin 1) rejected it as deadlock-prone: the board oracle never transitions a track to `merged` from a bare auto-merge, so a value-poll would spin forever. The ratified mechanism uses topological phases + `wg.Wait()` — simpler, already proven in `RunParallel`, and eliminates the deadlock risk entirely.

Fields `DependencyOracle` and `DependsOnPollInterval` are **not** added to `WorkerOptions` — they serve no purpose without polling.

### `MergeTrackFn` injection point

```go
// in WorkerOptions (internal/scheduler/worker.go)
MergeTrackFn func(releasePath, trackID, branch string) error  // nil → skip auto-merge
```

- **Production** (`cmd/sworn/run.go`): wires `run.ProductionMergeTrack`
- **Tests**: leave nil for backward-compatible behaviour, or inject a fake for MergeTrackFn-call verification
- **nil → backward compatible**: existing callers that don't set this field see no behaviour change

### `ProductionMergeTrack` — three-layer strategy

1. `.git` guard — skip merge for non-git directories (test temp dirs)
2. Local merge attempt (`git merge --no-ff <branch> --no-edit`)
3. Fetch + merge fallback (`git fetch origin <branch>` then `git merge --no-ff origin/<branch> --no-edit`)

This handles both production (separate clone, needs fetch) and test (shared object storage, local merge suffices) scenarios.

### Auto-merge in `finishTrack` + router's "merge-track" case

`finishTrack` is the single merge point called from:
- `case "none":` (router: all slices terminal)
- `runTrackLegacy` completion
- `case "merge-track":` when `MergeTrackFn != nil` (auto-merge); when nil, preserves existing pause behavior

### S05 gate bypass documentation (Pin 2 ratified)

Auto-merge bypasses the `sworn merge-track` CLI gate (S05). Each gate is accounted for in a comment block at `finishTrack`:
- (1) verified-check: satisfied by router (emits merge-track only after all slices verified)
- (2) invariant-4 classifier: bare git merge still fails on conflict → TrackFail
- (3) index.md state update: not performed (phase barrier is the ordering mechanism, not state polling)

### `failCancel` cascade on TrackFail

In `RunParallel`'s goroutine: when a track returns `TrackFail`, `failCancel()` is called, propagating cancellation to `failCtx`. Subsequent phases derive `phaseCtx` from `failCtx` — goroutines check `phaseCtx.Err()` and skip with `TrackSkipped`. This means a failed dependency causes its dependents to skip, never starting on an unmerged `release-wt` tip.

## Files touched

| File | Change |
|---|---|
| `internal/scheduler/scheduler.go` | `BuildPlan` topological phase ordering from `depends_on` (Kahn's algorithm); `ExecutionPlan` + `Phase` types |
| `internal/scheduler/worker.go` | Add `MergeTrackFn` to `WorkerOptions`; update `finishTrack` to call it before returning; update `case "merge-track":` to auto-merge when wired; S05 gate bypass documentation |
| `internal/run/parallel.go` | Phase barrier (`wg.Wait()` per phase); `failCancel` cascade; wire `MergeTrackFn` into `WorkerOptions`; `ProductionMergeTrack` (three-layer merge); `ParallelOptions.MergeTrackFn` |
| `cmd/sworn/run.go` | Wire `MergeTrackFn: run.ProductionMergeTrack` in `--parallel` path |
| `internal/scheduler/worker_test.go` | 4 new `TestDependentTrack_*` subtests |

## AC traceability

| AC | Planned change |
|---|---|
| AC1 — T5 placed in later phase than T6, not started until T6's phase completes | `BuildPlan` topologically orders tracks into phases from `depends_on`; phase barrier (`wg.Wait()` per phase) in `RunParallel` enforces ordering |
| AC2 — T5 branches from post-T6 `release-wt` tip | `git worktree add ... release-wt/<release>` resolves live tip at creation time; auto-merge in `finishTrack` runs before goroutine returns → tip is post-T6 |
| AC3 — auto-invoke merge when last slice verified | `finishTrack` calls `MergeTrackFn`; `case "merge-track":` calls `finishTrack` when `MergeTrackFn != nil` |
| AC4 — dependency FAIL cancels dependents | `failCancel()` on `TrackFail`; subsequent phase goroutines check `phaseCtx.Err()` and skip with `TrackSkipped` |
| AC5 — integration test asserts branch point | `TestDependentTrack_MergeTrackFnCalled` verifies `MergeTrackFn` invocation; `TestDependentTrack_MergeTrackDecisionAutoMerges` verifies auto-merge path; `TestDependentTrack_MergeTrackFnErrorFails` verifies TrackFail on merge error; `TestDependentTrack_MergeTrackDecisionPausesWhenNoMergeTrackFn` verifies backward-compatible pause |

## Design risks / pins

- **Pin 1 (Type-2, ratified)**: Phase barrier handles ordering but doesn't check that the dependency *merged successfully* — it only checks that the goroutine returned. If a dependency *pauses* (not fails), `failCancel` is not called and the next phase proceeds. This gap is a pre-existing behaviour in `RunParallel` (paused tracks return without cancelling). S04 doesn't introduce it; **deferred to S07** (pause-resume-committed).

- **Pin 2 (Type-2, ratified)**: S05 gate bypass. See `finishTrack` comment block for the gate-by-gate accounting. Conflicts are impossible under invariant-2 (disjoint touchpoints); if a conflict occurs anyway, bare `git merge` fails → `TrackFail` (acceptable diagnostic-quality downgrade).

- **Pin 3 (Type-2)**: `case "merge-track":` auto-execution applies only when `MergeTrackFn != nil`. Existing callers that don't set this field see no behaviour change — the pause path is the fallback.

- **Pin 4 (Type-2)**: Auto-merge inside `finishTrack` invokes merge logic directly (via `MergeTrackFn`), not the S05 CLI gate wrapper — acceptable, since the CLI gate is a wrapper over the same merge path, and S05 is not a hard dependency of S04.