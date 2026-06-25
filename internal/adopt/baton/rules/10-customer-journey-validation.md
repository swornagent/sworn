---
title: Rule 10 — Customer Journey Validation
description: Critical customer journeys are a first-class artefact — AI-drafted, human-ratified, version-controlled, fail-closed on absence or staleness. A journey is the unit of end-to-end evidence, and a journey walked over a mocked boundary proves nothing.
---

# Rule 10 — Customer Journey Validation

## The rule

**Critical customer journeys are a first-class artefact, not a per-release afterthought.** Before a release can ship, its customer journeys must be:

1. **Elicited** — the model drafts candidate critical journeys from the app. No draft means no journeys gate.
2. **Ratified** — a human reviews, edits, and ratifies the journeys. Model-only journeys are unratified and fail the gate.
3. **Durable** — journeys are persisted to a version-controlled artefact that survives session boundaries and is maintained release over release.

A journey is an ordered, end-to-end path a user type takes across the app to achieve an outcome. It is the unit of end-to-end evidence: if a release changes a user-visible surface, the journey that crosses that surface must be updated.

## Why

Rules 1, 6, and 7 verify **delivery against the spec** within a single slice. A slice spec scopes one slice, one outcome. A critical customer journey crosses many slices — it is the full path a user takes. If release work changes a surface a journey crosses, the journey (not just the slice) must be re-verified.

Journey validation sits at a different level of abstraction from slice verification:

| Artefact | Scope | Owned by | Gate |
|---|---|---|---|
| Slice spec | One slice, one user-reachable outcome | Planner + Verifier | Rule 7 (adversarial verification) |
| Journey | End-to-end user path across many slices | Human + Model | Rule 10 (elicitation + ratification) |

A slice that passes Rule 7 but leaves a journey stale is an integration defect no per-slice gate catches. Journey validation closes that gap: Rule 7 verifies the parts; Rule 10 verifies the assembled whole.

## The journey artefact

The journeys artefact is a version-controlled JSON document at a stable project path. It contains:

- **Version** — schema version for forward compatibility.
- **Journeys** — the list of critical journeys, each with an **id** (e.g. `J01-onboard-new-user`), a **user_type** (e.g. `free_user`, `pro_user`, `admin`), an **outcome** (what the user achieves), ordered **steps**, and an **entry_surface** (where the journey begins).
- **Ratification metadata** — `is_ratified`, `ratified_by`, `ratified_at`.

## Enforcement

A deterministic, fail-closed gate reads the journeys artefact and returns:

- **Exit 0** — artefact exists and is human-ratified; the journeys are listed.
- **Exit 1** — artefact is missing (elicitation not run) or exists but is unratified.
- **Exit 2** — unrecoverable error (parse failure, I/O error).

The gate is additive — it runs alongside per-slice verification (Rule 7), after all slices are verified but before the release merges. It does not replace any existing gate.

## No-mock boundary — the enforcement that makes a journey count

Journey validation exists to prove the **assembled system actually works end-to-end**. A journey walked over a *mocked* boundary proves nothing — the mock answers however the test author wired it to, not however the real system would. So the no-mock boundary is **constitutive of Rule 10, not a detachable add-on**: it is the enforcement that makes a walked journey count as proof.

The artefact and the gate are not two rules that happen to compose — they are one rule's two faces. The journey says *what* end-to-end path must work; the no-mock gate guarantees the walk that proves it didn't cheat at the boundary. A journey whose boundary is mocked is a journey that has not been validated at all, regardless of a green test.

**The validated boundaries** are: database (DB), authentication (auth), and entitlement (premium/subscription tier) — the integration points where a mock most easily hides a journey that doesn't really work.

**The constraint.** On an environment wall — when real infrastructure at a validated boundary cannot be reached — the implementer must **stop and surface the blocker**, never mock around it. A mock/stub/fake at a validated boundary is permitted only if it is a declared deferral with all three Rule 2 elements (why + tracking + acknowledgement). An *undeclared* boundary mock is an undeclared silent deferral and fails the gate closed.

This reads as a Rule 2 concern too — an undeclared mock is a species of silent deferral — but its home is Rule 10, because the specific failure it prevents is *a journey that lies about working*.

**Detection (deterministic first-pass).** A diff-scanning check flags lines that combine a mock/stub/fake marker (`mock`, `fake`, `stub`, …) with a validated-boundary keyword (`sql.DB`, `auth`, `premium`, …). If the flagged mock matches an open declared deferral, it is surfaced as a known deferral; otherwise it is an undeclared boundary mock and the gate exits non-zero, naming the offending mock and boundary.

**When the no-mock gate applies:** every slice whose diff introduces, uses, or constructs a mock/stub/fake at a validated boundary. **When it does not:** pure unit-test mocks that touch no validated boundary (a mock calculator, a mock string formatter), and the human walkthrough itself, where mocks are fully off and real journeys run against real infra.

## Workflow

1. A maintainer runs journey elicitation against the project.
2. The model drafts candidate journeys from the project structure.
3. The human reviews, edits, and ratifies the artefact (`is_ratified=true`, `ratified_by`, `ratified_at`).
4. The journeys gate passes; the artefact is committed and maintained as the project evolves.
5. At release cutover, the journeys that the release touches are re-walked against real boundaries (no-mock), and the walkthrough is human-attested before ship.

## Relationship to existing rules

| Rule | What it does | How Rule 10 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 10 ensures the integration point's journey is documented and re-walked |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | An undeclared boundary mock is a silent deferral caught by Rule 10's no-mock gate |
| Rule 6 — Proof Bundle | Closes AC → test → proof per slice | Rule 10 adds cross-slice journey evidence |
| Rule 7 — Adversarial Verification | Fresh-context verification of one slice | Rule 10 verifies the end-to-end paths that span slices |
| Rule 8 — Requirements Fidelity | Need → AC → test → proof horizontal trace | Rule 10 adds the vertical journey trace across the release |

## When this rule applies

- Any release that changes a user-visible surface (UI, API, CLI command, form, route).
- Pre-release cutover — the journeys gate runs after all slices are verified but before the release merges.

## When this rule does NOT apply

- Infrastructure-only releases with no user-visible change.
- A release with no ratifiable journeys (the tooling produces a minimal set; the human may ratify that minimal set).

## Provenance

Rule 10 was introduced in the `2026-06-16-fidelity-layer` cycle. It closes the integration gap above per-slice verification: a release of individually-verified slices can still leave a cross-slice user path broken or stale. The no-mock boundary is folded in as Rule 10's enforcement teeth — the recognition that an end-to-end journey only counts as evidence if it ran against real boundaries, not mocks.
