---
title: Rule 8 — Requirements Fidelity
description: The spec is not an axiom. Requirements are verified (quality), validated (sense-check), and traced (need -> AC -> test -> proof) so a need cannot drop silently between intake and spec.
---

# Rule 8 — Requirements Fidelity

## The rule

**The spec is not an axiom.** Before a slice enters implementation, its requirements must be:

1. **Verified** — each acceptance criterion is singular, unambiguous, complete, consistent, feasible, and verifiable (the ISO/IEC/IEEE 29148:2018 quality characteristics). A fresh-context gate checks this.
2. **Validated** — the requirement makes sense and serves the need. A human-owned scenario sense-check (positive AND negative) confirms the spec answers the right question, not just a well-formed one.
3. **Traced** — every need in the intake links to at least one acceptance criterion, every acceptance criterion links back to a need and forward to at least one test, and every slice links up a vertical golden thread (org objective → release benefit → slice, or the lightweight floor: slice → release goal).

A need that drops silently between intake and spec is a requirements-fidelity defect. The traceability matrix makes it visible and blocks the release.

## Why

Rules 1, 6, and 7 verify **delivery against the spec** rigorously. They treat the spec itself as an axiom — the spec is the contract, and the verifier checks the code against it. But the spec can be wrong, incomplete, or disconnected from what the user actually asked for. The front half of the fidelity chain — from intake need to spec acceptance criterion — is unverified by the delivery rules. A perfectly implemented, perfectly verified slice that answers the wrong question is a fidelity defect no amount of delivery rigour will catch.

The gap is structural: the delivery rules are **downstream** of the spec. They cannot see upstream. Rule 8 closes the front half.

This is the same insight the README frames around requirements failure: decades of post-mortems converge on *poor requirements* — lost, drifted, met-technically-but-missed-the-intent — as the dominant cause of project failure. Rules 1–7 keep delivery honest; Rule 8 keeps the requirement itself honest before delivery begins.

## The 2-D requirements traceability matrix (RTM)

The RTM is the enforcement mechanism. It has two axes and threads through the existing artefacts — no separate datastore.

### Horizontal: intake need → slice → acceptance criterion → test → proof

```
intake.md          status.json         spec.json               spec.json             proof.json
--------           ------------        --------               --------            --------
N-01: need  --->   covers_needs:  -->   - [ ] AC cites N-01    Required tests  ->  test results
                   [N-01, N-03]         - [ ] AC cites N-01                        reachability
```

- **Needs** are enumerated with stable ids (`N-01`, `N-02`, …) in `intake.md`. The planner assigns ids at planning time; they are never reused.
- **Slice coverage** — every slice declares which intake needs it delivers in `status.json` `covers_needs` (array of need IDs). This is the intake→slice link: a deterministic gate can verify every N-NN appears in at least one slice, and no slice claims a need it doesn't cite in its ACs.
- **Acceptance criteria** in each `spec.json` cite the need id(s) they satisfy, inline in the AC text (e.g. "WHEN … THE SYSTEM SHALL … (N-01)").
- **Required tests** in `spec.json` cite the acceptance check they exercise.
- **Proof** in `proof.json` closes `AC → test → proof` (already required by Rule 6).

The RTM now closes the full chain: `intake need → slice → AC → test → proof`. An orphaned need (no slice covers it, or no AC cites it), an orphaned AC (cites no need, or cites a need but has no test), or a slice that claims a need it doesn't cite in its ACs (mismatch) is a broken trace.

### Vertical: org objective → release benefit → slice

```
org objective  --->  release benefit  --->  slice
(optional)           (board.json)           (status.json)
```

- **Org objective** is opt-in. A solo founder or small team may have no declared objective — the vertical floor is `slice → release goal`.
- **Release benefit** is the value the release delivers, recorded in `board.json`.
- **Slice link** is the slice's contribution to the release benefit, recorded in `status.json`.

The vertical trace is the golden thread: line-of-sight from strategy (if declared) through release value to individual slices. For solo/small teams the floor is lightweight: `slice → release goal` satisfies the vertical trace without an org-objective link.

## Enforcement

A deterministic, fail-closed **trace gate** (reference implementation: `sworn trace <release-name>`) builds the matrix from `intake.md` / `spec.json` / `status.json` / `board.json` alone. It exits 0 on a fully-traced release, non-zero with enumerated violations on any break.

The gate checks:

