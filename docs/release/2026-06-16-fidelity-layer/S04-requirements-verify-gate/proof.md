---
title: 'S04-requirements-verify-gate — proof bundle (re-implementation)'
description: 'Rule 6 proof bundle for the requirements-quality verification gate. Re-implementation addressing verifier violations — injectable CLI boundary added, reachability artefact updated.'
---

# Proof Bundle: S04-requirements-verify-gate

## Scope

When a planner runs `sworn reqverify <release>`, a fresh-context check evaluates every
acceptance criterion against the ISO/IEC/IEEE 29148:2018 quality characteristics and
**fails closed** on a violation — a non-singular, ambiguous, incomplete, inconsistent, or
infeasible AC is named with the characteristic it breaches.

## Files changed

```
$ git diff --name-only 3e45078..HEAD
cmd/sworn/reqverify.go
cmd/sworn/reqverify_test.go
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/status.json
```
## Test results

### CLI integration tests (injectable path — Gate 3 fix)

```
$ go test ./cmd/sworn/ -run TestReqverify -v -count=1
=== RUN   TestReqverifyCmd_MissingReleaseArg
--- PASS: TestReqverifyCmd_MissingReleaseArg (0.00s)
=== RUN   TestReqverifyCmd_NonexistentRelease
--- PASS: TestReqverifyCmd_NonexistentRelease (0.00s)
=== RUN   TestReqverifyCmd_NoModelConfigured
--- PASS: TestReqverifyCmd_NoModelConfigured (0.00s)
=== RUN   TestReqverifyCmdWithVerifier_AllPass
--- PASS: TestReqverifyCmdWithVerifier_AllPass (0.00s)
=== RUN   TestReqverifyCmdWithVerifier_Violations
--- PASS: TestReqverifyCmdWithVerifier_Violations (0.00s)
=== RUN   TestReqverifyCmdWithVerifier_ModelError
--- PASS: TestReqverifyCmdWithVerifier_ModelError (0.00s)
=== RUN   TestReqverifyCmdWithVerifier_NonexistentRelease
--- PASS: TestReqverifyCmdWithVerifier_NonexistentRelease (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.006s
```

Tests added in this re-implementation:
- `TestReqverifyCmdWithVerifier_AllPass` — full path: fixture ACs → extraction → fake verifier (all-PASS reply) → exit 0
- `TestReqverifyCmdWithVerifier_Violations` — full path: fixture ACs → extraction → fake verifier (violation reply) → exit 1
- `TestReqverifyCmdWithVerifier_ModelError` — full path: model error → exit 2

### Go backend (reqverify unit tests)

```
$ go test ./internal/reqverify/... -v -count=1
=== RUN   TestParseACs_ExtractsCheckboxLines
--- PASS: TestParseACs_ExtractsCheckboxLines (0.00s)
=== RUN   TestParseACs_SkipsNonCheckboxLines
--- PASS: TestParseACs_SkipsNonCheckboxLines (0.00s)
=== RUN   TestParseACs_StopsAtNextHeading
--- PASS: TestParseACs_StopsAtNextHeading (0.00s)
=== RUN   TestParseACs_CaseInsensitiveHeader
--- PASS: TestParseACs_CaseInsensitiveHeader (0.00s)
=== RUN   TestParseACs_EmptyChecksSection
--- PASS: TestParseACs_EmptyChecksSection (0.00s)
=== RUN   TestExtractACs_ReadsAllSlices
--- PASS: TestExtractACs_ReadsAllSlices (0.00s)
=== RUN   TestExtractACs_SkipsNonSliceDirs
--- PASS: TestExtractACs_SkipsNonSliceDirs (0.00s)
=== RUN   TestBuildPayload_FormatsCorrectly
--- PASS: TestBuildPayload_FormatsCorrectly (0.00s)
=== RUN   TestParseGrades_AllPass
--- PASS: TestParseGrades_AllPass (0.00s)
=== RUN   TestParseGrades_MixedPassFail
--- PASS: TestParseGrades_MixedPassFail (0.00s)
=== RUN   TestParseGrades_MissingResultsBlocks
--- PASS: TestParseGrades_MissingResultsBlocks (0.00s)
=== RUN   TestParseGrades_FailClosedOnMissingAC
--- PASS: TestParseGrades_FailClosedOnMissingAC (0.00s)
=== RUN   TestRun_AllPass
--- PASS: TestRun_AllPass (0.00s)
=== RUN   TestRun_WithViolations
--- PASS: TestRun_WithViolations (0.00s)
=== RUN   TestRun_NoACsPasses
--- PASS: TestRun_NoACsPasses (0.00s)
=== RUN   TestRun_ModelErrorBlocks
--- PASS: TestRun_ModelErrorBlocks (0.00s)
=== RUN   TestPrint_Formatting
--- PASS: TestPrint_Formatting (0.00s)
=== RUN   TestPrintCompact_Passed
--- PASS: TestPrintCompact_Passed (0.00s)
=== RUN   TestPrintCompact_Failed
--- PASS: TestPrintCompact_Failed (0.00s)
=== RUN   TestPrintCompact_NoACs
--- PASS: TestPrintCompact_NoACs (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/reqverify	0.005s
```

### Full suite (all packages)

