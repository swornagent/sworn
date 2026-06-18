---
title: '2026-06-16-fidelity-layer — release board'
description: 'Fidelity layer (Baton Rules 8/9/10): requirements fidelity, design fidelity, and customer-journey / system-acceptance validation, as protocol + native sworn enforcement. 16 slices across 4 tracks.'
release_worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer
release_worktree_branch: release-wt/2026-06-16-fidelity-layer
tracks:
  - id: T1-fidelity-core
    slices: [S01-rtm-spine, S02-ears-ac-format, S04-requirements-verify-gate, S05-requirements-validate-gate, S07-design-fit-gate, S11-journey-elicitation, S16-lint-rename]
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
| `T1-fidelity-core` | S01 → S02 → S04 → S05 → S07 → S11 → S16 | — | `track/2026-06-16-fidelity-layer/T1-fidelity-core` | planned |
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
| `S02-ears-ac-format` | T1 | EARS acceptance-criteria notation + validator (`sworn lint ac`) | verified | human | [spec](./S02-ears-ac-format/spec.md) | [proof](./S02-ears-ac-format/proof.md) |
| `S04-requirements-verify-gate` | T1 | 29148 quality-characteristic check, fresh-context, fail-closed (`sworn reqverify`) | verified | human | [spec](./S04-requirements-verify-gate/spec.md) | [proof](./S04-requirements-verify-gate/proof.md) |
| `S05-requirements-validate-gate` | T1 | Human-owned scenario pos/neg + benefit-hypothesis validation (`sworn reqvalidate`) | planned | human | [spec](./S05-requirements-validate-gate/spec.md) | — |
| `S07-design-fit-gate` | T1 | Stakes-calibrated human-owned design decision (`sworn designfit`) | planned | human | [spec](./S07-design-fit-gate/spec.md) | — |
| `S11-journey-elicitation` | T1 | AI-drafts/human-ratifies critical journeys into a durable artefact (`sworn journeys`) | planned | human | [spec](./S11-journey-elicitation/spec.md) | — |
| `S16-lint-rename` | T1 | Documentation sweep — rename `sworn ears`→`lint ac` / `sworn rtm`→`lint trace` throughout; restore S02 proof.md | planned | human | [spec](./S16-lint-rename/spec.md) | — |
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
- Verified (awaiting merge): 3
- Failed verification: 0
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 3 / In progress: 1 / Merged: 0

## Recent activity

### 2026-06-18 — S04-requirements-verify-gate: PASS (fifth verdict, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. 24 unit tests + 9 CLI integration tests green in fresh session. Injectable path exercises full reqverify flow (singular, ambiguous, incomplete breaches confirmed). FreshContext flag recorded in output. Slice state → `verified`. T1-fidelity-core now has 3/7 slices verified.

### 2026-06-18 — S04-requirements-verify-gate: FAIL (fourth verdict, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 3 + Gate 6 failures. Gate 3: spec "Required tests" demands "characteristic-breach detection over fixture ACs (non-singular, ambiguous, incomplete)" but only `singular` is tested in both `reqverify_test.go` and `reqverify_test.go` (CLI); no test exercises an `ambiguous` or `incomplete` breach through the model-client seam. Gate 6: proof.md AC 2 evidence misidentifies the test — claims `TestParseGrades_MixedPassFail` "validates that an `ambiguous` characteristic breach is correctly parsed" but the test fixture replies with `FAIL — singular`. Gates 1, 2, 4, 5 all PASS. Fix: (1) add `ambiguous` and `incomplete` fixture AC tests to `internal/reqverify/reqverify_test.go`; (2) update proof.md AC 2 evidence to name the correct test(s).

### 2026-06-18 — S04-requirements-verify-gate: FAIL (third verdict, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2 failure (×2). (1) `.gitignore` appears in the re-implementation diff (adds `cmd/sworn/docs/`) but is not listed as a planned touchpoint and is not explained in proof.md "Divergence from plan". (2) Four planned touchpoints (`internal/reqverify/reqverify.go`, `internal/reqverify/reqverify_test.go`, `cmd/sworn/main.go`, `internal/prompt/requirements-verifier.md`) are absent from the re-implementation diff; proof.md "Not delivered" addresses only `internal/adopt/baton/rules/08-requirements-fidelity.md` — the other four have no entry. Gates 1, 3, 4, 5, 6 all PASS. Fix: (1) add `.gitignore` to proof.md "Divergence from plan" with one sentence; (2) add explanation in "Divergence from plan" for the four files implemented in round 1 and not re-touched.

### 2026-06-18 — S04-requirements-verify-gate: FAIL (second verdict, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2 failure — planned touchpoint `internal/adopt/baton/rules/08-requirements-fidelity.md` not modified by S04 (file last touched by S01/S02 commits); proof.md "Divergence from plan" and "Not delivered" do not acknowledge or explain the omission. Gates 1, 3, 4, 5, 6 all PASS — injectable CLI tests pass, all AC evidence is real and verified, no deferrals. State remains `failed_verification`. Fix: add one line to proof.md "Divergence from plan" explaining the file was not modified because it already contained the verification description from S01/S02 work.

### 2026-06-18 — S04-requirements-verify-gate: FAIL

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 3 failure — CLI integration test does not exercise `reqverify.Run()` through the CLI boundary; stops at "no model configured" before the reqverify logic runs. Spec specifies "local (stubbed model client; no live key needed)" but CLI is not injectable. Gate 4 derivative: smoke step requires a live model key, contradicting the spec. State → `failed_verification`. Fix: make `cmdReqverify` injectable and add a CLI-level passing + failing test using a `fakeVerifier` stub.

### 2026-06-18 — S02-ears-ac-format: PASS

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. 20 unit tests + 6 integration tests green in fresh session. Both smoke steps confirmed live (pass case: 74 ACs, exit 0; fail case: named violation, exit 1). Slice state → `verified`. T1-fidelity-core now has 2/7 slices verified.

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
