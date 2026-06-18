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
    state: merged
  - id: T2-delivery-cutover
    slices: [S06-definition-of-ready, S10-no-mock-boundary, S12-journey-impact-analysis, S13-walkthrough-attestation, S14-journey-regression-suite]
    depends_on: T1-fidelity-core
    worktree_path: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T2-delivery-cutover
    worktree_branch: track/2026-06-16-fidelity-layer/T2-delivery-cutover
    state: in_progress
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
| `T1-fidelity-core` | S01 → S02 → S04 → S05 → S07 → S11 → S16 | — | `track/2026-06-16-fidelity-layer/T1-fidelity-core` | merged |
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
| `S05-requirements-validate-gate` | T1 | Human-owned scenario pos/neg + benefit-hypothesis validation (`sworn reqvalidate`) | verified | human | [spec](./S05-requirements-validate-gate/spec.md) | [proof](./S05-requirements-validate-gate/proof.md) |
| `S07-design-fit-gate` | T1 | Stakes-calibrated human-owned design decision (`sworn designfit`) | verified | human | [spec](./S07-design-fit-gate/spec.md) | [proof](./S07-design-fit-gate/proof.md) |
| `S11-journey-elicitation` | T1 | AI-drafts/human-ratifies critical journeys into a durable artefact (`sworn journeys`) | verified | verifier | [spec](./S11-journey-elicitation/spec.md) | [proof](./S11-journey-elicitation/proof.md) |
| `S16-lint-rename` | T1 | Documentation sweep — adopt `sworn lint ac` / `sworn lint trace` canonical names throughout release docs; restore S02 proof.md | verified | human | [spec](./S16-lint-rename/spec.md) | [proof](./S16-lint-rename/proof.md) |
| `S06-definition-of-ready` | T2 | `planned→in_progress` gated on verified+validated+traced | planned | human | [spec](./S06-definition-of-ready/spec.md) | — |
| `S10-no-mock-boundary` | T2 | Fail-closed on environment; undeclared validated-boundary mock fails | planned | human | [spec](./S10-no-mock-boundary/spec.md) | — |
| `S12-journey-impact-analysis` | T2 | Per-release touched-journey set = validation scope (`sworn journeys --impact`) | planned | human | [spec](./S12-journey-impact-analysis/spec.md) | — |
| `S13-walkthrough-attestation` | T2 | `sworn ship` blocks →shipped without passing human journey walkthroughs | planned | human | [spec](./S13-walkthrough-attestation/spec.md) | — |
| `S14-journey-regression-suite` | T2 | Walked journeys accrete into automated regression tests (`sworn journeys --regen`) | planned | human | [spec](./S14-journey-regression-suite/spec.md) | — |
| `S03-spec-quality-firstpass` | T3 | Deterministic pre-code soundness + completeness from acceptance examples (`sworn specquality`) | planned | human | [spec](./S03-spec-quality-firstpass/spec.md) | — |
| `S08-design-system-input` | T3 | Design system (tokens + component library) as first-class project input | planned | human | [spec](./S08-design-system-input/spec.md) | — |
| `S09-design-conformance-audit` | T3 | Deterministic drift first-pass + human cohesion verdict (`sworn designaudit`) | planned | human | [spec](./S09-design-conformance-audit/spec.md) | — |
| `S15-sworn-top-evidence` | T4 | Read-only journey-validation green-board / kill-list (`sworn top`) | failed_verification | agent | [spec](./S15-sworn-top-evidence/spec.md) | [proof](./S15-sworn-top-evidence/proof.md) |

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

- Planned: 8
- In progress: 0
- Implemented (awaiting verification): 0
- Verified (awaiting merge): 7 (S01, S02, S04, S05, S07, S11, S16)
- Failed verification: 1 (S15)
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 3 / In progress: 0 / Merged: 1 (T1: merged at b8521f8)

## Recent activity

### 2026-06-18 — S15-sworn-top-evidence: FAIL (round 1, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Two violations. (1) **Gate 2**: `start_commit` (`a58733d`) is the implementation commit itself; `git diff --name-only a58733d..HEAD` returns only doc files — no planned touchpoints visible per protocol. proof.md "Not delivered" incorrectly says "None." Fix: set `start_commit` to `e3b0ec2`. (2) **Gate 3**: All 7 tests call `renderEvidenceSurface` directly, bypassing `cmdTop`; spec requires "Rule 1 via the command entry point." Fix: add test calling `cmdTop([]string{...})`. Implementation is functionally correct — both violations are protocol/test-layer. Slice state → `failed_verification`.

### 2026-06-18 — track `T1-fidelity-core` merged to release-wt (commit b8521f8)

- **Actor**: track integrator (/merge-track)
- **Note**: 7 verified slices merged: S01-rtm-spine, S02-ears-ac-format, S04-requirements-verify-gate, S05-requirements-validate-gate, S07-design-fit-gate, S11-journey-elicitation, S16-lint-rename. Forward-merged release-wt into track first (1 sibling commit; sync commit 0d93c46); all track tests re-ran green on merged base. Track state → merged.

