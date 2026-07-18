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

## 2026-07-18 — implementation resumed after Captain review

- The repository owner acknowledged `DECISION: PROCEED` and all four inline
  pins from `review.md`.
- Declared `internal/tui/board.go` before editing it, as authorised by PIN-1,
  because pure catalog-only board hydration belongs beside the existing board
  conversion code. The refresh path will preserve presentation state without
  calling `LoadBoardFromCatalog`, `ActiveMerges`, or another resolver.
- The shared catalog remains the sole list-and-board authority: one accepted
  discovery result will replace both values in one root-model transition.

## 2026-07-18 — stable implementation and proof checkpoint

- Committed and pushed the semantic implementation at `c99972fb66e2fab159746f292afb4fc0a31c95a1`.
- Required package tests, the repository-wide suite, vet, formatting, 4/4 AC
  coverage, mock lint, and the AC-satisfaction LLM check passed from live state.
- The Rule 6 proof-bundle gate returned `PASS` with exit code 0 and zero model
  cost. The committed terminal frames visibly demonstrate before, after, and
  deterministic refresh-error states.
