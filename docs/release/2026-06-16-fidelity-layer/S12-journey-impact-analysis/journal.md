---
title: Slice journal — S12-journey-impact-analysis
description: Implementation log for S12 — per-release journey-impact analysis.
---

# Journal: S12-journey-impact-analysis

## Session log

### 2026-06-23 10:00 — session start / implementation

- **State**: `planned → in_progress → implemented`
- **Notes**:
  - Core implementation in `internal/journey/impact.go`: `AnalyzeImpact()` reads journeys artefact, scans release slice directories for `status.json`, collects `planned_files` + `actual_files`, matches against journey step/entry surfaces via heuristic `surfacesTouch()`.
  - Heuristic matching has 3 levels: direct substring, token-level, and conventional mapping (CLI ↔ cmd/). Biased toward over-inclusion per spec Risk mitigation.
  - CLI update in `cmd/sworn/journeys.go`: added `--impact <release>` flag and `cmdJourneysImpact()` function.
  - Integration tests in `cmd/sworn/journeys_impact_test.go` cover all 4 acceptance checks via CLI.
  - Rule 10 doc updated with "Impact analysis (S12)" section.
  - Usage string in `main.go` updated.
  - **Divergence from plan**: Created new file `internal/journey/impact.go` instead of modifying `journey.go` (S11's file). Created `cmd/sworn/journeys_impact_test.go` for separate CLI integration tests. Updated `cmd/sworn/main.go` for documentation (not in planned_files but necessary).
  - **Open deferral acknowledged**: Step→surface matching precision tracks S11's provisional journey schema, refined via `/replan-release`. Accepted from planner.

## Open questions

None.

## Deferrals surfaced

- Step→surface matching precision tracks S11 provisional schema, refined via `/replan-release` (acknowledged 2026-06-16 by planner).

## Verifier verdicts received

(Fresh-context verifier session will run separately — Rule 7.)