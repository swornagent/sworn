---
title: Rule 10 — Customer Journey Validation
description: Critical customer journeys are a first-class platform artefact — AI-drafted, human-ratified, version-controlled, and fail-closed on absence or staleness. A journey is the unit of end-to-end evidence.
---

# Rule 10 — Customer Journey Validation

## The rule

**Critical customer journeys are a first-class platform artefact, not a per-release afterthought.** Before a release can ship, its customer journeys must be:

1. **Elicited** — the model drafts candidate critical customer journeys from the app; no draft means no journeys gate.
2. **Ratified** — a human reviews, edits, and ratifies the journeys; model-only journeys are unratified and fail the gate.
3. **Durable** — journeys are persisted to a version-controlled artefact (`.sworn/journeys.json`) that survives session boundaries and is maintained release over release.

A journey is an ordered, end-to-end path a user type takes across the app to achieve an outcome. It is the unit of end-to-end evidence — if a release changes a user-visible surface, the journey that crosses that surface must be updated.

## Why

Baton Rules 1/6/7 verify **delivery against the spec** within a single slice. The s

pec scopes one slice, one outcome. A critical customer journey crosses many slices — it is the full path a user takes. If release work changes a surface that a journey crosses, the journey (not just the slice) must be re-verified.

Journey validation exists at a different level of abstraction from slice verification:

| Artefact | Scope | Owned by | Gate |
|---|---|---|---|
| Slice spec | One slice, one user-reachable outcome | Planner + Verifier | Rule 7 (adversarial verification) |
| Journey | End-to-end user path across many slices | Human + Model | Rule 10 (elicitation + ratification) |

A slice that passes Rule 7 but leaves a journey stale is an integration defect that no per-slice gate catches. Journey validation closes that gap.

## The journey artefact

The journeys artefact lives at `<project-root>/.sworn/journeys.json`. It is a JSON document containing:

- **Version** — schema version for forward compatibility.
- **Journeys** — the list of critical customer journeys, each with:
  - **id** (e.g. `J01-onboard-new-user`)
  - **user_type** (e.g. `free_user`, `pro_user`, `admin`)
  - **outcome** — what the user achieves
  - **steps** — ordered sequence of actions
  - **entry_surface** — where the journey begins
- **Ratification metadata** — `is_ratified`, `ratified_by`, `ratified_at`

Provisional: the exact schema is refined by the live journey-validation hand-run (field-level detail appended via `/replan-release`; verified work stays immutable).

## Enforcement

`sworn journeys --check <project>` is a deterministic, fail-closed gate that reads `.sworn/journeys.json` and returns:

- **Exit 0** — artefact exists and is human-ratified. The journeys are listed.
- **Exit 1** — artefact is missing (elicitation has not been run) or exists but is unratified.
- **Exit 2** — unrecoverable error (parse failure, I/O error).

The gate is additive — it does not replace any existing gate. It runs alongside per-slice verification (Rule 7), not instead of it.

## Workflow

1. A maintainer runs `sworn journeys <project>`.
2. The model drafts candidate journeys from the project structure.
3. The human reviews, edits, and ratifies the artefact (sets `is_ratified=true`,
   `ratified_by`, `ratified_at`).
4. `sworn journeys --check <project>` passes.
5. The journeys artefact is committed to version control and maintained as the project evolves.

## When this rule applies

- Any release that changes a user-visible surface (UI, API, CLI command, form, route).
- Pre-release cutover checks — the journeys gate runs after all slices are verified but before the release merges.

## When this rule does NOT apply

- Infrastructure-only releases with no user-visible change.
- A release with no ratifiable journeys (the tooling produces a minimal set; the human may ratify that minimal set).

## Relationship to existing rules

| Rule | What it does | How Rule 10 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 10 ensures the integration point's journey is documented |
| Rule 6 — Proof Bundle | Closes AC -> test -> proof per slice | Rule 10 adds cross-slice journey evidence |
| Rule 7 — Adversarial Verification | Fresh-context verification of one slice | Rule 10 verifies the end-to-end paths that span slices |
| Rule 8 — Requirements Fidelity | Need -> AC -> test -> proof horizontal trace | Rule 10 adds the vertical journey trace across the release |


## No-mock boundary (S10 enforcement)

