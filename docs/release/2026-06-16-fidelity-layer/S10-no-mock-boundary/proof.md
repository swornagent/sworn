---
title: Proof bundle for S10-no-mock-boundary (re-implementation)
description: Generated from live repo state. Addresses two verifier violations from previous failed_verification: wiring OpenDeferrals at entry points and surfacing declared mocks in output.
---

# Proof Bundle: `S10-no-mock-boundary`

## Scope

When an implementer cannot reach real infrastructure at a validated boundary (DB / auth / entitlement tier), sworn requires it to stop and surface the blocker (a blocked-on-env state) rather than mock around it. `sworn` fails closed on an undeclared mock at a validated boundary: the mock must be declared as a Rule-2 deferral (why / tracking / acknowledgement) or the check fails and names it.

## Files changed

### Changes in this re-implementation session (diff from `cec70a6e` — round-1 FAIL verdict commit, before re-implementation began)

```
$ git diff --name-only cec70a61667b571acc413ee2afe2a6380f9b986e HEAD
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/journal.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/proof.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/run/run.go
internal/verify/verify.go
internal/verify/verify_test.go
sworn
```

Note: `sworn` binary appears because it was tracked in `bfdede8`. All planned touchpoints (`cmd/sworn/main.go`, `internal/run/run.go`, `internal/verify/verify.go`, `internal/verify/verify_test.go`) and all docs files appear in the verifier's canonical diff.

### Key changes by file

| File | Change |
|------|--------|
| `internal/verify/verify.go` | After model call, prepend declared mock info to result Rationale |
| `internal/verify/verify_test.go` | `TestRun_DeclaredBoundaryMockAllowed` now asserts rationale contains declared mock |
| `cmd/sworn/main.go` | Added `--deferral` repeatable flag to `sworn verify`; passed as `OpenDeferrals` |
| `internal/run/run.go` | Reads `open_deferrals` from status.json before `verify.Run()` call |
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
ok  	github.com/swornagent/sworn/internal/run	0.594s
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
ok  	github.com/swornagent/sworn/internal/bench	0.608s
ok  	github.com/swornagent/sworn/internal/board	0.006s
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
- **AC2: Declared boundary mock allowed AND surfaced in output** — evidence: `TestRun_DeclaredBoundaryMockAllowed` in `internal/verify/verify_test.go` now asserts `Rationale` contains "Declared boundary mock" and the mock type detail; `verify.Run()` in `internal/verify/verify.go` prepends declared mock info to `Rationale` after model call
- **AC2 — Entry point wiring (Violation 1 fix)**: `cmd/sworn/main.go` adds `--deferral` flag to `sworn verify`; `internal/run/run.go` reads `open_deferrals` from `status.json` — declared mocks are surfaced at every production entry point
- **AC3: Blocked-on-environment state support** — evidence: `internal/prompt/implementer.md` hard constraint instructs implementer to stop and record blocked-on-environment state; `CheckBoundaryMocks` in `internal/verify/verify.go` detects boundary mocks and returns FAIL for undeclared ones
- **AC4: Undeclared boundary mock = Rule-2 violation, absence fails closed** — evidence: `CheckBoundaryMocks` returns undeclared mocks as violations; `Run()` short-circuits with `verdict.Fail` before model dispatch; `internal/adopt/baton/rules/10-customer-journey-validation.md` documents the enforcement

## Not delivered

None.

## Divergence from plan

- `internal/bench/runner.go` not changed — benchmark tasks are synthetic and don't have a status.json context; not a production entry point. The original planned touchpoints listed `internal/verify/verify.go` and `internal/verify/verify_test.go`; the re-implementation adds `cmd/sworn/main.go` and `internal/run/run.go` (required by the verifier violations).

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S10-no-mock-boundary 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S10-no-mock-boundary
  slice dir:   docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  FAIL  state is 'in_progress' — slice not yet ready for verifier (expected — will transition to implemented)

== Diff vs main ==
  PASS  28 file(s) changed vs main

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md

checks passed: 17
checks failed: 1
(state: in_progress expected — completing transition to implemented)
```