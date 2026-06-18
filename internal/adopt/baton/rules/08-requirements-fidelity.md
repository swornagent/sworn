---
title: Rule 8 — Requirements Fidelity
description: The spec is not an axiom. Requirements are verified (quality), validated (sense-check), and traced (need -> AC -> test -> proof) so a need cannot drop silently between intake and spec.
---

# Rule 8 — Requirements Fidelity

## The rule

**The spec is not an axiom.** Before a slice enters implementation, its
requirements must be:

1. **Verified** — each acceptance criterion is singular, unambiguous,
   complete, consistent, feasible, and verifiable (ISO/IEC/IEEE 29148:2018
   quality characteristics). A fresh-context gate checks this.
2. **Validated** — the requirement makes sense and serves the need. A
   human-owned scenario sense-check (positive AND negative) confirms the
   spec answers the right question, not just a well-formed one.
3. **Traced** — every need in the intake links to at least one acceptance
   criterion, every acceptance criterion links to a need and at least one
   test, and every slice links up the vertical golden thread (org objective
   -> release benefit -> slice, or the lightweight floor: slice -> release
   goal). The 2-D requirements traceability matrix (RTM) enforces this
   fail-closed.

A need that drops silently between intake and spec is a requirements-fidelity
defect. The RTM makes it visible and blocks the release.

## Why

Baton Rules 1/6/7 verify **delivery against the spec** rigorously. They treat
the spec itself as an axiom — the spec is the contract, and the verifier
checks the code against it. But the spec can be wrong, incomplete, or
disconnected from what the user actually asked for. The front half of the
fidelity chain — from intake need to spec acceptance criterion — is
unverified by the delivery rules. A perfectly implemented, perfectly verified
slice that answers the wrong question is a fidelity defect that no amount of
delivery rigour will catch.

The gap is structural: the delivery rules are **downstream** of the spec.
They cannot see upstream. Rule 8 closes the front half.

## The 2-D requirements traceability matrix (RTM)

The RTM is the enforcement mechanism. It has two axes:

### Horizontal: need -> acceptance criterion -> test -> proof

```
intake.md          spec.md              spec.md             proof.md
--------           --------              --------            --------
N-01: need  --->   - [ ] AC cites N-01   Required tests  ->  test results
                   - [ ] AC cites N-01                      reachability
```

- **Needs** are enumerated with stable ids (`N-01`, `N-02`, ...) in
  `intake.md`. The planner assigns ids at planning time; they are never
  reused.
