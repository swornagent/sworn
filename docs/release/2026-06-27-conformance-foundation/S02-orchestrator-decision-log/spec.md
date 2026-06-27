---
title: 'S02 — Orchestrator decision-log'
description: 'Persist every router Decision and triage Output to the supervisor SQLite so the Coach can query the routing reasoning trail after a run.'
---

# Slice: `S02-orchestrator-decision-log`

## User outcome

After a `sworn run` session the Coach can run `sworn telemetry decisions --release <name>` (or equivalent query) and see each slice's routing decision and triage output (action, reason, timestamp) in chronological order, persisted to the supervisor SQLite.

## Entry point

`sworn run --release <name>` → `internal/scheduler/worker.go` — the worker goroutine records each Decision (from the router call) and each triage.Output (from `triage.Decide()`) immediately after they are produced, before any state mutation.

## In scope

- New `internal/supervisor/decisions.go`: `RecordDecision(db, sliceID, decision)` and `RecordTriage(db, sliceID, triage)` — write to a new `decisions` table in the supervisor SQLite
- `decisions` schema: `(id INTEGER PK, slice_id TEXT, release TEXT, role TEXT, action TEXT, reason TEXT, recorded_at DATETIME)`
- `internal/scheduler/worker.go`: call `RecordDecision` and `RecordTriage` after each router call and after each `triage.Decide()` call
- Query surface: `sworn telemetry decisions --release <name>` outputs the log in chronological order (JSON or human-readable table; human-readable is sufficient for this slice)
- The decisions table is created by the existing supervisor DB setup path if absent (lazy schema migration)

## Out of scope

- The durable cross-run event store (S25) — this slice adds a new table; S25 ensures the main events table is durable; the decisions table follows the same durability model but that is S25's concern
- Streaming the log to the TUI in real time (proof-visibility theme)
- Retaining decisions for > 30 runs (pruning policy is a future concern)

## Planned touchpoints

- `internal/supervisor/decisions.go` (new)
- `internal/supervisor/decisions_test.go` (new)
- `internal/scheduler/worker.go` (add RecordDecision + RecordTriage calls)
- `cmd/sworn/telemetry.go` (add `decisions` subcommand, or extend existing `sworn telemetry`)

## Acceptance checks

- [ ] WHEN a worker goroutine calls the router and receives a `SliceDecision`, THE SYSTEM SHALL call `RecordDecision(db, sliceID, decision)` before advancing state
- [ ] WHEN a worker goroutine calls `triage.Decide()` and receives an `Output`, THE SYSTEM SHALL call `RecordTriage(db, sliceID, output)` before acting on the output
- [ ] WHEN `sworn telemetry decisions --release <name>` is run after a `sworn run` session, THE SYSTEM SHALL output at least one row per recorded routing event for the named release, including slice_id, action, and reason columns
- [ ] IF the supervisor DB is unavailable at RecordDecision time, THE SYSTEM SHALL log a warning and continue (decision-log failure must not abort the run)
- [ ] `decisions_test.go` verifies: RecordDecision writes a row with correct fields; RecordTriage writes a row with correct fields; query returns rows in insertion order

## Required tests

- **Unit**: `internal/supervisor/decisions_test.go` — insert + query round-trip
- **Integration**: `internal/scheduler/worker_test.go` (or new worker_decisions_test.go) — run a mock slice, assert RecordDecision called once per routing event
- **Reachability artefact**: `go test ./internal/supervisor/... -v -run TestDecisions` exits 0; `sworn telemetry decisions --release <name>` on a real run returns non-empty output (manual smoke step)

## Risks

- The `decisions` table schema is a new migration; if the supervisor DB initialisation path is not idempotent, the table creation may fail on an existing DB — test with a pre-existing DB

## Deferrals allowed?

No. If the supervisor DB package structure makes adding a new table difficult without touching `supervisor.go` (which is T7's file), the implementer must surface this as a collision (not a silent workaround).
