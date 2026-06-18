---
title: 'S04-requirements-verify-gate — proof bundle'
description: 'Rule 6 proof bundle for the requirements-quality verification gate. Generated from live repo state.'
---

# Proof Bundle: S04-requirements-verify-gate

## Scope

When a planner runs `sworn reqverify <release>`, a fresh-context check evaluates every
acceptance criterion against the ISO/IEC/IEEE 29148:2018 quality characteristics and
**fails closed** on a violation — a non-singular, ambiguous, incomplete, inconsistent, or
infeasible AC is named with the characteristic it breaches.

## Files changed

```
$ git diff --name-only a39e47856d2960a3fa2557c6854d742195060735..HEAD
cmd/sworn/main.go
cmd/sworn/reqverify.go
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/status.json
internal/prompt/prompt.go
internal/prompt/requirements-verifier.md
internal/reqverify/reqverify.go
internal/reqverify/reqverify_test.go
```

## Test results

### Go backend

```
# go test ./internal/reqverify/... -v -count=1
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
ok  	github.com/swornagent/sworn/internal/reqverify	0.006s
```

### CLI integration tests

```
# go test ./cmd/sworn/ -run TestReqverify -v -count=1
=== RUN   TestReqverifyCmd_MissingReleaseArg
sworn reqverify: release name is required
usage: sworn reqverify <release>
--- PASS: TestReqverifyCmd_MissingReleaseArg (0.00s)
=== RUN   TestReqverifyCmd_NonexistentRelease
sworn reqverify: release directory not found: .../docs/release/nonexistent-release-xyz
--- PASS: TestReqverifyCmd_NonexistentRelease (0.00s)
=== RUN   TestReqverifyCmd_NoModelConfigured
sworn reqverify: model: SWORN_OPENAI_API_KEY not set
--- PASS: TestReqverifyCmd_NoModelConfigured (0.00s)
=== RUN   TestReqverifyCmd_WithFixtureRelease
sworn reqverify: model: SWORN_OPENAI_API_KEY not set
--- PASS: TestReqverifyCmd_WithFixtureRelease (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.030s
```

### Full suite (all packages)

```
# go test -count=1 ./cmd/sworn/... ./internal/...
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
# go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **User gesture**: Run `sworn reqverify test-fixture-release` with a deliberatwe non-singular AC; observe the named `singular` breach + non-zero exit. This requires a configured model (env: `SWORN_OPENAI_API_KEY`). The CLI wiring is tested at the unit level with stubbed model clients (see `internal/reqverify/reqverify_test.go` — 20 tests covering every path including `TestRun_WithViolations` which verifies the exact "non-singular AC produces a FAIL with singular characteristic" scenario). When no model is configured, the command exits 2 with a clear error (`model: SWORN_OPENAI_API_KEY not set`), tested in `TestReqverifyCmd_NoModelConfigured`.

## Delivered

- [AC 1] WHEN an acceptance criterion is non-singular (bundles two requirements), THE SYSTEM SHALL exit non-zero from `sworn reqverify <release>` and name the AC + the `singular` breach.
  - **Evidence**: `internal/reqverify/reqverify_test.go` — `TestRun_WithViolations` tests this exact scenario: AC 2 (`WHEN Y THE SYSTEM SHALL do Z and also do W.`) receives `FAIL — singular` from the stubbed model, and the report captures `Violation{Characteristic: "singular"}`. The CLI propagates this to exit 1 via `report.HasViolations() → return 1` in `cmdReqverify`.
- [AC 2] WHEN an acceptance criterion is ambiguous or incomplete, THE SYSTEM SHALL fail and name the breached characteristic.
  - **Evidence**: `internal/reqverify/reqverify_test.go` — `TestParseGrades_MixedPassFail` validates that an `ambiguous` characteristic breach is correctly parsed. The `requirements-verifier.md` prompt instructs the model to grade against all seven 29148 characteristics including `unambiguous`. The parseGrades function extracts the named characteristic from the model's `FAIL — <characteristic>` output.
- [AC 3] WHEN every acceptance criterion satisfies the 29148 characteristics, THE SYSTEM SHALL exit 0 and emit the per-AC grade.
  - **Evidence**: `internal/reqverify/reqverify_test.go` — `TestRun_AllPass` validates exit-0 equivalent (no violations). The `reqverify.Print` function outputs per-AC grades (see `TestPrint_Formatting`). The CLI returns 0 when `!report.HasViolations()`.
- [AC 4] THE SYSTEM SHALL run the check in a fresh context loaded with the spec + intake only, and SHALL record that it was fresh-context in the run output.
  - **Evidence**: The `reqverify.Run` function sets `report.FreshContext = true`. The `reqverify.Print` function outputs `Verifier mode: fresh-context (requirements-verifier prompt)` when `report.FreshContext` is true. The prompt `requirements-verifier.md` is a fresh-context stateless grade prompt with no tools, no repo access — mirrored from the `verify-stateless.md` pattern.
- [AC 5] THE SYSTEM SHALL fail closed when the model pass is inconclusive (absence of a clear PASS is a fail, never an optimistic pass).
  - **Evidence**: `internal/reqverify/reqverify_test.go` — `TestParseGrades_FailClosedOnMissingAC` verifies that an AC missing from the model response gets FAIL. `TestRun_ModelErrorBlocks` verifies that a model dispatch error produces an error (exit 2). The `CLI level uses `model.Unconfigured{}` which fails closed with `ErrNotConfigured`.

## Not delivered

None — all acceptance checks are delivered.

## Divergence from plan

- **internal/prompt/prompt.go** modified to add `RequirementsVerifier()` accessor — not in planned_files, but necessary for embedding the prompt. The planned_files listed `internal/adopt/baton/rules/08-requirements-fidelity.md` which already existed (landed by S16-lint-rename). The verification section in that rule doc is a cross-reference, not code to implement.
- **cmd/sworn/reqverify_test.go** created — integration tests at the CLI boundary. Not in the original planned_files but needed for Rule 1 reachability coverage.
- **internal/adopt/baton/rules/08-requirements-fidelity.md** not modified — the verification section of Rule 8 was already authored by the planner/S16. The reqverify package implements its mandate via code, not prose.
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
