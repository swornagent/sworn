---
title: Slice journal — S03-spec-quality-firstpass
description: Implementation log. Append-only.
---

# Journal: `S03-spec-quality-firstpass`

## Session log

### 2026-06-22 — implementation complete

- **State**: implemented
- **Notes**:
  - Implemented `internal/specquality/` package with soundness and completeness
    computation, mutation operators (flip exit code, negate assertion, remove
    keyword, uppercase, lowercase, swap zero/one), and `## Acceptance examples`
    parser (structured YAML-like and shorthand arrow format).
  - Created `cmd/sworn/specquality.go` — CLI command wiring with `--threshold` flag.
  - Updated `cmd/sworn/main.go` — additive `case "specquality"` + usage docs.
  - Created `bin/spec-quality.sh` — thin wrapper for CI/first-pass use.
  - Updated `internal/prompt/planner.md` — added acceptance-examples guidance
    as step 5 in Phase 4; renumbered steps 5-9 to 6-10.
  - Updated `internal/adopt/baton/rules/08-requirements-fidelity.md` — added
    "Spec-quality metric" section documenting the metric, enforcement, and
    relationship to verify/validate gates.
  - **Key decision**: mutation operators are deterministic text heuristics
    (pattern matching on exit codes, assertions, keywords). This is by design —
    the spec requires "no model call." The operators are deliberately simple
    and documented; they can be extended later. The score is always
    interpretable because every operator that ran is named.
  - **Trade-off**: the soundness check is limited to contradiction detection
    (expects failure vs pass-only criteria; command-name consistency). Full
    semantic soundness would require a model — that's S04's role. S03 is a
    cheap first-pass that catches the most obvious defects.
  - Bin/spec-quality.sh required `git add -f` because `/bin/` is in
    .gitignore. Noted in proof.md "Divergence from plan."
  - **Subagent dispatches**: none — single-session implementation.

### 2026-06-27 — re-implementation: fix verifier violations (proof bundle completeness gaps)

- **State**: in_progress → implemented
- **Notes**:
  - Three verifier violations were proof-bundle completeness gaps (not code bugs):
    1. Gate 2: `cmd/sworn/specquality_test.go` missing from proof.md "Divergence from plan"
       — Added the entry explaining the test file was added per spec's Required tests section
       (Rule 1 reachability gate), not part of planned touchpoints.
    2. Gate 3: Paraphrased `go test ./...` output replaced with actual full output from
       the correct track worktree (20 packages all passing).
    3. Gate 3: `release-verify.sh` placeholder replaced with actual output showing 17/18 passes
       (only state: in_progress fails, which is expected before marking implemented).
  - Root cause of code-file loss: merge commit `722e658` (release-wt into track/.../T3-leaf-gates)
    deleted `internal/specquality/`, `cmd/sworn/specquality.go`, `cmd/sworn/specquality_test.go`,
    and `bin/spec-quality.sh`. Restored from implementation commit `62319a7`.
  - All 5 ACs verified substantively correct by the verifier — no code changes needed.
  - **Subagent dispatches**: none — single-session re-implementation.

## Open questions

- None.

## Deferrals surfaced

- None.

## Verifier verdicts received

### 2026-06-19 — BLOCKED (round 2, fresh-context)

- **Verifier session**: fresh
- **Verdict body**:

  BLOCKED

  Slice: `S03-spec-quality-firstpass`
  Reason: Forward-merge of `release-wt/2026-06-16-fidelity-layer` into `track/2026-06-16-fidelity-layer/T3-leaf-gates` conflicted on `cmd/sworn/main.go`. Both `S15-sworn-top-evidence` (T4, already merged into release-wt via commit `a58733d`) and `S03` (T3) write to `cmd/sworn/main.go` with separate `case` additions. The index.md convention states this is "not treated as a touchpoint collision" (additive case blocks, distinct per slice), but the live merge proves a conflict exists. This is a contract defect — the touchpoint matrix incorrectly classifies `cmd/sworn/main.go` as collision-free for the parallel set {T2, T3, T4}.
  Proposed spec.md/index.md amendment: In the "Touchpoint matrix" section, add `cmd/sworn/main.go` as a shared touchpoint with a note that it is **sequential, not parallel** — each track that adds a `case` must either (a) depend on the prior track that also adds a `case`, or (b) the merge protocol must be made explicit (three-way merge with `ours`/`theirs` strategy documented). The current `T3-leaf-gates` `depends_on: T1-fidelity-core` must be changed to `depends_on: [T1-fidelity-core, T4-evidence-surface]`, or T4 must be moved to depend on T3, to restore the serialisation guarantee. This is the second occurrence of this issue on T3 (prior: merge commit `722e658` silently deleted S03 files; journal 2026-06-27).

- **Action taken**: Merge aborted. State unchanged (`implemented`). `verification.result` set to `blocked`. Next step: `/replan-release 2026-06-16-fidelity-layer`.



### 2026-06-19 00:15 — FAIL (round 1, fresh-context)

- **Verifier session**: fresh
- **Verdict body**:

  FAIL

  Slice: `S03-spec-quality-firstpass`

  Violations:
  1. Gate 2 — `cmd/sworn/specquality_test.go` is in the diff but absent from spec planned
     touchpoints and not documented in proof.md "Divergence from plan."
  2. Gate 3 — `go test ./...` output in proof.md is paraphrased (`# all packages pass`);
     live re-run failed on `internal/specquality: chdir: no such file or directory`.
  3. Gate 3 — proof.md "First-pass script output" contains a committed unfilled placeholder.

- **Action taken**: Re-entry via /implement-slice to fix the 3 proof-bundle gaps.
  All 3 violations addressed in re-implementation session (2026-06-27).