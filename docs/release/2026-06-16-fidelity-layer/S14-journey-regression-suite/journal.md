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

## Open questions

- None — deferred scaffold-completeness is already tracked in open_deferrals.

## Deferrals surfaced

- `Scaffold-not-complete-oracle`: sworn emits a structured starting test per journey + a coverage check, not a complete oracle. **Why** — a complete journey oracle is project-specific E2E work. **Tracking** — project E2E backlog per consuming project. **Acknowledged** — 2026-06-16 (from spec).
- `Provisional journey-schema detail`: the exact journey-schema fields were refined across S11 and may be further refined via /replan-release. **Acknowledged** — 2026-06-16 (from spec).

## Verifier verdicts received

None yet.