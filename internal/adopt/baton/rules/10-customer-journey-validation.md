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

## Provenance

Rule 10 was introduced in the `2026-06-16-fidelity-layer` release. It closes the cross-slice integration-evidence gap: per-slice verification (Rules 1/6/7) catches within-slice defects, but no artefact captures the end-to-end user path. Journey validation fills that gap with a lightweight, version-controlled artefact that survives release boundaries.