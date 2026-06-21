---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S30-lint-touchpoints`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fixes §3a #2 and #4 (theme T-A, ~35 rows —
the most common Captain-catch class) from the trial-log analysis
(`2026-06-21-captain-trial-log-harvest.md`). Designs repeatedly touch files/packages
they never declared in `planned_files`, or collide with another slice across tracks.
Evidence rows: `S02b-concurrent-scheduler` (undeclared `internal/board/`),
`S10-buy-property-action-form` (missing `fire-validation` package),
`S18-income-path-action-wiring` (`projection_init.go` + `types_shared.go` absent),
`S16-sankey-any-year` (5 files missing), `S17-offset-cascade-target` (S27 collides on
all 4 core files, unacknowledged); migration-number collision S13↔S17.

**Rationale:** mechanise touchpoint reconciliation — parse the design's referenced
files/packages, reconcile against planned_files AND the index.md collision matrix, and
detect duplicate migration numbers — so the dominant defect class is caught before code.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`). Shares
the `internal/lint` package with S29/S31; serialised within T12, no parallel collision.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.
