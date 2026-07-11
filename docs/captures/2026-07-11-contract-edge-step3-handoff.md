---
title: 'Handoff: contract-edge gates, sworn side (step 3) — lint contracts, assemble, BLOCKED short-circuit, schema handshake'
description: 'Baton has ratified and shipped the contract-edge governance (contracts-v1, mock-parity sub-rule, assembly stage). This is the sworn work list, with acceptance shapes, sequencing, and the fired validation corpus. Route scope into the current release via /replan-release.'
---

# Handoff: contract-edge gates — sworn side (step 3)

**Audience:** the live sworn session (route scope through `/replan-release` for the in-flight
release — `2026-06-28-driver-contract` at time of writing; confirm via the board oracle, not this
doc). This handoff is self-contained; do not assume any other conversation context.

## Why this work exists (one paragraph)

A 17-slice fired release (2026-07-10, v0.6.0) ran under full Baton discipline — every slice
PASS under fresh-context adversarial verification — and every build halt was still a
**planner-owned cross-slice contract seam**: an unpinned request-body shape, a mock encoding the
spec author's own wrong assumption, and an If-Match header specced on both ends while the CORS
`AllowHeaders` between them was owned by nobody. With strong models the failure class has moved
up a level: gates catch bad *contracts* now, not bad *implementations*. Node-level gates grade
slices against specs; these failures live on **edges between specs**. Full retro:
`fired` repo, `docs/captures/2026-07-10-baton-sworn-edge-contracts-proposal.md`.

## What Baton has already ratified and shipped (steps 1–2, done)

Per the ordering ruling (Baton owns artefact shapes, sworn grades them), the targets are stable:

