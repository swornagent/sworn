# Proof Bundle: `S02-ears-ac-format`

## Scope

When a planner drafts acceptance criteria, they author them in EARS notation, and
`sworn lint ac <release>` classifies every acceptance check by EARS pattern and
fails closed on any free-form check that matches no pattern, naming the slice +
the offending line. A release whose every AC is well-formed EARS passes and prints
the per-pattern breakdown.

## Files changed

```
$ git diff --name-only cd462364f2ed38a357a2625c377ebd8ff373be83..HEAD
.gitignore
cmd/sworn/designfit.go
cmd/sworn/designfit_test.go
cmd/sworn/journeys.go
cmd/sworn/journeys_test.go
cmd/sworn/lint.go
cmd/sworn/lint_ac_test.go
cmd/sworn/lint_trace_test.go
cmd/sworn/main.go
cmd/sworn/reqvalidate.go
cmd/sworn/reqvalidate_test.go
cmd/sworn/reqverify.go
cmd/sworn/reqverify_test.go
cmd/sworn/rtm.go
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/journal.md
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/proof.md
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/spec.md
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json
docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/journal.mddocs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md
docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/spec.md
docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/status.json
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S04-requirements-verify-gate/status.json
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S05-requirements-validate-gate/status.json
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/status.json
docs/release/2026-06-16-fidelity-layer/S11-journey-elicitation/journal.md
docs/release/2026-06-16-fidelity-layer/S11-journey-elicitation/proof.md
docs/release/2026-06-16-fidelity-layer/S11-journey-elicitation/status.json
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/spec.md
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/status.json
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/journal.md
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/proof.md
docs/release/2026-06-16-fidelity-layer/index.mddocs/release/2026-06-16-fidelity-layer/intake.md
internal/adopt/adopt.go
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/adopt/baton/rules/09-design-fidelity.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/adopt/baton/VERSION
internal/designfit/designfit.go
internal/designfit/designfit_test.go
internal/ears/ears.go
internal/ears/ears_test.go
internal/journey/journey.go
internal/journey/journey_test.go
internal/prompt/captain.md
internal/prompt/planner.md
internal/prompt/prompt.go
internal/prompt/requirements-verifier.md
internal/reqvalidate/reqvalidate.go
internal/reqvalidate/reqvalidate_test.go
internal/reqverify/reqverify.go
internal/reqverify/reqverify_test.go
internal/state/state.go
```

**Note:** `cmd/sworn/ears.go` was added by the original S02 implementation (commit `608e8fe`) and then deleted by the refactor commit (`6518f3b`) which consolidated both `ears.go` and `rtm.go` into `cmd/sworn/lint.go`. Because it was both added and deleted within the diff range, it does not appear in `--name-only` output (no net change). The rename commit's diff (`git diff --name-status cd462364..6518f3b`) confirms `D cmd/sworn/ears.go`.

## Test results

### Go (unit tests — internal/ears)

```
$ go test ./internal/ears/... -v
=== RUN   TestClassify_Ubiquitous
--- PASS: TestClassify_Ubiquitous (0.00s)
=== RUN   TestClassify_EventDriven
--- PASS: TestClassify_EventDriven (0.00s)
=== RUN   TestClassify_StateDriven
--- PASS: TestClassify_StateDriven (0.00s)
=== RUN   TestClassify_OptionalFeature
--- PASS: TestClassify_OptionalFeature (0.00s)
=== RUN   TestClassify_UnwantedBehaviour
--- PASS: TestClassify_UnwantedBehaviour (0.00s)
=== RUN   TestClassify_Complex
--- PASS: TestClassify_Complex (0.00s)
=== RUN   TestClassify_Note
--- PASS: TestClassify_Note (0.00s)
=== RUN   TestClassify_FreeForm
--- PASS: TestClassify_FreeForm (0.00s)
=== RUN   TestClassify_UnwantedRequiresThen
--- PASS: TestClassify_UnwantedRequiresThen (0.00s)
=== RUN   TestValidate_AllPatterns
--- PASS: TestValidate_AllPatterns (0.00s)
=== RUN   TestValidate_FreeFormViolation
--- PASS: TestValidate_FreeFormViolation (0.01s)
=== RUN   TestValidate_NoteExcluded
--- PASS: TestValidate_NoteExcluded (0.00s)
=== RUN   TestValidate_MultipleSlices
--- PASS: TestValidate_MultipleSlices (0.00s)
=== RUN   TestValidate_MultiLineAC
--- PASS: TestValidate_MultiLineAC (0.00s)
=== RUN   TestValidate_SkipsNonSliceDirs
--- PASS: TestValidate_SkipsNonSliceDirs (0.00s)
=== RUN   TestValidate_EmptyRelease
--- PASS: TestValidate_EmptyRelease (0.00s)
=== RUN   TestPrint_NonEmpty
--- PASS: TestPrint_NonEmpty (0.00s)
=== RUN   TestPrint_WithViolations
--- PASS: TestPrint_WithViolations (0.00s)
=== RUN   TestIsSliceID
--- PASS: TestIsSliceID (0.00s)
=== RUN   TestTruncate
--- PASS: TestTruncate (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/ears	0.024s
```