- **Orphaned need** — an intake need ID (N-NN) that appears in no slice's `covers_needs`. The intake→slice gap.
- **Invalid covers** — a slice's `covers_needs` references a need ID not in intake.md.
- **Unclaimed coverage** — a need ID in `covers_needs` with no AC in that slice's spec citing it. The slice→spec gap.
- **Free-form AC** — an acceptance check that lacks the EARS `shall` keyword and has no `NOTE:` escape. The AC→structure gap.
- **"See intake" reference** — any spec content that refers the implementer to intake.md. The spec must stand alone.
- **Vague AC / scope** — an AC or in-scope item describing no concrete artefact (file, testid, status code, label string, value). The content-density gap.

Run the trace gate at two points in the workflow: (a) planner Phase 6 before handoff, and (b) as the DoR gate at `planned → in_progress`. A release that fails the trace may not ship.

## EARS notation — structured acceptance criteria

The RTM enforces *traceability* (need → AC → test). EARS (Easy Approach to Requirements Syntax) enforces *structure* — each acceptance criterion follows a fixed keyword pattern, not free-form prose. Together they form the front-end fidelity gate: traced AND well-formed.

EARS was developed at Rolls-Royce PLC in 2009 (Mavin et al., IEEE RE'09) and is used worldwide by Airbus, Bosch, Dyson, Honeywell, Intel, NASA, Rolls-Royce, and Siemens.

A deterministic gate classifies every acceptance check in every slice's `spec.json` by EARS pattern and fails closed on any free-form check that matches no pattern, naming the slice and the offending line.

| Class | Pattern | Keywords | Example |
|---|---|---|---|
| Ubiquitous | `<system> shall <response>` | none (always active) | `The API shall return 200 for valid input.` |
| Event-driven | `When <trigger>, <system> shall <response>` | `When` | `When the user clicks Save, the form shall persist to the backend.` |
| State-driven | `While <state>, <system> shall <response>` | `While` | `While the modal is open, the page shall not scroll.` |
| Optional-feature | `Where <feature>, <system> shall <response>` | `Where` | `Where Premium is enabled, the export button shall be visible.` |
| Unwanted-behaviour | `If <condition>, then <system> shall <response>` | `If … then` | `If the database is unreachable, then the API shall return 503.` |
| Complex | Two or more keywords combined | e.g. `While … When …` | `While on mobile, when the user taps Edit, the settings sheet shall open.` |

ACs that use no EARS keyword pattern and no `NOTE:` escape are free-form and fail the gate. The `<system>` slot can be implicit (e.g. "the page", "the API", "the component") or omitted — the litmus is the keyword + `shall` structure, not the specific system noun.

## Spec-quality metric — pre-code soundness + completeness

Before a spec reaches verification or validation, a deterministic, pre-code first-pass computes soundness + completeness from a slice's **acceptance examples** alone — no source code, no model call.

### Structural completeness (the sniff-test gate)

The RTM verifies *traceability* (every need has an AC, every AC has a test) but not *content-density*. A spec can pass traceability while being a thin shadow of its intake section — "fix the windfall bug" passes the EARS check but captures none of the detail the intake elaborated. This is the decomposition-fidelity failure mode: the planner splits intake into slices but fails to decompose the intake-level description into spec-level precision.

Intake is the epic level — broad user outcomes, "what the human wants" in natural language. The spec is the feature/story level — decomposed into concrete, verifiable, implementation-precision acceptance criteria. "Replicate intake detail" is the wrong framing; the spec must *refine* intake detail into finer granularity. Intake says *what* (ticker search); the spec says *where* (`PortfolioEditor.tsx`), *how* (`<TickerSearch />` with `accessToken` prop, Name field `disabled={true}`), and *proves* (testids, status codes, screenshot paths).

A structural-completeness check runs at the `planned → in_progress` transition and fails closed on:

1. **Vague-scope spec** — an AC or in-scope item that could describe *any* slice of its kind ("fix the bug", "add the missing code", "wire up the component"). Every AC must name at least one concrete artefact (a file path, a label string, a data-testid, an assertion value, an HTTP status code). A spec without concretes is a spec that can't be verified — the verifier has nothing concrete to check against.
2. **Missing detail** — a behavioural detail present in the intake's "What the human wants" section for this slice's scope that has no corresponding AC, in-scope item, or planned touchpoint in the spec. A single unmatched intake detail fails the gate.
3. **"See intake" reference** — any spec content that refers the implementer to intake.md (directly or indirectly: "see intake", "refer to intake", "as described in the intake"). The spec owns every detail it covers.

This is a deterministic gate, not a model call — it checks for concrete terms (file paths, quoted strings, testids, status codes) and cross-references intake detail against spec content. A spec with no concretes or missing intake detail never reaches implementation.

### Numeric completeness (mutation analysis)

Every spec SHOULD carry a `## Acceptance examples` section with one or more **input → expected-output** pairs per acceptance check:

```
## Acceptance examples

- name: "valid-ears-pass"
  input: "a release where every AC matches an EARS pattern"
  expected: "the AC lint exits 0 and prints the per-pattern distribution"
- name: "free-form-fail"
  input: "a release with at least one free-form AC"
  expected: "the AC lint exits 1 naming the slice and line"
```

- **Soundness** — for each example, the expected output must be consistent with the acceptance criteria (the criteria must not reject a valid output). A limited deterministic check that flags contradictions like "expects failure where criteria describe only a pass case."
- **Completeness (mutation analysis)** — deterministic mutation operators are applied to the expected output (flip exit codes, negate assertions, remove keywords) and the gate checks what fraction the criteria would reject. The score is `caught / total`; below the threshold (default 50%) the gate fails closed.

Because it is the cheapest check (deterministic, no model, no human), spec-quality runs first. A spec with no acceptance examples or low completeness never reaches model-based verification.

## Validation — human-owned sense-check

Validation answers "are we building the *right* requirements?" — does the spec make sense and serve the need (distinct from verification's "are the requirements well-formed?"). This is the cheapest defect-catch point and is **human-owned**: the model drafts scenarios + a benefit hypothesis; the human ratifies. Spec validation has no oracle but the user, so this gate is never model self-certified.

Every slice carries a validation record in its `status.json`:

| Field | Required | Description |
|---|---|---|
| `human_ratified` | Yes | Must be `true`. Model-only validation is not a pass. |
| `ratified_by` | Yes | Who ratified (human identifier). |
| `ratified_at` | Yes | When ratified (ISO 8601). |
| `positive_scenarios` | Yes (≥1) | Scenarios where the requirement works as intended. |
| `negative_scenarios` | Yes (≥1) | Edge + failure flows; what should *not* happen. |
| `benefit_hypothesis` | Yes | This slice's benefit and its vertical link (slice → release benefit → objective). |

A deterministic gate fails closed on a missing record, model-only ratification, empty positive or negative scenarios, or a blank benefit hypothesis.

## Definition of Ready

The Definition of Ready (DoR) is the gate every slice passes before it can transition from `planned` to `in_progress`. It composes the three checks into a single fail-closed verdict:

1. **Traced** — the RTM verifies complete traceability (horizontal + vertical).
2. **Verified** — every acceptance criterion passes the 29148 quality-characteristic check via a fresh-context model pass.
3. **Validated** — the slice carries a human-ratified validation record.

If any gate fails, the transition is blocked and the failing gate(s) named. If any gate cannot be evaluated (missing artefact, no verifier model configured), the transition is also blocked — fail closed. There is no bypass: an explicit human re-plan is the only way to change a spec, never a silent skip.

## Relationship to existing rules

| Rule | What it does | How Rule 8 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 8 ensures the integration point is the *right* one — traced to a need |
| Rule 2 — No Silent Deferrals | Surfaces drift explicitly | Rule 8 makes a dropped need a hard, detectable trace break |
| Rule 6 — Proof Bundle | Closes AC → test → proof | Rule 8 adds the front half: need → AC. Together they form the full horizontal chain |
| Rule 7 — Adversarial Verification | Fresh-context verification of delivery | Rule 8 verifies the spec itself, before delivery verification runs |

## When this rule applies

- Any release with an `intake.md` that declares needs. The RTM is the enforcement; the planner constructs the trace as a by-product of planning.
- The `planned → in_progress` transition (Definition of Ready) gates on the RTM, verification, and validation all passing.

## When this rule does NOT apply

- Spikes or exploratory work without a release intake.
- A release with no declared needs (the RTM reports an empty matrix and exits 0 — no needs means no traces to break).

## Provenance

Rule 8 was introduced in the `2026-06-16-fidelity-layer` cycle. It closes the "front half" fidelity gap surfaced during the v0.5.0 cycle: the delivery rules (1/6/7) verify code against spec, but nothing verified the spec against the need. The RTM is the keystone — it threads through existing artefacts and enforces traceability fail-closed, so a need cannot drop silently between intake and spec.
