---
title: 'S05-requirements-validate-gate'
description: 'Human-owned requirements validation: positive/negative scenario sense-check plus a benefit/alignment hypothesis per slice. AI drafts, the human ratifies; never auto-passed.'
---

# Slice: `S05-requirements-validate-gate`

> Requirements **validation** = "are we building the *right* requirements?" — does the spec
> make sense and serve the need. The cheapest defect-catch point. **Human-owned**: the model
> drafts scenarios + a benefit hypothesis; the human ratifies. Per current research, spec
> validation has no oracle but the user, so this gate is **never** LLM self-certified.

## User outcome

When a planner reaches validation for a slice, sworn presents AI-drafted **positive and
negative scenarios** for each requirement plus a **benefit/alignment hypothesis** (this slice
-> release benefit -> objective), and the human ratifies or revises them. `sworn reqvalidate
<release>` then **fails closed** on any slice lacking a recorded human-ratified validation —
the attestation, the scenarios, and the benefit link must all be present.

## Entry point

- **Protocol (primary):** `internal/prompt/planner.md` drives the interactive scenario walk +
  benefit drafting during `/plan-release` (AI drafts, human ratifies).
- **Native (enforcement):** `sworn reqvalidate <release>` checks that each slice carries a
  human-ratified validation record; the record lives in `status.json`
  (`internal/state/state.go`).

## In scope

- **Scenario sense-check** (`internal/reqvalidate/`): for each requirement, positive scenarios
  (it works as intended) AND negative/alternate/exception scenarios (what should *not* happen,
  edge + failure flows). Drafted by the model, ratified by the human.
- **Benefit / alignment hypothesis**: each slice records its benefit and its vertical link
  (slice -> release benefit -> objective), reusing S01's vertical-trace field; validation
  confirms the ACs are necessary + sufficient for that benefit.
- **Human-ratified attestation**: a validation record in `status.json` (who ratified, when, the
  scenarios + benefit), required and checked fail-closed. Absent or model-only = fail.

## Out of scope

- **Quality-characteristic verification** (singular/unambiguous/…) — S04 (well-formedness).
- **The vertical-trace field structure itself** — S01 (S05 populates + validates it, does not
  define it).
- **Design fit** (is this the right solution) — S07.
- **Gating the `planned -> in_progress` transition on the combined verdict** — S06.

## Planned touchpoints

- `internal/reqvalidate/reqvalidate.go`, `internal/reqvalidate/reqvalidate_test.go` (new)
- `cmd/sworn/reqvalidate.go` (new command)
- `cmd/sworn/main.go` (additive `case "reqvalidate"`)
- `internal/state/state.go` (validation-attestation record on the status schema)
- `internal/prompt/planner.md` (interactive scenario walk + benefit drafting)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (validation section)

## Acceptance checks

- [ ] WHEN a slice has no recorded human-ratified validation, THE SYSTEM SHALL exit non-zero
      from `sworn reqvalidate <release>` and name the slice.
- [ ] WHEN a slice's validation record is present but model-authored with no human
      ratification, THE SYSTEM SHALL fail (model-only is not a pass).
- [ ] WHEN every slice carries ratified positive + negative scenarios and a benefit/alignment
      hypothesis linked to a release benefit, THE SYSTEM SHALL exit 0.
- [ ] THE SYSTEM SHALL require, for each requirement, at least one positive AND at least one
      negative/exception scenario before the validation record is considered complete.
- [ ] THE SYSTEM SHALL NOT auto-generate a passing validation; the human ratification field is
      mandatory and cannot be set by the model.

## Required tests

- **Unit**: `internal/reqvalidate/reqvalidate_test.go` — missing record fails; model-only
  (no ratification) fails; positive-without-negative fails; complete ratified record passes.
- **Integration**: `sworn reqvalidate` exercised on a fixture release (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn reqvalidate <fixture>` with one slice
  missing its negative scenario; observe the named failure; add + ratify it; observe pass." Plus
  an explicit note that the *interactive* scenario walk is exercised via the planner session.
- **E2E gate type**: `local`.

## Risks

- **Human ratification becoming a rubber stamp** — mitigate by requiring the negative scenarios
  explicitly (the part most often skipped) and recording *what* was ratified, not just a flag.
- **Drift into S07 (design)** — the boundary: surfacing that a standard approach *exists*
  informs the requirement (feasibility/scope); *choosing* it is design (S07). Keep S05 to
  "does the requirement make sense + serve the benefit".

## Deferrals allowed?

No.
