---
title: 'S07-design-fit-gate'
description: 'Stakes-calibrated, human-owned design-fit decision: AI surfaces options/trade-offs/prior-art; high-stakes (reversibility x blast-radius) choices require a recorded human decision, low-stakes proceed with a noted default.'
---

# Slice: `S07-design-fit-gate`

> Rule 9, Seam 2. Meeting a requirement is not the same as the right solution for the whole.
> Solution fit is a quality the delivery verifier (Rule 7) cannot see. This gate keeps design
> **human-owned**, AI-augmented, and calibrates how much human judgement each choice demands by
> its stakes. Creates the `09-design-fidelity.md` rule doc that S08/S09 (T3) extend.

## User outcome

When a planner reaches design for a slice, sworn surfaces AI-drafted design options with
trade-offs and prior art, and classifies each design choice by **stakes = reversibility x
blast-radius**. `sworn designfit <release>` then **fails closed** when any
high-stakes (structural / hard-to-reverse) choice lacks a recorded human decision; low-stakes
choices may proceed with a noted default. Architecturally-significant decisions cannot be made
by the model alone.

## Entry point

- **Protocol (primary):** `internal/prompt/planner.md` + `internal/prompt/captain.md` (the
  existing `/review-tldr` design seed) drive option-surfacing and the stakes classification
  during planning/design.
- **Native (enforcement):** `sworn designfit <release>` checks each slice's design-decision
  record; high-stakes choices require a human decision field. Record lives in `status.json`.

## In scope

- **Stakes classification** (`internal/designfit/`): each design choice tagged Type-1
  (hard-to-reverse / high blast-radius -> full human decision gate) or Type-2 (reversible /
  low blast-radius -> AI proceeds with a noted default). Threshold criterion = reversibility x
  blast-radius (architecturally-significant = always Type-1).
- **Option surfacing**: AI drafts >=2 options with trade-offs + prior art for each
  Type-1 choice; the human chooses; the decision + rationale are recorded.
- **Design-decision record** in `status.json`: choice, stakes class, options considered, human
  decision (for Type-1), rationale.
- **Fail-closed enforcement**: a Type-1 choice with no human decision fails.

## Out of scope

- **Design-system declaration** (tokens + component library) — S08 (T3).
- **Design-system conformance audit** (no hardcoded hex, token-scale spacing) — S09 (T3).
- **Requirements validation** (S05) — design fit assumes the requirement is already validated.

## Planned touchpoints

- `internal/designfit/designfit.go`, `internal/designfit/designfit_test.go` (new)
- `cmd/sworn/designfit.go` (new command)
- `cmd/sworn/main.go` (additive `case "designfit"`)
- `internal/state/state.go` (design-decision record on the status schema)
- `internal/prompt/planner.md`, `internal/prompt/captain.md` (option-surfacing + stakes)
- `internal/adopt/baton/rules/09-design-fidelity.md` (new rule doc — created here),
  `internal/adopt/baton/VERSION` (protocol version bump for Rule 9)

## Acceptance checks

- [ ] WHEN a slice has a Type-1 (high-stakes) design choice with no recorded human decision,
      THE SYSTEM SHALL exit non-zero from `sworn designfit <release>` and name the slice + choice.
- [ ] WHEN a design choice is Type-2 (low-stakes / reversible), THE SYSTEM SHALL allow it to
      proceed with a recorded noted default and not require a human decision.
- [ ] WHEN every Type-1 choice carries a human decision with options + rationale, THE SYSTEM
      SHALL exit 0.
- [ ] THE SYSTEM SHALL classify a design choice as Type-1 when it is architecturally
      significant (shapes the whole, hard to reverse), regardless of other factors.
- [ ] THE SYSTEM SHALL NOT record a human decision on the model's behalf for a Type-1 choice.

## Required tests

- **Unit**: `internal/designfit/designfit_test.go` — Type-1 without decision fails; Type-2 with
  noted default passes; architecturally-significant always Type-1; model-only Type-1 fails.
- **Integration**: `sworn designfit` exercised on a fixture release (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn designfit <fixture>` with one Type-1
  choice undecided; observe named failure; record the human decision; observe pass."
- **E2E gate type**: `local`.

## Risks

- **Mis-calibrated stakes** (everything tagged Type-2 to move fast) — mitigate by the
  architecturally-significant override and by recording the classification rationale so a
  re-plan can challenge it.
- **Scope creep into S08/S09** — S07 is the *decision* gate; the *visual/design-system*
  sub-dimension is S08/S09. Keep S07 medium-agnostic (applies to CLI + UI projects alike).

## Deferrals allowed?

No.
