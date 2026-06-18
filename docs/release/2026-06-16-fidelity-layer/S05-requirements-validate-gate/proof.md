---
title: Slice proof bundle
description: Rule 6 proof bundle for S05-requirements-validate-gate. Generated from live repo state.
---

# Proof Bundle: S05-requirements-validate-gate

## Scope

When a planner reaches validation for a slice, sworn presents AI-drafted **positive and negative scenarios** for each requirement plus a **benefit/alignment hypothesis** (this slice -> release benefit -> objective), and the human ratifies or revises them. `sworn reqvalidate <release>` then **fails closed** on any slice lacking a recorded human-ratified validation — the attestation, the scenarios, and the benefit link must all be present.

## Files changed

```
$ git diff --name-only main
cmd/sworn/main.go
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/prompt/planner.md
internal/state/state.go

New files (not yet tracked):
cmd/sworn/reqvalidate.go
internal/reqvalidate/reqvalidate.go
internal/reqvalidate/reqvalidate_test.go
```

## Test results

### Go (unit — reqvalidate)

```
$ go test ./internal/reqvalidate/... -v
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

### Go (full suite)

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.044s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	(cached)
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/ears	(cached)
ok  	github.com/swornagent/sworn/internal/git	(cached)
ok  	github.com/swornagent/sworn/internal/implement	(cached)
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

When called on a release where all slices have complete human-ratified validation records, `sworn reqvalidate` exits 0.

## Delivered

- [x] WHEN a slice has no recorded human-ratified validation, THE SYSTEM SHALL exit non-zero from `sworn reqvalidate <release>` and name the slice. — **Evidence**: `internal/reqvalidate/reqvalidate.go` `validateSlice()` returns Violation with slice ID when `human_ratified` is false; `TestValidateSlice_MissingRecordFails` covers this.
- [x] WHEN a slice's validation record is present but model-authored with no human ratification, THE SYSTEM SHALL fail (model-only is not a pass). — **Evidence**: `validateSlice()` checks `!v.HumanRatified` as first condition; `TestValidateSlice_ModelOnlyNoRatification` covers this.
- [x] WHEN every slice carries ratified positive + negative scenarios and a benefit/alignment hypothesis linked to a release benefit, THE SYSTEM SHALL exit 0. — **Evidence**: `validateSlice()` returns empty violations when all checks pass; `TestValidateSlice_CompleteRatifiedRecordPasses` and `TestRun_AllSlicesValidated` cover this.
- [x] THE SYSTEM SHALL require, for each requirement, at least one positive AND at least one negative/exception scenario before the validation record is considered complete. — **Evidence**: `validateSlice()` checks `len(v.PositiveScenarios) == 0` and `len(v.NegativeScenarios) == 0`; `TestValidateSlice_PositiveWithoutNegativeFails` and `TestValidateSlice_NegativeWithoutPositiveFails` cover these.
- [x] THE SYSTEM SHALL NOT auto-generate a passing validation; the human ratification field is mandatory and cannot be set by the model. — **Evidence**: `validateSlice()` checks `!v.HumanRatified`; the planner prompt (updated `planner.md`) instructs "Never auto-set `human_ratified`". Only a human can set it to true.
- [x] Validation record schema added to `state.go` (`ValidationRecord` with all required fields). — **Evidence**: `internal/state/state.go` — `ValidationRecord` struct with `HumanRatified`, `PositiveScenarios`, `NegativeScenarios`, `BenefitHypothesis` fields.
- [x] Planner prompt updated to draft scenarios + benefit hypothesis and require human ratification. — **Evidence**: `internal/prompt/planner.md` Phase 4 step 7.
- [x] Rule 8 doc updated with "Validation — human-owned sense-check" section. — **Evidence**: `internal/adopt/baton/rules/08-requirements-fidelity.md`.

## Not delivered

None.

## Divergence from plan

None.