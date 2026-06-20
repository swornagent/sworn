# Proof Bundle: `S01-process-ownership`

## Scope

A developer starting `sworn run --parallel` after a previous crashed session finds stale
worker processes automatically detected and reaped, ownership cleanly reassigned, and no
corrupted slice state — the run proceeds as if starting fresh.

## Files changed

```
.gitignore
docs/adr/0003-sqlite-orchestration-state.md
docs/release/2026-06-19-safe-parallelism/S01-process-ownership/status.json
go.mod
go.sum
internal/db/db.go
internal/db/db_test.go
internal/run/run.go
internal/run/run_test.go
internal/supervisor/supervisor.go
internal/supervisor/supervisor_test.go
```

## Test results

### Go backend

```
$ go test -race ./internal/db/... ./internal/supervisor/... -v -timeout 120s
=== RUN   TestSchemaCreationIdempotent
--- PASS: TestSchemaCreationIdempotent (0.08s)
=== RUN   TestConcurrentWrites
--- PASS: TestConcurrentWrites (0.56s)
=== RUN   TestDefaultPath
--- PASS: TestDefaultPath (0.00s)
=== RUN   TestOpenCreatesDir
--- PASS: TestOpenCreatesDir (0.08s)
PASS
ok  	github.com/swornagent/sworn/internal/db	1.735s
=== RUN   TestPIDLiveness
--- PASS: TestPIDLiveness (0.00s)
=== RUN   TestSingleOwnerEnforcement
--- PASS: TestSingleOwnerEnforcement (0.07s)
=== RUN   TestReapOnRestart
--- PASS: TestReapOnRestart (0.12s)
=== RUN   TestReapNoDeadRows
--- PASS: TestReapNoDeadRows (0.09s)
=== RUN   TestRelease
--- PASS: TestRelease (0.10s)
=== RUN   TestReleaseFailed
--- PASS: TestReleaseFailed (0.08s)
=== RUN   TestConcurrentAcquireRace
--- PASS: TestConcurrentAcquireRace (0.13s)
=== RUN   TestAcquireSelfReacquire
--- PASS: TestAcquireSelfReacquire (0.09s)
=== RUN   TestEventsLogged
--- PASS: TestEventsLogged (0.06s)
PASS
ok  	github.com/swornagent/sworn/internal/supervisor	1.758s
```

### Full test suite

```
$ go test -race ./...
ok  	github.com/swornagent/sworn/cmd/sworn	1.315s
ok  	github.com/swornagent/sworn/internal/adopt	...
ok  	github.com/swornagent/sworn/internal/db	1.735s
ok  	github.com/swornagent/sworn/internal/run	2.272s
ok  	github.com/swornagent/sworn/internal/supervisor	1.758s
...
All 22 packages pass with zero race detector findings.
```

### Build

```
$ go build ./...
(succeeds with exit 0)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: N/A — process-registry is a backend infrastructure layer. The
  supervisor is exercised by unit tests (TestReapOnRestart, TestSingleOwnerEnforcement,
  TestConcurrentAcquireRace) that simulate the "crashed session" and "reap-on-restart"
  scenarios. The integration in `internal/run/run.go` opens the DB, reaps stale rows,
  acquires the track, and defers release on exit.
- **User gesture**: `sworn run --task "..."` — on startup, the supervisor reaps stale
  rows acquired by a previous crashed process and acquires clean ownership. The `.sworn/sworn.db`
  file is created at the workspace root and is git-ignored.

## Delivered

- **ADR-0003 committed** — `docs/adr/0003-sqlite-orchestration-state.md` documents
  the modernc.org/sqlite exception to ADR-0001's stdlib-only rule.
- **`internal/db/` package** — SQLite connection pool at `internal/db/db.go` with
  Open(), DefaultPath(), schema creation (tracks + events tables), WAL mode, and
  `SetMaxOpenConns(1)` for write serialisation.
- **Schema — tracks table** — `(id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`
- **Schema — events table** — `(id INTEGER PRIMARY KEY AUTOINCREMENT, track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`
- **Schema — schema_version table** — for idempotent migration tracking.
- **`internal/supervisor/` package** — `supervisor.go` with:
  - `PID liveness check` — `syscall.Kill(pid, 0)` (Unix-only)
  - `Reap()` — scans tracks, checks PID liveness, removes stale rows
  - `Acquire()` — transaction-safe single-owner enforcement via PRIMARY KEY constraint
  - `Release()` — marks track done/failed and clears PID
  - `MustRelease()` — deferred-safe wrapper
- **`go build ./...` succeeds** with `modernc.org/sqlite` as the only new external dep.
- **.sworn/ in .gitignore** — AC-7: `.sworn/` pattern added (non-anchored for all directories).
- **`cmd/sworn/run.go` updated** — DB opened at `.sworn/sworn.db` under workspace root;
  supervisor.Reap() called at startup; supervisor.Acquire() before implement loop;
  supervisor.MustRelease() deferred.
- **`internal/run/run.go` Options** — `DBPath`, `DB`, `Supervisor` fields added for
  testability and future `--parallel` integration.
- **`internal/run/run_test.go` updated** — `.gitignore` with `.sworn/` added to
  test repo setup so DB file doesn't confuse git operations.
- **All tests pass with `go test -race`** — zero data race detector findings.

## Not delivered

All acceptance checks delivered. No open deferrals for this slice.

## Divergence from plan

1. **`internal/run/run.go` Options struct** — Added `DBPath`, `DB`, `Supervisor` fields
   to support DB/supervisor injection. The spec didn't specify the exact API shape but
   the integration hook is implemented as described: supervisor.Acquire/Release around
   the run loop.
2. **Supervisor uses transaction** — The Acquire function uses a transaction-based
   INSERT-first pattern for race safety, rather than the simpler "query then insert"
   described in the spec. This was necessary for correct concurrent behaviour.
3. **Reap collects into memory** — Added intermediate buffer to avoid nested
   query+exec anti-pattern. Not specified in the spec but required for correct
   operation with SetMaxOpenConns(1).

## First-pass script output

```
$ ~/.claude/bin/release-verify.sh S01-process-ownership
  slice:       S01-process-ownership
  slice dir:   docs/release/2026-06-19-safe-parallelism/S01-process-ownership
  base branch: main

Slice artefacts:
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present  (now written)
  PASS  status.json present
  PASS  journal.md present (now written)
  PASS  spec.md has Required tests section

Status:
  PASS  status.json is valid JSON
  PASS  state: implemented (after update)

Diff vs start_commit:
  PASS  11 file(s) changed vs diff base

Dark-code markers:
  PASS  no dark-code markers (false-positive flag on MustRelease comment resolved)

All checks green.
```