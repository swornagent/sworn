---
title: Proof bundle for S10-no-mock-boundary (round 4 — Gate 2 fix)
description: Generated from live repo state. Fixes round-3 verifier Gate 2 violation: start_commit reset to 4d866d6 (original docs: start implementation) so canonical diff includes all planned touchpoints.
---

# Proof Bundle: `S10-no-mock-boundary`

## Scope

When an implementer cannot reach real infrastructure at a validated boundary (DB / auth / entitlement tier), sworn requires it to stop and surface the blocker (a blocked-on-env state) rather than mock around it. `sworn` fails closed on an undeclared mock at a validated boundary: the mock must be declared as a Rule-2 deferral (why / tracking / acknowledgement) or the check fails and names it.

## Files changed

### Canonical diff from `start_commit` (`4d866d6`) — the original `docs: start implementation` commit

```
$ git diff --name-only 4d866d66af5b7fe33b1282eef458ea664dd30974 HEAD
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/journal.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/proof.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/prompt/implementer.md
internal/run/run.go
internal/verify/verify.go
internal/verify/verify_test.go
sworn
```

All four planned touchpoints appear: `internal/verify/verify.go`, `internal/verify/verify_test.go`, `internal/prompt/implementer.md`, `internal/adopt/baton/rules/10-customer-journey-validation.md`.

### Key changes by file

| File | Change |
|------|--------|
| `internal/verify/verify.go` | `CheckBoundaryMocks()` — heuristic boundary-mock detection; `Run()` short-circuits on undeclared mocks; prepends declared-mock info to Rationale |
| `internal/verify/verify_test.go` | 12 tests: undeclared-boundary-mock fails, declared-boundary-mock passes, non-boundary-mock not flagged, auth/entitlement/fake/stub detection, empty diff, multiple mocks, `TestRun_UndeclaredBoundaryMockFailsClosed`, `TestRun_DeclaredBoundaryMockAllowed` (asserts rationale) |
| `internal/prompt/implementer.md` | Stop-don't-mock hard constraint: on environment wall, STOP and surface blocker |
| `internal/adopt/baton/rules/10-customer-journey-validation.md` | No-mock-boundary enforcement section |
| `cmd/sworn/main.go` | `--deferral` repeatable flag on `sworn verify`; passed as `Input.OpenDeferrals` |
| `internal/run/run.go` | Reads `open_deferrals` from `status.json` before `verify.Run()` call |

## Test results

### Go — S10-specific tests (`internal/verify/`)

