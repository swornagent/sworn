---
title: 'S06-definition-of-ready'
description: 'Promote Gate 0 from section-presence to verified+validated+traced: the planned->in_progress transition fails closed unless the slice has passed the RTM, requirements-verify, and requirements-validate gates.'
---

# Slice: `S06-definition-of-ready`

> Closes the front-end loop: the existing "Spec completeness Gate 0" (sections present) is
> promoted to a real Definition of Ready. Consumes S01 (rtm), S04 (reqverify), S05 (reqvalidate)
> and gates the state transition on their combined verdict. T2; depends_on T1.

## User outcome

When an implementer tries to move a slice `planned -> in_progress`, sworn **fails closed**
unless that slice has passed the requirements-fidelity gates — its trace is complete (S01), its
acceptance criteria are well-formed (S04), and its requirements are human-validated (S05). A
slice that is merely structurally complete (sections present) but not verified/validated/traced
can no longer start implementation.

## Entry point

- **Native:** the `planned -> in_progress` transition in `internal/state` (and the implementer
  start path) now invokes the requirements-fidelity checks as the Definition of Ready.
- **Protocol:** `internal/prompt/implementer.md` Gate 0 is rewritten from "sections present" to
  "verified + validated + traced", instructing the implementer to confirm the DoR before code.

## In scope

- **Promote Gate 0** (`internal/implement/`, `internal/prompt/implementer.md`): the readiness
  check requires RTM pass + requirements-verify pass + requirements-validate pass.
- **Gate the transition** (`internal/state/state.go`): `planned -> in_progress` fails closed if
  the Definition of Ready is unmet; the unmet gate(s) are named.
- **Compose the verbs**: the DoR invokes the S01/S04/S05 gate packages (the autonomous-path
  composition recorded in the command-surface decision).

## Out of scope

- **The individual gates themselves** — S01/S04/S05 (S06 consumes their verdicts).
- **Design fit** (S07) — DoR is the requirements-readiness gate; design fit is its own gate.
- **System-level acceptance at cutover** — S13.

## Planned touchpoints

- `internal/implement/implement.go`, `internal/implement/implement_test.go` (Gate 0 promotion)
- `internal/state/state.go` (transition gated on the DoR verdict)
- `internal/prompt/implementer.md` (Gate 0 rewrite)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (Definition-of-Ready section)

## Acceptance checks

- [ ] WHEN a slice has an incomplete trace (S01 fails), THE SYSTEM SHALL block its
      `planned -> in_progress` transition and name the failed RTM check.
- [ ] WHEN a slice's acceptance criteria fail requirements-verification (S04), THE SYSTEM SHALL
      block the transition and name the breach.
- [ ] WHEN a slice has no human-ratified validation (S05), THE SYSTEM SHALL block the transition.
- [ ] WHEN a slice passes RTM + requirements-verify + requirements-validate, THE SYSTEM SHALL
      allow `planned -> in_progress`.
- [ ] THE SYSTEM SHALL fail closed: if any Definition-of-Ready gate cannot be evaluated, the
      transition is blocked, never optimistically allowed.

## Required tests

- **Unit**: `internal/state` / `internal/implement` tests — transition blocked when each gate
  fails (one test per gate); transition allowed when all pass; unevaluable gate blocks.
- **Integration**: drive the start-of-implementation path on a fixture slice that fails one DoR
  gate; assert the transition is refused with the named gate (Rule 1 via the real entry point).
- **Reachability artefact**: smoke step — "attempt `planned -> in_progress` on a fixture slice
  with an orphaned need; observe the blocked transition naming the RTM failure; complete the
  trace; observe the transition succeed."
- **E2E gate type**: `local`.

## Risks

- **Coupling to three gates** — a flaky upstream gate blocks all starts. Mitigate: each gate's
  verdict is read from its own deterministic check; failures name the specific gate so the fix
  is obvious.
- **Bypass temptation** — there must be no silent override of the DoR; an explicit human
  re-plan is the only path to change a spec, never a Gate 0 skip.

## Deferrals allowed?

No.
