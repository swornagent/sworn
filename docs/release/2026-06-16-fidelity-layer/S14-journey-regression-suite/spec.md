---
title: 'S14-journey-regression-suite'
description: 'Each validated, human-walked journey is codified into an automated regression test; the suite accretes release over release, shrinking the manual-walkthrough set over time.'
---

# Slice: `S14-journey-regression-suite`

> The maturity path behind "human walks ALL critical journeys (for now)". Each journey that has
> been ratified (S11) and walked (S13) is codified as an automated regression test, so the
> regression set grows and the manual walkthrough scope shrinks toward the new/changed journeys.
> T2; depends_on T1.

## User outcome

When a maintainer runs `sworn journeys --regen <release>` (or as part of cutover), sworn emits
or updates an automated regression test for each validated, human-walked journey, and **fails
closed** if a journey marked for regression has no corresponding committed test. The journey
regression suite is runnable and accretive — last release's walked journeys are this release's
automated coverage.

## Entry point

- **Native:** `sworn journeys --regen <release>` (extends the `journeys` command;
  implementation extends `cmd/sworn/journeys.go`). Emits/updates regression test scaffolds.
- **Protocol:** `internal/adopt/baton/rules/10-customer-journey-validation.md` documents the
  accretion rule (walked -> codified -> regression).

## In scope

- **Journey -> test codification** (`internal/journey/`): from a ratified + walked journey,
  generate or update an automated regression test scaffold (steps -> assertions) for the
  project's E2E harness; mark the journey as having regression coverage.
- **Coverage check**: a journey flagged for regression with no committed test fails closed.
- **Accretion**: the suite is additive across releases; previously-codified journeys remain
  covered.

## Out of scope

- **Running the project's E2E harness itself** — sworn emits/checks the scaffolds; the project
  owns its test runner.
- **The human walkthrough** (S13) — codification is downstream of a walked journey.
- **Eliciting journeys** (S11) / **impact scope** (S12).

## Planned touchpoints

- `internal/journey/journey.go`, `internal/journey/regression_test.go` (codification +
  coverage check)
- `cmd/sworn/journeys.go` (the `--regen` subcommand path)
- `internal/adopt/baton/rules/10-customer-journey-validation.md` (accretion section)

## Acceptance checks

- [ ] WHEN a journey is ratified + walked but flagged for regression with no committed test, THE
      SYSTEM SHALL exit non-zero from `sworn journeys --regen <release>` and name the gap.
- [ ] WHEN `sworn journeys --regen` runs for a walked journey, THE SYSTEM SHALL emit or update a
      regression test scaffold whose steps mirror the journey's steps.
- [ ] WHEN a journey already has regression coverage from a prior release, THE SYSTEM SHALL
      preserve it (accretive, not regenerated-from-scratch in a way that drops coverage).
- [ ] THE SYSTEM SHALL only codify journeys that have a passing human-walkthrough attestation
      (S13) — an un-walked journey is not auto-codified.

## Required tests

- **Unit**: `internal/journey/regression_test.go` — walked journey emits a scaffold; flagged
  journey without a test fails; prior coverage preserved; un-walked journey not codified.
- **Integration**: `sworn journeys --regen <fixture-release>` end-to-end; assert scaffold
  emission + the coverage-gap failure (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn journeys --regen <fixture>` for a walked
  journey with no test; observe the named gap; generate the scaffold; re-run; observe coverage."
- **E2E gate type**: `local`.

## Risks

- **Scaffold vs real test** — sworn emits a scaffold mirroring the steps; turning it into a
  fully-asserting test is project work. Be honest: S14 guarantees a *structured starting test
  per journey + a coverage check*, not a complete oracle. (The complete-oracle gap is a Rule-2
  boundary, surfaced here, owned by the project's E2E work.)
- **Harness diversity** — projects use different E2E harnesses; the scaffold format must be
  configurable. Mitigate: a pluggable emitter; default to a generic step-list the project adapts.

## Deferrals allowed?

The scaffold-not-complete-oracle boundary is an explicit Rule-2 deferral: **Why** — a complete
journey oracle is project-specific E2E work; sworn provides the structured scaffold + coverage
check. **Tracking** — project E2E backlog per consuming project. **Acknowledged** — 2026-06-16.
Provisional journey-schema detail tracks S11 (refined via `/replan-release`).
