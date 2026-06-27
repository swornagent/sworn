---
title: 'S01-process-ownership — process registry + supervisor'
description: 'SQLite-backed process registry with reap-on-restart and single-owner identity per slice. Foundation for safe concurrent track execution.'
---

# Slice: `S01-process-ownership`

## User outcome

A developer running `sworn run --task` after a previous crashed session finds stale
worker processes automatically detected and reaped, ownership cleanly reassigned, and no
corrupted slice state — the run proceeds as if starting fresh.

## Entry point

`sworn run --task` at startup — the supervisor reads `.sworn/sworn.db`, checks
registered PIDs for liveness (`kill(pid, 0)`), and reaps any dead entries before the
implement loop begins.

## In scope

- ADR-0003: documents `modernc.org/sqlite` as an exception to ADR-0001's stdlib-only
  rule; rationale: ACID transactions required for safe concurrent track ownership at 8+
  workers; pure-Go driver preserves zero *runtime* OS dependency
- `internal/db/` package: SQLite connection pool, schema creation, idempotent migrations
- Schema — `tracks` table:
  `(id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT,
   PRIMARY KEY (id, release))`
- Schema — `events` table:
  `(id INTEGER PRIMARY KEY, track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`
- `internal/supervisor/` package:
  - PID liveness check: `syscall.Kill(pid, 0) == nil`
  - Reap-on-restart: on startup, scan tracks table for this release; kill(0) each pid;
    delete rows where pid is dead
  - Single-owner enforcement: INSERT OR FAIL on the tracks table primary key; a second
    process attempting to claim the same track gets a constraint error, not silent overlap
  - Clean release on normal exit: UPDATE tracks SET state='done'/state='failed' + pid=0
- DB file location: `.sworn/sworn.db` relative to workspace root (git-ignored)
- Integration hook: `sworn run` startup calls `supervisor.Acquire(release, trackID)`;
  deferred `supervisor.Release(release, trackID)` on exit

## Out of scope

- The concurrent scheduler that uses the registry (S02)
- TUI display of process status (S04)
- Cross-machine coordination — single-machine only in R3
- Graceful shutdown / SIGTERM handling — processes reap on next startup, not on signal
- Credits metering or SwornAgent API calls

## Planned touchpoints

- `docs/adr/0003-sqlite-orchestration-state.md` (new)
- `internal/db/db.go` (new — connection, schema, migrations)
- `internal/db/db_test.go` (new)
- `internal/supervisor/supervisor.go` (new — registry, acquire, release, reap)
- `internal/supervisor/supervisor_test.go` (new)
- `cmd/sworn/run.go` (touch — open DB, pass supervisor to run options)
- `go.mod`, `go.sum` (touch — add `modernc.org/sqlite`)

## Acceptance checks

- [ ] `go build ./...` succeeds with `modernc.org/sqlite` as the only new external dep;
  ADR-0003 is committed at `docs/adr/0003-sqlite-orchestration-state.md`
- [ ] On first run, `.sworn/sworn.db` is created with the correct schema (verified by
  `sqlite3 .sworn/sworn.db .schema` or equivalent in-process check)
- [ ] Subsequent runs do not re-run migrations or error on an already-initialised schema
- [ ] `supervisor.Acquire(release, "T1")` with a live PID succeeds; a second call with a
  different PID and the same key returns an ownership-conflict error
- [ ] On restart after a simulated crash (stale row with a dead PID), `supervisor.Reap()`
  removes the stale row and `supervisor.Acquire()` succeeds for the new process
- [ ] `go test -race ./internal/supervisor/...` and `go test -race ./internal/db/...`
  pass with zero data race detector findings
- [ ] `.sworn/` is added to `.gitignore` (or the workspace root `.gitignore` already
  covers it; verified in the proof)

## Required tests

- **Unit**: `internal/supervisor/supervisor_test.go`
  — `TestReapOnRestart`: populate a stale row with a guaranteed-dead PID (e.g. PID 1
    killed via kill(0) trick using a temp child process); call Reap(); assert row removed
  — `TestSingleOwnerEnforcement`: two goroutines race to Acquire the same track; exactly
    one wins, the other gets a conflict error; no panic
  — `TestPIDLiveness`: verify kill(0) returns nil for os.Getpid() and non-nil for a
    known-dead PID
- **Integration**: `internal/db/db_test.go`
  — `TestSchemaCreationIdempotent`: open, close, re-open; no error, same schema
  — `TestConcurrentWrites`: 8 goroutines insert rows concurrently; all succeed; no
    corruption; final row count matches insertion count
- **Reachability artefact**: smoke step — run `sworn run --task '...'`; confirm
  `.sworn/sworn.db` created; kill the process; re-run; confirm stale row reaped and run
  proceeds. Document exact commands in `proof.md`.

## Risks

- `modernc.org/sqlite` significantly increases binary size (~8MB). Accept; documented
  in ADR-0003 with rationale (ACID requirement for 8+ concurrent workers outweighs size).
- `syscall.Kill` is Unix-only. Windows support deferred; note in ADR-0003.
  For cross-platform: `os.FindProcess(pid).Signal(syscall.Signal(0))` is portable but
  behaves differently on Windows (always succeeds). Defer Windows support to post-R3.
- The `.sworn/` directory must be gitignored before this slice is implemented; if the
  implementer forgets, accidental DB commits are a risk. Acceptance check AC-7 covers this.

## Deferrals allowed?

No. ADR-0003 and the DB schema are the foundation for S02–S08. Any deferral here
blocks all downstream parallelism work.
