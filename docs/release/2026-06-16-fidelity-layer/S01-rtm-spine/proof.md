# Proof Bundle: `S01-rtm-spine`

## Scope

When a planner runs `sworn rtm <release>`, sworn reports the release's 2-D requirements traceability matrix and fails closed on any broken trace: an intake need with no acceptance criterion, an acceptance criterion with no need or no test, or a slice with no link up to a release benefit are each named and cause a non-zero exit. A fully-traced release prints the matrix and exits 0.

## Files changed

```
$ git diff --name-only release-wt/2026-06-16-fidelity-layer
cmd/sworn/main.go
cmd/sworn/rtm.go
cmd/sworn/rtm_test.go
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/journal.md
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/proof.md
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/adopt.go
internal/adopt/baton/README.md
internal/adopt/baton/VERSION
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/board/index.go
internal/board/index_test.go
internal/prompt/planner.md
internal/rtm/rtm.go
internal/rtm/rtm_test.go
internal/state/state.go
internal/state/state_test.go
```

## Test results

### Go

```
$ go test ./internal/rtm/... -v
=== RUN   TestBuild_FullyTraced
--- PASS: TestBuild_FullyTraced (0.00s)
=== RUN   TestBuild_OrphanedNeed
--- PASS: TestBuild_OrphanedNeed (0.00s)
=== RUN   TestBuild_OrphanedAC_NoNeed
--- PASS: TestBuild_OrphanedAC_NoNeed (0.00s)
=== RUN   TestBuild_OrphanedAC_NoTest
--- PASS: TestBuild_OrphanedAC_NoTest (0.00s)
=== RUN   TestBuild_SliceNoVertical
--- PASS: TestBuild_SliceNoVertical (0.00s)
=== RUN   TestBuild_SoloFloor_NoObjective
--- PASS: TestBuild_SoloFloor_NoObjective (0.00s)
=== RUN   TestBuild_AC_CitesNonExistentNeed
--- PASS: TestBuild_AC_CitesNonExistentNeed (0.00s)
=== RUN   TestPrint_NonEmpty
--- PASS: TestPrint_NonEmpty (0.00s)
=== RUN   TestParseNeeds
--- PASS: TestParseNeeds (0.00s)
=== RUN   TestParseAcceptanceChecks
--- PASS: TestParseAcceptanceChecks (0.00s)
=== RUN   TestParseRequiredTests
--- PASS: TestParseRequiredTests (0.00s)
=== RUN   TestIsSliceID
--- PASS: TestIsSliceID (0.00s)
=== RUN   TestTruncate
--- PASS: TestTruncate (0.00s)
PASS
ok  github.com/swornagent/sworn/internal/rtm  0.003s

$ go test ./cmd/sworn/ -run TestRtm -v
=== RUN   TestRtmCmd_MissingReleaseArg
sworn rtm: release name is required
usage: sworn rtm <release>
--- PASS: TestRtmCmd_MissingReleaseArg (0.00s)
=== RUN   TestRtmCmd_NonexistentRelease
sworn rtm: release directory not found: /home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core/cmd/sworn/docs/release/nonexistent-release-xyz
--- PASS: TestRtmCmd_NonexistentRelease (0.00s)
=== RUN   TestRtmCmd_FullyTracedRelease
Requirements Traceability Matrix: test-release
============================================================

Horizontal trace (need -> AC -> test -> proof)
------------------------------------------------------------
  Need N-01: First need for testing
    -> AC [S01-test-slice]: WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
       -> test: **Integration**: exercise the command end-to-end
       -> test: **Unit**: internal/rtm/rtm_test.go — basic tests
  Need N-02: Second need for testing
    -> AC [S01-test-slice]: WHEN a test runs, THE SYSTEM SHALL verify N-02.
       -> test: **Integration**: exercise the command end-to-end
       -> test: **Unit**: internal/rtm/rtm_test.go — basic tests

Vertical trace (objective -> release benefit -> slice)
------------------------------------------------------------
  Objective: (none declared — solo/small-team floor)
  Release benefit: The release delivers value to users.
  Release goal: The release goal text for testing.

  Slice S01-test-slice -> benefit: The release delivers value to users.

All traces verified. 2 needs, 2 acceptance criteria, 2 tests, 1 slices.
--- PASS: TestRtmCmd_FullyTracedRelease (0.00s)
=== RUN   TestRtmCmd_OrphanedNeed
Requirements Traceability Matrix: test-release
============================================================

Horizontal trace (need -> AC -> test -> proof)
------------------------------------------------------------
  Need N-01: First need for testing
    -> AC [S01-test-slice]: WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
       -> test: **Unit**: some test
  Need N-02: Orphaned need with no AC
    -> (no linked acceptance criterion)

Vertical trace (objective -> release benefit -> slice)
------------------------------------------------------------
  Objective: (none declared — solo/small-team floor)
  Release benefit: the release goal from index
  Release goal: The release goal text for testing.

  Slice S01-test-slice -> (floor: release goal)

1 trace violation(s) found:
  [orphaned_need] need N-02 (Orphaned need with no AC) has no linked acceptance criterion
--- PASS: TestRtmCmd_OrphanedNeed (0.00s)
=== RUN   TestRtmCmd_SoloFloorNoObjective
Requirements Traceability Matrix: test-release
============================================================

Horizontal trace (need -> AC -> test -> proof)
------------------------------------------------------------
  Need N-01: First need for testing
    -> AC [S01-test-slice]: WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
       -> test: **Unit**: some test

Vertical trace (objective -> release benefit -> slice)
------------------------------------------------------------
  Objective: (none declared — solo/small-team floor)
  Release benefit: the release goal from index
  Release goal: The release goal text for testing.

  Slice S01-test-slice -> (floor: release goal)

All traces verified. 1 needs, 1 acceptance criteria, 1 tests, 1 slices.
--- PASS: TestRtmCmd_SoloFloorNoObjective (0.00s)
PASS
ok  github.com/swornagent/sworn/cmd/sworn  0.007s

$ go test ./internal/state/... -v -run TestTraceFields
=== RUN   TestTraceFieldsRoundTrip
--- PASS: TestTraceFieldsRoundTrip (0.00s)
PASS
ok  github.com/swornagent/sworn/internal/state  0.002s

$ go test ./internal/board/... -v -run TestParseVerticalTrace
=== RUN   TestParseVerticalTrace
=== RUN   TestParseVerticalTrace/both_fields_present
=== RUN   TestParseVerticalTrace/only_release_benefit
=== RUN   TestParseVerticalTrace/neither_field_(solo_floor)
--- PASS: TestParseVerticalTrace (0.00s)
    --- PASS: TestParseVerticalTrace/both_fields_present (0.00s)
    --- PASS: TestParseVerticalTrace/only_release_benefit (0.00s)
    --- PASS: TestParseVerticalTrace/neither_field_(solo_floor) (0.00s)
PASS
ok  github.com/swornagent/sworn/internal/board  0.003s

$ go test ./...
ok  github.com/swornagent/sworn/cmd/sworn  0.034s
ok  github.com/swornagent/sworn/internal/adopt  (cached)
ok  github.com/swornagent/sworn/internal/agent  (cached)
ok  github.com/swornagent/sworn/internal/bench  (cached)
ok  github.com/swornagent/sworn/internal/board  0.003s
ok  github.com/swornagent/sworn/internal/config  (cached)
ok  github.com/swornagent/sworn/internal/git  (cached)
ok  github.com/swornagent/sworn/internal/implement  (cached)
ok  github.com/swornagent/sworn/internal/model  (cached)
ok  github.com/swornagent/sworn/internal/prompt  (cached)
ok  github.com/swornagent/sworn/internal/rtm  (cached)
ok  github.com/swornagent/sworn/internal/run  (cached)
ok  github.com/swornagent/sworn/internal/state  (cached)
?  github.com/swornagent/sworn/internal/verdict  [no test files]
ok  github.com/swornagent/sworn/internal/verify  (cached)

$ go vet ./...
(clean)

$ gofmt -l internal/rtm/ cmd/sworn/rtm.go cmd/sworn/rtm_test.go cmd/sworn/main.go internal/board/index.go internal/board/index_test.go internal/state/state.go internal/state/state_test.go internal/adopt/adopt.go
(clean)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (command-line invocation)
- **User gesture**: "Run `sworn rtm <release>`; observe the printed matrix and exit code; introduce a deliberately orphaned need in the fixture intake; re-run; observe the named orphan and non-zero exit."

Evidence: the integration test `TestRtmCmd_FullyTracedRelease` in `cmd/sworn/rtm_test.go` drives the actual `cmdRtm` entry point (Rule 1) on a fixture release tree and asserts exit 0. `TestRtmCmd_OrphanedNeed` introduces an orphaned need and asserts non-zero exit. `TestRtmCmd_SoloFloorNoObjective` verifies the lightweight floor (no org objective, release goal present) passes.

Live smoke run against the actual release:

```
$ ./bin/sworn rtm 2026-06-16-fidelity-layer
Requirements Traceability Matrix: 2026-06-16-fidelity-layer
============================================================

