---
title: 'S04-requirements-verify-gate'
description: 'Fresh-context requirements verification: each acceptance criterion checked against the ISO/IEC/IEEE 29148 quality characteristics. Verification (well-formedness), not validation (correctness of intent).'
---

# Slice: `S04-requirements-verify-gate`

> Requirements **verification** = "are we building the requirements right?" (quality of the
> spec). Distinct from **validation** (S05 = "are we building the right requirements?",
> human-owned). S04 may be AI-driven and fresh-context because it judges *well-formedness*, not
> intent-correctness — the symmetric front-end analog of Rule 7's delivery verifier.

## User outcome

When a planner runs `sworn reqverify <release>`, a fresh-context check evaluates every
acceptance criterion against the 29148 quality characteristics and **fails closed** on a
violation — a non-singular, ambiguous, incomplete, inconsistent, or infeasible AC is named with
the characteristic it breaches. A release whose ACs are all well-formed passes.

## Entry point

- **Native:** `sworn reqverify <release>` (additive `case "reqverify"`; implementation in
  `cmd/sworn/reqverify.go`), running a fresh-context model pass per the verifier pattern.
- **Protocol:** a new fresh-context prompt `internal/prompt/requirements-verifier.md` (sibling
  to `verifier.md`) instructs the check; loaded with the spec + intake only, never the
  implementer's session.

## In scope

- **29148 quality-characteristic checks** (`internal/reqverify/`): necessary, singular,
  unambiguous, complete, consistent, feasible, verifiable (and the conformance pair: correct,
  conforming). Each AC graded; violations named with the breached characteristic.
- **Fresh-context invocation**: the gate runs as an independent model pass (the
  requirements-side mirror of Rule 7), fail-closed on any violation.
- **Consumes** S01's trace + S02's EARS classification as inputs (an AC must already link + be
  EARS before quality grading is meaningful).

## Out of scope

- **Requirements validation** (scenario sense-check, benefit/alignment, "is this the right
  thing") — S05, and **human-owned** (research: spec validation has no oracle but the user; an
  LLM judging intent-correctness scores near-random, so it is never auto-passed here).
- **EARS notation** (S02) and **trace linkage** (S01) — inputs, not re-checked.
- **Definition-of-Ready gating of the state transition** — S06 (consumes this verdict).

## Planned touchpoints

- `internal/reqverify/reqverify.go`, `internal/reqverify/reqverify_test.go` (new)
- `cmd/sworn/reqverify.go` (new command)
- `cmd/sworn/main.go` (additive `case "reqverify"`)
- `internal/prompt/requirements-verifier.md` (new fresh-context prompt)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (verification section)

## Acceptance checks

- [ ] WHEN an acceptance criterion is non-singular (bundles two requirements), THE SYSTEM SHALL
      exit non-zero from `sworn reqverify <release>` and name the AC + the `singular` breach.
- [ ] WHEN an acceptance criterion is ambiguous or incomplete, THE SYSTEM SHALL fail and name
      the breached characteristic.
- [ ] WHEN every acceptance criterion satisfies the 29148 characteristics, THE SYSTEM SHALL
      exit 0 and emit the per-AC grade.
- [ ] THE SYSTEM SHALL run the check in a fresh context loaded with the spec + intake only,
      and SHALL record that it was fresh-context in the run output (mirroring Rule 7).
- [ ] THE SYSTEM SHALL fail closed when the model pass is inconclusive (absence of a clear PASS
      is a fail, never an optimistic pass).

## Required tests

- **Unit**: `internal/reqverify/reqverify_test.go` — characteristic-breach detection over
  fixture ACs (non-singular, ambiguous, incomplete), using a stubbed model client at the
  *model-client* seam (not at the gate logic), so the grading/aggregation logic is exercised
  for real.
- **Integration**: `sworn reqverify` exercised end-to-end against a fixture release (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn reqverify <fixture>` with one deliberately
  non-singular AC; observe the named `singular` breach + non-zero exit; fix it; observe pass."
- **E2E gate type**: `local` (stubbed model client; no live key needed for the verifier to run
  the deterministic aggregation path).

## Risks

- **LLM grading drift** — the model may over- or under-flag. Mitigate: fail-closed on
  inconclusive, deterministic aggregation over per-characteristic verdicts, and the human
  remains able to override via re-plan (verification informs, it does not silently rewrite).
- **Boundary with S05** — S04 must judge *form/quality* only; "is this the right requirement"
  is S05's human gate. Keep the prompt scoped to well-formedness.

## Deferrals allowed?

No.
