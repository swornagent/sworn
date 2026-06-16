---
title: 'S03-spec-quality-firstpass'
description: 'Deterministic, pre-code spec-quality first-pass: soundness + completeness metrics computed from a slice acceptance examples alone, flagging specs that would not catch a wrong implementation.'
---

# Slice: `S03-spec-quality-firstpass`

> The single most directly implementable research primitive — the requirements-side analog of
> the delivery first-pass script (`release-verify.sh`). A spec's acceptance examples are scored
> for soundness + completeness **before any code**, deterministically. T3; depends_on T1.

## User outcome

When a planner runs `sworn specquality <release>`, sworn computes, from each slice's acceptance
examples, a **soundness** score (the criteria accept every valid implementation — no false
rejection) and a **completeness** score (the fraction of output mutations the criteria reject —
mutation analysis), and **fails closed** when a slice falls below the completeness threshold —
i.e. its acceptance examples would not catch a wrong output. Computed pre-code, no model call.

## Entry point

- **Native:** `sworn specquality <release>` (additive `case "specquality"`; implementation in
  `cmd/sworn/specquality.go`) wrapping the deterministic metric in `internal/specquality/`. A
  thin `bin/spec-quality.sh` exposes it for first-pass / CI use.
- **Protocol:** `internal/prompt/planner.md` + the spec template instruct attaching **acceptance
  examples** (input -> expected-output pairs) to acceptance criteria so the metric has data.

## In scope

- **Acceptance-examples input**: a structured set of input -> expected-output pairs per slice
  (drawn from / alongside the acceptance criteria), read from the spec.
- **Soundness** (`internal/specquality/`): every example's expected output is accepted by the
  criteria (the criteria do not reject a valid output).
- **Completeness** (mutation analysis): for each example, mutate the expected output; the
  fraction of mutations the criteria reject is the completeness score.
- **Threshold gate**: fail closed when a slice's completeness is below the configured threshold;
  report soundness + completeness per slice.
- **Deterministic, pre-code, no model call** — the defining property.

## Out of scope

- **The 29148 fresh-context quality judgement** — S04 (model-driven; different enforcement mode).
- **EARS notation** (S02) and **trace linkage** (S01) — inputs.
- **Authoring the examples** — planner does this interactively; S03 scores what exists.

## Planned touchpoints

- `internal/specquality/specquality.go`, `internal/specquality/specquality_test.go` (new)
- `cmd/sworn/specquality.go` (new command)
- `cmd/sworn/main.go` (additive `case "specquality"`)
- `bin/spec-quality.sh` (new — first-pass wrapper)
- `internal/prompt/planner.md` (acceptance-examples authoring guidance — shared with T1 via
  depends_on)
- `internal/adopt/baton/rules/08-requirements-fidelity.md` (spec-quality-metric section)

## Acceptance checks

- [ ] WHEN a slice's acceptance examples reject one of their own valid expected outputs, THE
      SYSTEM SHALL report a soundness violation and name the example.
- [ ] WHEN a slice's completeness (mutation-detection fraction) is below the configured
      threshold, THE SYSTEM SHALL exit non-zero from `sworn specquality <release>` and name the
      slice + its score.
- [ ] WHEN every slice is sound and meets the completeness threshold, THE SYSTEM SHALL exit 0
      and print per-slice soundness + completeness.
- [ ] THE SYSTEM SHALL compute both metrics from the acceptance examples alone, with no source
      code and no model call (fully deterministic).
- [ ] WHEN a slice has no acceptance examples, THE SYSTEM SHALL fail and direct the planner to
      add them (the metric cannot be computed without data).

## Required tests

- **Unit**: `internal/specquality/specquality_test.go` — a sound+complete example set passes; an
  example set that accepts a mutated output scores low completeness and fails; an unsound set
  (rejects a valid output) is flagged; missing examples fail.
- **Integration**: `sworn specquality <fixture-release>` end-to-end + `bin/spec-quality.sh`
  invocation (Rule 1).
- **Reachability artefact**: smoke step — "run `sworn specquality <fixture>` on a slice whose
  examples miss a mutation; observe the low-completeness failure; tighten the examples; observe
  pass."
- **E2E gate type**: `local`.

## Risks

- **Mutation strategy scope** — output-mutation operators must be meaningful for the domain.
  Mitigate: start with a small, documented operator set (boundary, negation, omission);
  expandable; report which operators ran so the score is interpretable.
- **Abstractness for protocol specs** — sworn's own acceptance examples are command-behaviour
  I/O (release-with-orphan -> non-zero+named); the metric applies but examples must be concrete.
  The spec mandates concrete examples; vague ones fail the no-examples check.

## Deferrals allowed?

No.
