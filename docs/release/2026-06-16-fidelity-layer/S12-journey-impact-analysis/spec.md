---
title: 'S12-journey-impact-analysis'
description: 'Per-release journey-impact analysis: identify which critical journeys a release touches, defining the release-specific validation + walkthrough scope.'
---

# Slice: `S12-journey-impact-analysis`

> Ties Rule 10 into the per-release flow. The ratified journeys (S11) are platform-level; this
> slice computes, for a given release, the subset of journeys it touches — that subset is the
> release's E2E-validation + human-walkthrough scope. T2; depends_on T1 (needs S11's journeys).

## User outcome

When a maintainer runs `sworn journeys --impact <release>`, sworn reports which critical
journeys the release touches (derived from the release's slices and the surfaces they change)
and **fails closed** if the journeys artefact is missing. The reported set is the release's
validation scope: the journeys that must be walked and re-tested before cutover.

## Entry point

- **Native:** `sworn journeys --impact <release>` (extends the S11 `journeys` command;
  implementation extends `cmd/sworn/journeys.go`).
- **Protocol:** the planner / implementer records, per release, the touched-journey set in the
  release board so cutover (S13) knows its scope.

## In scope

- **Impact computation** (`internal/journey/`): map a release's slices (their planned/actual
  touchpoints + entry points) to the journeys that cross those surfaces; output the touched set.
- **Validation scope**: the touched set is recorded as the release's journey-validation scope
  (read by S13 cutover + S14 regression).
- **Fail-closed on missing journeys**: if no ratified journeys artefact exists, impact analysis
  cannot run and fails (depends on S11).

## Out of scope

- **Eliciting / ratifying the journeys** — S11 (read-only consumer here).
- **The human walkthrough + attestation** — S13.
- **Codifying journeys as regression tests** — S14.

## Planned touchpoints

- `internal/journey/journey.go`, `internal/journey/impact_test.go` (impact computation +
  release-scope mapping; extends the S11 model)
- `cmd/sworn/journeys.go` (the `--impact` subcommand path; extends S11's command via depends_on)
- `internal/adopt/baton/rules/10-customer-journey-validation.md` (impact-analysis section)

## Acceptance checks

- [ ] WHEN `sworn journeys --impact <release>` runs against a release with a ratified journeys
      artefact, THE SYSTEM SHALL output the set of journeys the release touches.
- [ ] WHEN no ratified journeys artefact exists, THE SYSTEM SHALL exit non-zero and direct the
      user to run elicitation (S11) first.
- [ ] WHEN a release touches no journeys (e.g. internal-only refactor), THE SYSTEM SHALL report
      an empty touched-set explicitly rather than failing.
- [ ] THE SYSTEM SHALL derive the touched-set from the release's slice touchpoints + entry
      points, not from a hand-maintained list.

## Required tests

- **Unit**: `internal/journey/impact_test.go` — a release touching a known surface maps to the
  expected journeys; missing-artefact fails; empty touched-set reported explicitly.
- **Integration**: `sworn journeys --impact <fixture-release>` end-to-end (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn journeys --impact <fixture>`; observe the
  touched-journey set; remove the journeys artefact; re-run; observe the directed failure."
- **E2E gate type**: `local`.

## Risks

- **Imprecise mapping** (slice touchpoints -> journeys) — a journey step references surfaces;
  matching is heuristic. Mitigate: bias toward over-inclusion (a journey is in-scope if any of
  its steps' surfaces are touched), so the walkthrough scope errs safe.
- **Provisional journey schema** (S11) — the step->surface reference is part of the provisional
  schema; impact precision improves as the hand-run refines it. Acknowledged.

## Deferrals allowed?

Provisional only: the step->surface matching precision tracks S11's provisional schema, refined
via `/replan-release` (acknowledged 2026-06-16). No other deferrals.