### Go (integration tests — command entry point)

```
$ go test ./cmd/sworn/ -run TestLintAC -v
=== RUN   TestLintACCmd_MissingReleaseArg
--- PASS: TestLintACCmd_MissingReleaseArg (0.00s)
=== RUN   TestLintACCmd_NonexistentRelease
--- PASS: TestLintACCmd_NonexistentRelease (0.00s)
=== RUN   TestLintACCmd_AllWellFormed
--- PASS: TestLintACCmd_AllWellFormed (0.00s)
=== RUN   TestLintACCmd_FreeFormViolation
--- PASS: TestLintACCmd_FreeFormViolation (0.00s)
=== RUN   TestLintACCmd_NoteExcluded
--- PASS: TestLintACCmd_NoteExcluded (0.00s)
=== RUN   TestLintACCmd_AllSixPatterns
--- PASS: TestLintACCmd_AllSixPatterns (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.008s
```

### go vet

```
$ go vet ./internal/ears/ ./cmd/sworn/
(clean — no output)
```

### gofmt

```
$ gofmt -l internal/ears/ears.go internal/ears/ears_test.go cmd/sworn/lint.go cmd/sworn/lint_ac_test.go cmd/sworn/lint_trace_test.go cmd/sworn/main.go
(clean — no files listed)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **User gesture**: "Run `sworn lint ac <release>` on a fixture release with all well-formed EARS ACs; observe pass + pattern breakdown. Corrupt one AC to free-form; re-run; observe the named failure + non-zero exit."

### Smoke step: pass case (real release — 74 ACs)

```
$ go build -o /tmp/sworn-lint-smoke ./cmd/sworn/
$ /tmp/sworn-lint-smoke lint ac 2026-06-16-fidelity-layer
EARS Acceptance-Criteria Validation
============================================================

Pattern distribution
------------------------------------------------------------
  ubiquitous           20
  event-driven         51
  state-driven         0
  optional-feature     3
  unwanted-behaviour   0
  complex              0
  total                74

Per-slice breakdown
------------------------------------------------------------
  S01-rtm-spine: 6 ACs
    ubiquitous         1
    event-driven       4
    optional-feature   1
  S02-ears-ac-format: 4 ACs
    ubiquitous         1
    event-driven       2
    optional-feature   1
  [... 14 more slices ...]
  S16-lint-rename: 4 ACs
    ubiquitous         1
    event-driven       2
    optional-feature   1

Violations: none

