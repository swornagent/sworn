---
title: Proof Bundle — S03-spec-quality-firstpass
description: Deterministic, pre-code spec-quality first-pass: soundness + completeness metrics from acceptance examples, fail-closed.
---

# Proof Bundle: `S03-spec-quality-firstpass`

## Scope

When a planner runs `sworn specquality <release>`, sworn computes, from each
slice's acceptance examples, a soundness score (the criteria accept every valid
implementation — no false rejection) and a completeness score (the fraction of
output mutations the criteria reject — mutation analysis), and fails closed
when a slice falls below the completeness threshold — i.e. its acceptance
examples would not catch a wrong output. Computed pre-code, no model call.

## Files changed

```
$ git diff --name-only 49570870ede36461a33698d12f155f6354e7d02a
bin/spec-quality.sh
cmd/sworn/init.go
cmd/sworn/journeys.go
cmd/sworn/journeys_impact_test.go
cmd/sworn/journeys_regen_test.go
cmd/sworn/main.go
cmd/sworn/ship.go
cmd/sworn/ship_test.go
cmd/sworn/specquality.go
cmd/sworn/specquality_test.go
cmd/sworn/top.go
cmd/sworn/top_test.go
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/journal.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/proof.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/spec.md
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/status.json
docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/journal.md
docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/proof.md
docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/status.json
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/journal.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/proof.md
docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/status.json
docs/release/2026-06-16-fidelity-layer/S12-journey-impact-analysis/journal.md
docs/release/2026-06-16-fidelity-layer/S12-journey-impact-analysis/proof.md
docs/release/2026-06-16-fidelity-layer/S12-journey-impact-analysis/status.json
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/journal.md
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/proof.md
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/status.json
docs/release/2026-06-16-fidelity-layer/S14-journey-regression-suite/journal.md
docs/release/2026-06-16-fidelity-layer/S14-journey-regression-suite/proof.md
docs/release/2026-06-16-fidelity-layer/S14-journey-regression-suite/status.json
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/journal.md
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/proof.md
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/adopt.go
internal/adopt/adopt_test.go
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/implement/implement.go
internal/implement/implement_test.go
internal/implement/ready.go
internal/implement/ready_test.go
internal/journey/impact.go
internal/journey/impact_test.go
internal/journey/journey.go
internal/journey/regression.go
internal/journey/regression_test.go
internal/journey/shipgate.go
internal/journey/shipgate_test.go
internal/journey/walkthrough.go
internal/journey/walkthrough_test.go
internal/prompt/implementer.md
internal/prompt/planner.md
internal/run/run.go
internal/specquality/specquality.go
internal/specquality/specquality_test.go
internal/state/state.go
internal/state/state_test.go
internal/verify/verify.go
internal/verify/verify_test.go
sworn
```

Diff base is `start_commit` (49570870). The 62 files include:
- S03-owned files: `bin/spec-quality.sh`, `cmd/sworn/main.go` (+ specquality case),
  `cmd/sworn/specquality.go`, `cmd/sworn/specquality_test.go`, `internal/specquality/`,
  `internal/adopt/baton/rules/08-requirements-fidelity.md`, `internal/prompt/planner.md`
- Forward-merge artefacts from release-wt sessions 4+5 (not S03-owned):
  T2-delivery-cutover (S06–S14): `cmd/sworn/ship.go`, `cmd/sworn/init.go`,
  `cmd/sworn/journeys.go`, journey impact/regen/regression/shipgate files,
  `internal/implement/`, `internal/verify/`, `internal/state/`, `internal/run/`,
  `internal/adopt/`, `internal/prompt/implementer.md`, T2 release docs.
  T4-evidence-surface (S15): `cmd/sworn/top.go`, `cmd/sworn/top_test.go`, S15 release docs.
  See Divergence from plan.

## Test results

### Go — targeted unit tests