- **Acceptance criteria** in each `spec.md` cite the need id(s) they satisfy.
  The citation is inline in the AC text (e.g. "WHEN ... THE SYSTEM SHALL ...
  (N-01)").
- **Required tests** in `spec.md` cite the acceptance check they exercise.
- **Proof** in `proof.md` closes `AC -> test -> proof` (already enforced by
  Rule 6).

The RTM adds the front half: `need -> AC`. An orphaned need (no linked AC) or
an orphaned AC (cites no need, or cites a need but has no test) is a broken
trace.

### Vertical: org objective -> release benefit -> slice

```
org objective  --->  release benefit  --->  slice
(optional)          (index.md)            (status.json)
```

- **Org objective** is opt-in. A solo founder or small team may have no
  declared objective — the vertical floor is `slice -> release goal`.
- **Release benefit** is the value the release delivers, recorded in
  `index.md`.
- **Slice link** is the slice's contribution to the release benefit, recorded
  in `status.json` (`release_benefit` field).

The vertical trace is the golden thread: it carries line-of-sight from
strategy (if declared) through release value to individual slices. For
solo/small teams, the floor is lightweight: `slice -> release goal` satisfies
the vertical trace without an org-objective link.

## Enforcement

`sworn lint trace <release>` builds the matrix from `intake.md` / `spec.md` /
`status.json` / `index.md` alone — no separate datastore. It fails closed
(exit non-zero) on:

- An orphaned need (need with no linked acceptance criterion).
- An orphaned acceptance criterion (cites no need id, or cites a need but has
  no linked test).
- A slice with no vertical link (no release goal in intake and no release
  benefit or org objective on the slice).

A fully-traced release prints the matrix and exits 0.

## Lightweight by default

The RTM and the front-end gates must not over-proceduralise solo / small-team
work. The design choices that keep it lightweight:

- **Stable but simple need ids** — `N-01`, not a database. The planner
  assigns them; they survive edits.
- **Inline citation** — need ids are cited inline in AC text, not in a
  separate mapping file. The RTM parses them from the spec.
- **Vertical floor** — `slice -> release goal` is enough when no org
  objective is declared. Enterprise depth is opt-in.
- **No separate datastore** — the RTM threads through existing artefacts.

## Relationship to existing rules

| Rule | What it does | How Rule 8 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 8 ensures the integration point is the *right* one — traced to a need |
| Rule 6 — Proof Bundle | Closes AC -> test -> proof | Rule 8 adds the front half: need -> AC. Together they form the full horizontal chain |
| Rule 7 — Adversarial Verification | Fresh-context verification of delivery | Rule 8 verifies the spec itself, before delivery verification runs |

## When this rule applies

- Any release with an `intake.md` that declares needs. The RTM is the
  enforcement; the planner constructs the trace as a by-product of planning.
- The `planned -> in_progress` transition (Definition of Ready, Rule 8 +
  S06) gates on the RTM passing.

## When this rule does NOT apply

- Spikes or exploratory work without a release intake.
- A release with no declared needs (the RTM reports an empty matrix and
  exits 0 — no needs means no traces to break).

## Spec-quality metric — pre-code soundness + completeness

Before a spec reaches verification or validation, `sworn specquality <release>`
provides a deterministic, pre-code first-pass: soundness + completeness metrics
computed from a slice's **acceptance examples** alone, with no source code and
no model call.

### Acceptance examples

Every spec SHOULD include a `## Acceptance examples` section with one or more
**input → expected-output** pairs per acceptance check. These are the data the
metric operates on. Two formats are accepted:

**Structured** (preferred for complex specs):
```
## Acceptance examples

- name: "whole-ears-pass"
  input: "a release where every AC matches an EARS pattern"
  expected: "sworn lint ac exits 0 and prints per-pattern distribution"

- name: "free-form-fail"
  input: "a release with at least one free-form AC"
  expected: "sworn lint ac exits 1 naming the slice and line"
```

**Shorthand** (for simple cases where input/expected is clear):
```
## Acceptance examples

- valid ears release → exits 0
- free-form ac present → exits 1
```

### Soundness

For each example, `specquality` checks that the expected output is consistent
with the acceptance criteria — the criteria must not reject a valid output.
This is a limited deterministic check: it flags contradictions like "expects
failure where criteria describe only a pass case" or "references commands not
mentioned in the criteria."

### Completeness (mutation analysis)

For each example, `specquality` applies deterministic mutation operators to the
expected output (flip exit codes, negate assertions, remove keywords, change
case) and checks what fraction the criteria would reject. The completeness
score is `caught / total`. Below the configurable threshold (default 50%),
the gate fails closed.

### Enforcement

`sworn specquality <release> [--threshold <0.0-1.0>]` iterates every slice in
a release, computes soundness + completeness from each slice's acceptance
examples, and fails closed (exit non-zero) on:

- **Missing examples** — a slice with no `## Acceptance examples` section
- **Unsound example** — expected output contradicts the criteria
- **Low completeness** — below the threshold (default 0.5 / 50%)

A fully passing release prints per-slice scores and exits 0.

### Relationship to verify and validate

| Gate | What it checks | When | Owner | Tool |
|---|---|---|---|---|
| **Spec-quality** (S03) | Soundness + completeness of acceptance examples | Pre-code, pre-verification | Deterministic | `sworn specquality` |
| **Verify** (S04) | Quality characteristics per 29148 | After spec-quality passes | Model (fresh context) | `sworn reqverify` |
| **Validate** (S05) | Scenarios + benefit (sense-check) | After verify passes | Human | `sworn reqvalidate` |

The three gates form a pipeline: spec-quality is the cheapest (deterministic,
no model, no human), so it runs first. A spec with no acceptance examples or
low completeness never reaches model-based verification.

## Provenance
Rule 8 was introduced in the `2026-06-16-fidelity-layer` release. It closes
the "front half" fidelity gap identified during the v0.5.0 release cycle: the
delivery rules (1/6/7) verify code against spec, but nothing verified the
spec against the need. The RTM is the keystone — it threads through existing
artefacts and enforces traceability fail-closed, so a need cannot drop
silently between intake and spec.
## EARS notation — structured acceptance criteria

The RTM enforces *traceability* (need -> AC -> test). EARS (Easy Approach to
Requirements Syntax) enforces *structure* — each acceptance criterion is a
single sentence with a fixed keyword shape, not free-form prose. Together
they form the front-end fidelity gate: traced AND well-formed.

`sworn lint ac <release>` classifies every acceptance check in every slice's
`spec.md` by EARS pattern and fails closed on any free-form check that matches
no pattern, naming the slice + the offending line. A release whose every AC is
well-formed EARS passes and prints the per-pattern distribution.

### The six EARS pattern classes

| Class | Pattern | Example |
|---|---|---|
| Ubiquitous | `THE SYSTEM SHALL <action>` | `THE SYSTEM SHALL display the dashboard.` |
| Event-driven | `WHEN <trigger> THE SYSTEM SHALL <action>` | `WHEN a user clicks save THE SYSTEM SHALL persist the form.` |
| State-driven | `WHILE <state> THE SYSTEM SHALL <action>` | `WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.` |
| Optional-feature | `WHERE <feature> THE SYSTEM SHALL <action>` | `WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.` |
| Unwanted-behaviour | `IF <condition> THEN THE SYSTEM SHALL <action>` | `IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.` |
| Complex | Two or more preconditions combined | `WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.` |

### The NOTE: escape

A line prefixed with `NOTE:` is a deliberate non-requirement note and is
excluded from validation. Use it for context that is not a testable
requirement (e.g. a design constraint, a cross-reference, a rationale note).
Without the escape, such lines would fail the gate as free-form.

### Why EARS, not Gherkin

Gherkin (Given-When-Then) was considered and rejected. EARS is lighter (one
sentence per requirement, no scenario tables), is the de-facto notation for
agent-authored requirements, and maps cleanly to the checkbox format already
used in `spec.md` acceptance checks. The decision is recorded; do not
re-litigate at implementation.

## Validation — human-owned sense-check

Validation answers "are we building the *right* requirements?" — does the spec
make sense and serve the need (as distinct from verification's "are the
requirements well-formed?"). This is the cheapest defect-catch point and is
**human-owned**: the model drafts scenarios + a benefit hypothesis; the human
ratifies. Spec validation has no oracle but the user, so this gate is never LLM
self-certified.

### Validation record

Every slice must carry a validation record in its `status.json` under the
`validation` field (see `state.ValidationRecord`):

| Field | Required | Description |
|---|---|---|
| `human_ratified` | Yes | Must be `true`. Model-only validation is not a pass. |
| `ratified_by` | Yes | Who ratified (human identifier). |
| `ratified_at` | Yes | When ratified (ISO 8601). |
| `positive_scenarios` | Yes (≥1) | Scenarios where the requirement works as intended. |
| `negative_scenarios` | Yes (≥1) | Edge + failure flows; what should *not* happen. |
| `benefit_hypothesis` | Yes | This slice's benefit and its vertical link (slice -> release benefit -> objective). |

### Enforcement

`sworn reqvalidate <release>` reads every slice's `status.json` and fails
closed on:

- **Missing record** — no `validation` field at all
- **Model-only** — `human_ratified` is false or absent
- **Missing positive scenarios** — empty `positive_scenarios` array
- **Missing negative scenarios** — empty `negative_scenarios` array
- **Missing benefit hypothesis** — empty or blank `benefit_hypothesis`

A fully-validated release exits 0 and prints the per-slice summary.

### Relationship to verification

| Gate | What it checks | Owner | Tool |
|---|---|---|---|
| **Verify** (S04) | Quality characteristics per 29148 (well-formedness) | Model (fresh context) | `sworn reqverify` |
| **Validate** (S05) | Scenarios + benefit (sense-check) | Human | `sworn reqvalidate` |

The two gates are complementary. A spec can be perfectly well-formed (pass
reqverify) but answer the wrong question (fail reqvalidate), and vice versa.
Both must pass before a slice enters implementation (Definition of Ready, S06).