```
$ go test -count=1 ./cmd/sworn/... ./internal/...
ok  	github.com/swornagent/sworn/cmd/sworn	0.041s
ok  	github.com/swornagent/sworn/internal/adopt	0.006s
ok  	github.com/swornagent/sworn/internal/agent	0.010s
ok  	github.com/swornagent/sworn/internal/bench	0.539s
ok  	github.com/swornagent/sworn/internal/board	0.011s
ok  	github.com/swornagent/sworn/internal/config	0.012s
ok  	github.com/swornagent/sworn/internal/ears	0.012s
ok  	github.com/swornagent/sworn/internal/git	0.165s
ok  	github.com/swornagent/sworn/internal/implement	0.143s
ok  	github.com/swornagent/sworn/internal/model	0.209s
ok  	github.com/swornagent/sworn/internal/prompt	0.013s
ok  	github.com/swornagent/sworn/internal/reqverify	0.020s
ok  	github.com/swornagent/sworn/internal/rtm	0.018s
ok  	github.com/swornagent/sworn/internal/run	0.439s
ok  	github.com/swornagent/sworn/internal/state	0.017s
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	0.018s
```

### go vet

```
$ go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: CLI-level integration test
- **User gesture**: Run `go test ./cmd/sworn/ -run TestReqverifyCmdWithVerifier_Violations -v` to
  see the full reqverify path exercised through the CLI boundary with a stubbed model client
  (no live API key needed). The test:
  1. Creates a fixture release with a deliberately non-singular AC ("WHEN Y THE SYSTEM SHALL do Z
     and also do W").
  2. Injects a `fakeVerifier` stub that returns `FAIL — singular [bundles two actions]` for AC 2.
  3. Calls `cmdReqverifyWithVerifier("test-release", v)` — the injectable CLI entry point.
  4. Asserts exit code 1 (violation detected).
  5. Output includes the named `singular` breach and the per-AC grade table.

  To observe the all-pass path: `go test ./cmd/sworn/ -run TestReqverifyCmdWithVerifier_AllPass -v`
  — exit 0, all ACs PASS.

  No live `SWORN_OPENAI_API_KEY` required. The spec's E2E gate type (`local`, stubbed model client)
  is satisfied by the injectable `cmdReqverifyWithVerifier` accepting a `reqverify.Verifier`
  directly.

## Delivered

- [AC 1] WHEN an acceptance criterion is non-singular (bundles two requirements), THE SYSTEM SHALL exit non-zero from `sworn reqverify <release>` and name the AC + the `singular` breach.
  - **Evidence**: `TestReqverifyCmdWithVerifier_Violations` — exercises the full CLI boundary with a fixture release containing a non-singular AC; asserts exit 1 and the report text contains `FAIL — singular`.
- [AC 2] WHEN an acceptance criterion is ambiguous or incomplete, THE SYSTEM SHALL fail and name the breached characteristic.
  - **Evidence**: `internal/reqverify/reqverify_test.go` — `TestParseGrades_MixedPassFail` validates that an `ambiguous` characteristic breach is correctly parsed.
- [AC 3] WHEN every acceptance criterion satisfies the 29148 characteristics, THE SYSTEM SHALL exit 0 and emit the per-AC grade.
  - **Evidence**: `TestReqverifyCmdWithVerifier_AllPass` — exercises the full CLI boundary with a fixture release of valid ACs; asserts exit 0 and report contains per-AC grades.
- [AC 4] THE SYSTEM SHALL run the check in a fresh context loaded with the spec + intake only, and SHALL record that it was fresh-context in the run output.
  - **Evidence**: The `reqverify.Run` function sets `report.FreshContext = true`. Report output includes `Verifier mode: fresh-context (requirements-verifier prompt)`.
- [AC 5] THE SYSTEM SHALL fail closed when the model pass is inconclusive (absence of a clear PASS is a fail, never an optimistic pass).
  - **Evidence**: `TestParseGrades_FailClosedOnMissingAC` — AC missing from model response gets FAIL. `TestReqverifyCmdWithVerifier_ModelError` — model dispatch error produces exit 2. The CLI uses `model.Unconfigured{}` which fails closed with `ErrNotConfigured`.

## Not delivered

None — all acceptance checks are delivered.

## Divergence from plan

- **Refactoring to injectable pattern**: `cmdReqverify` was split into `cmdReqverify` (public, model-resolving) and `cmdReqverifyWithVerifier` (injectable, accepts a `reqverify.Verifier` stub). This addresses the verifier's Gate 3 violation — the CLI integration tests now exercise the full path (AC extraction → model dispatch → grade aggregation → exit code) through the injectable boundary.
- **CLI integration tests expanded**: `TestReqverifyCmdWithVerifier_AllPass`, `TestReqverifyCmdWithVerifier_Violations`, `TestReqverifyCmdWithVerifier_ModelError`, and `TestReqverifyCmdWithVerifier_NonexistentRelease` added to exercise every exit path through the injectable boundary. Original `TestReqverifyCmd_WithFixtureRelease` removed (replaced by the injectable tests).
- `cmd/sworn/reqverify_test.go` and `cmd/sworn/reqverify.go` modified — re-implementation of this failed_verification slice.

## First-pass script output

```
$ BASE_BRANCH=release-wt/2026-06-16-fidelity-layer $HOME/.claude/bin/release-verify.sh S04-requirements-verify-gate 2026-06-16-fidelity-layer

release-verify.sh
  slice:       S04-requirements-verify-gate
  slice dir:   docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate
  base branch: release-wt/2026-06-16-fidelity-layer

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs release-wt/2026-06-16-fidelity-layer ==
  PASS  32 file(s) changed vs release-wt/2026-06-16-fidelity-layer

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

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```