# Design TL;DR — S04 scheduler-dependent-track

## Approach

Three cooperating changes land the dependent-track ordering guarantee:

1. **`finishTrack` auto-merge** — when a track's last slice is done, the scheduler merges the track branch into `release-wt/<release>` directly rather than pausing for a human `/merge-track`.

2. **`waitForDependencies` poll loop** — before materialising a track worktree, `RunTrack` polls the board oracle until every `DependsOn` track has `State == "merged"`. Polls every `DependsOnPollInterval` (default 30 s); respects `ctx.Done()` for cancellation.

3. **Worktree branches from live tip** — `git worktree add ... -b <track-branch> release-wt/<release>` already passes the branch name, so it resolves to the current HEAD at call time. Because auto-merge happens inside `finishTrack` *before* the goroutine returns `TrackPass`, the phase barrier (`wg.Wait()`) in `RunParallel` guarantees the release branch is updated before the next phase (dependent track) starts.

## Key design choices

### New `WorkerOptions` fields (injection points)

```go
DependencyOracle       board.OracleReader        // nil → no dependency check
MergeTrackFn           func(releasePath, branch string) error  // nil → skip auto-merge
DependsOnPollInterval  time.Duration              // 0 → 30 s default
```

Injecting via options (not globals or build tags) keeps tests fast and deterministic. The production `RunParallel` wires a real `board.OracleReaderAdapter` and a `productionMergeTrack` helper. A nil value in either field preserves the pre-S04 behaviour for any caller that hasn't set them yet.

### Auto-merge in `finishTrack` + router's "merge-track" case

`finishTrack` is the single merge point called from:
- `case "none":` (router: all slices terminal)
- `runTrackLegacy` completion

Additionally, `case "merge-track":` in `runTrackRouter` is updated: when `MergeTrackFn != nil`, call `finishTrack` directly instead of pausing. When nil, the existing human-gated pause is preserved (backward compatible).

Production merge:
```go
git merge --no-ff <track-branch> --no-edit
```
run in `opts.ReleaseWorktreePath`. Error → `TrackFail`.

### Oracle for dependency check

`board.OracleReader.ReadBoard` returns `*BoardState` with `[]TrackState{ID, State, ...}`. `waitForDependencies` checks `ts.State == "merged"` for each ID in `opts.TrackInfo.DependsOn`. The oracle is already wired in production (`OracleReaderAdapter`) — we only need to thread it through `WorkerOptions`.

## Files touched

| File | Change |
|---|---|
| `internal/scheduler/worker.go` | Add `DependencyOracle`, `MergeTrackFn`, `DependsOnPollInterval` to `WorkerOptions`; add `waitForDependencies`; update `finishTrack` to auto-merge; update `case "merge-track":` to auto-merge when `MergeTrackFn != nil` |
| `internal/run/parallel.go` | Wire `MergeTrackFn` (production `productionMergeTrack`) and `DependencyOracle` (oracle already constructed) into `WorkerOptions` for each goroutine |

No new packages; no new imports beyond `"time"` in `worker.go`.

## AC traceability

| AC | Planned change |
|---|---|
| AC1 — don't start T5 until T6 merged | `waitForDependencies` polls before worktree materialisation |
| AC2 — T5 branches from post-T6 `release-wt` tip | auto-merge in `finishTrack` runs before goroutine returns → `git worktree add ... release-wt/<release>` resolves live tip |
| AC3 — auto-invoke merge-track when last slice verified | `finishTrack` calls `MergeTrackFn`; `case "merge-track":` calls `finishTrack` when wired |
| AC4 — no deadlock on stuck dependency | poll loop respects `ctx.Done()`; logs stall to stderr every interval; cancellation propagates TrackSkipped |
| AC5 — integration test asserts branch point | `TestDependentTrack` in `internal/scheduler/` creates a real git repo; two-track scenario; asserts T_main's first commit is descended from T_dep's merge commit |

## Design risks / pins

- **Pin 1 (Type-2)**: Auto-merge failure (e.g. conflict) returns `TrackFail` rather than pausing for the human. If a conflict occurs in production, the track fails and the human re-runs. Acceptable because invariant-2 (disjoint touchpoints) should make conflicts impossible; the error message includes the full `git merge` output.

- **Pin 2 (Type-2)**: `board.OracleReader` interface is already imported in `worker.go` (via `internal/board`); no new package dependency.

- **Pin 3 (Type-2)**: `case "merge-track":` auto-execution applies only when `MergeTrackFn != nil`. Existing callers that don't set this field see no behaviour change — the pause path is the fallback.
