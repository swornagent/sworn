# Proof Bundle: S07-design-fit-gate

> Generated from live repo state. Base: a1b2672 (pre-S07 on T1-fidelity-core track).

## Scope

When a planner reaches design for a slice, sworn surfaces AI-drafted design options with trade-offs and prior art, and classifies each design choice by stakes = reversibility x blast-radius. `sworn designfit <release>` then fails closed when any high-stakes (structural / hard-to-reverse) choice lacks a recorded human decision; low-stakes choices may proceed with a noted default. Architecturally-significant decisions cannot be made by the model alone.

## Files changed

```
$ git diff --name-only a1b2672..HEAD
cmd/sworn/designfit.go
cmd/sworn/designfit_test.go
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/journal.md
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/proof.md
docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/adopt.gointernal/adopt/baton/VERSION
internal/adopt/baton/rules/09-design-fidelity.md
internal/designfit/designfit.go
internal/designfit/designfit_test.go
internal/prompt/captain.md
internal/prompt/planner.md
internal/state/state.go
```

## Test results

### Go (core + CLI)

```
$ go test ./internal/designfit/... -v
=== RUN   TestDesignfit_Type1WithoutDecision
--- PASS: TestDesignfit_Type1WithoutDecision (0.00s)
=== RUN   TestDesignfit_Type2WithNotedDefault
--- PASS: TestDesignfit_Type2WithNotedDefault (0.00s)
=== RUN   TestDesignfit_Type1WithHumanDecision
--- PASS: TestDesignfit_Type1WithHumanDecision (0.00s)
=== RUN   TestDesignfit_Type2WithoutDecision
--- PASS: TestDesignfit_Type2WithoutDecision (0.00s)
=== RUN   TestDesignfit_ArchitecturallySignificantMustBeType1
--- PASS: TestDesignfit_ArchitecturallySignificantMustBeType1 (0.00s)
=== RUN   TestDesignfit_ArchitecturallySignificantType1Passes
--- PASS: TestDesignfit_ArchitecturallySignificantType1Passes (0.00s)
=== RUN   TestDesignfit_MultipleSlices
--- PASS: TestDesignfit_MultipleSlices (0.00s)
=== RUN   TestDesignfit_Print_RoundTrip
--- PASS: TestDesignfit_Print_RoundTrip (0.00s)
=== RUN   TestDesignfit_EmptyRelease
--- PASS: TestDesignfit_EmptyRelease (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/designfit	0.005s

$ go test ./cmd/sworn/ -run TestDesignfit -v
=== RUN   TestDesignfitCmd_MissingReleaseArg
--- PASS: TestDesignfitCmd_MissingReleaseArg (0.00s)
=== RUN   TestDesignfitCmd_NonexistentRelease
--- PASS: TestDesignfitCmd_NonexistentRelease (0.00s)
=== RUN   TestDesignfitCmd_Type1NoDecision
--- PASS: TestDesignfitCmd_Type1NoDecision (0.00s)
=== RUN   TestDesignfitCmd_AllPass
--- PASS: TestDesignfitCmd_AllPass (0.00s)
=== RUN   TestDesignfitCmd_MultipleSlices
--- PASS: TestDesignfitCmd_MultipleSlices (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.007s
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Description**: Run `sworn designfit` on a fixture release with one Type-1 choice undecided; observe named failure; record the human decision; observe pass.
- **User gesture**: "User runs `sworn designfit smoke-test` with an undecided Type-1 choice, sees exit 1 naming slice + choice. User records the decision in status.json, re-runs, sees exit 0."
- **Evidence** (from live test run):

  Step 1 — Type-1 without decision:
  ```
  $ sworn designfit smoke-test
  Design-fit gate report for release "smoke-test"
  Slices checked: 1

  1 design-fit violation(s) found:

  1. S01-test: Type-1 choice "database-engine" has no recorded human decision

  DESIGNFIT FAIL — 1 violation(s) across 1 slice(s)
  EXIT: 1
  ```

  Step 2 — decision recorded:
  ```
  $ sworn designfit smoke-test
  Design-fit gate report for release "smoke-test"
  Slices checked: 1

  All design decisions have recorded human decisions where required — PASS.
  DESIGNFIT PASS — 1 slice(s) checked, all design-fit gates clear
  EXIT: 0
  ```

## Delivered

- **AC1** — WHEN a slice has a Type-1 (high-stakes) design choice with no recorded human decision, THE SYSTEM SHALL exit non-zero from `sworn designfit <release>` and name the slice + choice.
  - Evidence: `TestDesignfit_Type1WithoutDecision` (unit), `TestDesignfitCmd_Type1NoDecision` (CLI), reachability smoke step.

- **AC2** — WHEN a design choice is Type-2 (low-stakes / reversible), THE SYSTEM SHALL allow it to proceed with a recorded noted default and not require a human decision.
  - Evidence: `TestDesignfit_Type2WithNotedDefault`, `TestDesignfit_Type2WithoutDecision`, reachability smoke step.

- **AC3** — WHEN every Type-1 choice carries a human decision with options + rationale, THE SYSTEM SHALL exit 0.
  - Evidence: `TestDesignfit_Type1WithHumanDecision`, `TestDesignfitCmd_AllPass`, reachability smoke step.

- **AC4** — THE SYSTEM SHALL classify a design choice as Type-1 when it is architecturally significant (shapes the whole, hard to reverse), regardless of other factors.
  - Evidence: `TestDesignfit_ArchitecturallySignificantMustBeType1`, `TestDesignfit_ArchitecturallySignificantType1Passes`, reachability smoke step.

- **AC5** — THE SYSTEM SHALL NOT record a human decision on the model's behalf for a Type-1 choice.
  - Evidence: The checker enforces that `human_decision` must be non-empty for Type-1 choices. The field is never auto-populated by the designfit checker — it reads only what the planner records. `TestDesignfit_Type1WithoutDecision` verifies the fail-closed enforcement.

## Not delivered

None — all 5 acceptance checks are delivered.

## Divergence from plan

- `internal/adopt/adopt.go` was modified to add `09-design-fidelity.md` to the `Materialise` files list (implied by planned touchpoints but not explicitly listed).
- `cmd/sworn/designfit_test.go` was added as an additional test file (not in `planned_files` but required for Rule 1 reachability testing).

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S07-design-fit-gate 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S07-design-fit-gate
  slice dir:   docs/release/2026-06-16-fidelity-layer/S07-design-fit-gate
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented

== Diff vs main ==
  PASS  16 file(s) changed vs main

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

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 15
  checks failed: 0

FIRST-PASS PASS
```