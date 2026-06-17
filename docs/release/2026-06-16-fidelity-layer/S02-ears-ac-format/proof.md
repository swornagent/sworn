# Proof Bundle: `S02-ears-ac-format`

## Scope

When a planner drafts acceptance criteria, they author them in EARS notation, and `sworn lint ac <release>` classifies every acceptance check by EARS pattern and fails closed on any free-form check that matches no pattern, naming the slice + the offending line. A release whose every AC is well-formed EARS passes and prints the per-pattern breakdown.

## Files changed

```
$ git diff --name-only bf5b76b..HEAD
cmd/sworn/lint.go
cmd/sworn/lint_ac_test.go
cmd/sworn/main.go
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/ears/ears.go
internal/ears/ears_test.go
internal/prompt/planner.md
```

## Test results

### Go (unit tests)

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
--- PASS: TestValidate_FreeFormViolation (0.00s)
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
ok  	github.com/swornagent/sworn/internal/ears	0.008s
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
ok  	github.com/swornagent/sworn/cmd/sworn	0.012s
```

### Go (full suite)

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.029s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	(cached)
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/ears	0.009s
ok  	github.com/swornagent/sworn/internal/git	(cached)
ok  	github.com/swornagent/sworn/internal/implement	(cached)
ok  	github.com/swornagent/sworn/internal/model	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/rtm	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

### go vet

```
$ go vet ./...
(clean — no output)
```

### gofmt

```
$ gofmt -l internal/ears/ cmd/sworn/lint.go cmd/sworn/lint_ac_test.go cmd/sworn/main.go
(clean — no files listed)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (live binary invocation)
- **User gesture**: "Run `sworn lint ac <release>` on a fixture release with all well-formed EARS ACs; observe pass + pattern breakdown. Corrupt one AC to free-form; re-run; observe the named failure + non-zero exit."

### Smoke step 1: pass case (real release)

```
$ go build -o /tmp/sworn-lint-smoke ./cmd/sworn/
$ /tmp/sworn-lint-smoke lint ac 2026-06-16-fidelity-layer
EARS Acceptance-Criteria Validation
============================================================

Pattern distribution
------------------------------------------------------------
  ubiquitous           19
  event-driven         49
  state-driven         0
  optional-feature     2
  unwanted-behaviour   0
  complex              0
  total                70

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
  [... 13 more slices ...]

Violations: none

All 70 acceptance checks are well-formed EARS. 0 note(s) excluded.
EXIT: 0
```

### Smoke step 2: fail case (fixture with one free-form AC)

```
$ mkdir -p /tmp/ears-smoke-fail/docs/release/smoke-test/S01-test-slice
$ cat > /tmp/ears-smoke-fail/docs/release/smoke-test/S01-test-slice/spec.md << 'EOF'
## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] Make sure the form is saved.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.
EOF
$ cd /tmp/ears-smoke-fail && /tmp/sworn-lint-smoke lint ac smoke-test
EARS Acceptance-Criteria Validation
============================================================

Pattern distribution
------------------------------------------------------------
  ubiquitous           1
  event-driven         1
  total                3

Violations (1 free-form ACs)
------------------------------------------------------------
  S01-test-slice: line 14: Make sure the form is saved.

1 EARS violation(s) found:
  S01-test-slice: line 14: Make sure the form is saved.
EXIT: 1
```

## Delivered

- **AC1: WHEN a slice's spec.md contains an acceptance check matching no EARS pattern, THE SYSTEM SHALL exit non-zero from `sworn lint ac <release>` and name the slice + the line.** — evidence: `cmd/sworn/lint.go` returns exit 1 on violations; `TestLintACCmd_FreeFormViolation` in `cmd/sworn/lint_ac_test.go` asserts non-zero exit; `TestValidate_FreeFormViolation` in `internal/ears/ears_test.go` asserts the violation names the slice + line; smoke step 2 above shows the live binary behaviour.
- **AC2: WHEN every acceptance check across the release matches an EARS pattern, THE SYSTEM SHALL exit 0 and print the per-pattern distribution.** — evidence: `cmd/sworn/lint.go` prints `ears.Print(report)` and returns 0 when no violations; `TestLintACCmd_AllWellFormed` and `TestLintACCmd_AllSixPatterns` assert exit 0; `TestPrint_NonEmpty` asserts the distribution output; smoke step 1 above shows the live binary on the real release (70 ACs, exit 0).
- **AC3: THE SYSTEM SHALL recognise all six EARS pattern classes (ubiquitous, event-driven, state-driven, optional-feature, unwanted-behaviour, complex).** — evidence: `TestClassify_Ubiquitous`, `TestClassify_EventDriven`, `TestClassify_StateDriven`, `TestClassify_OptionalFeature`, `TestClassify_UnwantedBehaviour`, `TestClassify_Complex` in `internal/ears/ears_test.go` each assert the correct pattern; `TestValidate_AllPatterns` asserts all six are classified in a single fixture; `TestLintACCmd_AllSixPatterns` drives the command entry point with all six.
- **AC4: WHERE an acceptance check is a deliberate non-requirement note, THE SYSTEM SHALL provide an explicit escape (e.g. a leading `NOTE:`) so it is excluded rather than failing the gate.** — evidence: `Classify` in `internal/ears/ears.go` returns `PatternNote` for `NOTE:`-prefixed lines; `TestClassify_Note` asserts the classification; `TestValidate_NoteExcluded` asserts NOTEs are excluded from the AC count and do not cause violations; `TestLintACCmd_NoteExcluded` drives the command entry point.

## Not delivered

None. All four acceptance checks are demonstrably true.

## Divergence from plan

- **Multi-line AC handling**: The spec's planned touchpoints did not mention multi-line ACs, but the real release's spec.md files use continuation indentation (checkbox line + indented continuation). The `classifySpec` function joins continuation lines into the AC text before classification. This is an additive implementation detail, not a scope change — the spec's acceptance checks themselves are multi-line EARS. Added `TestValidate_MultiLineAC` to cover this.
- **`cmd/sworn/lint_ac_test.go` (unplanned test file)**: Added as the integration test for `cmd/sworn/lint.go`; implied by the spec's "Required tests" section but not explicitly listed as a planned touchpoint. Required by Rule 1 (Reachability Gate) — tests must be at the integration point that owns the user-facing affordance (`sworn lint ac <release>`).

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
  PASS  state is implemented (eligible for verifier review)

== Diff vs cd462364f2ed38a357a2625c377ebd8ff373be83 ==
  PASS  10 file(s) changed vs cd462364f2ed38a357a2625c377ebd8ff373be83
  (first 20)
    cmd/sworn/lint.go
    cmd/sworn/lint_ac_test.go
    cmd/sworn/main.go
    docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/journal.md
    docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md
    docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/status.json
    internal/adopt/baton/rules/08-requirements-fidelity.md
    internal/ears/ears.go
    internal/ears/ears_test.go
    internal/prompt/planner.md

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