# S03-tui-live-board-refresh journal

## 2026-07-18 — hotfix slice planned

- Reproduction: an already-open TUI omitted S19-S22 from
  `2026-07-15-baton-v0.15-conformance`, while a direct
  `sworn board --release` query included them.
- Diagnosis: `internal/tui/tui.go` invokes `LoadReleases` once before starting
  Bubble Tea; no production path invokes it again. The S02 catalog record is
  therefore immutable for the entire TUI process rather than for one coherent
  refresh transaction.
- Ratified outcome: an operator can monitor in-flight releases without
  restarting. Refresh must remain asynchronous, non-overlapping, and sourced
  from the shared board catalog.
