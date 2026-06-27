---
title: 'S10-no-mock-boundary'
description: 'Fail-closed on environment: an agent that cannot reach real infra STOPS and surfaces the blocker instead of mocking around it. An undeclared mock at a validated boundary is an undeclared Rule 2 deferral and fails.'
---

# Slice: `S10-no-mock-boundary`

> The false-green killer. Root cause of "all slices verified but the app is broken": agents hit
> an environment wall (no DB, not a premium account) and route around it with a mock instead of
> stopping. An undeclared mock at the validated boundary = an undeclared Rule-2 deferral, and it
> slips Rule 1 (the test "reaches" a mocked integration point). T2; depends_on T1.

## User outcome

When an implementer cannot reach real infrastructure at a validated boundary (DB / auth /
entitlement tier), sworn requires it to **stop and surface the blocker** (a blocked-on-env
state) rather than mock around it. `sworn` **fails closed** on an **undeclared mock at a
validated boundary**: the mock must be declared as a Rule-2 deferral (why / tracking /
acknowledgement) or the check fails and names it.

## Entry point

- **Native:** a boundary-mock check in the verification path (`internal/verify`) that flags
  undeclared mocks at the validated boundary; surfaced via the existing verify entry and a
  `blocked-on-env` slice state.
- **Protocol:** `internal/prompt/implementer.md` instructs: on an environment wall, STOP and
  surface (declare a blocker) — never mock around it; any mock at the validated boundary must be
  an explicit declared deferral.

## In scope

- **Boundary-mock detection** (`internal/verify/`): identify mocks/stubs at the validated
  boundary (DB, auth, entitlement) in the slice diff + tests; cross-check against the slice's
  declared deferrals.
- **Declared-mock registry**: a mock at a validated boundary is permitted only if declared as a
  Rule-2 deferral in `status.json` (`open_deferrals`); an undeclared one fails closed.
- **Fail-closed-on-environment**: an implementer that cannot reach real infra records a
  blocked-on-environment state and surfaces it, rather than producing a green-against-mock pass.
- **Implementer guidance**: the stop-don't-mock principle written into `implementer.md`.

## Out of scope

- **The human walkthrough at cutover** (mocks fully off, real journeys) — S13.
- **Standing up real infra / test environments** — project concern, not sworn's; sworn enforces
  the *declaration*, it does not provision infra.
- **Per-release journey scope** — S12.

## Planned touchpoints

- `internal/verify/verify.go`, `internal/verify/verify_test.go` (boundary-mock detection +
  declared-deferral cross-check)
- `internal/prompt/implementer.md` (stop-don't-mock principle)
- `internal/adopt/baton/rules/10-customer-journey-validation.md` (the no-mock-boundary section)

## Acceptance checks

- [ ] WHEN a slice's tests mock at a validated boundary (DB / auth / entitlement) and the mock
      is not declared as a Rule-2 deferral, THE SYSTEM SHALL exit non-zero and name the
      undeclared mock + boundary.
- [ ] WHEN a boundary mock IS declared with why + tracking + acknowledgement, THE SYSTEM SHALL
      allow it and surface it in the run output as a known deferral.
- [ ] WHEN an implementer cannot reach real infra, THE SYSTEM SHALL support recording a
      blocked-on-environment state rather than a passing-against-mock result.
- [ ] THE SYSTEM SHALL treat an undeclared validated-boundary mock as a Rule-2 violation, not a
      pass (absence of declaration fails closed).

## Required tests

- **Unit**: `internal/verify/verify_test.go` — undeclared boundary mock fails; declared mock
  (with the three components) passes-with-note; a non-boundary mock (pure unit) is not flagged.
- **Integration**: run the verification path on a fixture slice whose test mocks the DB without
  declaration; assert the named failure (Rule 1 via the verify entry point).
- **Reachability artefact**: smoke step — "verify a fixture slice that stubs auth at the
  boundary undeclared; observe the named failure; declare it as a deferral; observe
  pass-with-note."
- **E2E gate type**: `local`.

## Risks

- **Boundary detection precision** — distinguishing a legitimate unit stub from a
  validated-boundary mock is heuristic. Mitigate: a configurable boundary list (DB/auth/
  entitlement by default), flag-not-block ambiguous cases into the declared-deferral path so the
  human adjudicates; bias toward surfacing.
- **False sense of safety** — S10 catches *undeclared* mocks; it does not prove the real path
  works. That proof is S13 (the human walkthrough). Keep the claim honest.

## Deferrals allowed?

No (the slice that bans undeclared deferrals cannot itself carry one).