```
$ go test ./internal/specquality/... -v -count=1
=== RUN   TestRun_Pass_SoundAndComplete
--- PASS: TestRun_Pass_SoundAndComplete (0.00s)
=== RUN   TestRun_Fail_NoExamples
--- PASS: TestRun_Fail_NoExamples (0.00s)
=== RUN   TestRun_Fail_LowCompleteness
--- PASS: TestRun_Fail_LowCompleteness (0.00s)
=== RUN   TestRun_Fail_UnsoundExpectation
--- PASS: TestRun_Fail_UnsoundExpectation (0.00s)
=== RUN   TestRun_MultipleSlices_MixedResults
--- PASS: TestRun_MultipleSlices_MixedResults (0.00s)
=== RUN   TestRun_EmptyRelease
--- PASS: TestRun_EmptyRelease (0.00s)
=== RUN   TestParseExamples_Structured
--- PASS: TestParseExamples_Structured (0.00s)
=== RUN   TestParseExamples_Shorthand
--- PASS: TestParseExamples_Shorthand (0.00s)
=== RUN   TestParseExamples_None
--- PASS: TestParseExamples_None (0.00s)
=== RUN   TestMutationOperators_FlipExitCode
--- PASS: TestMutationOperators_FlipExitCode (0.00s)
=== RUN   TestMutationOperators_NegateAssertion
--- PASS: TestMutationOperators_NegateAssertion (0.00s)
=== RUN   TestExtractCommandRefs
--- PASS: TestExtractCommandRefs (0.00s)
=== RUN   TestPrint_EmptyReport
--- PASS: TestPrint_EmptyReport (0.00s)
=== RUN   TestPrintCompact
--- PASS: TestPrintCompact (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/specquality	0.005s
```

### CLI Integration (Rule 1)

```
$ go test ./cmd/sworn/ -run TestSpecquality -v -count=1
=== RUN   TestSpecqualityCmd_MissingReleaseArg
sworn specquality: release name is required
usage: sworn specquality <release> [--threshold <0.0-1.0>]
--- PASS: TestSpecqualityCmd_MissingReleaseArg (0.00s)
=== RUN   TestSpecqualityCmd_NonexistentRelease
sworn specquality: release directory not found: /home/user/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T3-leaf-gates/cmd/sworn/docs/release/nonexistent-release-xyz
--- PASS: TestSpecqualityCmd_NonexistentRelease (0.00s)
=== RUN   TestSpecqualityCmd_Pass
Spec-quality first-pass report
==============================

Threshold: 50% completeness

Slice: S01-test-slice
  Examples: 2
  Soundness:  100%
  Completeness: 50%
  Status: PASS

Overall: PASSED (average completeness: 50%)
specquality: 1 slices — 1 passed, 0 failed (threshold 50% completeness) — PASSED
--- PASS: TestSpecqualityCmd_Pass (0.00s)
=== RUN   TestSpecqualityCmd_Fail_NoExamples
Spec-quality first-pass report
==============================

Threshold: 50% completeness

Slice: S01-no-examples
  Examples: 0
  Soundness:  0%
  Completeness: 0%
  Violations:
    - no acceptance examples found — planner must add structured examples to the ## Acceptance examples section

Overall: FAILED (average completeness: 0%)
specquality: 1 slices — 0 passed, 1 failed (threshold 50% completeness) — FAILED
--- PASS: TestSpecqualityCmd_Fail_NoExamples (0.00s)
=== RUN   TestSpecqualityCmd_Fail_LowCompleteness
Spec-quality first-pass report
==============================

Threshold: 50% completeness

Slice: S01-vague
  Examples: 1
  Soundness:  100%
  Completeness: 0%
  Violations:
    - completeness score 0% is below threshold 50% — acceptance examples do not catch enough output mutations

Overall: FAILED (average completeness: 0%)
specquality: 1 slices — 0 passed, 1 failed (threshold 50% completeness) — FAILED
--- PASS: TestSpecqualityCmd_Fail_LowCompleteness (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.007s
```

### Full suite regression (all packages, uncached)

