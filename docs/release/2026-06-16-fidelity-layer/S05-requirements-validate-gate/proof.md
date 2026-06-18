---
title: Slice proof bundle
description: Rule 6 proof bundle for S05-requirements-validate-gate. Generated from live repo state.
---

# Proof Bundle: S05-requirements-validate-gate

## Scope

When a planner reaches validation for a slice, sworn presents AI-drafted **positive and negative scenarios** for each requirement plus a **benefit/alignment hypothesis** (this slice -> release benefit -> objective), and the human ratifies or revises them. `sworn reqvalidate <release>` then **fails closed** on any slice lacking a recorded human-ratified validation — the attestation, the scenarios, and the benefit link must all be present.

## Files changed

```
$ git diff --name-only 12ef38a28a05cda5b837a78087f3542476cc00eb..HEAD
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/status.json
```## Test results

### Go (CLI integration — reqvalidate)

```
$ go test -count=1 ./cmd/sworn/ -run TestReqvalidateCmd -v
=== RUN   TestReqvalidateCmd_MissingReleaseArg
sworn reqvalidate: release name is required
usage: sworn reqvalidate <release>
--- PASS: TestReqvalidateCmd_MissingReleaseArg (0.00s)
=== RUN   TestReqvalidateCmd_NonexistentRelease
sworn reqvalidate: release directory not found: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core/cmd/sworn/docs/release/nonexistent-release-xyz
--- PASS: TestReqvalidateCmd_NonexistentRelease (0.00s)
=== RUN   TestReqvalidateCmd_WithFixtureRelease
Requirements validation report
==============================

Total slices: 1 | Validated: 0 | Failed: 1

Violations:
  S01-test-slice: validation record missing human ratification (human_ratified is false or absent)
  S01-test-slice: validation record has no positive scenarios
  S01-test-slice: validation record has no negative/exception scenarios
  S01-test-slice: validation record missing benefit/alignment hypothesis

Per-slice results:
  S01-test-slice: FAIL — validation record missing human ratification (human_ratified is false or absent); validation record has no positive scenarios; validation record has no negative/exception scenarios; validation record missing benefit/alignment hypothesis
reqvalidate: 1 slices — 0 validated, 1 failed — FAILED
--- PASS: TestReqvalidateCmd_WithFixtureRelease (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.007s
```

### Go (unit — reqvalidate)

```
$ go test -count=1 ./internal/reqvalidate/... -v
=== RUN   TestValidateSlice_MissingRecordFails
--- PASS: TestValidateSlice_MissingRecordFails (0.00s)
=== RUN   TestValidateSlice_ModelOnlyNoRatification
--- PASS: TestValidateSlice_ModelOnlyNoRatification (0.00s)
=== RUN   TestValidateSlice_PositiveWithoutNegativeFails
--- PASS: TestValidateSlice_PositiveWithoutNegativeFails (0.00s)
=== RUN   TestValidateSlice_NegativeWithoutPositiveFails
--- PASS: TestValidateSlice_NegativeWithoutPositiveFails (0.00s)
=== RUN   TestValidateSlice_MissingBenefitHypothesisFails
--- PASS: TestValidateSlice_MissingBenefitHypothesisFails (0.00s)
=== RUN   TestValidateSlice_CompleteRatifiedRecordPasses
--- PASS: TestValidateSlice_CompleteRatifiedRecordPasses (0.00s)
=== RUN   TestValidateSlice_MissingStatusJSONFails
--- PASS: TestValidateSlice_MissingStatusJSONFails (0.00s)
=== RUN   TestRun_AllSlicesValidated
--- PASS: TestRun_AllSlicesValidated (0.00s)
=== RUN   TestRun_MixedValidationResults
--- PASS: TestRun_MixedValidationResults (0.00s)
=== RUN   TestRun_NoSlicesPasses
--- PASS: TestRun_NoSlicesPasses (0.00s)
=== RUN   TestRun_SkipsNonSliceDirs
--- PASS: TestRun_SkipsNonSliceDirs (0.00s)
=== RUN   TestPrint_Formatting
--- PASS: TestPrint_Formatting (0.00s)
=== RUN   TestPrintCompact_Passed
--- PASS: TestPrintCompact_Passed (0.00s)
=== RUN   TestPrintCompact_Failed
--- PASS: TestPrintCompact_Failed (0.00s)
=== RUN   TestPrintCompact_NoSlices
--- PASS: TestPrintCompact_NoSlices (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/reqvalidate	0.007s
```
### go vet

```
$ go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **User gesture**: `sworn reqvalidate <release>` reads every slice's `status.json`, checks for a complete human-ratified validation record, fails closed. Run against the 2026-06-16-fidelity-layer release (16 slices, none with validation records yet) returns exit 1 and names every slice.

### Smoke step verification

```
$ sworn reqvalidate 2026-06-16-fidelity-layer
Requirements validation report
==============================

