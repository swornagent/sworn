---
title: Slice journal
description: Implementation log for S14-journey-regression-suite. Append-only.
---

# Journal: `S14-journey-regression-suite`

## Session log

### 2026-06-26 — session start / implementation complete

- **State**: `planned → in_progress → implemented`
- **Notes**:
  - Added `HasRegression bool` and `RegressionTestPath string` fields to `journey.Journey` struct (backward-compatible, both `omitempty`).
  - Created `internal/journey/regression.go` — core codification logic:
    - `RegressionCoverageGaps()` — checks for walked-but-uncovered journeys
    - `CodifyJourney()` — generates a single journey's Go test scaffold
    - `CodifyWalkedJourneys()` — batch codification with accretion
  - Created `internal/journey/regression_test.go` — 10 tests covering all 4 acceptance checks.
  - Updated `cmd/sworn/journeys.go` — added `--regen <release>` flag and `cmdJourneysRegen()` handler.
  - Updated `internal/adopt/baton/rules/10-customer-journey-validation.md` — added full "Regression codification (S14)" section describing algorithm, coverage check, accretion semantics, and relationship to S13.
  - All existing tests pass (0 regressions).
  - `go vet ./...` clean.
  - **Design decision**: Scaffold output defaults to `tests/e2e/journeys/` — configurable via `outputDir` parameter in future. Chose `journey_<id>_test.go` naming for discoverability.
  - **Design decision**: `CodifyJourney` does NOT overwrite existing files — accretion is file-existence-gated, not flag-gated.
  - **Design decision**: The `--regen` command runs coverage check BEFORE and AFTER codification; gaps filled during the same run are reported as success. Remaining gaps after codification trigger a fail-closed exit 1.

### 2026-06-26 — re-entry after failed_verification — fix verifier violations

- **State**: `failed_verification → in_progress → implemented`
- **Verifier violations addressed**:
  1. **Gate 2** — `internal/journey/regression.go` missing from planned touchpoints / Divergence from plan: Added full Divergence explanation in proof.md (separate file justified by Go convention, mirroring existing `impact.go` / `walkthrough.go` pattern).
  2. **Gate 3** — No CLI integration test: Created `cmd/sworn/journeys_regen_test.go` with 4 CLI integration tests following the existing pattern (`cmdJourneys()` called as Go function with fixture artefacts, not compiled binary). Tests cover gap-filled, full-coverage, scaffold-emission, and un-walked-exclusion scenarios.
  3. **Gate 4** — Reachability artefact was unit tests only: Updated proof.md reachability artefact to reference the CLI integration tests (evidence type: `cli-integration-test`), and all 4 test outputs are captured in the proof bundle.
- **Notes**:
  - The forward-merge of release-wt into the T2 track worktree was required to pick up walkthrough/attestation types needed by the CLI integration tests.
  - `test_commands` in status.json updated to include the CLI integration test runner.
  - All 22 journeys tests pass (0 regressions), build + vet clean.

## Open questions
- None — deferred scaffold-completeness is already tracked in open_deferrals.

## Deferrals surfaced

- `Scaffold-not-complete-oracle`: sworn emits a structured starting test per journey + a coverage check, not a complete oracle. **Why** — a complete journey oracle is project-specific E2E work. **Tracking** — project E2E backlog per consuming project. **Acknowledged** — 2026-06-16 (from spec).
- `Provisional journey-schema detail`: the exact journey-schema fields were refined across S11 and may be further refined via /replan-release. **Acknowledged** — 2026-06-16 (from spec).

## Verifier verdicts received

### 2026-06-19 — Verifier verdict: FAIL

```
FAIL

Slice: `S14-journey-regression-suite`

Violations:
1. Gate 2 — `internal/journey/regression.go` (new file, 238 lines) is not in the
   planned touchpoints and is not mentioned in proof.md "Divergence from plan".
   Evidence: `git diff --name-only ad34339..HEAD` includes `internal/journey/regression.go`;
   spec.md "Planned touchpoints" lists `regression_test.go` but not `regression.go`.

2. Gate 3 — spec.md "Required tests" explicitly demands an integration test:
   "Integration: `sworn journeys --regen <fixture-release>` end-to-end; assert scaffold
   emission + the coverage-gap failure (Rule 1)." No file in `cmd/sworn/` covers the
   `--regen` path (only `journeys.go` was changed; no `journeys_regen_test.go` was
   created). The proof.md "Divergence from plan" acknowledges this absence but does NOT
   surface it as a full Rule 2 deferral (Why + Tracking + Ack all three are not present —
   tracking reference and human ack are missing). The rationale given ("requires full binary
   build + fixture setup") is also incorrect: existing `cmd/sworn/journeys_test.go` and
   `cmd/sworn/journeys_impact_test.go` call `cmdJourneys()` and `cmdJourneysImpact()` as
   Go functions without a compiled binary, establishing the pattern.

3. Gate 4 — Reachability artefact in proof.md substitutes package-level unit tests
   ("The unit tests demonstrate this smoke step programmatically") for the required CLI
   smoke run. The spec requires: "run `sworn journeys --regen <fixture>` for a walked
   journey with no test; observe the named gap; generate the scaffold; re-run; observe
   coverage." No CLI run was performed; no captured CLI output exists on disk. Per Rule 1,
   "a green typecheck plus green unit suite is not a reachability artefact." The
   referenced tests (`TestCodifyJourney_GeneratesScaffold`, `TestRegressionCoverageGaps_*`)
   call package functions at `internal/journey/`, not the `sworn journeys --regen` binary.

Required to address:
1. Add `internal/journey/regression.go` to proof.md "Divergence from plan" — note that
   the implementation was placed in a new dedicated file rather than inlined into
   `journey.go`, and explain why.
2. Add a CLI-level integration test in `cmd/sworn/` (e.g. `journeys_regen_test.go`)
   calling `cmdJourneysRegen(projectRoot, releaseName)` or
   `cmdJourneys([]string{"--regen", releaseName, projectRoot})` with a fixture journeys
   artefact + attestation that has at least one WalkPass entry. Assert: (a) scaffold file
   emitted at expected path, (b) exit 0 on full coverage, (c) exit 1 when a walked journey
   has no test. This follows the existing pattern in journeys_test.go and
   journeys_impact_test.go (no compiled binary needed).
3. Update proof.md "Reachability artefact" to reference the CLI integration test output
   (item 2 above), or capture actual `sworn journeys --regen <fixture>` CLI output to a
   file and reference that path. The current text does not satisfy Rule 1.
```