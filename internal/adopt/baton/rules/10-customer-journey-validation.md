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
- **Regression + boundary metadata** (per journey, added in v0.7.0) — `regression_test_path`, the path to the regression test asserted by `has_regression` (present once `has_regression` is true); and `no_mock_boundary`, the entitlement/infra boundary this journey must cross against **real** infrastructure when walked (its absence means no boundary is declared for the journey). `no_mock_boundary` is the machine-readable home of the "No-mock boundary" enforcement below — the gate reads it to know which boundary a walk may not mock.
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

**What "mock at a boundary" means — a code construct, not a string.** A mock at a validated boundary is a **code construct**: a call, binding, or type that *substitutes* the real boundary (a fake `sql.DB`, a stubbed auth client, a hand-rolled entitlement double). It is not the mere textual appearance of the words `mock` / `fake` / `stub` / `@no-mock`. The distinction is load-bearing: code that legitimately *handles the boundary-mock vocabulary* — a slice whose job is to parse `// @no-mock` / `// @mock-boundary` annotations — contains those tokens inside **string literals and comments** without mocking anything (2026-07-12 dogfood, finding 5: an assemble slice failed its own gate closed because a string literal `"// @no-mock\n// @mock-boundary (boundary: entitlement)"` matched the detector).

**Detection (deterministic first-pass).** A diff-scanning check flags **code tokens** — spans that are neither string literals nor comments (AST-level, or a lexer that skips string/comment spans) — combining a mock/stub/fake construct with a validated-boundary reference (`sql.DB`, `auth`, `premium`, …). Occurrences inside string literals or comments are **not** mocks and do not trip the gate. If a flagged construct matches an open declared deferral, it is surfaced as a known deferral; otherwise it is an undeclared boundary mock and the gate exits non-zero, naming the offending construct and boundary. The gate stays fail-closed on real substitutions while not penalising code that handles the annotation vocabulary.

**When the no-mock gate applies:** every slice whose diff introduces, uses, or constructs a mock/stub/fake at a validated boundary. **When it does not:** pure unit-test mocks that touch no validated boundary (a mock calculator, a mock string formatter), and the human walkthrough itself, where mocks are fully off and real journeys run against real infra.

## Mock parity at registered contract boundaries (sub-rule)

The no-mock boundary above activates at release level, at the validated infra boundaries (DB, auth, entitlement). This sub-rule applies the same principle **earlier and lower**: at slice-implementation time, at every boundary registered in the release's contract registry (`contracts.json`, `contracts-v1` schema).

**The sub-rule.** A consumer slice may mock a registered boundary **only with fixtures recorded by the owner's live contract test.** The owner's proof bundle commits actual request/response pairs captured from its passing live test (`fixtures/` in the slice folder, path recorded in the registry entry). Consumer tests load those fixtures as mock data. A hand-written mock at a registered boundary is a silent deferral (Rule 2) unless the consumer includes at least one unmocked in-process round-trip against the real handler.

**Why.** A mock and a spec written from the same assumption share the same blind spot. The observed failure (2026-07-10): a consumer slice passed legitimate fresh-context adversarial verification with a latently-400 PUT, because its mock and its spec both encoded the spec author's wrong body-shape assumption — implementer, tests, and verifier were structurally blind together. Owner-recorded fixtures break the symmetry: the fixture can only contain what the real handler actually accepted, so a wrong consumer assumption goes red at the consumer's own test run instead of surviving to assembly.

**Freshness invariant.** Fixtures are regenerated by the owner's live contract test on green, and must be **newer than the owner's last production-code change to the surface**. A stale fixture is treated as no fixture: drift between handler and fixture must break the consumer's tests visibly, never silently re-agree with an outdated shape. (How freshness is checked is the gate's concern; the invariant is the rule.)

**Mechanics are file-based, inside the existing artefact model** — pact-style without a broker: owner test writes `fixtures/<surface>.json` `{request, response}`; the registry's `fixtures` field points at it; the consumer's mocks import from that path (a grep-level check suffices to start).

**Status: advisory until the grading gate ships.** Per the skew-window policy (baton#59), planners and implementers follow this discipline now, enforced by review; it flips to fail-closed when the reference gate (`sworn lint contracts` fixture checks) ships.

## The assembly stage

Per-slice verification proves the parts; the journey walk proves the whole — but between them sits the assembled system, and every decisive 2026-07-10 failure (a CORS preflight no unit, handler, or per-slice test could see) was caught by an assembly phase that existed only as orchestrator improvisation. Rule 10 makes it first-class: the machine half of validating the assembled whole, run **before** the human half.

**Release-level chain:** `tracks-merged → assembled → journey-validated → merged`.

- **`assembled` is a derived state, not a stored one** (the same invariant that keeps `board.json` a pure plan): a release is `assembled` when `docs/release/<name>/assembly-proof.json` (schema `assembly-proof-v1`, `https://baton.sawy3r.net/schemas/assembly-proof-v1.json`) exists on `release-wt/<name>` with `verdict: "pass"`. No record stores the state; the artefact's existence and verdict are the state.
- **The assembly run** (reference implementation: `sworn assemble <release>`) brings up the stack from the release worktree, runs the release's deferred end-to-end set — no-mock, serially, with verified teardown — and emits `assembly-proof.json` (per-suite results, boundary/preflight observations, screenshot paths, and the authoritative verdict). Non-zero exit on any non-excepted failure. The record is structurally fail-closed on the two things a runner most easily drops silently: an unexplained non-pass result (a `fail`/`skip` must carry a disposition, and any excepted disposition must carry tracking — Rule 2), and an undeclared server teardown after the stack was brought up (Rule 11 guaranteed-restore).
- **The human walk comes after.** The touched journeys are re-walked against real infra **after the assembly run passes**, not merely "after all slices verify" — the machine half catches the wire-level seams (the CORS class) so the human walk spends its attention on journey semantics, not transport failures.
- **`/merge-release` gates on `assembled`** the way it gates on per-slice `verified`. Until the reference implementation ships, the gate is advisory (a missing `assembly-proof.json` is a surfaced warning, a failing one is a hard block); it flips to fail-closed when `sworn assemble` ships (baton#59 skew-window policy).

## Workflow

1. A maintainer runs journey elicitation against the project.
2. The model drafts candidate journeys from the project structure.
3. The human reviews, edits, and ratifies the artefact (`is_ratified=true`, `ratified_by`, `ratified_at`).
4. The journeys gate passes; the artefact is committed and maintained as the project evolves.
5. After all tracks merge to `release-wt/<name>`, the assembly run executes and emits a passing `assembly-proof.json` (the release is now `assembled`).
6. At release cutover, the journeys that the release touches are re-walked against real boundaries (no-mock) **after the assembly run passes**, and the walkthrough is human-attested before ship.

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
