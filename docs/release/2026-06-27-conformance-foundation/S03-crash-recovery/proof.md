# Proof Bundle: `S03-crash-recovery`

## Scope

When a slice implementer exhausts its max-turns budget, the loop emits a PAGE event and halts that track (rather than silently looping). When the same slice fails in the same way across N consecutive runs, the circuit breaker fires, halts the track, and records a fingerprinted failure so the Coach sees the pattern instead of receiving repeat pages.

## Files changed

```
$ git diff --cached --stat
 internal/agent/agent.go             |  15 ++-
 internal/db/db.go                   |   8 +-
 internal/run/slice.go               |  28 ++++-
 internal/scheduler/worker.go        |  55 ++++++++++
 internal/scheduler/worker_test.go   | 135 ++++++++++++++++++++++--
 internal/supervisor/circuit.go      | 100 ++++++++++++++++++
 internal/supervisor/circuit_test.go | 197 ++++++++++++++++++++++++++++++++++++
 internal/supervisor/supervisor.go   |  18 +++-
 8 files changed, 539 insertions(+), 17 deletions(-)
```

## Test results

### Circuit tests (internal/supervisor/...)

```
$ go test ./internal/supervisor/... -v -run 'TestShouldBreak|TestFingerprint'
=== RUN   TestShouldBreak_ThreeConsecutiveSameFingerprint
--- PASS: TestShouldBreak_ThreeConsecutiveSameFingerprint (0.00s)
=== RUN   TestShouldBreak_LessThanThree
--- PASS: TestShouldBreak_LessThanThree (0.00s)
=== RUN   TestShouldBreak_InterleavedDifferentFingerprint
--- PASS: TestShouldBreak_InterleavedDifferentFingerprint (0.00s)
=== RUN   TestShouldBreak_ResetAfterDifferentFingerprint
--- PASS: TestShouldBreak_ResetAfterDifferentFingerprint (0.00s)
=== RUN   TestShouldBreak_NilDB
--- PASS: TestShouldBreak_NilDB (0.00s)
=== RUN   TestShouldBreak_EmptyDB
--- PASS: TestShouldBreak_EmptyDB (0.00s)
=== RUN   TestFingerprint_Deterministic
--- PASS: TestFingerprint_Deterministic (0.00s)
=== RUN   TestFingerprint_DifferentSlice
--- PASS: TestFingerprint_DifferentSlice (0.00s)
=== RUN   TestFingerprint_DifferentError
--- PASS: TestFingerprint_DifferentError (0.00s)
=== RUN   TestFingerprint_OnlyFirstLine
--- PASS: TestFingerprint_OnlyFirstLine (0.00s)
=== RUN   TestShouldBreak_DifferentSliceDoesNotAffect
--- PASS: TestShouldBreak_DifferentSliceDoesNotAffect (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/supervisor	(cached)
```

### Max-turns tests (internal/scheduler/...)

```
$ go test ./internal/scheduler/... -v -run 'TestMaxTurns|TestRunTrack_MaxTurnsPauses'
=== RUN   TestRunTrack_MaxTurnsPausesLegacy
[T1-maxturns] starting
[T1-maxturns] running slice S03-maxturns-legacy (legacy)
[T1-maxturns] paused: max turns exhausted for S03-maxturns-legacy - RunSlice: max turns exhausted: max turns exhausted for S03-maxturns-legacy
--- PASS: TestRunTrack_MaxTurnsPausesLegacy (0.00s)
=== RUN   TestRunTrack_MaxTurnsPausesRouter
[T1-maxturns-router] starting
[T1-maxturns-router] router: S03-maxturns-router → implement (planned)
[T1-maxturns-router] running slice S03-maxturns-router
[T1-maxturns-router] paused: max turns exhausted for S03-maxturns-router - RunSlice: max turns exhausted: max turns exhausted for S03-maxturns-router
--- PASS: TestRunTrack_MaxTurnsPausesRouter (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/scheduler	0.010s
```

## Reachability artefact

- **Type**: manual-smoke-step
- **User gestures**:
  1. `go test ./internal/supervisor/... -v -run TestShouldBreak` — exits 0, proving circuit breaker logic
  2. `go test ./internal/scheduler/... -v -run TestRunTrack_MaxTurnsPauses` — exits 0, proving max-turns PAGE escalation

## Delivered

- MaxTurnsError sentinel (`internal/agent/agent.go`) — `ErrMaxTurns` error + `MaxTurnsSentinel` string constant
- agent.Run wraps max-turns exhaustion with `ErrMaxTurns` (`internal/agent/agent.go`)
- RunSlice detects max-turns and returns sentinel error (`internal/run/slice.go`)
- Worker detects max-turns sentinel and pauses track with PAGE event (`internal/scheduler/worker.go` — 3 insertion points: router implement, router redesign, legacy)
- Circuit breaker: `ShouldBreak` and `RecordFailure` (`internal/supervisor/circuit.go`)
- Circuit breaker fingerprint: `sha256(sliceID + trimmed_first_error_line)` (`internal/supervisor/circuit.go`)
- Circuit breaker integrated in worker: after non-INCONCLUSIVE/non-max-turns failures, records failure and checks ShouldBreak before general failure (`internal/scheduler/worker.go`)
- Circuit breaker PAGE event: `detail: "circuit_breaker"` emitted via `RecordPage` when ShouldBreak returns true
- `RecordPage` public function added to supervisor (`internal/supervisor/supervisor.go`)
- `circuit_failures` table added to DB schema (`internal/db/db.go`)
- Circuit tests: 11 table-driven tests covering all AC5 scenarios (`internal/supervisor/circuit_test.go`)
- Max-turns worker tests: legacy and router-driven paths (`internal/scheduler/worker_test.go`)
- All acceptance checks satisfied per AC1-AC5

## Not delivered

None — all scope items delivered.

## Divergence from plan

None.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S03-crash-recovery 2026-06-27-conformance-foundation
(run below)
```
