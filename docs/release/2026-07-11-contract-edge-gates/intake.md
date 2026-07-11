---
title: 'Release Intake: 2026-07-11-contract-edge-gates'
description: 'The contract-edge grading gates — sworn lint contracts (W3) + sworn assemble (W4). Grades the Baton v0.10.0 contract-edge artefacts the driver-contract release vendored.'
---

# Release Intake: `2026-07-11-contract-edge-gates`

## Release goal

sworn gains the two **grading gates** for Baton's contract-edge governance: `sworn lint contracts <release>` (deterministic, fail-closed grading of a release's `contracts.json` registry + the mock-parity sub-rule) and `sworn assemble <release>` (the machine half of the Rule 10 assembly stage — brings up the assembled release, runs its deferred end-to-end set no-mock with verified teardown, emits `assembly-proof.json`). Together they catch the failure class that strong-model per-slice verification structurally cannot: **cross-slice wire seams** (an unpinned request-body shape, a mock encoding the spec author's own wrong assumption, a CORS `AllowHeaders` owned by nobody). "Shipped" = both commands exist, fail closed, and fire correctly on the fired 2026-07-10 validation corpus; the Baton schemas they grade (`contracts-v1`, `assembly-proof-v1`) are already vendored advisory (driver-contract release, v0.10.0), and each flips required when its grader ships here.

## Source of truth

- **Human stakeholder**: Brad (Coach)
- **Origin handoffs**: `docs/captures/2026-07-11-contract-edge-step3-handoff.md` (W1-W4 work list + acceptance shapes + validation corpus), `docs/captures/2026-07-11-replan-driver-contract-contract-edges.md` (routing: W2 vendoring homed in the driver-contract release; W3/W4 = this follow-on)
- **Baton epic + ordering ruling**: `sawy3r/baton#59`; ratified governance PR baton#60 (v0.10.0)
- **Predecessor**: `2026-06-28-driver-contract` (merged to release/v0.1.0 @ade1268) — vendored contracts-v1 + assembly-proof-v1 (advisory), delivered W1 (sworn#88) and W2 (S11 vendoring + S15 doctor handshake)

## Needs

- **N-01 (W3 — `sworn lint contracts <release>`)**: a new subcommand on the existing `sworn lint` surface (siblings: ac/trace/deps/touchpoints/coverage/design/mock) that grades a release's `contracts.json`, fail-closed. FAIL conditions (proposal Rec 1, minus what the schema self-enforces): record does not validate against vendored `contracts-v1`; a spec.json in-scope/AC references a wire-level artefact (header name, endpoint path, env var `[A-Z_]{4,}`, `schemaVersion`, storage key) with no registry entry; a contract's `live_test` does not resolve to a real test (same resolution as the trace gate's `test_refs`); two owners for one `surface`, or an owner slice whose touchpoints can't plausibly contain the surface. **Mock-parity (Rec 2)**: when a contract names `fixtures`, FAIL if the fixture file is missing or older than the owner's last production-code commit touching the surface; FAIL if a consumer's tests mock the boundary without importing the fixture path AND without an unmocked in-process round-trip. **Advisory window**: absent `contracts.json` → warn, exit 0 (flips required when this gate ships, coordinated with baton). Depends on vendored contracts-v1 (LANDED in v0.1.0).
- **N-02 (W4 — `sworn assemble <release>`)**: a new `internal/assemble` package + `sworn assemble` command — bring up the stack from the release worktree, run the release's deferred end-to-end set **no-mock, serially, with verified teardown**, emit `docs/release/<name>/assembly-proof.json` (validated against the vendored `assembly-proof-v1`); exit non-zero on any non-excepted failure. `/merge-release` (baton side, landed) reads that artefact. Depends on vendored assembly-proof-v1 (LANDED in v0.1.0) AND on **fired#1168** (`derive_ports` must handle board.json-era releases without `index.md`) — status to verify at planning; where the fix lives (fired extension vs sworn) determines W4 scope.

## Constraints and non-negotiables

- **Gates and artefacts only** (proposal boundary): NO change to effort/complexity tiering, NO loosening of per-slice fresh-context verification, NO new agent roles.
- Public-safe repo; single Go binary; minimal justified deps (stdlib preferred).
- Fail closed: exit 0 only on PASS. Follow the existing `internal/lint` / gate conventions (W3 is a subcommand in an established package).
- Skew policy (binding, baton#59): every new gate is advisory-with-warning until it ships, then flips required — coordinate the flip with baton.
- Baton owns artefact shapes; sworn grades them. Do NOT invent or fork a schema under an existing `$id` (the baton#54/#55/#58 divergence class).

## Validation corpus (the acceptance harness)

The fired `docs/release/2026-07-10-one-current-position/` release folder, reconstructed WITHOUT its two fix slices (S15/S17), is the acceptance harness — assert each gate fires:
- (a) `lint contracts` on the S01/S02/S14 specs with no CP-PUT registry entry → FAIL (seam 1).
- (b) mock-parity on S14's test file → FAIL (seam 2 — mock loads no owner fixture).
- (c) `assemble` on the pre-S17 tree → the CORS preflight failure surfaces (seam 3).
- (d) the loop BLOCKED-replay is W1, already delivered by S14 in the predecessor release — not re-scoped here.

## Open questions (Phase 2 decisions)

- **A-01**: W3 decomposition — one slice (`lint contracts` incl. mock-parity) or split registry-grading from mock-parity?
- **A-02**: W4 fired#1168 handling — block W4 until the derive_ports fix is confirmed/landed, or scope W4 with an explicit dependency and a narrower first cut?
- **A-03**: track grouping — W3 and W4 as separate parallel tracks (disjoint: internal/lint vs internal/assemble)?
- **A-04**: target_version — continue v0.1.0 (unshipped) or bump to v0.2.0 for the new gate surface?

## Decisions made during planning

**2026-07-11 (Brad, Coach):**
- **A-01 — W3 decomposition = TWO slices.** S01-lint-contracts-registry (schema-validate + wire-ref completeness + live_test resolution + ownership sanity) and S02-lint-contracts-mock-parity (fixture freshness + consumer-mock-import). Mock-parity has distinct git-timestamp + mock-detection logic and its own failure mode; splitting keeps each within the file ceiling and independently verifiable.
- **A-02 — W4 fired#1168 = scope with explicit dependency, verify at start.** S03-assemble-command is scoped normally; fired#1168 (derive_ports without index.md) is a start-of-implementation verification item and R-01. If unresolved, the board.json-era-without-index.md path is a declared Rule 2 deferral in the assembly proof — the common case (index.md present) ships regardless.
- **A-03 — Track grouping = TWO parallel tracks.** T1-lint-contracts (S01, S02) and T2-assemble (S03), disjoint touchpoints, no depends_on edge — run in parallel.
- **A-04 — target_version = continue v0.1.0.** Same integration branch (release/v0.1.0, not yet prod-deployed, can still grow); graders land alongside the contracts they grade. Bump to v0.2.0 at a real prod-cut boundary later.

## Track plan + touchpoint matrix (Phase 3b)

Two tracks, no `depends_on` edge — parallel-safe because their touchpoints are disjoint:

| File / area | T1-lint-contracts (S01, S02) | T2-assemble (S03) |
|---|---|---|
| `cmd/sworn/lint.go` | ✅ (contracts subcommand) | — |
| `internal/lint/contracts.go` (+tests, testdata) | ✅ | — |
| `cmd/sworn/assemble.go` | — | ✅ (new command) |
| `internal/assemble/` (new pkg, +tests, testdata) | — | ✅ |

Both tracks add to `cmd/sworn/` but in **different files** (`lint.go` vs a new `assemble.go`) — no shared-file collision. `internal/lint` and `internal/assemble` are disjoint packages. Matrix proves T1 ∥ T2 safe.

Within T1, S01 → S02 is serial (S02 extends S01's `internal/lint/contracts.go`). T2's S03 is a single slice (may split at design_review if the file estimate exceeds the ceiling — flagged in its effort rationale).

## Screenshots / references

_(none yet)_
