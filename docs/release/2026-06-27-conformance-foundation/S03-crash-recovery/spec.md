---
title: 'S03 — Crash recovery: max_turns PAGE + cross-run circuit breaker'
description: 'Add error_max_turns→PAGE escalation and a cross-run failure-fingerprint circuit breaker to prevent infinite retry loops and surface persistent failures to the Coach.'
---

# Slice: `S03-crash-recovery`

## User outcome

When a slice implementer exhausts its max-turns budget, the loop emits a PAGE event and halts that track (rather than silently looping). When the same slice fails in the same way across N consecutive runs, the circuit breaker fires, halts the track, and records a fingerprinted failure so the Coach sees the pattern instead of receiving repeat pages.

## Entry point

`sworn run --release <name>` → `internal/scheduler/worker.go` — the per-slice dispatch loop detects max-turns exhaustion and cross-run repeat failures.

## In scope

- `internal/scheduler/worker.go`: detect when a model dispatch returns an `error_max_turns` signal (or equivalent: implement a MaxTurnsError sentinel); emit a PAGE supervisor event with `reason: "max_turns"` and halt the track for that slice
- Cross-run circuit breaker: `internal/supervisor/decisions.go` (or a new `internal/supervisor/circuit.go`): `ShouldBreak(db, sliceID, fingerprint) bool` + `RecordFailure(db, sliceID, fingerprint)` — returns true after 3 consecutive same-fingerprint failures for a slice; caller halts the track if ShouldBreak returns true
- Fingerprint: `sha256(sliceID + trimmed_first_error_line)` — simple, deterministic, does not encode session ID
- PAGE event: same supervisor event table used by other PAGE escalations; `event_type: "page"`, `detail: "max_turns | circuit_breaker"`, `slice_id`, `release`
- Test: `internal/scheduler/worker_test.go` (add max-turns scenario) + `internal/supervisor/circuit_test.go`

## Out of scope

- Configurable circuit breaker threshold (hardcoded 3 for this slice; per-slice config is a future concern)
- Automatic retry after the coach resolves the PAGE (that is resume behaviour, handled by S07)
- The cross-run event store durability (S25 owns that); this slice records circuit-breaker events using whatever durability exists at the time

## Planned touchpoints

- `internal/scheduler/worker.go` (add max-turns detection + PAGE emit + circuit breaker check)
- `internal/supervisor/circuit.go` (new — ShouldBreak, RecordFailure)
- `internal/supervisor/circuit_test.go` (new)

## Acceptance checks

- [ ] WHEN a model dispatch returns a max-turns exhaustion signal (MaxTurnsError sentinel or equivalent), THE SYSTEM SHALL emit a supervisor PAGE event with `detail: "max_turns"` and set track result to `TrackPaused` (or equivalent halt)
- [ ] WHEN `ShouldBreak(db, sliceID, fingerprint)` is called 3 consecutive times for the same sliceID+fingerprint with no intervening non-matching fingerprint, THE SYSTEM SHALL return true
- [ ] WHEN the circuit breaker fires (ShouldBreak returns true), THE SYSTEM SHALL emit a supervisor PAGE event with `detail: "circuit_breaker"` and halt the track
- [ ] IF the supervisor DB is unavailable, THE SYSTEM SHALL default to ShouldBreak returning false (fail open on circuit breaker; fail closed on PAGE emit is a separate concern)
- [ ] `circuit_test.go` verifies: three consecutive same-fingerprint calls → ShouldBreak returns true; interleaved different fingerprint resets the counter; < 3 calls → returns false

## Required tests

- **Unit**: `internal/supervisor/circuit_test.go` — table-driven, covers all counter scenarios
- **Integration**: `internal/scheduler/worker_test.go` add scenario: mock dispatch returns MaxTurnsError → assert PAGE event emitted + track halted
- **Reachability artefact**: `go test ./internal/supervisor/... -v -run TestCircuit` exits 0; `go test ./internal/scheduler/... -v -run TestMaxTurns` exits 0

## Risks

- MaxTurnsError may not currently be a distinct error type — implementer must define the sentinel and update the call site(s) that receive model responses to surface it correctly

## Deferrals allowed?

No — but the cross-run circuit breaker depends on persistence; if supervisor DB is in-memory only at this point (S25 not yet merged), record the circuit breaker events anyway; they will be durable after S25 merges.
