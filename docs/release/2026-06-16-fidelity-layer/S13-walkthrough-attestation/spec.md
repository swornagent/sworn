---
title: 'S13-walkthrough-attestation'
description: 'Fail-closed human-walkthrough acceptance at cutover: sworn ship blocks ->shipped unless every touched journey carries a human attestation that it was walked against real infra, mocks off.'
---

# Slice: `S13-walkthrough-attestation`

> The capstone gate. all-slices-`verified` is necessary, not sufficient — integration defects
> live in the seams between slices that no slice's spec owns. The fail-closed philosophy extends
> from slice to **system**: no cutover without a human walking the touched journeys, mocks off,
> against real infra. The human is the acceptance authority. T2; depends_on T1.

## User outcome

When a maintainer runs `sworn ship <release>`, sworn **fails closed** unless every journey in
the release's validation scope (S12) carries a recorded human-walkthrough attestation: the
human walked it against real infrastructure with mocks off, and recorded pass or fail. A release
whose touched journeys are all human-attested pass can move to `shipped`; any un-walked or
failed journey blocks the cutover and is named.

## Entry point

- **Native:** `sworn ship <release>` (new command; additive `case "ship"` in
  `cmd/sworn/main.go`, implementation in `cmd/sworn/ship.go`). Gates the `verified -> shipped`
  transition.
- **Protocol:** `internal/adopt/baton/rules/10-customer-journey-validation.md` documents the
  mandatory human walkthrough at cutover.

## In scope

- **Walkthrough attestation record** (`internal/journey/` + `internal/state/state.go`): per
  touched journey — walked-by (human), timestamp, real-infra asserted, mocks-off asserted,
  pass/fail, notes. Model cannot author the human attestation.
- **Cutover gate** (`sworn ship` / `internal/state`): `verified -> shipped` fails closed unless
  every journey in the S12 validation scope has a passing human attestation.
- **Kill-list output**: failed/un-walked journeys are named (the cutover kill-list).

## Out of scope

- **Computing the touched-journey scope** — S12 (consumed here).
- **Codifying journeys into automated regression** — S14.
- **The read-only status surface** — S15 (renders these attestations; does not gate).

## Planned touchpoints

- `cmd/sworn/ship.go` (new command)
- `cmd/sworn/main.go` (additive `case "ship"`)
- `internal/state/state.go` (the `verified -> shipped` cutover gate + attestation record)
- `internal/journey/journey.go`, `internal/journey/walkthrough_test.go` (attestation model)
- `internal/adopt/baton/rules/10-customer-journey-validation.md` (cutover-walkthrough section)

## Acceptance checks

- [ ] WHEN a journey in the release's validation scope has no human-walkthrough attestation, THE
      SYSTEM SHALL block `sworn ship <release>` (non-zero exit) and name the un-walked journey.
- [ ] WHEN a touched journey's attestation records a failed walkthrough, THE SYSTEM SHALL block
      cutover and name it in the kill-list.
- [ ] WHEN every touched journey has a passing human attestation asserting real-infra +
      mocks-off, THE SYSTEM SHALL allow `verified -> shipped`.
- [ ] THE SYSTEM SHALL NOT permit the model to author a walkthrough attestation; the
      walked-by-human field is mandatory and human-set.
- [ ] THE SYSTEM SHALL require both the real-infra and mocks-off assertions on each attestation;
      an attestation missing either is incomplete and blocks cutover.

## Required tests

- **Unit**: `internal/journey/walkthrough_test.go` + `internal/state` tests — un-walked journey
  blocks; failed walkthrough blocks; model-authored attestation rejected; complete passing
  attestations allow the transition; missing real-infra/mocks-off assertion blocks.
- **Integration**: drive `sworn ship <fixture-release>` with one un-walked journey; assert the
  blocked cutover + kill-list (Rule 1 via the ship entry point).
- **Reachability artefact**: smoke step — "run `sworn ship <fixture>` with one journey
  un-walked; observe the blocked cutover naming it; record a passing human attestation; re-run;
  observe cutover allowed."
- **E2E gate type**: `local`.

## Risks

- **Attestation friction** — walking all journeys by hand is heavy; this is deliberate "for now"
  (S14's regression suite is the maturity path that shrinks the manual set over time). Keep the
  record lightweight so the act, not the bookkeeping, is the cost.
- **Provisional journey schema** (S11) — the attestation references journeys; field detail may
  be refined via re-plan. Acknowledged.

## Deferrals allowed?

Provisional only: attestation fields track S11's provisional journey schema, refined via
`/replan-release` (acknowledged 2026-06-16). No other deferrals.