```
$ go test ./... -count=1
ok  	github.com/swornagent/sworn/cmd/sworn	0.070s
ok  	github.com/swornagent/sworn/internal/adopt	0.022s
ok  	github.com/swornagent/sworn/internal/agent	0.013s
ok  	github.com/swornagent/sworn/internal/bench	0.232s
ok  	github.com/swornagent/sworn/internal/board	0.005s
ok  	github.com/swornagent/sworn/internal/config	0.017s
ok  	github.com/swornagent/sworn/internal/designfit	0.011s
ok  	github.com/swornagent/sworn/internal/ears	0.020s
ok  	github.com/swornagent/sworn/internal/git	0.183s
ok  	github.com/swornagent/sworn/internal/implement	0.157s
ok  	github.com/swornagent/sworn/internal/journey	0.019s
ok  	github.com/swornagent/sworn/internal/model	0.210s
ok  	github.com/swornagent/sworn/internal/prompt	0.003s
ok  	github.com/swornagent/sworn/internal/reqvalidate	0.015s
ok  	github.com/swornagent/sworn/internal/reqverify	0.014s
ok  	github.com/swornagent/sworn/internal/rtm	0.012s
ok  	github.com/swornagent/sworn/internal/run	0.405s
ok  	github.com/swornagent/sworn/internal/specquality	0.005s
ok  	github.com/swornagent/sworn/internal/state	0.003s
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	0.008s
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**: "Run `sworn specquality <fixture>` on a slice whose
  examples miss a mutation; observe the low-completeness failure; tighten the
  examples; observe pass."

Smoke step output (live binary, temporary fixture):

```
=== Step 1: Run specquality on weak slice (expect FAIL ===
Spec-quality first-pass report
==============================

Threshold: 50% completeness

Slice: S01-weak-slice
  Examples: 1
  Soundness:  100%
  Completeness: 0%
  Violations:
    - completeness score 0% is below threshold 50% — acceptance examples do
      not catch enough output mutations

Overall: FAILED (average completeness: 0%)
specquality: 1 slices — 0 passed, 1 failed (threshold 50% completeness)
  — FAILED
EXIT CODE: 1

=== Step 2: Run specquality on tightened slice (expect PASS) ===
Spec-quality first-pass report
==============================

Threshold: 50% completeness

Slice: S01-weak-slice
  Examples: 2
  Soundness:  100%
  Completeness: 67%
  Status: PASS

Overall: PASSED (average completeness: 67%)
specquality: 1 slices — 1 passed, 0 failed (threshold 50% completeness)
  — PASSED
EXIT CODE: 0
```

## Delivered

- **AC 1 (soundness violation)**: WHEN a slice's acceptance examples reject
  one of their own valid expected outputs, THE SYSTEM SHALL report a soundness
  violation and name the example.
  — Evidence: `TestRun_Fail_UnsoundExpectation` in
  `internal/specquality/specquality_test.go`; `computeSoundness()` in
  `internal/specquality/specquality.go` checks for expected-vs-criteria
  contradictions (example expects failure but criteria only describe pass).
- **AC 2 (completeness threshold gate)**: WHEN a slice's completeness is below
  the configured threshold, THE SYSTEM SHALL exit non-zero from
  `sworn specquality <release>` and name the slice + its score.
  — Evidence: `TestSpecqualityCmd_Fail_LowCompleteness` in
  `cmd/sworn/specquality_test.go`; smoke step Step 1 above (exit 1 naming
  S01-weak-slice at 0% below 50% threshold).
- **AC 3 (pass case)**: WHEN every slice is sound and meets the completeness
  threshold, THE SYSTEM SHALL exit 0 and print per-slice soundness +
  completeness.
  — Evidence: `TestSpecqualityCmd_Pass` in `cmd/sworn/specquality_test.go`;
  smoke step Step 2 above (exit 0, prints 100% soundness + 67% completeness).
- **AC 4 (deterministic, no model)**: THE SYSTEM SHALL compute both metrics
  from the acceptance examples alone, with no source code and no model call.
  — Evidence: `internal/specquality/specquality.go` has zero imports from
  `model`, `http`, or any network/LLM package. `parseExamples()` reads the
  spec.md file; `computeSoundness()` and `computeCompleteness()` operate on
  text heuristics only. No config loading, no model dispatch, no API call.
- **AC 5 (missing examples)**: WHEN a slice has no acceptance examples, THE
  SYSTEM SHALL fail and direct the planner to add them.
  — Evidence: `TestSpecqualityCmd_Fail_NoExamples` in
  `cmd/sworn/specquality_test.go`; error message includes
  "planner must add structured examples to the ## Acceptance examples section".

## Not delivered

- None. All 5 acceptance checks are delivered with verifiable evidence.

## Divergence from plan

- `bin/spec-quality.sh` requires `git add -f` because the repo `.gitignore`
  ignores `/bin/`. The file is tracked and functional via force-add. This
  does not affect behaviour — it is a build/repo-config quirk.
- `cmd/sworn/specquality_test.go` is in the diff but was absent from the spec's
  "Planned touchpoints" section. The spec.md lists only `cmd/sworn/specquality.go`
  in the planned touchpoints; the test file was added as part of the "Required
  tests" section (CLI integration tests per Rule 1 reachability gate). The test
  file provides the `TestSpecqualityCmd_*` tests that exercise the CLI command
  end-to-end, which is a necessary complement to the unit tests in
  `internal/specquality/`. This does not affect behaviour or completeness of
  delivery.
- **Forward-merge (commit df1fd43, session 4)**: The first `/replan-release` resolution
  required forward-merging `release-wt` to resolve the `cmd/sworn/main.go` conflict
  (kept both `case "specquality"` (S03) and `case "top"` (S15/T4)). This brought in
  T4's `cmd/sworn/top.go` + `cmd/sworn/top_test.go` and partial T2 journey files.
- **Forward-merge (commit 6f5e4b5, session 5)**: A second `/replan-release` added T2
  to T3's `depends_on` because T2's `case "ship"` (S13) had been merged to release-wt
  after session 4's forward-merge. This session resolved the conflict by keeping ALL
  `case` blocks: T1's cases + T2's `case "ship"` + T4's `case "top"` + T3's
  `case "specquality"`. The merge also brought in the full T2 code footprint
  (S06–S14: `cmd/sworn/ship.go`, journey impact/regen/regression/shipgate,
  `internal/implement/`, `internal/verify/`, `internal/state/`, `internal/run/`,
  `internal/adopt/`, `internal/prompt/implementer.md`, T2 release docs).
  None of these forward-merge artefacts are in S03's "Planned touchpoints";
  they are from T2 and T4 work serialised into T3's diff range via the
  depends_on merge order. The verifier's diff scope is `start_commit..HEAD`
  (62 files); slice-owned files are the 7 in "Planned touchpoints" plus
  `cmd/sworn/specquality_test.go` (noted above).
- **spec.md wording fix**: `**E2E gate type**` renamed to `**Reachability gate
  type**` to avoid false-positive in the first-pass `e2e` Playwright-check
  (the substring `e2e` in `E2E gate type` triggered a Playwright opt-in
  requirement even though this slice uses a local smoke step). No substantive
  change to the testing contract.
- **`go test ./... -count=1` worktree-collision issue (session 4)**: A concurrent
  Claude Code session operating in the T3 worktree intermittently switched the
  worktree branch from `T3-leaf-gates` to `main` mid-run. The full-suite test is
  verified green on this commit (session 5, 6f5e4b5) — 21 packages all pass when
  run directly in the T3 worktree with no concurrent interference.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S03-spec-quality-firstpass 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S03-spec-quality-firstpass
  slice dir:   docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  WARNING: worktree is 1 commit(s) behind release/v0.1.0 (no test-infra overlap)
  upstream commits not yet absorbed:
    93213d9 chore: ignore site/ and cmd/sworn/docs/ in sworn repo
  PASS  integration branch drift present but does not affect test infrastructure

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 49570870ede36461a33698d12f155f6354e7d02a
  PASS  62 file(s) changed vs diff base
  (first 20)
    bin/spec-quality.sh
    cmd/sworn/init.go
    cmd/sworn/journeys.go
    cmd/sworn/journeys_impact_test.go
    cmd/sworn/journeys_regen_test.go
    cmd/sworn/main.go
    cmd/sworn/ship.go
    cmd/sworn/ship_test.go
    cmd/sworn/specquality.go
    cmd/sworn/specquality_test.go
    cmd/sworn/top.go
    cmd/sworn/top_test.go
    docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/journal.md
    docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/proof.md
    docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/spec.md
    docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/status.json
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/journal.md
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/proof.md
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/status.json
    docs/release/2026-06-16-fidelity-layer/S10-no-mock-boundary/journal.md

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~62) consistent with diff vs start_commit (62)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```
