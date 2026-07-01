---
title: 'S25 — Durable cross-run event store'
description: 'Make the supervisor SQLite event store durable across process restarts; events written during a run persist to disk and are queryable after a new sworn run starts.'
---

# Slice: `S25-event-store-durable`

## User outcome

After a `sworn run` session completes (or crashes), `sworn telemetry events --release <name>` returns the events from that session. A subsequent `sworn run` session for the same release can read prior run events. The supervisor DB survives process exit.

## Entry point

`internal/supervisor/supervisor.go` — the supervisor DB initialisation path determines whether the SQLite file is in-memory (`:memory:`) or on disk.

## In scope

- `internal/supervisor/supervisor.go`: change the supervisor DB from in-memory (`:memory:`) to a file-backed SQLite DB at a well-known path, e.g. `.sworn/supervisor-<release-name>.db` or `$HOME/.sworn/supervisor.db`
- Migration: `supervisor.Open(path)` creates the DB file if absent; creates the schema (tables: `events`, `decisions` from S02) if they don't exist; does not migrate existing data (new DB per release is acceptable for this slice)
- Path convention: `.sworn/supervisor-<release-name>.db` in the project root — one DB file per release (clean isolation; no cross-release pollution)
- `cmd/sworn/telemetry.go` (existing): wire `sworn telemetry events --release <name>` to query from the on-disk DB
- The switch from in-memory to file-backed must not break any existing tests that depend on the in-memory behavior; use `t.TempDir()` in tests to get a temp DB path

## Out of scope

- S02 decisions table (already specced; S25 ensures the DB file it writes to is durable)
- DB pruning / TTL (future concern)
- Cross-project supervisor aggregation

## Planned touchpoints

- `internal/supervisor/supervisor.go` (change `:memory:` to file-backed path + schema creation)
- `cmd/sworn/telemetry.go` (wire events query to disk DB)

## Acceptance checks

- [ ] WHEN `sworn run --release <name>` writes events to the supervisor DB and exits, a `.sworn/supervisor-<name>.db` file exists on disk
- [ ] WHEN a new `sworn run` session starts for the same release, it opens the existing `.sworn/supervisor-<name>.db` file (not a fresh in-memory DB)
- [ ] WHEN `sworn telemetry events --release <name>` is called after a completed run, THE SYSTEM SHALL return at least one event row from that run
- [ ] Existing supervisor tests that use in-memory DB MUST be updated to use `t.TempDir()` (no test should fail due to the in-memory → file change)
- [ ] `supervisor_test.go` verifies: write an event → process exit → new DB open → query returns the event (file persistence test)

## Required tests

- **Unit**: `internal/supervisor/supervisor_test.go` — add persistence test (write → close → reopen → query)
- **Reachability artefact**: `go test ./internal/supervisor/... -v -run TestPersistence` exits 0; `sworn telemetry events --release <name>` returns output after a real run

## Risks

- SQLite file locking: if two `sworn run` processes run against the same release concurrently, they will both try to open the same `.sworn/supervisor-<name>.db` file; use WAL mode (`PRAGMA journal_mode=WAL`) to allow concurrent readers

## Deferrals allowed?

No.