- **`contracts-v1.json`** — published and resolvable at
  `https://baton.sawy3r.net/schemas/contracts-v1.json`; source of truth
  `sawy3r/baton` `schemas/contracts-v1.json` (main, merged PR baton#60, commit `07c6585`).
  Shape: release-level `contracts.json` sibling to `board.json`; entries
  `{id: C-NN, kind, surface, shape, owner, consumers[], live_test, fixtures, edge_config, rationale}`.
  Kind enum (11): `http-endpoint | header | cookie | env-var | edge-config | schema-version |
  db-schema | storage-key | feature-flag | auth-scope | event`. Schema-level fail-closed
  conditions already carried by the record itself: `live_test` **required when `consumers` is
  non-empty**; `edge_config` (a sibling `C-NN` ref or literal `n/a: <reason>`) **required for
  `http-endpoint`/`header`/`cookie`**.
- **Planner emission** — `role-prompts/planner.md` Phase 3b step 3 now instructs planners to emit
  `docs/release/<name>/contracts.json`; Layer 4 has a wire-surface enumeration step. Consumer
  projects will start producing these records **now**.
- **Rule 10 sub-rule: mock parity** — a consumer slice may mock a *registered* boundary only with
  fixtures recorded by the owner's live contract test; escape hatch = at least one unmocked
  in-process round-trip; freshness invariant = fixtures newer than the owner's last
  production-code change to the surface. (`baton/customer-journey-validation.md`.)
- **Rule 10: assembly stage** — release chain `tracks-merged → assembled → journey-validated →
  merged`; `assembled` is **derived** (existence + passing verdict of
  `docs/release/<name>/assembly-proof.json` on `release-wt/<name>`), never stored; `/merge-release`
  gates on it (advisory while `sworn assemble` is unshipped; failing proof already hard-blocks).
- **Skew policy (binding)** — every new record is **optional-with-advisory** until the
  corresponding sworn gate ships, then flips required. The scar this manages: baton schemas
  v0.7.0 vs sworn binary v0.6.3 silently grading stale shapes (baton#54/#55/#58).

## The sworn work list

### W1 — `loop`: BLOCKED short-circuits the retry lane (sworn#88 — independent, do first)

Already tracked as **swornagent/sworn#88**. No new artefact shape; ship independently of
everything below.

- FAIL (verifier) → feed numbered violations to a retry implementer; consume retry budget
  (unchanged).
- BLOCKED (implementer **or** verifier) → terminate the lane immediately, zero further
  dispatches, route to `/replan-release` per handoff-directionality, surface the blocker
  verbatim in the runner's exit report.
- Implementer return schema gains explicit `blocked: true`, distinct from `completed: false`
  (retryable incompleteness) — the runner must not infer intent from prose.
- **Acceptance:** replay of the S05 journal verdict sequence (fired repo,
  `docs/release/2026-07-10-one-current-position/S05-section-owned-saves/journal.md`, a recorded
  triple-BLOCKED) through the loop → **one dispatch, not three**, blocker in the exit report.

### W2 — Vendor `contracts-v1` + version handshake (`doctor --sync-baton`)

Prerequisite for W3. `internal/baton/` already has the fetch/transform/validate machinery.

- Vendor `contracts-v1.json` into the vendored schema set, from the baton repo copy (decide and
  record the canonical-source direction — this is the baton#55 divergence class; do not fork the
  shape under the same `$id`).
- Extend `sworn doctor` (natural home: `--sync-baton`) to **declare which baton schema versions
  this binary grades**, so the next protocol/runner skew is a visible warning instead of a
  silent behaviour gap.
- **Acceptance:** doctor output names the graded schema set + versions; vendored contracts-v1
  byte-matches (or provably transforms from) the published `$id` content.

### W3 — `sworn lint contracts <release>` (the grading gate for Rec 1 + Rec 2)

Deterministic, fail-closed, following the existing lint/gate conventions. Semantics ratified in
the proposal (Rec 1) minus what the schema now enforces by itself:

- **Advisory window:** absence of `contracts.json` → warning, exit 0 (flips to required when
  this gate ships in a release — coordinate the flip with baton). Presence → full grading, fail
  closed.
- FAIL: record does not validate against vendored `contracts-v1`.
- FAIL: any `spec.json` whose in-scope/ACs reference a wire-level artefact with no registry
  entry. Heuristics from the proposal: header names, endpoint paths, env vars matching
  `[A-Z_]{4,}`, `schemaVersion`, storage keys.
- FAIL: a contract whose `live_test` does not resolve to a real test — same resolution logic as
  the trace gate's `test_refs`.
- FAIL: two owners for one `surface`; or an owner slice whose touchpoints cannot plausibly
  contain the surface (e.g. an `http-endpoint` owned by a slice with no server-side touchpoints).
- **Mock parity (Rec 2) checks:** when a contract names `fixtures` — FAIL if the fixture file
  does not exist or is older than the owner's last production-code commit touching the surface;
  FAIL if a consumer's tests mock the boundary without importing from the fixture path
  (grep-level is sufficient to start) and without an unmocked in-process round-trip.
- **Acceptance:** the validation corpus below, plus green on a release with a well-formed
  registry (`baton` repo `baton/release-mode-template/contracts.json` is a valid exemplar).

### W4 — `sworn assemble <release>` (last; two dependencies, one of them baton-side)

The machine half of the Rule 10 assembly stage: bring up the stack from the release worktree,
run the release's deferred end-to-end set — **no-mock, serially, with verified teardown** — and
emit `docs/release/<name>/assembly-proof.json`; exit non-zero on any non-excepted failure.
`/merge-release` (baton side, already landed) reads that artefact.

One dependency remains; surface it at replan:

1. **`assembly-proof-v1` — RATIFIED AND PUBLISHED (2026-07-11).** Resolvable at
   `https://baton.sawy3r.net/schemas/assembly-proof-v1.json`; source of truth `sawy3r/baton`
   `schemas/assembly-proof-v1.json` (main, commit `0a75b02`). `sworn assemble` emits this record;
   the derived `assembled` state is its existence + `verdict: "pass"`. Shape (grounded in the
   fired improvised run): `environment` (worktree_branch, services, and a **required** ss-verified
   `teardown` block — Rule 11), `suites[]` with per-test `outcome` + `disposition`
   (`product-regression` / `spec-defect` / `by-design-excepted` / `environment-blocked`) +
   `tracking`, `observations[]` (the boundary/preflight surface — the CORS class), `screenshots[]`,
   and the authoritative `verdict`. Structurally fail-closed: a non-pass result must carry a
   disposition; an excepted disposition must carry tracking (Rule 2). Vendor it in W2 alongside
   contracts-v1. Exemplar: baton `baton/release-mode-template/assembly-proof.json`. **W4 is now
   Ready on the baton side.**
2. **fired#1168** — `derive_ports` must handle board.json-era releases without `index.md`;
   the improvised assembly phase hit this. Verify where that fix actually lives (fired extension
   vs sworn) before scoping. This is the one remaining W4 blocker.

### Explicitly not in scope (from the proposal)

No change to effort/complexity tiering, no loosening of per-slice fresh-context verification,
no new agent roles — gates and artefacts only.

## Validation corpus (fired repo — use it as the acceptance harness)

Reconstruct `docs/release/2026-07-10-one-current-position/` WITHOUT S15/S17 (the two fix slices)
and assert each gate fires:

- (a) `lint contracts` on the S01/S02/S14 specs with no CP-PUT registry entry → FAIL (seam 1).
- (b) mock-parity check on S14's test file → FAIL (seam 2 — the mock does not load an
  owner-recorded fixture).
- (c) `assemble` on the pre-S17 tree → the CORS preflight failure surfaces (seam 3).
- (d) `loop` replay of S05's journal verdicts → one dispatch, not three (W1).

Evidence artefacts: the release folder (17 spec/proof/journal bundles), fired issues
#1167–#1171, fired capture `2026-07-10-one-cp-assembly-verification.md`.

## Sequencing

1. **W1 now** (sworn#88) — independent, no baton dependency, immediate token savings.
2. **W2 → W3** as one arc (W3 grades against W2's vendored schema).
3. **W4 last** — blocked on `assembly-proof-v1` ratification (baton) + the #1168 verification.

## Routing

This is added scope on an in-flight release: enter via **`/replan-release`** (new track(s) or
appended slices; touchpoint-disjointness against the in-flight tracks must be re-proven), with
specs meeting Rule 8 DoR before any slice starts. Whether this joins the current release or
becomes its own release is the planner + Coach's call — W1 is small enough to append; W2–W4 may
deserve their own release.

## Anchors

- Epic + ordering ruling + skew policy: `sawy3r/baton#59`
- Ratified governance (schema, rule text, gate wording): baton PR #60, merged `41c5797`
- Rec 4 issue: `swornagent/sworn#88`
- Published schema: `https://baton.sawy3r.net/schemas/contracts-v1.json`
- Registry exemplar: baton repo `baton/release-mode-template/contracts.json`
- Full retro: fired `docs/captures/2026-07-10-baton-sworn-edge-contracts-proposal.md`
