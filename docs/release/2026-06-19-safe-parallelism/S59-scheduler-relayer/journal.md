# S59-scheduler-relayer — Journal

## Session 1 — 2026-07-15

### Design decisions

- **Wrap vs replace**: Wrapping — keep `scheduler.BuildPlan` (dependency resolution), worktree isolation, and `supervisor` ownership; replace only the worker's execution heart.
- **Pause set**: `coach_decision`, `replan-release` → pause/surface (not fail). `error`/exhausted → fail-closed.
- **Router interface**: Define `SliceRouter` interface in `internal/scheduler` so tests can inject a fake without importing `internal/router`.
- **RunSlice handles implement+verify**: `run.RunSlice` already does the full implement→verify loop. The worker treats both `implement` and `verify` router decisions as "run the slice" — after `RunSlice` completes, the slice is `verified` and the router advances.
- **Resumability**: Inherited from the router — when the process restarts, the router reads committed state and routes accordingly. Already-verified slices are skipped.