```
$ go test ./internal/verify/ -v -run "S10|BoundaryMock|Mock|Run_Undeclared|Run_Declared"
=== RUN   TestCheckBoundaryMocks_UndeclaredDbMockFails
--- PASS: TestCheckBoundaryMocks_UndeclaredDbMockFails (0.00s)
=== RUN   TestCheckBoundaryMocks_DeclaredDbMockPasses
--- PASS: TestCheckBoundaryMocks_DeclaredDbMockPasses (0.00s)
=== RUN   TestCheckBoundaryMocks_NonBoundaryMockNotFlagged
--- PASS: TestCheckBoundaryMocks_NonBoundaryMockNotFlagged (0.00s)
=== RUN   TestCheckBoundaryMocks_AuthMockUndeclaredFails
--- PASS: TestCheckBoundaryMocks_AuthMockUndeclaredFails (0.00s)
=== RUN   TestCheckBoundaryMocks_EntitlementMockUndeclaredFails
--- PASS: TestCheckBoundaryMocks_EntitlementMockUndeclaredFails (0.00s)
=== RUN   TestCheckBoundaryMocks_FakeDbDetected
--- PASS: TestCheckBoundaryMocks_FakeDbDetected (0.00s)
=== RUN   TestCheckBoundaryMocks_EmptyDiffReturnsEmpty
--- PASS: TestCheckBoundaryMocks_EmptyDiffReturnsEmpty (0.00s)
=== RUN   TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged
--- PASS: TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged (0.00s)
=== RUN   TestRun_UndeclaredBoundaryMockFailsClosed
--- PASS: TestRun_UndeclaredBoundaryMockFailsClosed (0.00s)
=== RUN   TestRun_DeclaredBoundaryMockAllowed
--- PASS: TestRun_DeclaredBoundaryMockAllowed (0.00s)
=== RUN   TestCheckBoundaryMocks_StubAuthDetected
--- PASS: TestCheckBoundaryMocks_StubAuthDetected (0.00s)
=== RUN   TestCheckBoundaryMocks_StubDbDetected
--- PASS: TestCheckBoundaryMocks_StubDbDetected (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

### Go — run package (`internal/run/`)

```
$ go test ./internal/run/
ok  	github.com/swornagent/sworn/internal/run	(cached)
```

### Go vet — all affected packages

```
$ go vet ./internal/verify/ ./internal/run/ ./cmd/sworn/
(no output — clean)
```

### Full internal test suite

```
$ go test ./internal/...
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	0.005s
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/designfit	(cached)
ok  	github.com/swornagent/sworn/internal/ears	(cached)
ok  	github.com/swornagent/sworn/internal/git	(cached)
ok  	github.com/swornagent/sworn/internal/implement	(cached)
ok  	github.com/swornagent/sworn/internal/journey	(cached)
ok  	github.com/swornagent/sworn/internal/model	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/reqvalidate	(cached)
ok  	github.com/swornagent/sworn/internal/reqverify	(cached)
ok  	github.com/swornagent/sworn/internal/rtm	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/proof.md`
- **User gesture**: 
  1. Run `go test ./internal/verify/ -v -run "TestRun_UndeclaredBoundaryMockFailsClosed"` — undeclared DB mock fails closed with `FAIL/boundary_mock`.
  2. Run `go test ./internal/verify/ -v -run "TestRun_DeclaredBoundaryMockAllowed"` — declared DB mock passes AND surfaces "Declared boundary mock" in rationale.
  3. Run `sworn verify --spec spec.md --diff diff.patch --deferral "db mock for integration tests - S10 boundary"` — declared mock passes through at the CLI entry point.

## Delivered

- **AC1: Undeclared boundary mock fails closed** — evidence: `TestCheckBoundaryMocks_UndeclaredDbMockFails`, `TestRun_UndeclaredBoundaryMockFailsClosed` in `internal/verify/verify_test.go`
- **AC2: Declared boundary mock allowed AND surfaced in output** — evidence: `TestRun_DeclaredBoundaryMockAllowed` in `internal/verify/verify_test.go` asserts `Rationale` contains "Declared boundary mock" and the mock type detail; `verify.Run()` in `internal/verify/verify.go` prepends declared mock info to `Rationale` after model call
- **AC2 — Entry point wiring**: `cmd/sworn/main.go` adds `--deferral` flag to `sworn verify`; `internal/run/run.go` reads `open_deferrals` from `status.json` — declared mocks are surfaced at every production entry point
- **AC3: Blocked-on-environment state support** — evidence: `internal/prompt/implementer.md` hard constraint instructs implementer to stop and record blocked-on-environment state; `CheckBoundaryMocks` in `internal/verify/verify.go` detects boundary mocks and returns FAIL for undeclared ones
- **AC4: Undeclared boundary mock = Rule-2 violation, absence fails closed** — evidence: `CheckBoundaryMocks` returns undeclared mocks as violations; `Run()` short-circuits with `verdict.Fail` before model dispatch; `internal/adopt/baton/rules/10-customer-journey-validation.md` documents the enforcement

## Not delivered

None.

## Divergence from plan

- `internal/bench/runner.go` not changed — benchmark tasks are synthetic and don't have a status.json context; not a production entry point.
- `cmd/sworn/main.go` and `internal/run/run.go` added beyond original planned touchpoints — required by round-1 verifier violations to wire `OpenDeferrals` at both production entry points (`sworn verify` CLI and `sworn run` loop).
- `sworn` binary appears in the diff — accidentally tracked in commit `bfdede8`; not a planned production source file. The `sworn` binary was added to `.gitignore` in a subsequent release.
- `internal/prompt/implementer.md` and `internal/adopt/baton/rules/10-customer-journey-validation.md` were delivered in the original feat commit `72dfaee` and remain correctly updated in the working tree. Prior round-2/round-3 diffs from `cec70a6` (a FAIL verdict commit after `72dfaee`) incorrectly excluded them; this round restores `start_commit` to `4d866d6` so they correctly appear in the canonical diff.