Horizontal trace (need -> AC -> test -> proof)
------------------------------------------------------------
  (no needs found in intake.md)

Vertical trace (objective -> release benefit -> slice)
------------------------------------------------------------
  Objective: (none declared — solo/small-team floor)
  Release benefit: the fidelity layer — Baton Rules 8 (requirements), 9 (design), 10 (customer-journey
  Release goal: Baton today verifies **delivery against the spec** rigorously (Rule...

  Slice S01-rtm-spine -> (floor: release goal)
  Slice S02-ears-ac-format -> (floor: release goal)
  ... (15 slices, all on floor: release goal)

70 trace violation(s) found:
  [orphaned_ac_no_need] acceptance criterion in S01-rtm-spine cites no need id: ...
  ... (70 violations — all ACs lack need ids because the release was specced before the RTM existed)

exit=1
```

The non-zero exit is correct fail-closed behavior: the release's specs were written before the RTM existed and don't yet cite need ids. A future `/replan-release` would add need ids to the intake and cite them in specs. The RTM correctly identifies and names every broken trace.

## Delivered

- **AC1** (orphaned need fails) — evidence: `TestBuild_OrphanedNeed` in `internal/rtm/rtm_test.go` (passes); `TestRtmCmd_OrphanedNeed` in `cmd/sworn/rtm_test.go` (passes, asserts non-zero exit)
- **AC2** (orphaned AC fails — no need or no test) — evidence: `TestBuild_OrphanedAC_NoNeed` and `TestBuild_OrphanedAC_NoTest` in `internal/rtm/rtm_test.go` (pass); `TestBuild_AC_CitesNonExistentNeed` (passes — AC citing N-99 which doesn't exist)
- **AC3** (slice with no vertical link fails) — evidence: `TestBuild_SliceNoVertical` in `internal/rtm/rtm_test.go` (passes)
- **AC4** (fully-traced release exits 0 and prints matrix) — evidence: `TestBuild_FullyTraced` in `internal/rtm/rtm_test.go` (passes, 0 violations); `TestRtmCmd_FullyTracedRelease` in `cmd/sworn/rtm_test.go` (passes, asserts exit 0); `TestPrint_NonEmpty` (passes, matrix output contains both axes)
- **AC5** (builds from intake/spec/status/index alone — no datastore) — evidence: `rtm.Build()` in `internal/rtm/rtm.go` reads only `intake.md`, `index.md`, `spec.md`, `status.json` from the release directory; no database or external store is introduced
- **AC6** (solo floor: no objective, slice -> release goal accepted) — evidence: `TestBuild_SoloFloor_NoObjective` in `internal/rtm/rtm_test.go` (passes, 0 violations with no org objective); `TestRtmCmd_SoloFloorNoObjective` in `cmd/sworn/rtm_test.go` (passes, asserts exit 0)

## Not delivered

None. All six acceptance checks are delivered.

## Divergence from plan

The spec's "Planned touchpoints" list does not include `internal/adopt/adopt.go` or `internal/adopt/baton/README.md`, but both files received functional changes that are necessary corollaries of adding the new Rule 8 doc (`internal/adopt/baton/rules/08-requirements-fidelity.md`, which IS in the planned list):

- **`internal/adopt/adopt.go`** — Two functional changes: (1) the `//go:embed baton/rules/*` directive already covers the new file via the wildcard, but the `Materialise` function's explicit file list needed a new entry `{"baton/rules/08-requirements-fidelity.md", ...}` so the rule is written to consumer repos that run `sworn init`. Without this, the new rule would be embedded in the binary but never materialised to disk. (2) A minor `gofmt` whitespace fix (blank line after the import block). This file is the adoption mechanism — it is the bridge between the embedded Baton protocol and the consumer repo. The change is additive and region-separable from any other track's work.

- **`internal/adopt/baton/README.md`** — Added Rule 8 to the embedded Baton README's numbered rule index (the human-readable summary of all seven-now-eight rules). This is the documentation surface that `sworn init` writes to consumer repos. Without this entry, the README would list seven rules while the `rules/` directory contains eight, creating an inconsistency. The change is a 5-line additive block at the end of the rule list, region-separable.

- **`internal/state/state_test.go`** — The `gofmt -w` pass realigned struct literal fields because the new `NeedIDs`, `ReleaseBenefit`, and `OrgObjective` fields changed the longest field name in the `Status` struct. This is a cosmetic alignment change, not a functional divergence.

- **`internal/board/index_test.go`** and **`cmd/sworn/rtm_test.go`** — New test files for the board vertical-trace parsing and the `sworn rtm` command integration tests, respectively. These are test-only additions that follow from the planned touchpoints (`internal/board/index.go` and `cmd/sworn/rtm.go`); they are listed in `actual_files` but not `planned_files` because the spec lists only the implementation files, not their test files.

- The release-verify.sh dark-code marker check flags "deferred items" in `internal/adopt/adopt.go` when diffing against `main` — this is a false positive on pre-existing text in the Baton AGENTS.md fragment (Rule 5 text: "deferred items, next steps"). It is not a code deferral marker. Diffing against the correct base (`release-wt/2026-06-16-fidelity-layer`) produces a clean pass.

## First-pass script output

```
$ BASE_BRANCH=release-wt/2026-06-16-fidelity-layer $HOME/.claude/bin/release-verify.sh S01-rtm-spine 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S01-rtm-spine
  slice dir:   docs/release/2026-06-16-fidelity-layer/S01-rtm-spine
  base branch: release-wt/2026-06-16-fidelity-layer

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

== Diff vs release-wt/2026-06-16-fidelity-layer ==
  PASS  18 file(s) changed vs release-wt/2026-06-16-fidelity-layer
  (first 20)
    cmd/sworn/main.go
    cmd/sworn/rtm.go
    cmd/sworn/rtm_test.go
    docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/journal.md
    docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/proof.md
    docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json
    docs/release/2026-06-16-fidelity-layer/index.md
    internal/adopt/adopt.go
    internal/adopt/baton/README.md
    internal/adopt/baton/VERSION
    internal/adopt/baton/rules/08-requirements-fidelity.md
    internal/board/index.go
    internal/board/index_test.go
    internal/prompt/planner.md
    internal/rtm/rtm.go
    internal/rtm/rtm_test.go
    internal/state/state.go
    internal/state/state_test.go

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