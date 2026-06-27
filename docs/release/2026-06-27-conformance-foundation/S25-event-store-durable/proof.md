# S25-event-store-durable — Proof Bundle

## Scope

Make the supervisor SQLite event store durable across process restarts. Events written during a `sworn run` session persist to a `.sworn/supervisor-<release>.db` file and are queryable after process exit via `sworn telemetry events --release <name>`.

This is a re-implementation after verifier FAIL (2026-06-28): the prior implementation opened `eventDB` but never wired it into `RunParallel`, so events landed in `sworn.db` rather than `supervisor-<release>.db`. This session fixes that wiring.

## Files changed

```
git diff 3122d5f..HEAD --stat
 cmd/sworn/run.go                                        |  4 ++--
 .../S25-event-store-durable/status.json                 | 24 ++++++++++--------------
 internal/run/parallel.go                                |  9 +++++++--
 internal/scheduler/worker.go                            | 10 +++++++++-
 internal/supervisor/supervisor.go                       | 17 ++++++++++++++---
 5 files changed, 42 insertions(+), 32 deletions(-)
```

## Test results

```
$ go test ./internal/supervisor/... -v -run TestPersistence
=== RUN   TestPersistence
--- PASS: TestPersistence (0.05s)
PASS
ok      github.com/swornagent/sworn/internal/supervisor      0.056s
```

Full supervisor test suite:
```
$ go test ./internal/supervisor/... -v
=== RUN   TestPIDLiveness
--- PASS: TestPIDLiveness (0.00s)
=== RUN   TestSingleOwnerEnforcement
--- PASS: TestSingleOwnerEnforcement (0.05s)
=== RUN   TestReapOnRestart
--- PASS: TestReapOnRestart (0.04s)
=== RUN   TestReapNoDeadRows
--- PASS: TestReapNoDeadRows (0.03s)
=== RUN   TestRelease
--- PASS: TestRelease (0.06s)
=== RUN   TestReleaseFailed
--- PASS: TestReleaseFailed (0.05s)
=== RUN   TestConcurrentAcquireRace
--- PASS: TestConcurrentAcquireRace (0.06s)
=== RUN   TestAcquireSelfReacquire
--- PASS: TestAcquireSelfReacquire (0.04s)
=== RUN   TestEventsLogged
--- PASS: TestEventsLogged (0.04s)
=== RUN   TestPersistence
--- PASS: TestPersistence (0.03s)
PASS
ok      github.com/swornagent/sworn/internal/supervisor      0.409s
```

Affected package tests:
```
$ go test ./internal/run/... ./internal/scheduler/... -v
# internal/run: 31 tests PASS
# internal/scheduler: 24 tests PASS
```

`go vet ./internal/supervisor/... ./internal/run/... ./internal/scheduler/...` — clean.

## Reachability artefact

`go test ./internal/supervisor/... -v -run TestPersistence` exits 0 — validates write → close → reopen → query flow against the release-specific DB.

The key reachability proof is the wiring trace:

1. `cmd/sworn/run.go:114-119` — `supervisor.Open(releaseName, ".")` opens `.sworn/supervisor-<release>.db`
2. `cmd/sworn/run.go:135` — `EventDB: eventDB` passes it to `ParallelOptions`
3. `internal/run/parallel.go:220` — `EventDB: opts.EventDB` passes it to `WorkerOptions`
4. `internal/scheduler/worker.go:136-138` — `sup.SetEventDB(opts.EventDB)` wires it to the supervisor
5. `internal/supervisor/supervisor.go:248-253` — `logEvent` writes to `s.eventDB` when non-nil

All existing tests (including `TestEventsLogged` and `TestPersistence`) pass without modification — backward-compatible when `EventDB` is nil.

## Delivered

- [x] `Supervisor.eventDB` field + `SetEventDB(*sql.DB)` method. File: `internal/supervisor/supervisor.go:40,246-249`
- [x] `logEvent` routes to `s.eventDB` when non-nil, else `s.db`. File: `internal/supervisor/supervisor.go:252-255`
- [x] `ParallelOptions.EventDB` field. File: `internal/run/parallel.go:29-33`
- [x] `WorkerOptions.EventDB` field. File: `internal/scheduler/worker.go:83-86`
- [x] `RunTrack` calls `sup.SetEventDB(opts.EventDB)` before `Acquire`. File: `internal/scheduler/worker.go:136-138`
- [x] `cmd/sworn/run.go` passes `EventDB: eventDB` to `ParallelOptions`. File: `cmd/sworn/run.go:135`
- [x] AC1: `.sworn/supervisor-<release>.db` file created by `supervisor.Open()` (unchanged from prior implementation)
- [x] AC2: `db.Open()` is idempotent — re-opening existing file does not error (unchanged)
- [x] AC3: `sworn telemetry events --release <name>` now queries the same DB that `sworn run --parallel` writes to (the wiring fix)
- [x] AC4: All existing tests continue to use `t.TempDir()` — `newTestSupervisor` unchanged
- [x] AC5: `TestPersistence` validates the full persistence cycle (unchanged)

## Not delivered

None.

## Divergence from plan

- `internal/scheduler/worker.go` was edited beyond the planned touchpoints — `EventDB` field added to `WorkerOptions` and `SetEventDB` call inserted in `RunTrack`. This was necessary to thread the event DB handle from `RunParallel` through to the supervisor.
- `internal/run/parallel.go` was edited beyond the planned touchpoints — `EventDB` field added to `ParallelOptions` and passed to `WorkerOptions`. This was necessary to accept the event DB from the CLI layer.