**An undeclared mock at a validated boundary is an undeclared Rule-2 deferral and fails closed.** The validated boundaries are: database (DB), authentication (auth), and entitlement (premium/subscription tier). A mock/stub/fake at one of these boundaries is permitted only if declared as a Rule-2 deferral in the slice's `status.json` `open_deferrals` with all three elements (why + tracking + acknowledgement).

### Detection

The `sworn verify` first-pass gate (`internal/verify.CheckBoundaryMocks`) scans the slice's diff for lines that combine:
1. A mock/stub/fake marker (`mock`, `fake`, `stub`, etc.)
2. A validated-boundary keyword (`sql.DB`, `auth`, `premium`, etc.)

Lines matching both patterns are flagged. If the mock's boundary + type matches any open deferral (case-insensitive substring match on boundary name + mock/fake/stub keyword), it is treated as declared and surfaced as a known deferral. Otherwise it is an undeclared boundary mock — the gate exits non-zero (FAIL) and names the offending mock + boundary.

### Implementer guidance

An implementer that cannot reach real infrastructure at a validated boundary must **stop and surface the blocker** (record a blocked-on-environment state) rather than mock around it. The implementer role prompt (`internal/prompt/implementer.md`) instructs this under "Hard constraints" — the stop-don't-mock principle is a binding constraint, not advisory.

### Relationship to journey validation

A journey that crosses a validated boundary (every journey touches the DB; most touch auth) must have its boundary mocks declared as deferrals for the verification gate to pass. This prevents silent mock-around in journey integration tests — if a journey test mocks auth at the boundary without declaring it, the slice fails. Journey validation (the artefact) is separate from no-mock enforcement (the gate), but they compose: the journey tells you what to test; the no-mock gate tells you how you must test it (no silent mocking).

### When this applies

- Every slice whose diff introduces, uses, or constructs a mock/stub/fake at a validated boundary.
- Every implementer session that operates the S10 implementer prompt.

### When this does NOT apply

- Pure unit test mocks that do not touch a validated boundary (e.g. a mock calculator, a mock string formatter). These are internal to the unit and are not flagged.
- The human walkthrough (S13), where mocks are fully off and real journeys run against real infra — that slice is out of scope for S10.


## Impact analysis (S12)

**Per-release journey-impact analysis ties Rule 10 into the release workflow.** For a given release, `sworn journeys --impact <release>` computes which critical journeys the release touches, derived from the release's slice planned/actual files and the journeys' step surfaces. The output is the release's validation scope: the set of journeys that must be walked and re-tested before cutover.

### Algorithm

1. **Load the journeys artefact** from `.sworn/journeys.json` — fail-closed if missing or unratified.
2. **Collect slice touchpoints** — scan `docs/release/<release>/S*/status.json` for each slice's `planned_files` and `actual_files` (the files the release changes).
3. **Heuristic surface matching** — for each journey, its `entry_surface` and each step's `surface` are matched against the collected touchpoint files:
   - Level 1: direct substring match (normalised to lowercase).
   - Level 2: token-level match (alphanumeric tokens from both file path and surface).
   - Level 3: conventional mapping (surface "CLI" maps to files under `cmd/`).
4. **Output the touched set** — a journey is in-scope if any of its surfaces match any touchpoint file. The heuristic is biased toward over-inclusion (a journey is touched if any step's surface is touched), so the walkthrough scope errs safe.

### Fail-closed on missing artefact

If no ratified journeys artefact exists at `.sworn/journeys.json`, impact analysis cannot run and directs the user to run elicitation (S11) first. An unratified artefact also fails — ratification is required before impact analysis.

### Empty touched set

A release that touches no journeys (e.g. an internal-only refactor with touchpoints that no journey surface matches) reports an empty touched-set explicitly rather than failing. This allows infrastructure-only releases to pass the gate.

### CLI

```
sworn journeys --impact <release> [project-path]
```

Exit codes:
- **0** — success; the touched-journey set is reported (may be empty).
- **1** — journeys artefact missing or unratified.
- **2** — unrecoverable error (I/O or parse failure).

The impact result is consumed by S13 (walkthrough attestation) and S14 (journey regression suite) to determine the release's validation scope.


## Provenance
Rule 10 was introduced in the `2026-06-16-fidelity-layer` release. It closes the cross-slice integration-evidence gap: per-slice verification (Rules 1/6/7) catches within-slice defects, but no artefact captures the end-to-end user path. Journey validation fills that gap with a lightweight, version-controlled artefact that survives release boundaries.