---
title: '2026-06-16-fidelity-layer — release board'
description: 'Fidelity layer (Baton Rules 8/9/10): requirements fidelity, design fidelity, and customer-journey / system-acceptance validation, as protocol + native sworn enforcement. 15 slices across 4 tracks.'
release_worktree_path: # <set by first /implement-slice in the release>
release_worktree_branch: release-wt/2026-06-16-fidelity-layer
tracks:
  - id: T1-fidelity-core
    slices: [S01-rtm-spine, S02-ears-ac-format, S04-requirements-verify-gate, S05-requirements-validate-gate, S07-design-fit-gate, S11-journey-elicitation]
    depends_on: null
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core
    worktree_branch: track/2026-06-16-fidelity-layer/T1-fidelity-core
    state: in_progress
  - id: T2-delivery-cutover
    slices: [S06-definition-of-ready, S10-no-mock-boundary, S12-journey-impact-analysis, S13-walkthrough-attestation, S14-journey-regression-suite]
    depends_on: T1-fidelity-core
    worktree_path:
    worktree_branch: track/2026-06-16-fidelity-layer/T2-delivery-cutover
    state: planned
  - id: T3-leaf-gates
    slices: [S03-spec-quality-firstpass, S08-design-system-input, S09-design-conformance-audit]
    depends_on: T1-fidelity-core
    worktree_path:
    worktree_branch: track/2026-06-16-fidelity-layer/T3-leaf-gates
    state: planned
  - id: T4-evidence-surface
    slices: [S15-sworn-top-evidence]
    depends_on: T1-fidelity-core
    worktree_path:
    worktree_branch: track/2026-06-16-fidelity-layer/T4-evidence-surface
    state: planned
---

# Release Board: `2026-06-16-fidelity-layer`

> Frontmatter is the machine-readable registry; the tables below mirror it. Keep them in sync.
> Parallelism model: track mode. T2/T3/T4 each `depends_on` T1 and are mutually touchpoint-
> disjoint, so they run in parallel **after** T1 merges.

## Release summary

- **Goal**: the fidelity layer — Baton Rules 8 (requirements), 9 (design), 10 (customer-journey
  / system-acceptance) — as protocol + native sworn enforcement; see `intake.md`.
