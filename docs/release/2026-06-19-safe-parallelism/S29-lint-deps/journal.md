---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S29-lint-deps`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fix §3a #1 (theme T-B) from the Captain
design-review trial-log analysis (`2026-06-21-captain-trial-log-harvest.md`). A
slice that adds a Go dependency without declaring `go.mod`/`go.sum` in `planned_files`
trips Gate 2 at verify. Evidence rows: `S04a-tui-foundation` (bubbletea + lipgloss →
Gate 2 FAIL risk), `S08b-mcp-ops-tools` (yaml.v3 claimed in go.sum but absent), and
`S31-newrelic-windout-backend` (fired; go.sum diff-review step absent). The fix is a
`sworn lint deps` check that diffs go.mod/go.sum against the slice's planned_files and
fails closed, paired with a planner note to auto-add those files on any dep change.

**Rationale:** mechanise the most-deferred dependency-declaration check so it runs
pre-verify rather than surfacing as a late Gate-2 diff failure.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`) with the
other harvested harness-hardening lints (S30, S31, S32, S33, S35).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.
