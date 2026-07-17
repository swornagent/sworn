# Design TL;DR — S02-tui-ref-aware-release-navigation

**Slice:** S02-tui-ref-aware-release-navigation · **Track:** T1-ref-aware-board · **Release:** 2026-07-17-ref-aware-board-discovery  
**State:** design_review (Rule 9 gate — no production code written)  
**User outcome:** A SwornAgent TUI operator can select and open the same ref-aware catalog board that `sworn board --json` reports, with explicit uncommitted evidence markers.

## 1. Approach

1. Move TUI release-list discovery from a filesystem-only source to the S01 catalog record shape:
   - In `internal/tui/releases.go`, keep list rendering intact but make each entry a catalog-backed release record (id, sourceRef, sourceLabel, per-slice evidence).
   - Keep ordering bytewise by release id and preserve current loading/error UX.
2. Thread catalog-sourced state through the Enter flow instead of re-resolving board state:
   - In `internal/tui/model.go`, when Enter is pressed on a release entry, include that entry’s catalog snapshot and `sourceRef` in the async board-load command payload.
   - Keep async cancellation/staleness guards keyed by `(release, sourceRef)` so a delivered result from a different ref cannot overwrite current selection.
3. Make board rendering consume the catalog snapshot verbatim:
   - In `internal/tui/board.go`, add a `LoadFromCatalog` path that hydrates tracks/slices from `board.BoardState` on the selected catalog entry, including `stateSource`, `stateDurability`, `lastUpdated`, and verification fields directly from the oracle data.
   - Remove fallback resolution paths after entering the command (no `board.ReadBoard`, no `ReleaseRefFor`, no status re-election, no path-based re-read).
4. Surface durability as text and list-level aggregate marker:
   - In `internal/tui/model.go` + `internal/tui/releases.go`, append ` [uncommitted]` to release list labels only when selected evidence is `uncommitted`.
   - In `internal/tui/board.go`, render `[uncommitted]` beside slice entries with `stateDurability == uncommitted` using text, not color alone.
5. Preserve failures and non-Git behavior:
   - If catalog discovery errors, keep existing TUI error surface semantics and avoid partial board/list state.
   - In non-Git/no-HEAD startup, render S01-provided filesystem fallback records as uncommitted without introducing a local parser.

## 2. Design choices and review pins

- **PIN-1 — single-catalog authority.** The TUI becomes a strict consumer of S01 catalog records; it must not re-rank candidates or call any independent resolver after it receives a snapshot.
- **PIN-2 — selection identity key.** Async board loads are only applied when both `release` and `sourceRef` match the currently selected release; mismatched-ref results are discarded.
- **PIN-3 — textual provenance signalling.** Uncommitted visibility is textual only and must use the exact suffix ` [uncommitted]`.
- **PIN-4 — deterministic error surface.** Any catalog error should flow through existing root error handling and set `Error: ...` view output unchanged.

## 3. Files to touch

| File | Planned responsibility | ACs |
|---|---|---|
| `internal/tui/releases.go` | Consume catalog release records for list display and maintain existing navigation/loading behavior | AC-01, AC-03, AC-04 |
| `internal/tui/model.go` | Carry selected catalog payload, guard async result identity, and keep list aggregate uncommitted suffix | AC-01, AC-02, AC-03, AC-04 |
| `internal/tui/board.go` | Populate board tracks/slices from catalog snapshot and show `[uncommitted]` markers only from `stateDurability` | AC-01, AC-02, AC-04 |
| `internal/tui/tui_test.go` | Extend/add integration-level tests for catalog-driven list/board parity and stale-result protection | AC-01, AC-02, AC-03, AC-04 |

## 4. Risks / tradeoffs

- **R1: stale async delivery race.** Stale results from earlier releases or refs can overwrite visible state unless selection-aware guards are enforced; this is pinned to the `(release, sourceRef)` guard.
- **R2: hidden fallback behavior.** Non-Git mode can look successful with local fallback and silently drop errors; all failure paths should remain hard errors with existing UI messaging.
- **R3: accidental authority duplication.** Re-reading local files/status after catalog receive would diverge from CLI behavior and break parity.

## 5. AC traceability

- AC-01/AC-03: covered by release list and fallback/error tests.
- AC-02: covered by `stateDurability` rendering assertions and release aggregate suffix assertions.
- AC-04: covered by keyboard navigation and stale-result sourceRef rejection tests with explicit command return-state assertions.

## 6. Effort/complexity confirmation

The slice stays at **low effort / high complexity / puzzle** as planned in spec: small file surface, but subtle correctness on async sourceRef coupling and evidence rendering parity.