### 2026-06-18 — S16-lint-rename: PASS (round 3, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. `TestLintAC` (6 tests) and `TestLintTrace` (5 tests) pass fresh from worktree. `sworn lint ac 2026-06-16-fidelity-layer` exits 0 (74 ACs, 0 violations) confirmed from worktree binary. Grep gate clean (zero stale `sworn ears` / `sworn rtm` references outside S16's own artefacts carve-out). No silent deferrals. All 4 ACs satisfied with verifiable evidence. T1-fidelity-core now has 7/7 slices verified. Next: `/merge-track T1-fidelity-core 2026-06-16-fidelity-layer`.

### 2026-06-18 — S16-lint-rename: BLOCKED (round 2, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gates 1–5 all PASS (TestLintAC and TestLintTrace pass, `sworn lint ac 2026-06-16-fidelity-layer` exits 0, grep gate clean, no silent deferrals). Gate 6 BLOCKED on AC N-S16-03 spec defect: the AC requires `S02-ears-ac-format` in `implemented` state, but S02 is in `verified` state — this transition happened legitimately after S16's round 1 FAIL, and rolling it back would violate the state machine. Cannot be fixed by the implementer; requires planner to amend AC N-S16-03 to say "in `implemented` or `verified` state." Also found two implementer-fixable issues (proof "Files changed" used `git diff --name-only HEAD` instead of `git diff --name-only b820a183`, showing 5 of 11 files; three unplanned changed files not explained in Divergence). Board row corrected from stale `failed_verification` → `implemented`. Next step: `/replan-release 2026-06-16-fidelity-layer` to ratify the AC amendment.

### 2026-06-18 — S02-ears-ac-format: PASS (round 4, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Re-verified after commit `95cdb86` reset status.json from `verified` to `implemented`/`pending` during S16's documentation sweep. All six gates passed on current HEAD `6280030`. 20 unit tests + 6 integration tests green fresh (-count=1). Live binary confirmed: pass case (74 ACs, exit 0) and fail case (named violation, exit 1). All 4 ACs delivered with verifiable evidence. Slice state → `verified`. T1-fidelity-core now has 6/7 slices verified.

### 2026-06-18 — S16-lint-rename: FAIL (round 1, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Three violations. (1) **AC N-S16-01**: grep gate produces 8 matches in S16's own artefacts (spec.md ×2, journal.md ×1, proof.md ×5); AC requires zero matches outside `docs/captures/`; proof misstates "only in spec.md". (2) **AC N-S16-03**: S02/proof.md "Files changed" omits `docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json`, which IS in `git diff --name-only cd462364..HEAD` (60 files total; proof lists 57). (3) **AC N-S16-04**: S01-rtm-spine/status.json `actual_files` still contains `cmd/sworn/rtm.go` (line 31) and `cmd/sworn/rtm_test.go` (line 32); AC requires all occurrences in `planned_files` or `actual_files` to be replaced; proof falsely claims no remaining occurrences. Gates 1–5 all PASS: `TestLintAC` and `TestLintTrace` pass, `sworn lint ac 2026-06-16-fidelity-layer` exits 0, no silent deferrals. Board updated: S02 moved `verified → implemented` (S16 reset it; board was stale). Slice state → `failed_verification`. Next: `/implement-slice S16-lint-rename 2026-06-16-fidelity-layer` in a fresh session to address the 3 numbered violations.

### 2026-06-22 — S11-journey-elicitation: PASS (round 3, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. 14 unit tests + 8 CLI integration tests green in fresh session. Live binary confirmed smoke step: fail-closed on missing/unratified artefact, exits 0 with listed journeys after ratification. No deferral markers in production files. All 5 ACs have verifiable evidence. Verified at commit `1143afb`. Slice state → `verified`. T1-fidelity-core now has 6/7 slices verified. Next: `/implement-slice S16-lint-rename 2026-06-16-fidelity-layer`.

### 2026-06-20 — S11-journey-elicitation: FAIL (round 2, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2. `cmd/sworn/journeys_test.go` appears in `git diff --name-only 0535a74..HEAD` but is not mentioned in proof.md "Divergence from plan". The file is the integration test companion to `cmd/sworn/journeys.go` and fulfills the spec's required Rule 1 integration test. Fix: add one sentence to proof.md "Divergence from plan" acknowledging this file. All six other gates pass — all 14 journey-package tests and 9 CLI integration tests green in fresh session; all 5 ACs verifiable; no deferrals. Slice state → `failed_verification`.

### 2026-06-20 — S11-journey-elicitation: FAIL (round 1, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 3 (primary): `internal/journey/journey.go:274` has the `DraftTemplate` function declaration fused into the tail of a comment line; the function body (lines 275–329) is orphaned code outside any function. `go build ./...` exits 1 with `syntax error: non-declaration statement outside function body`. Proof.md's 14-test passing output is impossible from this commit. Gate 2 (secondary): `internal/adopt/adopt.go` is in the diff but neither listed in planned touchpoints nor explained in proof.md "Divergence from plan". Fix: (1) split line 274 to properly terminate the comment and declare `DraftTemplate` on a separate line; rerun both test commands with live output; (2) add `internal/adopt/adopt.go` to proof.md "Divergence from plan". Slice state → `failed_verification`.

