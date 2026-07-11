---
title: 'Replan handoff: fold Baton v0.10.0 contract-edge vendoring into the 2026-06-28-driver-contract release'
description: 'Baton shipped v0.10.0 (contracts-v1, assembly-proof-v1, mock parity, assembly stage) mid-build. This is the release-specific replan routing for the in-flight driver-contract release: re-target S11 to v0.10.0 + vendor the two new schemas, reconcile W1 (already done by S14), and decide W3/W4 placement. Companion to the step-3 work list.'
---

# Replan handoff: Baton v0.10.0 contract edges → 2026-06-28-driver-contract

**For:** the live sworn planner (`/replan-release 2026-06-28-driver-contract`). Read this and
the step-3 work list (`docs/captures/2026-07-11-contract-edge-step3-handoff.md`) together — that
doc has the full W1–W4 detail and acceptance shapes; **this doc is the release-specific routing**
and supersedes the step-3 doc's generic "route via /replan-release" section.

## What changed upstream (why you're replanning)

Baton tagged **v0.10.0** on 2026-07-11 (`sawy3r/baton`, tag `v0.10.0`, commit `a5ab2aa`; GitHub
release published). It adds the contract-edge gates: `contracts-v1` schema + planner emission,
the Rule 10 **mock-parity** sub-rule, and the Rule 10 **assembly stage** + `assembly-proof-v1`
schema. All 13 schemas are hosted at `baton.sawy3r.net/schemas/`. Everything is
**additive-with-advisory** per the skew policy — nothing existing changed shape.

This lands **mid-build**: your release `2026-06-28-driver-contract` (target v0.1.0) already
contains a **T7-baton-revendor** track whose whole job is pinning vendored Baton to the current
upstream. That track was scoped to **v0.9.0** — now stale by one minor before it has started.

## Current release state (reconciled from `release-wt/2026-06-28-driver-contract`, 2026-07-11)

12 of 14 slices verified. Only T7's two slices remain, both `planned` (not started — safe to edit
per replan rules):

| Slice | Track | State |
|---|---|---|
| S01–S10, S13, **S14-blocked-terminal** | T1–T6, T8, T4 | **verified** |
| **S11-baton-revendor** | T7-baton-revendor | **planned** |
| **S12-record-migration** | T7-baton-revendor | **planned** |

Vendored Baton is currently **v0.6.3** (`internal/adopt/baton/VERSION`). Two embed roots:
`internal/baton/schemas/` and `internal/adopt/baton/`. Neither `contracts-v1.json` nor
`assembly-proof-v1.json` is vendored yet.

## The replan actions

### 1. Re-target S11-baton-revendor: v0.9.0 → v0.10.0 (mandatory, surgical)

S11 is `planned`, so this is a clean spec edit on `release-wt` — not a new slice. Revendoring to
v0.9.0 now would ship an **already-stale** vendor and immediately reopen the schema-skew scar
(baton#54/#55/#58: vendored schemas diverging from the binary's graded set under identical
`$id`). Change S11 so it:

- Pins the vendored `VERSION` to **v0.10.0** and refreshes `upstream-sha` / `upstream-digest`
  from the `v0.10.0` **tag** (RELEASING.md: vendor from a tag, never `main`).
- Adds **`contracts-v1.json`** and **`assembly-proof-v1.json`** to the vendored schema set in
  **both** embed roots (`internal/baton/schemas/` and `internal/adopt/baton/`), with an AC per
  schema asserting it byte-matches (or provably transforms from) the published `$id` content.
- Updates `user_outcome`, `acceptance_criteria`, and the S11 rationale to name v0.10.0 and the
  two new schemas. Re-run the Rule 8 DoR (trace + verify + validate) on the edited spec before it
  leaves `planned`.

This is the **W2 vendoring half** of the step-3 list, now homed in an existing slice.

### 2. Fold in the version handshake (`doctor --sync-baton`) — W2 part 2

Append to S11 (same "align to upstream" concern) or add **S15-baton-version-handshake** to T7 if
S11's surface gets too broad: `sworn doctor` declares which Baton schema versions this binary
grades, so the next protocol/runner skew is a visible warning, not a silent gap. Keep it in T7 —
it shares the vendoring touchpoints and must not collide with another track.