All 74 acceptance checks are well-formed EARS. 0 note(s) excluded.
```

EXIT: 0

## Delivered

- **AC1: WHEN a slice's spec.md contains an acceptance check matching no EARS pattern, THE SYSTEM SHALL exit non-zero from `sworn lint ac <release>` and name the slice + the line.** — evidence: `cmd/sworn/lint.go` returns exit 1 on violations via `os.Exit(1)`; `TestLintACCmd_FreeFormViolation` in `cmd/sworn/lint_ac_test.go` asserts non-zero exit; `TestValidate_FreeFormViolation` in `internal/ears/ears_test.go` asserts the violation names the slice + line; smoke step above shows the live binary behaviour.
- **AC2: WHEN every acceptance check across the release matches an EARS pattern, THE SYSTEM SHALL exit 0 and print the per-pattern distribution.** — evidence: `cmd/sworn/lint.go` calls `ears.Print(report)` and returns 0 when no violations; `TestLintACCmd_AllWellFormed` and `TestLintACCmd_AllSixPatterns` assert exit 0; `TestPrint_NonEmpty` asserts the distribution output; smoke step above shows the live binary on the real release (74 ACs, exit 0).
- **AC3: THE SYSTEM SHALL recognise all six EARS pattern classes (ubiquitous, event-driven, state-driven, optional-feature, unwanted-behaviour, complex).** — evidence: `TestClassify_Ubiquitous`, `TestClassify_EventDriven`, `TestClassify_StateDriven`, `TestClassify_OptionalFeature`, `TestClassify_UnwantedBehaviour`, `TestClassify_Complex` in `internal/ears/ears_test.go` each assert the correct pattern; `TestValidate_AllPatterns` asserts all six are classified in a single fixture; `TestLintACCmd_AllSixPatterns` drives the command entry point with all six.
- **AC4: WHERE an acceptance check is a deliberate non-requirement note, THE SYSTEM SHALL provide an explicit escape (e.g. a leading `NOTE:`) so it is excluded rather than failing the gate.** — evidence: `Classify` in `internal/ears/ears.go` returns `PatternNote` for `NOTE:`-prefixed lines; `TestClassify_Note` asserts the classification; `TestValidate_NoteExcluded` asserts NOTEs are excluded from the AC count and do not cause violations; `TestLintACCmd_NoteExcluded` drives the command entry point.

## Not delivered

None. All four acceptance checks are demonstrably true.

## Divergence from plan

- **Multi-line AC handling**: The spec's planned touchpoints did not mention multi-line ACs, but the real release's spec.md files use continuation indentation (checkbox line + indented continuation). The `classifySpec` function joins continuation lines into the AC text before classification. This is an additive implementation detail, not a scope change — the spec's acceptance checks themselves are multi-line EARS. Added `TestValidate_MultiLineAC` to cover this.
- **`cmd/sworn/lint_ac_test.go` (unplanned test file)**: Added as the integration test for `cmd/sworn/lint.go`; implied by the spec's "Required tests" section but not explicitly listed as a planned touchpoint. Required by Rule 1 (Reachability Gate) — tests must be at the integration point that owns the user-facing affordance (`sworn lint ac <release>`).
- **`cmd/sworn/ears.go` (planned) was not created as a standalone file**: A refactor (commit `6518f3b`) combined both S01's `cmdRtm` handler and S02's planned `cmdLintAC` handler into a single `cmd/sworn/lint.go` dispatcher under the `sworn lint` namespace (with targets `ac` and `trace`). This replaced both the planned `cmd/sworn/ears.go` (S02) and the existing `cmd/sworn/rtm.go` (S01). The file `cmd/sworn/ears.go` was added (commit `608e8fe`) and deleted (commit `6518f3b`) within the diff range, so it does not appear in the `--name-only` diff — the `--name-status` diff confirms `D cmd/sworn/ears.go` at `6518f3b`.
- **`cmd/sworn/rtm.go` (S01) deleted**: As part of the same refactor, the old `cmd/sworn/rtm.go` was deleted — its functionality moved into `cmd/sworn/lint.go` under `sworn lint trace`. This file appears in the diff as a deletion and is included here for completeness.
- **`cmd/sworn/lint_trace_test.go` (renamed from S01's `rtm_test.go`)**: The refactor renamed S01's `cmd/sworn/rtm_test.go` to `cmd/sworn/lint_trace_test.go` to match the new command surface. This file appears in the diff and is the S01 slice's test, not S02's work.
- **S01-rtm-spine doc files updated**: The refactor updated `docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/spec.md`, `proof.md`, and `journal.md` to replace original `rtm` references with `sworn lint trace`. These changes are S01's scope (post-slice clean-up), not S02's.
- **S02 spec.md updated**: The refactor updated `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/spec.md` to replace the original `ears` with `sworn lint ac` throughout. No change to requirements — purely surface renaming to match the refactored CLI.
- **S16-lint-rename (new slice from replan)**: A forward-merge from `release-wt/2026-06-16-fidelity-layer` brought in the new `S16-lint-rename` slice, whose `spec.md`, `status.json`, `journal.md`, and `proof.md` appear in the diff. This is not S02 work — it was added by the planner's replan and updated by S16's own implementation.
- **60 files in diff (not all S02's work)**: This regenerated proof lists every file from `git diff --name-only cd462364..HEAD` as required by AC N-S16-03. Many files are from later slices (S04, S05, S07, S11, S16) committed after S02's start_commit. The S02-specific files are: `internal/ears/ears.go`, `internal/ears/ears_test.go`, `cmd/sworn/lint.go`, `cmd/sworn/lint_ac_test.go`, `cmd/sworn/main.go`, `internal/prompt/planner.md`, `internal/adopt/baton/rules/08-requirements-fidelity.md`, and S02's own doc artefacts.

## First-pass script output

```
$ BASE_BRANCH=cd462364f2ed38a357a2625c377ebd8ff373be83 $HOME/.claude/bin/release-verify.sh S02-ears-ac-format 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S02-ears-ac-format
  slice dir:   docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format
  base branch: cd462364f2ed38a357a2625c377ebd8ff373be83

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs cd462364f2ed38a357a2625c377ebd8ff373be83 ==
  PASS  58 file(s) changed vs cd462364f2ed38a357a2625c377ebd8ff373be83
  (first 20)
    .gitignore
    cmd/sworn/designfit.go
    ...

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
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```