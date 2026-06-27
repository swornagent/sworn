---
title: 'S15-sworn-top-evidence'
description: 'A read-only journey-validation evidence surface in sworn top: the green-board / kill-list of journey status. Evidence, not workflow — sworn surfaces that it has been done; it does not own phase gating.'
---

# Slice: `S15-sworn-top-evidence`

> The evidence surface. Holds the line on the sworn value prop: produce + gate on EVIDENCE, do
> not own the WORKFLOW (that stays the customer's tracker). `sworn top` renders journey-
> validation status read-only — the green-board / kill-list — never a workflow manager.
> T4; depends_on T1 (reads board + journey via their existing APIs).

## User outcome

When a maintainer runs `sworn top`, sworn renders a read-only evidence surface for the active
release: each critical journey in scope with its validation status (un-walked / walked-pass /
walked-fail), assembled into a green-board when all pass and a kill-list when any fail. The
surface only reads and displays; it issues no state transitions and gates nothing.

## Entry point

- **Native:** `sworn top` (additive `case "top"` in `cmd/sworn/main.go`, implementation in
  `cmd/sworn/top.go`). Reads the release board (`internal/board`) and journey attestations
  (`internal/journey`) via their existing public APIs.
- **Protocol:** none new — this is a read surface over artefacts other slices produce.

## In scope

- **Read-only render** (`cmd/sworn/top.go`): for the active release, list journeys in the
  validation scope (S12) with their walkthrough status (S13); show a green-board when all pass,
  a kill-list of failed/un-walked journeys otherwise.
- **Evidence-only**: no writes to state, no transitions, no gating — strictly a display.

## Out of scope

- **Computing journey scope** (S12) / **recording attestations** (S13) — read here, owned there.
- **Any workflow / phase-gating** — explicitly excluded (Jira/Linear/GH own workflow; sworn
  surfaces evidence).
- **Full TUI for slice/track orchestration** (`sworn top`'s broader scope) — this slice adds
  the *journey-validation evidence pane* only.

## Planned touchpoints

- `cmd/sworn/top.go` (new — the journey-evidence render; reads board + journey via public API)
- `cmd/sworn/main.go` (additive `case "top"`)

## Acceptance checks

- [ ] WHEN `sworn top` runs for a release whose touched journeys all have passing attestations,
      THE SYSTEM SHALL render a green-board listing each journey as walked-pass.
- [ ] WHEN any touched journey is un-walked or failed, THE SYSTEM SHALL render it in a kill-list
      with its status.
- [ ] THE SYSTEM SHALL be strictly read-only: running `sworn top` SHALL issue no state
      transition and modify no artefact.
- [ ] WHEN no journeys artefact / validation scope exists yet, THE SYSTEM SHALL render an empty
      evidence pane with a hint to run elicitation (S11), not an error.

## Required tests

- **Unit**: `cmd/sworn/top_test.go` — green-board render when all pass; kill-list when one
  fails/un-walked; empty-state hint; an assertion that no write/transition API is invoked
  (read-only guarantee).
- **Integration**: `sworn top` against a fixture release with mixed journey statuses (Rule 1 via
  the command entry point).
- **Reachability artefact**: smoke step — "run `sworn top` on a fixture with one failed journey;
  observe the kill-list; mark it walked-pass in the fixture; re-run; observe the green-board."
- **E2E gate type**: `local`.

## Risks

- **Functional sequencing** — `sworn top` renders S13's attestations, so it is most useful after
  S13 lands; it is only *touchpoint*-gated on T1 (board + journey exist), so it can be built
  against empty/fixture attestation data and renders the empty state cleanly until S13 is live.
  (Recorded in cross-track notes.)
- **Scope creep into workflow** — the strongest risk to the value prop. The read-only assertion
  test is the guardrail; any transition/gating here is out of bounds.

## Deferrals allowed?

No.
