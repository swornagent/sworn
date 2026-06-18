---
title: Slice journal S11-journey-elicitation
description: Implementation log for the journey model, CLI, and gate.
---

# Journal: S11-journey-elicitation

## Session log

### 2026-06-20 10:00 — initial implementation

- **State**: planned → in_progress → implemented
- **Notes**:
  - Created `internal/journey/journey.go` with the journey model (Journey, JourneyStep, JourneyArtefact) and functions for Load, Save, Check, DraftTemplate
  - Created `internal/journey/journey_test.go` with 14 tests covering all acceptance checks
  - Created `cmd/sworn/journeys.go` implementing the `sworn journeys` CLI command with `--check` flag
  - Created `cmd/sworn/journeys_test.go` with 9 integration tests
  - Updated `cmd/sworn/main.go` adding `case "journeys"` to the switch and usage text
  - Created `internal/adopt/baton/rules/10-customer-journey-validation.md` — Rule 10 rule doc
  - Updated `internal/adopt/baton/VERSION` to include Rule 10
  - Updated `internal/adopt/adopt.go` to materialise Rule 10's rule doc
  - Updated `internal/prompt/planner.md` with journey elicitation guidance section
  - **Trade-off**: DraftTemplate scans file system for now; model-assisted AI draft deferred as provisional per spec
  - **Trade-off**: Journeys artefact at `.sworn/journeys.json` (JSON, version-controlled) following sworn config pattern
  - **No subagent dispatches** — single implementer session

## Open questions

- None — all spec acceptance checks delivered.

## Deferrals surfaced

- **Provisional journey-artefact schema fields** — Why: The exact schema (step granularity, how a step references slices/surfaces) is provisional per spec. Tracking: status.json open_deferrals. Acknowledged: 2026-06-16 by planner.

## Verifier verdicts received

- *Pending — no verifier session yet.*