- **Target version / integration branch**: `release/v0.1.0` (the accumulating pre-1.0 milestone)
- **Started**: 2026-06-16
- **Target ship**: uncommitted
- **Intake**: `intake.md`
- **Stakeholder**: Brad (maintainer)
- **Tracking issue**: [#4](https://github.com/swornagent/sworn/issues/4) — Epic: fidelity-layer (Baton Rules 8/9/10)

## Tracks

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
| `T1-fidelity-core` | S01 → S02 → S04 → S05 → S07 → S11 | — | `track/2026-06-16-fidelity-layer/T1-fidelity-core` | planned |
| `T2-delivery-cutover` | S06 → S10 → S12 → S13 → S14 | T1 | `track/2026-06-16-fidelity-layer/T2-delivery-cutover` | planned |
| `T3-leaf-gates` | S03 → S08 → S09 | T1 | `track/2026-06-16-fidelity-layer/T3-leaf-gates` | planned |
| `T4-evidence-surface` | S15 | T1 | `track/2026-06-16-fidelity-layer/T4-evidence-surface` | planned |

### Touchpoint matrix

> T1 owns the shared core; T2/T3/T4 must be **mutually disjoint** (each `depends_on` T1, so any
> file they share *with T1* is serialised by the dependency edge). No file carries `✓` in two
> columns of the parallel set {T2, T3, T4}.

| File / surface | T1 | T2 | T3 | T4 |
|---|---|---|---|---|
| `internal/prompt/planner.md` | ✓ | | (T1 via dep) | |
| `internal/prompt/captain.md` | ✓ | | | |
| `internal/prompt/implementer.md` | | ✓ | | |
| `internal/prompt/requirements-verifier.md` (new) | ✓ | | | |
| `internal/state/state.go` | ✓ | (T1 via dep) | | |
| `internal/board/index.go` | ✓ | | | (read-only) |
| `internal/rtm/` (new) | ✓ | | | |
| `internal/ears/` (new) | ✓ | | | |
| `internal/reqverify/` (new) | ✓ | | | |
| `internal/reqvalidate/` (new) | ✓ | | | |
| `internal/designfit/` (new) | ✓ | | | |
| `internal/journey/` (new) | ✓ | (T1 via dep) | | (read-only) |
| `internal/implement/` | | ✓ | | |
| `internal/verify/` | | ✓ | | |
| `cmd/sworn/ship.go` (new) | | ✓ | | |
| `internal/specquality/` (new) | | | ✓ | |
| `internal/designaudit/` (new) | | | ✓ | |
| `internal/config/` | | | ✓ | |
| `bin/*.sh` (new gate scripts) | | | ✓ | |
| `cmd/sworn/top.go` (new) | | | | ✓ |
| `internal/adopt/baton/rules/08-requirements-fidelity.md` (new) | ✓ | (T1 via dep) | (T1 via dep) | |
| `internal/adopt/baton/rules/09-design-fidelity.md` (new) | ✓ | | (T1 via dep) | |
| `internal/adopt/baton/rules/10-customer-journey-validation.md` (new) | ✓ | (T1 via dep) | | |

**Convention (recorded in intake):** `cmd/sworn/main.go` carries an **additive command switch**;
each command-adding slice (S01 `rtm`, S02 `ears`, S03 `specquality`, S04 `reqverify`, S05
`reqvalidate`, S07 `designfit`, S09 `designaudit`, S11 `journeys`, S13 `ship`, S15 `top`)
contributes a distinct `case`. Per the prior release's parallel command registration, this is
**not** treated as a touchpoint collision. Command *implementations* live in their own
`cmd/sworn/<cmd>.go` files (disjoint).

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| `S01-rtm-spine` | T1 | 2-D requirements traceability matrix, threaded through artefacts, fail-closed (`sworn lint trace`) | verified | human | [spec](./S01-rtm-spine/spec.md) | [proof](./S01-rtm-spine/proof.md) |
| `S02-ears-ac-format` | T1 | EARS acceptance-criteria notation + validator (`sworn lint ac`) | failed_verification | human | [spec](./S02-ears-ac-format/spec.md) | [proof](./S02-ears-ac-format/proof.md) |
| `S04-requirements-verify-gate` | T1 | 29148 quality-characteristic check, fresh-context, fail-closed (`sworn reqverify`) | planned | human | [spec](./S04-requirements-verify-gate/spec.md) | — |
| `S05-requirements-validate-gate` | T1 | Human-owned scenario pos/neg + benefit-hypothesis validation (`sworn reqvalidate`) | planned | human | [spec](./S05-requirements-validate-gate/spec.md) | — |
| `S07-design-fit-gate` | T1 | Stakes-calibrated human-owned design decision (`sworn designfit`) | planned | human | [spec](./S07-design-fit-gate/spec.md) | — |
| `S11-journey-elicitation` | T1 | AI-drafts/human-ratifies critical journeys into a durable artefact (`sworn journeys`) | planned | human | [spec](./S11-journey-elicitation/spec.md) | — |
| `S06-definition-of-ready` | T2 | `planned→in_progress` gated on verified+validated+traced | planned | human | [spec](./S06-definition-of-ready/spec.md) | — |
| `S10-no-mock-boundary` | T2 | Fail-closed on environment; undeclared validated-boundary mock fails | planned | human | [spec](./S10-no-mock-boundary/spec.md) | — |
| `S12-journey-impact-analysis` | T2 | Per-release touched-journey set = validation scope (`sworn journeys --impact`) | planned | human | [spec](./S12-journey-impact-analysis/spec.md) | — |
| `S13-walkthrough-attestation` | T2 | `sworn ship` blocks →shipped without passing human journey walkthroughs | planned | human | [spec](./S13-walkthrough-attestation/spec.md) | — |
| `S14-journey-regression-suite` | T2 | Walked journeys accrete into automated regression tests (`sworn journeys --regen`) | planned | human | [spec](./S14-journey-regression-suite/spec.md) | — |
| `S03-spec-quality-firstpass` | T3 | Deterministic pre-code soundness + completeness from acceptance examples (`sworn specquality`) | planned | human | [spec](./S03-spec-quality-firstpass/spec.md) | — |
| `S08-design-system-input` | T3 | Design system (tokens + component library) as first-class project input | planned | human | [spec](./S08-design-system-input/spec.md) | — |
| `S09-design-conformance-audit` | T3 | Deterministic drift first-pass + human cohesion verdict (`sworn designaudit`) | planned | human | [spec](./S09-design-conformance-audit/spec.md) | — |
| `S15-sworn-top-evidence` | T4 | Read-only journey-validation green-board / kill-list (`sworn top`) | planned | human | [spec](./S15-sworn-top-evidence/spec.md) | — |

### State legend

| State | Meaning | Who can move out of it |
|---|---|---|
| `planned` | Spec written, awaiting implementation | Implementer |
| `in_progress` | Implementer session active | Implementer |
| `implemented` | Implementer claims done; awaiting fresh-context verification | Verifier |
| `verified` | Fresh-context verifier returned PASS | Human (`/merge-track`) |
| `failed_verification` | Verifier returned FAIL; fix and re-submit | Implementer |
| `deferred` | Slice carved out per Rule 2; not in this release | Human |
| `shipped` | Slice is live in production | — (terminal) |

## Aggregate state

- Planned: 13
- In progress: 0
- Implemented (awaiting verification): 0
- Verified (awaiting merge): 1
- Failed verification: 1
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 3 / In progress: 1 / Merged: 0

## Recent activity

### 2026-06-18 (round 2) — S02-ears-ac-format verifier verdict: FAIL

- **Actor**: verifier (fresh-context)
- **Note**: Gate 2 violations (5) — all are proof.md documentation gaps introduced by refactor commit `6518f3b` (renamed `sworn ears`→`sworn lint ac`, `sworn rtm`→`sworn lint trace`). Proof.md "Files changed" was captured before the refactor and omits `cmd/sworn/rtm.go` (deleted S01 file), `cmd/sworn/lint_trace_test.go` (renamed from S01's `rtm_test.go`), and three S01-rtm-spine doc files. "Divergence from plan" does not explain the planned `cmd/sworn/ears.go` → actual `cmd/sworn/lint.go` substitution, the S01 file deletions, or the S01 doc modifications. Gates 1/3/4/5/6 all pass (entry point wired, 26 tests pass live, smoke artefact present, no dark-code markers, all 4 ACs delivered with evidence). Fix is proof.md update only — no code changes needed. Slice stays `failed_verification`. Implementer should re-open `/implement-slice S02-ears-ac-format 2026-06-16-fidelity-layer`.

### 2026-06-18 — S02-ears-ac-format verifier verdict: FAIL

- **Actor**: verifier (fresh-context)
- **Note**: Gate 2 violation — `cmd/sworn/ears_test.go` is changed but not listed in planned
  touchpoints and not explained in proof.md "Divergence from plan". All other gates passed (entry
  point wired, all 26 tests pass live, reachability artefact verified, no silent deferrals, all
  four ACs delivered with evidence). Fix is proof.md update only (add one sentence to
  "Divergence from plan"). Slice moves to `failed_verification`. Implementer should re-open
  `/implement-slice S02-ears-ac-format 2026-06-16-fidelity-layer`.

### 2026-06-17 — S01-rtm-spine verifier verdict: PASS

- **Actor**: verifier (fresh-context)
- **Note**: All six gates passed. `sworn lint trace` entry point wired and reachable; all 9 planned
  touchpoints changed; 13 unit + 5 integration tests pass (integration tests drive `cmdRtm`
  directly — Rule 1 satisfied); reachability artefact is the integration test suite + live smoke
  run; no silent deferrals; all six ACs verified with evidence. Slice moves to `verified`.
  Next step: `/implement-slice S02-ears-ac-format 2026-06-16-fidelity-layer` in a fresh session.

### 2026-06-18 — S01-rtm-spine verifier verdict: FAIL (second round)

- **Actor**: verifier (fresh-context)
- **Note**: Gate 2 violation — `start_commit` in `status.json` is set to `925cb07` (the
  re-implementation restart commit, after the actual implementation commit `67f287b`). Live diff
  `git diff --name-only 925cb07..HEAD` shows only 4 docs files; all 8 planned touchpoints are
  absent. `proof.md` "Files changed" silently uses `release-wt` as base (not `start_commit`).
  Fix: set `start_commit` to `8767fc7` (original start-implementation commit), regenerate
  proof.md "Files changed", update "Divergence from plan" to acknowledge bookkeeping commits in
  scope. All tests pass; FAIL is metadata-only. Implementer should re-open
  `/implement-slice S01-rtm-spine 2026-06-16-fidelity-layer`.

### 2026-06-17 — S01-rtm-spine verifier verdict: FAIL

- **Actor**: verifier (fresh-context)
- **Note**: Gate 2 violation — `proof.md` Divergence section does not explain functional changes
  to `internal/adopt/adopt.go` (added Rule 8 to embed/Materialise) or `internal/adopt/baton/README.md`
  (added Rule 8 documentation). Fix is proof.md update only; no code changes needed. Slice moves
  to `failed_verification`. Implementer should re-open `/implement-slice S01-rtm-spine 2026-06-16-fidelity-layer`.

### 2026-06-16 — release planned

- **Actor**: planner (human + Claude)
- **Note**: 15 slices across 4 tracks specced to `planned`. T1 fidelity-core; T2/T3/T4
  `depends_on` T1 and run in parallel after it. Handed off for implementation.

## Decisions deferred (Rule 2)

- **Track C provisional schema** (S11/S12/S13/S14): journey-artefact field detail is refined via
  `/replan-release` as the live journey-validation hand-run delivers evidence. **Why**: the
  hand-run is the source of truth for the journey schema. **Tracking**: intake "Open questions";
  refined post-hand-run. **Acknowledged**: 2026-06-16.
- **S14 scaffold-not-complete-oracle**: sworn emits a structured regression scaffold + coverage
  check per journey, not a complete journey oracle. **Why**: a complete oracle is project-
  specific E2E work. **Tracking**: consuming project's E2E backlog. **Acknowledged**: 2026-06-16.

## Cross-slice / cross-track notes

- **Keystone first.** S01 (RTM spine) writes the shared native core (`state` + `board`) and must
  land first; T2/T3/T4 `depends_on` T1 for that reason.
- **Rule docs created in T1, extended downstream via depends_on.** `08`/`09`/`10` are created in
  T1 (S01/S07/S11) and extended by their owning lane after T1 merges; no parallel-set collision.
- **`internal/adopt/baton/VERSION`** bumps once per new rule (S01 → Rule 8, S07 → Rule 9, S11 →
  Rule 10). All on T1; serialised within the track.
- **S15 functional sequencing.** `sworn top` renders S13's attestations, so it is most useful
  after S13 lands; it is only *touchpoint*-gated on T1 and renders an empty state until S13 is
  live. Prefer scheduling T4 after T2's S13, though it is not touchpoint-blocked.
- **Native/protocol composition.** Standalone command verbs are the primitive; the autonomous
  path composes them (S06's Definition of Ready invokes the S01/S04/S05 gates at the
  `planned→in_progress` transition).
