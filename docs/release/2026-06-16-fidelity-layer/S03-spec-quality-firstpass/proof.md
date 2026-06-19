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
$ git diff --name-only release-wt/2026-06-16-fidelity-layer
bin/spec-quality.sh
cmd/sworn/main.go
cmd/sworn/specquality.go
cmd/sworn/specquality_test.go
docs/release/2026-06-16-fidelity-layer/S03-spec-quality-firstpass/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/prompt/planner.md
internal/specquality/specquality.go
internal/specquality/specquality_test.go
```

Note: `docs/release/2026-06-16-fidelity-layer/index.md` change is the track
worktree materialisation (T3-leaf-gates `worktree_path` + `state: in_progress`),
not a code change from this slice.

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
sworn specquality: release directory not found: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T3-leaf-gates/cmd/sworn/docs/release/nonexistent-release-xyz
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

== Status ==
  PASS  status.json is valid JSON
  state: in_progress

== Diff vs main ==
  PASS  6 file(s) changed vs main
  (first 20)
    bin/spec-quality.sh
    cmd/sworn/specquality.go
    cmd/sworn/specquality_test.go
    internal/specquality/specquality.go
    internal/specquality/specquality_test.go
    sworn

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

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 17
  checks failed: 1

FIRST-PASS FAIL
Address the failures above before invoking the LLM verifier session.
```

Note: The single remaining failure is `state: in_progress` — the release-verify.sh
expects `implemented` state before verifier dispatch. This is correct: the slice
is in re-implementation. Once proof.md is committed and status.json is updated
to `implemented`, the first-pass will show all green.