---
title: 'S12 — journey-impact-analysis proof bundle'
description: 'Rule 6 proof bundle for S12: per-release journey-impact analysis.'
---

# Proof Bundle: S12-journey-impact-analysis

## Scope

When a maintainer runs `sworn journeys --impact <release>`, sworn reports which critical journeys the release touches (derived from the release's slices and the surfaces they change) and fails closed if the journeys artefact is missing.

## Files changed

```
$ git diff --name-only release-wt/2026-06-16-fidelity-layer
cmd/sworn/journeys.go
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S12-journey-impact-analysis/status.json
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/journey/impact.go
internal/journey/impact_test.go
cmd/sworn/journeys_impact_test.go
```

(Only S12's files are listed; other files in the worktree diff belong to prior slices S06 and S10.)

## Test results

### Go backend

```
$ go test ./internal/journey/... -v -run "TestImpact|TestSurface|TestToken"
=== RUN   TestImpactAnalysis_MissingArtefact
--- PASS: TestImpactAnalysis_MissingArtefact (0.00s)
=== RUN   TestImpactAnalysis_UnratifiedArtefact
--- PASS: TestImpactAnalysis_UnratifiedArtefact (0.00s)
=== RUN   TestImpactAnalysis_TouchedJourneys
--- PASS: TestImpactAnalysis_TouchedJourneys (0.00s)
=== RUN   TestImpactAnalysis_EmptyTouchedSet
--- PASS: TestImpactAnalysis_EmptyTouchedSet (0.00s)
=== RUN   TestImpactAnalysis_DerivedFromTouchpoints
--- PASS: TestImpactAnalysis_DerivedFromTouchpoints (0.00s)
=== RUN   TestImpactAnalysis_WithActualFiles
--- PASS: TestImpactAnalysis_WithActualFiles (0.00s)
=== RUN   TestSurfaceTouch
--- PASS: TestSurfaceTouch (0.00s)
=== RUN   TestTokenize
--- PASS: TestTokenize (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/journey	0.006s

$ go test ./internal/journey/...
ok  	github.com/swornagent/sworn/internal/journey	0.013s
```

### CLI integration tests

```
$ go test ./cmd/sworn/ -run TestJourneysImpact -v
=== RUN   TestJourneysImpactCmd_MissingArtefact
--- PASS: TestJourneysImpactCmd_MissingArtefact (0.00s)
=== RUN   TestJourneysImpactCmd_UnratifiedArtefact
--- PASS: TestJourneysImpactCmd_UnratifiedArtefact (0.00s)
=== RUN   TestJourneysImpactCmd_TouchedJourneys
--- PASS: TestJourneysImpactCmd_TouchedJourneys (0.00s)
=== RUN   TestJourneysImpactCmd_EmptyTouchedSet
--- PASS: TestJourneysImpactCmd_EmptyTouchedSet (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.007s

$ go test ./cmd/sworn/ -run TestJourneys -v
=== RUN   TestJourneysCmd_MissingCheck
--- PASS: TestJourneysCmd_MissingCheck (0.00s)
=== RUN   TestJourneysCmd_UnratifiedCheck
--- PASS: TestJourneysCmd_UnratifiedCheck (0.00s)
=== RUN   TestJourneysCmd_PassCheck
--- PASS: TestJourneysCmd_PassCheck (0.00s)
=== RUN   TestJourneysCmd_Elicit
--- PASS: TestJourneysCmd_Elicit (0.00s)
=== RUN   TestJourneysCmd_ElicitWithExistingArtefact
--- PASS: TestJourneysCmd_ElicitWithExistingArtefact (0.00s)
=== RUN   TestJourneysCmd_PassPrint
--- PASS: TestJourneysCmd_PassPrint (0.00s)
=== RUN   TestJourneysCmd_NoArgs
--- PASS: TestJourneysCmd_NoArgs (0.00s)
=== RUN   TestJourneysCmd_NonExistentPath
--- PASS: TestJourneysCmd_NonExistentPath (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.009s
```

### Full project suite

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn
ok  	github.com/swornagent/sworn/internal/journey
... (20 packages, all PASS)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**: Run `sworn journeys --impact <fixture-release>` against a fixture with a ratified journeys artefact; observe the touched-journey set; remove the journeys artefact; re-run; observe the directed failure.

**Smoke test output:**

```
=== Test 1: Missing artefact ===
FAIL: no journeys artefact found at ... — run 'sworn journeys <project>' to elicit journeys first (S11)
Exit code: 1

=== Test 2: With ratified artefact ===
Release: fidelity-layer
Journeys artefact: found and ratified

Journeys touched by this release (3):
  - J01-verify-flow
  - J02-init-setup
  - J03-walkthrough
Exit code: 0

=== Test 3: After removing artefact ===
FAIL: no journeys artefact found at ... — run 'sworn journeys <project>' to elicit journeys first (S11)
Exit code: 1
```

## Delivered

- **AC1** (output touched-journey set): WHEN `sworn journeys --impact <release>` runs against a release with a ratified journeys artefact, THE SYSTEM SHALL output the set of journeys the release touches.
  - Evidence: `TestJourneysImpactCmd_TouchedJourneys` (CLI integration test) and `TestImpactAnalysis_TouchedJourneys` (unit test). Smoke test Test 2 shows all 3 journeys correctly identified.
- **AC2** (fail-closed on missing artefact): WHEN no ratified journeys artefact exists, THE SYSTEM SHALL exit non-zero and direct the user to run elicitation (S11) first.
  - Evidence: `TestJourneysImpactCmd_MissingArtefact` and `TestImpactAnalysis_MissingArtefact` (both PASS). Smoke tests Test 1 and Test 3 confirm exit code 1 with clear direction.
- **AC3** (empty touched-set): WHEN a release touches no journeys, THE SYSTEM SHALL report an empty touched-set explicitly rather than failing.
  - Evidence: `TestJourneysImpactCmd_EmptyTouchedSet` and `TestImpactAnalysis_EmptyTouchedSet` (both PASS). Output shows "Journeys touched by this release (0): (none — release touches no critical journeys)".
- **AC4** (derived from touchpoints): THE SYSTEM SHALL derive the touched-set from the release's slice touchpoints + entry points, not from a hand-maintained list.
  - Evidence: `TestImpactAnalysis_DerivedFromTouchpoints` (PASS). The implementation reads `planned_files` + `actual_files` from each slice's `status.json` — there is no hand-maintained list. The `surfacesTouch` heuristic operates on these derived paths.

## Not delivered

None. All 4 acceptance checks are delivered.

## Divergence from plan

- **New file `internal/journey/impact.go`**: The planned_files listed `internal/journey/journey.go` (extending S11's model), but the impact analysis logic needed its own file for clarity. `journey.go` was not modified — S12's code lives entirely in `impact.go` and `impact_test.go`.
- **New file `cmd/sworn/journeys_impact_test.go`**: The planned_files did not list this, but a separate test file was needed to keep CLI impact tests from mixing with S11's elicitation tests.
- **`cmd/sworn/main.go` updated**: Usage string and journeys description were updated to document `--impact`. This file was not in `planned_files` but is a necessary documentation surface.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S12-journey-impact-analysis 2026-06-16-fidelity-layer

release-verify.sh
  slice:       S12-journey-impact-analysis
  slice dir:   docs/release/2026-06-16-fidelity-layer/S12-journey-impact-analysis
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
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs main ==
  PASS  30 file(s) changed vs main

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
  PASS  proof.md contains no unfilled template placeholders

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
```