Total slices: 16 | Validated: 0 | Failed: 16

Violations:
  S01-rtm-spine: validation record missing human ratification ...
  S02-ears-ac-format: validation record missing human ratification ...
  ...

Per-slice results:
  S01-rtm-spine: FAIL — validation record missing human ratification ...
  ...

reqvalidate: 16 slices — 0 validated, 16 failed — FAILED
exit: 1
```

When called on a release where all slices have complete human-ratified validation records, `sworn reqvalidate` exits 0:

```
$ sworn reqvalidate test-release
Requirements validation report
==============================

Total slices: 1 | Validated: 1 | Failed: 0

Per-slice results:

1 slice(s) fully validated.
reqvalidate: 1 slices — 1 validated, 0 failed — PASSED
exit: 0
```

The interactive scenario walk (the "draft + ratify" loop) is exercised through the updated `internal/prompt/planner.md` Phase 4 step 7, which instructs the model to draft positive/negative scenarios and a benefit hypothesis and requires the human to ratify before setting `human_ratified: true`. The planner prompt enforces: "Never auto-set `human_ratified` — only a human can ratify."

## Delivered
- [x] WHEN a slice has no recorded human-ratified validation, THE SYSTEM SHALL exit non-zero from `sworn reqvalidate <release>` and name the slice. — **Evidence**: `internal/reqvalidate/reqvalidate.go` `validateSlice()` returns Violation with slice ID when `human_ratified` is false; `TestValidateSlice_MissingRecordFails` covers this.
- [x] WHEN a slice's validation record is present but model-authored with no human ratification, THE SYSTEM SHALL fail (model-only is not a pass). — **Evidence**: `validateSlice()` checks `!v.HumanRatified` as first condition; `TestValidateSlice_ModelOnlyNoRatification` covers this.
- [x] WHEN every slice carries ratified positive + negative scenarios and a benefit/alignment hypothesis linked to a release benefit, THE SYSTEM SHALL exit 0. — **Evidence**: `validateSlice()` returns empty violations when all checks pass; `TestValidateSlice_CompleteRatifiedRecordPasses` and `TestRun_AllSlicesValidated` cover this.
- [x] THE SYSTEM SHALL require, for each requirement, at least one positive AND at least one negative/exception scenario before the validation record is considered complete. — **Evidence**: `validateSlice()` checks `len(v.PositiveScenarios) == 0` and `len(v.NegativeScenarios) == 0`; `TestValidateSlice_PositiveWithoutNegativeFails` and `TestValidateSlice_NegativeWithoutPositiveFails` cover these.
- [x] THE SYSTEM SHALL NOT auto-generate a passing validation; the human ratification field is mandatory and cannot be set by the model. — **Evidence**: `validateSlice()` checks `!v.HumanRatified`; the planner prompt (updated `planner.md`) instructs "Never auto-set `human_ratified`". Only a human can set it to true.
- [x] Validation record schema added to `state.go` (`ValidationRecord` with all required fields). — **Evidence**: `internal/state/state.go` — `ValidationRecord` struct with `HumanRatified`, `PositiveScenarios`, `NegativeScenarios`, `BenefitHypothesis` fields.
- [x] Planner prompt updated to draft scenarios + benefit hypothesis and require human ratification. — **Evidence**: `internal/prompt/planner.md` Phase 4 step 7.
- [x] Rule 8 doc updated with "Validation — human-owned sense-check" section. — **Evidence**: `internal/adopt/baton/rules/08-requirements-fidelity.md`.
- [x] CLI integration point tested (Rule 1) — `cmdReqvalidate()` exercised at the CLI wiring layer, not only the leaf library. — **Evidence**: `cmd/sworn/reqvalidate_test.go` — `TestReqvalidateCmd_MissingReleaseArg`, `_NonexistentRelease`, `_WithFixtureRelease`; all 3 pass.

## Not delivered

None.

## Divergence from plan

- **`cmd/sworn/reqvalidate_test.go` added beyond planned touchpoints**: The spec's "Planned touchpoints" lists `cmd/sworn/reqvalidate.go` but not a corresponding test file. `cmd/sworn/reqvalidate_test.go` was added to satisfy the Rule 1 CLI integration test requirement (the spec's "Required tests" calls for "Integration: `sworn reqvalidate` exercised on a fixture release (Rule 1)"). This mirrors S04's pattern (`cmd/sworn/reqverify_test.go`) and exercises `cmdReqvalidate()` at the CLI integration point, not just the leaf library.

## First-pass script output

```
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

== Diff vs main ==
  PASS  4 file(s) changed vs main
  (first 20)
    docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/journal.md
    docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/proof.md
    docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/status.json
    docs/release/2026-06-16-fidelity-layer/index.md

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
```
