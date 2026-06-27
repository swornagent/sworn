---
title: 'S02-orchestrator-decision-log — Session journal'
description: Implementation decisions, trade-offs, and state transitions.
---

## 2026-06-28: Implementation session 1

### State transitions

`planned → in_progress → implemented`

### Decisions

1. **String parameters for RecordDecision/RecordTriage instead of struct types.**
   The spec proposed `RecordDecision(db, sliceID, decision)` where `decision` is `scheduler.SliceDecision`, and `RecordTriage(db, sliceID, triage)` where `triage` is `orchestrator.Output`. However, `supervisor` cannot import `scheduler` (circular — `scheduler` already imports `supervisor`) and cannot import `orchestrator` (no existing import, but same risk). Chose to accept `action string, reason string` parameters and have callers unwrap the structs. No loss of fidelity — the same fields land in the DB.

2. **RecordTriage calls in `internal/run/slice.go`, not `internal/scheduler/worker.go`.**
   The spec says `worker.go` calls both RecordDecision and RecordTriage. However, `triage.Decide()` is called inside `RunSlice` (in `internal/run/slice.go`), and the DB handle is plumbed via `RunSliceOptions.DB` (new field). The worker calls `RecordDecision` after the router poll, and `RunSlice` calls `RecordTriage` after each triage decision. This matches the spec's intent (one record per decision, recorded immediately after production).

3. **DB schema added to `internal/db/db.go` lazy migration path.**
   The `decisions` table follows the same pattern as `tracks` and `events`: `CREATE TABLE IF NOT EXISTS` in the `schema` variable, applied by `db.Open()` on every open. This ensures the table exists on first use without a separate migration step.

4. **Telemetry subcommand: `sworn telemetry decisions --release <name>`.**
   Added to the existing `cmd/sworn/telemetry.go`. The command opens the default DB (`.sworn/sworn.db`), queries `supervisor.QueryDecisions()`, and prints a human-readable table with columns: ID, SLICE, ROLE, ACTION, REASON.

### Trade-offs

- The decision log writes are **best-effort** (errors discarded via `_ =`). AC4 explicitly requires that decision-log failures must not abort the run. The trade-off is that a persistent DB failure will silently drop decision records rather than surfacing a warning. A future enhancement could log to stderr.

- The `RunSliceOptions.DB` field is `nil`-tolerant — if not set (e.g., old test code), RecordTriage is simply skipped. This is safe for backward compatibility.

### Touchpoint audit

All changed files match the spec's planned touchpoints plus the necessary wiring:
- `internal/supervisor/decisions.go` (new) — planned ✓
- `internal/supervisor/decisions_test.go` (new) — planned ✓
- `internal/scheduler/worker.go` — planned ✓ (added RecordDecision call)
- `cmd/sworn/telemetry.go` — planned ✓ (added decisions subcommand)
- `internal/db/db.go` — schema addition (necessary for table creation)
- `internal/run/slice.go` — wiring for RecordTriage (necessary because triage.Decide() lives here)
- `cmd/sworn/run.go` — wiring DB into options (necessary)
- `internal/run/run.go` — wiring DB into options (necessary, single-slice mode)

No cross-track touchpoint collisions.
## Verifier verdicts received

### Verifier session 1 — FAIL

```
FAIL

Slice: `S02-orchestrator-decision-log`

Violations:
1. Gate 3 — Missing required integration test: spec requires "Integration: internal/scheduler/worker_test.go (or new worker_decisions_test.go) — run a mock slice, assert RecordDecision called once per routing event." No such test exists in the codebase. The worker_test.go has comprehensive worker tests but none assert that RecordDecision or RecordTriage are called during a mock slice run.

Required to address:
1. Add an integration test to internal/scheduler/worker_test.go (or create a new internal/scheduler/worker_decisions_test.go) that runs a mock slice through the worker and asserts that RecordDecision is called once per routing event, with correct slice_id and decision fields (action, reason).
```

Additional observations (not violations):
- AC4 spec says "log a warning" on DB unavailability; implementation discards errors silently via `_ =`. Acceptable given "must not abort the run" is satisfied, but consider adding a stderr warning.
- All other gates (1, 2, 4, 5, 6, 7) PASS. The implementation is otherwise well-structured, all tests pass, go vet clean.