### 3. Leave S12-record-migration's scope alone — do NOT backfill contracts.json

S12 migrates existing `spec-v1`-era records (quadrant `chore`→`quick`, `epic`→`beast`,
`in_scope`/`out_of_scope` presence) across the integration branch. `contracts.json` is
**advisory and planner-emitted going forward** — it is not backfilled onto historical releases.
Explicitly note this in the replan so S12 does not scope-creep into "emit contracts.json for every
old release." S12's only v0.10.0 interaction: if its migration validates records against vendored
schemas, it must validate against the **S11-updated** vendor (T7 ordering already serialises
S11 → S12 within the track — keep it).

### 4. Reconcile W1 — it is already delivered; close sworn#88

**S14-blocked-terminal is `verified` (PASS)** and already implements the BLOCKED-terminal-for-the-
lane runner behaviour with an explicit blocked signal (`internal/run/slice.go`,
`internal/run/resolve.go`, `internal/driver/driver.go`). Do **not** create a duplicate slice.
Action: confirm S14's acceptance covers sworn#88's check (replay of the fired S05 journal verdict
sequence → **one dispatch, not three**); if it does, **close swornagent/sworn#88** citing S14 as
the delivering slice. If S14 is narrower than #88 (e.g. it handles the verifier-BLOCKED path but
not an implementer `blocked:true` return, or vice-versa), the gap is a small appended T4 follow-up
— but verify before assuming a gap.

### 5. Coach decision — do W3 (`lint contracts`) + W4 (`assemble`) join this release?

These are net-new gates, not vendoring. Two options:

- **(a) This release** — add **T9-lint-contracts** (`sworn lint contracts`) and **T10-assemble**
  (`sworn assemble`) as new tracks. Requires re-proving touchpoint-disjointness against T7 (they'll
  touch `internal/lint/` and a new `internal/assemble/` — likely disjoint from T7's vendor paths,
  but the matrix must show it). Grows a driver-contract release well beyond its theme.
- **(b) Follow-on release** *(recommended)* — the driver-contract release is thematically
  "driver contract + honest cost + blocked-terminal"; the vendoring (S11/S12) belongs here because
  T7 already owns it, but the *graders* are a coherent next release (e.g.
  `2026-07-12-contract-edge-gates`). W3 depends on W2's vendored schema either way; W4 has an
  external blocker (below), so it cannot fully land in a rushed fold.

Either way, **W4 (`assemble`) is baton-Ready** — `assembly-proof-v1` is published — but still
blocked on **fired#1168** (`derive_ports` must handle board.json-era releases without `index.md`).
Confirm where that fix lives before scheduling W4. Recommendation: take (b), and let this release
finish on the vendoring + handshake so it can merge.

## Routing mechanics

- `/replan-release 2026-06-28-driver-contract`. Commit all planning artefacts to
  **`release-wt/2026-06-28-driver-contract`** (never the integration branch) — the release
  worktree already exists at
  `$HOME/projects/sworn-worktrees/release-2026-06-28-driver-contract`.
- S11/S12 are `planned` (not started), so editing their specs is legal — no `start_commit`
  anchoring is broken. Re-run the touchpoint matrix for any added slice (handshake, or W3/W4 if
  the Coach takes option (a)).
- Step 6 forward-merges `release-wt` into the T7 track branch so the edited S11/S12 specs reach it
  before `/implement-slice` resumes there.
- The fired `2026-07-10-one-current-position` release folder is the acceptance corpus for the
  graders when they are built (reconstruct without S15/S17; assert each gate fires) — see step-3
  doc §"Validation corpus".

## Anchors

- Upstream release: `sawy3r/baton` tag **v0.10.0**, GitHub release, compare `v0.9.0...v0.10.0`
- Schemas: `baton.sawy3r.net/schemas/{contracts-v1,assembly-proof-v1}.json`
- Step-3 work list (W1–W4 detail + acceptance): `docs/captures/2026-07-11-contract-edge-step3-handoff.md`
- Epic: `sawy3r/baton#59`; Rec 4 issue (W1, close via S14): `swornagent/sworn#88`
- Registry + assembly exemplars: baton `baton/release-mode-template/{contracts,assembly-proof}.json`
