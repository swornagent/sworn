# S04-scheduler-dependent-track — Implementation Journal

## 2026-07-28 — implementation session

### State transition: design_review → in_progress → implemented

Design review acknowledged with DECISION: PROCEED. Two mechanical pins applied inline:

**Pin 1**: Dropped `waitForDependencies` entirely. The phase barrier in `RunParallel` (`wg.Wait()` per phase) already enforces AC1 (dependent tracks don't start until dependency-phase goroutines return). `finishTrack` calls `MergeTrackFn` before returning, so the release-wt tip is updated before the phase barrier releases the next phase. AC4 handled by `ctx.Done()` + `failCancel` on `TrackFail`.

**Pin 2**: Documented S05 gate bypass in `finishTrack` comment block:
- (1) verified-check: satisfied by router (emits merge-track only after all slices verified)
- (2) invariant-4 classifier: bare git merge still fails on conflict → TrackFail (acceptable downgrade)
- (3) index.md state update: not performed (phase barrier is the ordering mechanism, not state polling)

### Changes made

1. **`WorkerOptions.MergeTrackFn`** — new field: `func(releasePath, trackID, branch string) error`. Nil by default (backward-compatible). When set, `finishTrack` calls it before returning.

2. **`finishTrack`** — now accepts `ctx context.Context` (was `_`). Calls `MergeTrackFn` after push. Returns `TrackFail` on merge error. Includes S05 gate bypass documentation.

3. **`case "merge-track":`** — when `MergeTrackFn != nil`, calls `finishTrack` directly (auto-merge). When nil, preserves existing pause behavior.

4. **`ProductionMergeTrack`** — new exported function in `internal/run/`. Three-layer strategy: `.git` guard (skip for non-git dirs), local merge attempt, fetch+merge fallback.

5. **`ParallelOptions.MergeTrackFn`** — new optional field, wired from `WorkerOptions`. Tests leave nil; CLI sets `run.ProductionMergeTrack`.

6. **`cmd/sworn/run.go`** — wires `MergeTrackFn: run.ProductionMergeTrack` in `--parallel` path.

7. **Tests** — 4 new `TestDependentTrack_*` subtests in `worker_test.go`:
   - `MergeTrackFnCalled`: verifies finishTrack calls MergeTrackFn
   - `MergeTrackFnErrorFails`: verifies TrackFail on merge error
   - `MergeTrackDecisionAutoMerges`: verifies merge-track auto-merges when MergeTrackFn set
   - `MergeTrackDecisionPausesWhenNoMergeTrackFn`: verifies backward-compatible pause when nil

### Decisions

- Chose resolution (a) for Pin 1: drop waitForDependencies entirely. The phase barrier is simpler, already proven, and eliminates the deadlock risk entirely.
- `DependencyOracle` and `DependsOnPollInterval` fields NOT added to `WorkerOptions` — unnecessary without waitForDependencies.
- `ProductionMergeTrack` uses `.git` guard + local-merge-first + fetch fallback to handle both production (separate clone) and test (shared object storage) scenarios.
- `MergeTrackFn` made injectable through `ParallelOptions` (not hardcoded) so tests can control merge behavior without git repos.

### Trade-offs

- The phase barrier handles ordering but doesn't check that the dependency actually *merged* successfully — it only checks that the goroutine returned. If a dependency track *pauses* (not fails), `failCancel` is not called and the next phase proceeds. This is a pre-existing behavior in `RunParallel` (paused tracks return without cancelling). S04 doesn't introduce this gap; S07 (pause-resume-committed) may address it.
- `pauseSet` map declared at worker.go:54-62 is dead code (noted by the Captain). Removed as out of scope for S04 — low-priority cleanup.