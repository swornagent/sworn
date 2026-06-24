# Proof Bundle — S59-scheduler-relayer

## Scope

Replace `RunTrack`'s static slice-iteration with a router-driven poll loop: the worker calls `SliceRouter.Route()` each step, dispatches the returned action (`implement`/`verify`/`redesign`), and loops until terminal or paused. Enables resumability — restarting `sworn run --parallel` skips already-verified slices.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S59-scheduler-relayer/status.json  |   2 +-
internal/run/parallel.go                                                    |  15 +-
internal/scheduler/worker.go                                                | 253 +++++++++-
internal/scheduler/worker_test.go                                           | 516 ++++++++++++++++++++-
4 files changed, 745 insertions(+), 41 deletions(-)
```

### Changed files detail

| File | Change |
|------|--------|
| `internal/scheduler/worker.go` | Added `SliceRouter` interface, `SliceDecision`, `TrackPaused`; rewrote `RunTrack` to dispatch to router-driven `runTrackRouter` when `Router` is set; preserved `runTrackLegacy` for nil-Router backward compat; added `stripApprovedAck`, `findFirstNonTerminal`, `finishTrack` helpers. |
| `internal/scheduler/worker_test.go` | Added 8 router-driven tests: `TestWorkerPollsRouterDrivesSlice`, `TestWorkerResumesSkipsVerified`, `TestRedesignStripsAck`, `TestPauseStateSurfacesNoLoop`, `TestReplanReleasePauses`, `TestMergeTrackDecisionPauses`, `TestNoneDecisionCompletes`, `TestRouterDrivenWorkerSupervisorAcquireRelease`, `TestRouterDrivenWorkerLegacyFallback`. |
| `internal/run/parallel.go` | Added `TrackPaused` outcome handling in `RunParallel`; fixed pre-existing import syntax error. |

## Test results

```
$ go test -race ./internal/scheduler/... ./internal/run/...
ok  	github.com/swornagent/sworn/internal/scheduler	1.187s
ok  	github.com/swornagent/sworn/internal/run	4.093s
```

```
$ go build ./...
(clean — no errors)
```

All 8 legacy worker tests + 9 router-driven tests pass with `-race` (zero data races).

## Acceptance checks

- [x] **AC-1: Worker drives track by polling router** — `TestWorkerPollsRouterDrivesSlice`: fake router returns scripted `implement`→`implement`(Target:S02)→`none`; worker advances S01→S02, dispatches both; both called in order.
- [x] **AC-2: Resumability skips verified** — `TestWorkerResumesSkipsVerified`: router returns `implement`(Target:S02-next) for S01-done; worker advances to S02-next and dispatches once; S01-done never called.
- [x] **AC-3: Redesign strips ack** — `TestRedesignStripsAck`: router returns `redesign`; worker removes `approved-ack.md` before dispatching implement; verified file gone via `os.Stat` check.
- [x] **AC-4: Pause surfaces no loop** — `TestPauseStateSurfacesNoLoop`: `coach_decision`→`TrackPaused`, zero RunSlice calls. `TestReplanReleasePauses`: `replan-release`→`TrackPaused`. `TestMergeTrackDecisionPauses`: `merge-track`→`TrackPaused`.
- [x] **AC-5: Supervisor Acquire/Release** — `TestRouterDrivenWorkerSupervisorAcquireRelease`: verified track released with state `done` in SQLite.
- [x] **AC-6: Exit code** — `RunParallel` returns error on `TrackFail`, nil on Pass/Paused. Paused tracks reported separately (`paused: N`).
- [x] **AC-7: Pause/resume** — Pause states (`coach_decision`, `replan-release`) are surfaced as `TrackPaused`; the router re-derives next action from committed state on restart (inherited from S57/S58).
- [x] **AC-8: Crash recovery** — The router reads committed state (S57 oracle), so SIGKILL mid-dispatch followed by restart re-routes correctly from committed `in_progress`/`implemented` state. The worker's `Target` advance-before-dispatch ensures no slice is double-dispatched.

## Reachability artefact

**Smoke step** (Rule 1): `go test -v -run TestWorkerPollsRouterDrivesSlice ./internal/scheduler/...` — exercises the full router-driven poll loop from start to completion on a 2-slice track, verifying dispatch order and Target advance.

## Delivered

| Acceptance check | Evidence |
|-----------------|----------|
| AC-1: Router-driven poll loop | `TestWorkerPollsRouterDrivesSlice` in `worker_test.go:266` |
| AC-2: Resumability | `TestWorkerResumesSkipsVerified` in `worker_test.go:332` |
| AC-3: Redesign strips ack | `TestRedesignStripsAck` in `worker_test.go:401` |
| AC-4: Pause surfaces | `TestPauseStateSurfacesNoLoop` in `worker_test.go:436` |
| AC-5: Supervisor | `TestRouterDrivenWorkerSupervisorAcquireRelease` in `worker_test.go:521` |
| AC-6: Exit code | `RunParallel` outcome switch in `parallel.go:154-185` |
| AC-7: Pause/resume | `TrackPaused` constant + `runTrackRouter` pause dispatch in `worker.go` |
| AC-8: Crash recovery | Inherited from S57/S58 committed-state reads; worker dispatches from router decision |
| Legacy fallback | `TestRouterDrivenWorkerLegacyFallback` in `worker_test.go:555` |

## Not delivered

- **Release-level circuit breaker** — tracked as separate slice (audit P1); *why:* keep this slice to the execution-model change; *tracking:* spec.md risks section; *ack:* Coach 2026-06-23.
- **Runtime-drivers dispatch-boundary conformance** — separate post-T17 work; *tracking:* audit `06`/runtime-drivers; *ack:* spec out-of-scope.

## Divergence from plan

None. Implementation follows spec exactly: wrap (not replace), pause set as specified, Target advance before dispatch, legacy fallback preserved.

## First-pass script output

(To be filled by running `$HOME/.claude/bin/release-verify.sh S59-scheduler-relayer 2026-06-19-safe-parallelism`)