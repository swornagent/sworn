---
title: 'Slice spec — S72-tui-gate-display'
description: 'Extend the sworn TUI to display per-slice gate results — trace status, coverage %, design violations, architecture findings — alongside the existing track/slice state display.'
---

# Slice: S72-tui-gate-display

## User outcome

A developer opens the sworn TUI (`sworn`) and sees, for each slice in the release board, the gate status: whether lint checks passed, coverage percentage, design violations count, and LLM check results. A slice that mechanically passes but has 3 architecture violations is visually distinct from a clean slice.

## Entry point

Extends `internal/tui/` — specifically the board/slice views built by S04a/S04b/S04c. New `internal/tui/gate.go` handles gate result formatting. CLI registration via `internal/command` registry.

## In scope

- Display per-slice gate results in the TUI board view:
  - Trace: PASS/FAIL badge
  - Coverage: percentage (covered/total ACs)
  - Design: violation count
  - Mock: clean/flagged
  - LLM: latest check result (if run)
- Colour coding: green (clean), yellow (warnings), red (violations)
- Compact inline format — doesn't break the existing table layout
- Reads gate results from the sworn lint commands (S65-S70) or cached results from DB

## Out of scope

- Running gate checks from the TUI (display only — checks run via CLI or agent loop)
- Historical gate result tracking (that's the verdict ledger, T16)

## Planned touchpoints

- `internal/tui/gate.go` (new)
- `internal/tui/gate_test.go` (new)
- `cmd/sworn/top.go` (extend — wire gate display into board view)

## Acceptance checks

- [ ] Per-slice gate status visible in TUI board view
- [ ] PASS/FAIL/coverage %/violation count displayed compactly
- [ ] Colour coding distinguishes clean from flagged slices
- [ ] TUI remains responsive at 1s polling with gate data
- [ ] Slices without gate results show "not checked" neutral state

## Required tests

- **Unit**: `internal/tui/gate_test.go` — gate result formatting with fixture data
- **Reachability artefact**: Screenshot of TUI showing per-slice gate status
- **E2E gate type**: local
