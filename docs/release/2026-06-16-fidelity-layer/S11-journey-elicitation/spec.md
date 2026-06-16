---
title: 'S11-journey-elicitation'
description: 'AI drafts the critical customer journeys from the app; the human ratifies them into a durable, version-controlled platform artefact. Creates the journey model the rest of Rule 10 builds on.'
---

# Slice: `S11-journey-elicitation`

> Rule 10, Seam 3, first slice. The journey is to validation what the slice is to
> implementation. Creates `internal/journey/` + the `10-customer-journey-validation.md` rule
> doc that T2 (S12/S13/S14) extends. **Provisional**: the journey-artefact schema is refined by
> the live journey-validation hand-run via `/replan-release` — see "Provisional" below.

## User outcome

When a maintainer runs `sworn journeys <project>`, sworn presents AI-drafted critical customer
journeys inferred from the app, the human ratifies/adjusts them, and the result is written to a
durable, version-controlled journeys artefact. `sworn journeys --check` **fails closed** if the
artefact is missing or unratified — journeys are a first-class platform artefact, maintained
over time, not a per-release afterthought.

## Entry point

- **Native:** `sworn journeys <project>` (draft + list) and `sworn journeys --check` (validate
  presence + ratification). Additive `case "journeys"`; implementation in
  `cmd/sworn/journeys.go`.
- **Protocol:** `internal/prompt/planner.md` (or a dedicated elicitation prompt) drives the
  AI-draft / human-ratify loop, same pattern as requirements + design fidelity.

## In scope

- **Journey model** (`internal/journey/`): a critical customer journey = an ordered,
  end-to-end path a user type takes across the app to achieve an outcome (crossing many
  slices). Fields (provisional): id, user type, outcome, ordered steps, the entry surface.
- **Elicitation**: the model drafts candidate journeys from the app; the human
  ratifies/edits/adds; ratification is recorded.
- **Durable artefact**: journeys persisted to a version-controlled file (location decided at
  implementation, e.g. `docs/journeys/` or a project-level `journeys.md`), maintained release
  over release.
- **Presence/ratification check**: `sworn journeys --check` fails closed on a missing or
  unratified artefact.

## Out of scope

- **Per-release journey-impact analysis** (which journeys a release touches) — S12 (T2).
- **Human walkthrough + attestation at cutover** — S13 (T2).
- **Journeys -> automated regression tests** — S14 (T2).
- **The read-only journey status surface in `sworn top`** — S15 (T4).

## Provisional (refined via `/replan-release` post hand-run)

- The exact journey-artefact **schema** (step granularity, how a step references the slices /
  surfaces it crosses) is provisional; the live journey-validation hand-run is the source of
  truth and will refine it. This slice establishes the *model + elicitation loop + durable
  artefact + presence check*; field-level detail may be appended (verified work stays immutable).

## Planned touchpoints

- `internal/journey/journey.go`, `internal/journey/journey_test.go` (new — model + elicitation
  + presence/ratification check)
- `cmd/sworn/journeys.go` (new command)
- `cmd/sworn/main.go` (additive `case "journeys"`)
- `internal/prompt/planner.md` (elicitation loop guidance)
- `internal/adopt/baton/rules/10-customer-journey-validation.md` (new rule doc — created here),
  `internal/adopt/baton/VERSION` (protocol version bump for Rule 10)

## Acceptance checks

- [ ] WHEN no journeys artefact exists for a project, THE SYSTEM SHALL exit non-zero from
      `sworn journeys --check` and state that elicitation has not been run.
- [ ] WHEN a journeys artefact exists but is unratified by a human, THE SYSTEM SHALL fail and
      name it as unratified.
- [ ] WHEN `sworn journeys <project>` runs, THE SYSTEM SHALL draft >=1 candidate journey from
      the app and present it for human ratification.
- [ ] WHEN the artefact exists and is human-ratified, THE SYSTEM SHALL exit 0 from
      `sworn journeys --check` and list the journeys.
- [ ] THE SYSTEM SHALL persist ratified journeys to a version-controlled file so they survive
      session boundaries.

## Required tests

- **Unit**: `internal/journey/journey_test.go` — missing artefact fails; unratified fails;
  ratified artefact passes + parses; model-drafted-but-unratified is distinguished from ratified.
- **Integration**: `sworn journeys --check` exercised on a fixture project (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn journeys --check` with no artefact;
  observe failure; run `sworn journeys`, ratify a drafted journey; re-run `--check`; observe
  pass + listed journeys."
- **E2E gate type**: `local` (the draft step's model call stubbed at the model-client seam).

## Risks

- **Schema churn from the hand-run** — mitigated by scoping this slice to the model +
  elicitation loop + presence check, and marking field-level schema provisional (refined via
  re-plan; verified work immutable).
- **Journeys becoming a workflow tool** — out of bounds. This is an *evidence* artefact
  (AI-drafts / human-ratifies), not a phase-gating workflow; keep it to elicitation + the file.

## Deferrals allowed?

Provisional schema fields only, refined via `/replan-release` as the hand-run lands (tracked in
intake "Open questions"; acknowledged 2026-06-16). No other deferrals.
