---
title: 'S02-ears-ac-format'
description: 'EARS acceptance-criteria notation: a template the planner authors against and a validator that flags free-form, non-EARS acceptance checks.'
---

# Slice: `S02-ears-ac-format`

> Notation only. S02 makes each acceptance check structured + classifiable; it does not check
> the linkage (S01) or the deeper 29148 quality characteristics (S04).

## User outcome

When a planner drafts acceptance criteria, they author them in EARS notation, and
`sworn ears <release>` classifies every acceptance check by EARS pattern and **fails closed**
on any free-form check that matches no pattern, naming the slice + the offending line. A
release whose every AC is well-formed EARS passes and prints the per-pattern breakdown.

## Entry point

- **Native:** `sworn ears <release>` (additive `case "ears"` in `cmd/sworn/main.go`,
  implementation in `cmd/sworn/ears.go`).
- **Protocol:** `internal/prompt/planner.md` instructs authoring acceptance checks in EARS; the
  release-mode `spec.md` template's "Acceptance checks" section documents the EARS patterns.

## In scope

- **EARS pattern set** (`internal/ears/`): ubiquitous (`THE SYSTEM SHALL …`), event-driven
  (`WHEN <trigger> THE SYSTEM SHALL …`), state-driven (`WHILE <state> THE SYSTEM SHALL …`),
  optional-feature (`WHERE <feature> THE SYSTEM SHALL …`), unwanted-behaviour
  (`IF <condition> THEN THE SYSTEM SHALL …`), and complex (combinations).
- **Validator:** classify each acceptance check in a slice's `spec.md`; flag any that match no
  pattern; report the per-pattern distribution.
- **Authoring guidance:** planner.md + the spec template teach the patterns so ACs are EARS by
  construction.

## Out of scope

- **Trace linkage** (need <-> AC <-> test) — S01.
- **29148 quality characteristics** (singular, unambiguous, complete, feasible …) beyond
  "matches an EARS shape" — S04.
- **Gherkin / Given-When-Then** — explicitly not adopted; EARS is lighter and the de-facto
  agent notation. (Recorded as a considered-and-rejected alternative.)

## Planned touchpoints

- `internal/ears/ears.go`, `internal/ears/ears_test.go` (new — pattern set + classifier)
- `cmd/sworn/ears.go` (new command)
- `cmd/sworn/main.go` (additive `case "ears"`)
- `internal/prompt/planner.md` (author-in-EARS guidance)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (EARS section — extends the doc S01
  creates; same track, serialised)

## Acceptance checks

- [ ] WHEN a slice's `spec.md` contains an acceptance check matching no EARS pattern, THE
      SYSTEM SHALL exit non-zero from `sworn ears <release>` and name the slice + the line.
- [ ] WHEN every acceptance check across the release matches an EARS pattern, THE SYSTEM SHALL
      exit 0 and print the per-pattern distribution.
- [ ] THE SYSTEM SHALL recognise all six EARS pattern classes (ubiquitous, event-driven,
      state-driven, optional-feature, unwanted-behaviour, complex).
- [ ] WHERE an acceptance check is a deliberate non-requirement note, THE SYSTEM SHALL provide
      an explicit escape (e.g. a leading `NOTE:`) so it is excluded rather than failing the gate.

## Required tests

- **Unit**: `internal/ears/ears_test.go` — each pattern class classifies correctly; a free-form
  line is flagged; the `NOTE:` escape is excluded.
- **Integration**: `sworn ears` exercised on a fixture release with one malformed AC (Rule 1:
  drives the command entry point).
- **Reachability artefact**: smoke step — "run `sworn ears <fixture>`; observe pass + pattern
  breakdown; corrupt one AC to free-form; re-run; observe the named failure + non-zero exit."
- **E2E gate type**: `local`.

## Risks

- **Over-strict classification** rejecting legitimate phrasings — mitigate with a permissive
  matcher (case-insensitive, whitespace-tolerant) and the `NOTE:` escape; the gate flags, the
  human adjusts.
- **EARS vs Gherkin debate resurfacing** — the decision is recorded; do not re-litigate at
  implementation.

## Deferrals allowed?

No.
