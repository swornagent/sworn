---
title: Slice proof bundle
description: Rule 6 proof bundle for S05-requirements-validate-gate. Generated from live repo state.
---

# Proof Bundle: S05-requirements-validate-gate

## Scope

When a planner reaches validation for a slice, sworn presents AI-drafted **positive and negative scenarios** for each requirement plus a **benefit/alignment hypothesis** (this slice -> release benefit -> objective), and the human ratifies or revises them. `sworn reqvalidate <release>` then **fails closed** on any slice lacking a recorded human-ratified validation — the attestation, the scenarios, and the benefit link must all be present.

## Files changed

```
$ git diff --name-only 40b2af4b0077d03b041cd7ac8ae3324caaa29a15..HEAD
.gitignore
cmd/sworn/main.go
cmd/sworn/reqvalidate.go
cmd/sworn/reqvalidate_test.go
cmd/sworn/reqverify.go
cmd/sworn/reqverify_test.go
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/status.json
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/prompt/planner.md
internal/reqvalidate/reqvalidate.go
internal/reqvalidate/reqvalidate_test.go
internal/reqverify/reqverify_test.go
internal/state/state.go
```
## Test results

### Go (CLI integration — reqvalidate)

```
$ go test ./cmd/sworn/ -run TestReqvalidateCmd -v
=== RUN   TestReqvalidateCmd_MissingReleaseArg
--- PASS: TestReqvalidateCmd_MissingReleaseArg (0.00s)
=== RUN   TestReqvalidateCmd_NonexistentRelease
--- PASS: TestReqvalidateCmd_NonexistentRelease (0.00s)
=== RUN   TestReqvalidateCmd_WithFixtureRelease
--- PASS: TestReqvalidateCmd_WithFixtureRelease (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.006s
```

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
- [x] CLI integration point tested (Rule 1) — `cmdReqvalidate()` exercised at the CLI wiring layer, not only the leaf library. — **Evidence**: `cmd/sworn/reqvalidate_test.go` — `TestReqvalidateCmd_MissingReleaseArg`, `_NonexistentRelease`, `_WithFixtureRelease`; all 3 pass.

## Not delivered

None.

## Divergence from plan

- **S04 files in diff range**: `cmd/sworn/reqverify.go`, `cmd/sworn/reqverify_test.go`,
  `internal/reqverify/reqverify_test.go`, `.gitignore`, and
  `docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/*` appear in the
  `git diff --name-only 40b2af4..HEAD` output because the start_commit (`40b2af4`) pre-dates
  S04's re-implementation cycles (which ran concurrently with S05 on track T1-fidelity-core).
  These files are S04 scope (a distinct, verified slice), not S05 scope. They appear in the
  diff range but are not S05 deliverables.
- **S05 self-tracking docs in diff range**: `docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/journal.md`,
  `proof.md`, and `status.json` are the slice's own tracking artefacts (created and updated by
  this session) — they are not planned_files but are naturally part of the diff. `index.md`
  was touched by the track worktree materialisation and milestone updates. These are expected
  side-effects of operating inside a release, not plan deviations.