### 2026-06-18 — S07-design-fit-gate: PASS (round 2, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. 9 unit tests + 5 CLI integration tests green in fresh session. `sworn designfit` wired in `main.go`; `cmdDesignfit` calls `designfit.Run()` directly. Smoke step output in proof.md consistent with live code behavior. No dark-code markers in changed source files. All 5 ACs have verifiable evidence. Verified at commit `4d78424`. Slice state → `verified`. T1-fidelity-core now has 5/7 slices verified. Next: `/implement-slice S11-journey-elicitation 2026-06-16-fidelity-layer`.

### 2026-06-18 — S07-design-fit-gate: FAIL (round 1, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2. `start_commit` in `status.json` is set to the implementation commit itself (`f4a3bfbe`, `feat(designfit): land S07`), not a pre-implementation "start" commit. `git diff --name-only f4a3bfbe..HEAD` returns only 3 proof-bundle documentation files — none of the planned touchpoints from `spec.md` appear in the verifier's independent diff. Per protocol, the verifier must use `start_commit` from `status.json`. Identical pattern to S05 round-4 FAIL. Gates 1, 3, 4, 5, 6 all PASS; implementation correct (9 unit + 5 CLI integration tests green). Fix: correct `start_commit` to `a1b2672` (S05 PASS commit, immediately before S07 feat). Slice state → `failed_verification`.

### 2026-06-18 — S05-requirements-validate-gate: PASS (round 5, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: All six gates passed. 15 unit tests + 3 CLI integration tests green in fresh session. Both smoke steps confirmed live (fail-closed: exit 1 on real release naming 16 slices; pass: exit 0 on fully-validated fixture). Verified at commit `bf7e776`. Slice state → `verified`. T1-fidelity-core now has 4/7 slices verified. Next: `/implement-slice S07-design-fit-gate 2026-06-16-fidelity-layer`.

### 2026-06-18 — S05-requirements-validate-gate: FAIL (round 4)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2. `start_commit` was reset to `12ef38a` (round-4 docs-only re-implementation commit), so `git diff --name-only 12ef38a..HEAD` shows only 3 doc files. All 7 planned touchpoints absent from diff range; proof.md "Divergence from plan" explains the extra `reqvalidate_test.go` but not why all planned implementation files are absent. Fix: set `start_commit` to `031e1cf` (S04 PASS, immediately before first S05 feat commit `7832963`) — that range shows all planned files, the extra file already explained, no S04 files. Implementation verified correct: 15 unit + 3 CLI integration tests pass, both smoke steps confirmed. Slice state → `failed_verification`.

### 2026-06-18 — S05-requirements-validate-gate: FAIL (round 3)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2 + Gate 4 (×2). Gate 2: `cmd/sworn/reqvalidate_test.go` is in the actual
  diff and `actual_files` but not acknowledged in proof.md "Divergence from plan". Gate 4(1):
  spec requires smoke step to show "add + ratify it; observe pass" — proof only asserts the
  pass case verbally, no captured CLI output. Gate 4(2): spec requires "an explicit note that
  the *interactive* scenario walk is exercised via the planner session" — absent from proof.
  Implementation verified correct (all 15 unit tests + 3 CLI integration tests pass; pass-case
  smoke independently confirmed: `sworn reqvalidate fixture-release` exits 0). Slice state →
  `failed_verification`.

### 2026-06-18 — S05-requirements-validate-gate: FAIL (round 2)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 2. proof.md "Files changed" section is stale — actual `git diff --name-only
  start_commit..HEAD` includes `cmd/sworn/reqverify.go`, `cmd/sworn/reqverify_test.go`,
  `internal/reqverify/reqverify_test.go`, `.gitignore` (S04 re-implementation files in the diff
  range because start_commit pre-dates S04's re-implementation cycles). proof.md "Divergence from
  plan" says "None" — these files are not explained. Gates 1, 3, 4, 5, 6 all PASS. All tests
  (15 unit + 3 CLI integration) pass in fresh context. Smoke step verified live. Fix: update
  proof.md "Files changed" + "Divergence from plan" to acknowledge the S04 files in range.
  Slice state → `failed_verification`.

### 2026-06-18 — S05-requirements-validate-gate: FAIL (round 1)

- **Actor**: verifier (fresh-context session)
- **Note**: Gate 3 (Rule 1). No `cmd/sworn/reqvalidate_test.go` exercises the CLI integration
  point `cmdReqvalidate()`. Only leaf-level unit tests in `internal/reqvalidate/` exist. The
  comparable S04 has `cmd/sworn/reqverify_test.go`. Fix: add integration test file calling
  `cmdReqvalidate()` with fixture release. Slice state → `failed_verification`.

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
