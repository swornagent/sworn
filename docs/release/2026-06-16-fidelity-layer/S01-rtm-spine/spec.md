---
title: 'S01-rtm-spine'
description: 'The 2-D requirements traceability matrix — horizontal need->AC->test->proof plus vertical objective->release-benefit->slice — threaded through existing artefacts and enforced fail-closed.'
---

# Slice: `S01-rtm-spine`

> Keystone of the release. Establishes the trace structure + enforcement that every other
> Rule 8/9/10 slice hangs off. Linkage + enforcement only — notation (S02), quality (S04),
> validation content (S05) are separate slices.

## User outcome

When a planner runs `sworn lint trace <release>`, sworn reports the release's 2-D requirements
traceability matrix and **fails closed** on any broken trace: an intake need with no
acceptance criterion, an acceptance criterion with no need or no test, or a slice with no
link up to a release benefit are each named and cause a non-zero exit. A fully-traced release
prints the matrix and exits 0.

## Entry point

- **Native:** `sworn lint trace <release>` (new subcommand; additive `case "rtm"` in
  `cmd/sworn/main.go`, implementation in `cmd/sworn/rtm.go`).
- **Protocol:** `internal/prompt/planner.md` instructs the planner to assign stable need ids
  in `intake.md`, reference them from each `spec.md` acceptance check, and record the vertical
  link (slice -> release benefit -> objective) so the trace is constructed *as a by-product of
  planning*, not a separate documentation phase.

## In scope

- **Trace data model** (`internal/rtm/`):
  - *Horizontal:* `need -> acceptance-criterion -> test -> proof`. Needs are enumerated with
    stable ids in `intake.md`; each `spec.md` acceptance check cites the need id(s) it
    satisfies; `required tests` cite the acceptance check; the proof bundle already closes
    `AC -> test -> proof`.
  - *Vertical (golden thread):* `org objective -> release benefit -> slice`. Recorded in
    `index.md` (release-level benefit + optional objective) and per-slice (the slice's link to
    the release benefit).
- **Enforcement** (`sworn lint trace`): read intake/spec/status/index, build the matrix, fail closed
  on an orphaned need, an orphaned AC (no need, or no test), or a slice with no vertical link.
- **Native schema fields** carrying the linkage: trace fields on `status.json`
  (`internal/state/state.go`) and the vertical-link fields on the board
  (`internal/board/index.go`).
- **Lightweight floor:** where no org objective is declared (solo / small team), the vertical
  trace floor is `slice -> release goal`; an org-objective link is opt-in, not required.

## Out of scope

- **EARS acceptance-criteria notation** — S02. S01 enforces that an AC *links*, not how it is
  phrased.
- **29148 quality-characteristic checking** — S04.
- **Scenario sense-check + benefit-hypothesis authoring/validation content** — S05. S01 carries
  the vertical-link *field*; S05 fills the validation semantics behind it.
- **Definition-of-Ready gating of `planned -> in_progress`** — S06 (consumes the RTM result).

## Planned touchpoints

- `internal/rtm/rtm.go`, `internal/rtm/rtm_test.go` (new — trace model + matrix build + checks)
- `internal/state/state.go` (add horizontal trace fields to the status schema)
- `internal/board/index.go` (parse + validate the vertical-trace fields)
- `cmd/sworn/rtm.go` (new command implementation)
- `cmd/sworn/main.go` (additive `case "rtm"` only — see intake cross-track convention)
- `internal/prompt/planner.md` (instruct trace construction during planning)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (new rule doc — the RTM section)
  and `internal/adopt/baton/VERSION` (protocol version bump)

## Acceptance checks

- [ ] WHEN a release has an intake need with no linked acceptance criterion, THE SYSTEM SHALL
      exit non-zero from `sworn lint trace <release>` and name the orphaned need id.
- [ ] WHEN an acceptance criterion cites no need id, or cites a need but has no linked test,
      THE SYSTEM SHALL exit non-zero and name the orphaned acceptance criterion.
- [ ] WHEN a slice has no vertical link to a release benefit (and no release-goal floor link),
      THE SYSTEM SHALL exit non-zero and name the slice.
- [ ] WHEN every need links to >=1 AC, every AC links to a need and >=1 test, and every slice
      links up to a release benefit, THE SYSTEM SHALL exit 0 and print the 2-D matrix.
- [ ] THE SYSTEM SHALL build the matrix from `intake.md` / `spec.md` / `status.json` /
      `index.md` alone — no separate datastore is introduced.
- [ ] WHERE no org objective is declared for the release, THE SYSTEM SHALL accept
      `slice -> release goal` as the vertical floor and SHALL NOT require an org-objective link.

## Required tests

- **Unit**: `internal/rtm/rtm_test.go` — orphaned-need fails; orphaned-AC (no need / no test)
  fails; missing vertical link fails; fully-traced fixture passes; solo floor (no objective)
  passes on `slice -> release goal`.
- **Integration**: exercise `sworn lint trace` end-to-end on a fixture release tree (Rule 1: the test
  drives the actual command entry point, not just the `rtm` package).
- **Reachability artefact**: smoke step — "run `sworn lint trace <fixture-release>`; observe the
  printed matrix and exit 0; introduce a deliberately orphaned need in the fixture intake;
  re-run; observe the named orphan and non-zero exit."
- **E2E gate type**: `local` (no persona creds; the verifier can run it against a fixture).

## Risks

- **Trace id stability** — need ids must survive intake edits/renames or traces silently break.
  Mitigate: an explicit, stable id scheme (e.g. `N-01`) assigned at planning, never reused;
  `sworn lint trace` reports dangling references rather than silently dropping them.
- **Over-proceduralisation for solo/small teams** — mitigated by the release-goal vertical
  floor and lightweight ids; the gate must stay cheap for a one-person release.
- **Scope bleed into S05** — S01 must carry the vertical-link *field + enforcement* only, and
  not absorb benefit-hypothesis authoring or scenario validation; keep the seam clean.

## Deferrals allowed?

No.
