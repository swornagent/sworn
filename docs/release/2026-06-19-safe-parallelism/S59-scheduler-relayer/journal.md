# S59-scheduler-relayer — Journal

## Session 1 — 2026-07-15

### Design decisions

- **Wrap vs replace**: Wrapping — keep `scheduler.BuildPlan` (dependency resolution), worktree isolation, and `supervisor` ownership; replace only the worker's execution heart.
- **Pause set**: `coach_decision`, `replan-release` → pause/surface (not fail). `error`/exhausted → fail-closed.
- **Router interface**: Define `SliceRouter` interface in `internal/scheduler` so tests can inject a fake without importing `internal/router`.
- **RunSlice handles implement+verify**: `run.RunSlice` already does the full implement→verify loop. The worker treats both `implement` and `verify` router decisions as "run the slice" — after `RunSlice` completes, the slice is `verified` and the router advances.
- **Resumability**: Inherited from the router — when the process restarts, the router reads committed state and routes accordingly. Already-verified slices are skipped.

### Completion — 2026-07-15

State transition: `in_progress` → `implemented`.

**Implemented:**
- `SliceRouter` interface in `internal/scheduler` with `SliceDecision` type
- Router-driven `runTrackRouter` poll loop: Route → advance Target → dispatch → repeat
- Legacy `runTrackLegacy` fallback when Router is nil
- `TrackPaused` result type for human-gated pause states
- `stripApprovedAck` helper for redesign dispatch
- `TrackPaused` outcome handling in `RunParallel`
- 9 new router-driven tests + all 8 legacy tests preserved

**Key design decision — Target advance before dispatch:** The router's `Target` field tells the worker which slice to work on. Advancing before dispatch means the worker dispatches the correct slice immediately, rather than dispatching the current slice and advancing after. This is correct: when the router returns `{Type: "implement", Target: "S02-next"}`, it means "implement S02-next now."

**Deferrals carried forward:**
- Release-level circuit breaker (separate slice, audit P1)
- Runtime-drivers dispatch-boundary conformance (post-T17)

**First-pass:** 22/22 PASS.
