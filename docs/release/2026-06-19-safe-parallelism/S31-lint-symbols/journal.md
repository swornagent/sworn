---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S31-lint-symbols`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fix §3a #3 ("grep the symbol", theme T-C,
~25 rows) from the trial-log analysis (`2026-06-21-captain-trial-log-harvest.md`).
Designs name a function/field/constant/table that does not exist or is the wrong one —
a guaranteed compile error or empty query if shipped. Evidence rows: `S04b-tui-live`
(`started_at` in the wrong table), `S30-fullstate-journey-snapshot` (wrong constant/
function names), `S05-drift-api` re-review (`LoadEnvelopeByID` did not exist),
`S16-other-asset-change-rate-engine` (`Calculate` vs `CalculateFIRE`).

**Rationale:** extract back-ticked identifiers from the design and grep them against the
live codebase; unresolved → advisory warning. Advisory (not hard fail) because the lint
cannot distinguish a symbol the slice introduces from a typo.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`); shares
the `internal/lint` package with S29/S30, serialised within T12.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.
