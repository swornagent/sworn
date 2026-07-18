# Design TL;DR — S03-tui-live-board-refresh

**Slice:** S03-tui-live-board-refresh · **Track:** T2-tui-live-refresh · **Release:** 2026-07-17-ref-aware-board-discovery  
**State:** design_review (Rule 9 gate — no production code written)  
**User outcome:** An operator can leave the TUI open while releases change and see the releases pane and selected board move together to the newest completed shared-catalog snapshot without losing selection.

## 1. Approach

1. Separate catalog conversion from discovery in `internal/tui/releases.go`:
   - Add a pure helper that converts one complete `[]board.CatalogRecord` into a sorted `[]ReleaseInfo`, returning `ErrNoReleases` for an empty catalog.
   - Keep `LoadReleases` as the synchronous startup adapter, but make both startup and background refresh use the same conversion path.
2. Add a distinct serial refresh protocol in `internal/tui/model.go`:
   - Define dedicated schedule/due/result message types; do not reuse `tickMsg` or `logTickMsg`.
   - `Model.Init` schedules generation 1 after a package-level refresh interval. The due message starts one asynchronous `board.DiscoverCatalog` command only when its generation is still current and no discovery is in flight.
   - An accepted result clears the in-flight marker, applies or reports the complete result, increments the generation, and returns exactly one command scheduling the next cycle. Stale/duplicate messages are ignored without changing UI state or scheduling another chain.
   - Use a five-second delay *after completion*, not a fixed wall-clock ticker. In the current repository discovery takes roughly 8.6 seconds, so this avoids continuous overlapping Git work while still converging an open session in about one discovery duration plus five seconds.
3. Apply one successful snapshot atomically:
   - Before replacement, capture the release-list selection by release ID and the selected board slice by slice ID.
   - Replace the complete releases value, restore the release cursor by ID, and clamp to the prior index only when that ID disappeared.
   - Locate `Board.ReleaseName` in that same releases value. Hydrate a replacement `BoardView` directly from its `CatalogRecord`, then preserve presentation-only state that catalog refresh does not own: `SortMode`, matching slice cursor, and existing gate results/loading flags for still-present slices.
   - If the board release disappeared, clear the board. Only an operator currently in `viewBoard` is returned to `viewReleases`; live/log/blocked/settings view state is not reinterpreted by this slice.
4. Isolate background refresh failures:
   - Keep a refresh-specific error field rather than clearing unrelated key/gate/live errors in `errMsg`.
   - Render the refresh error through the same `Error: ` root presentation. Retain the last good releases and board values, then re-arm the serial refresh. A later accepted success clears only the refresh error.
5. Start the lifecycle from `internal/tui/tui.go` through the existing `Model.Init` path; startup discovery remains synchronous so the first frame retains current behavior.

## 2. Design choices and Captain pins

- **PIN-1 — no overlap by construction.** A completion schedules the next delay; a periodic ticker is forbidden. Generation plus in-flight state rejects injected duplicate/stale messages without spawning a second discovery.
- **PIN-2 — one catalog, one transition.** List and selected board are derived from the same result payload and installed in one `Model.Update` call. Board refresh must not call `DiscoverCatalog`, `LoadBoard`, or another status resolver.
- **PIN-3 — identity, not index.** Release and slice selections are restored by ID. The previous numeric index is only a deterministic clamp fallback when an identity vanished.
- **PIN-4 — preserve non-catalog UI state.** Sort mode and already-loaded gate decorations survive for matching slices; catalog refresh neither recomputes nor silently discards gates.
- **PIN-5 — error ownership.** Refresh recovery clears only its own error. Existing `errMsg` values remain untouched, avoiding a successful poll masking an unrelated operator-visible failure.
- **PIN-6 — cadence is completion-relative.** Five seconds is a reversible constant, deliberately separate from discovery duration and from the one-second live/log tick chains.

## 3. Files to touch

| File | Planned responsibility | ACs |
|---|---|---|
| `internal/tui/tui.go` | Keep startup load intact and ensure the root model owns the automatic lifecycle | AC-01 |
| `internal/tui/model.go` | Refresh messages/commands, generation and in-flight guards, atomic apply, selection restoration, error rendering, re-arm | AC-01, AC-02, AC-03, AC-04 |
| `internal/tui/releases.go` | Pure complete-catalog conversion shared by startup and refresh | AC-01, AC-03, AC-04 |
| `internal/tui/tui_test.go` | Deterministic message-driven coverage without sleeps or real-time polling | AC-01, AC-02, AC-03, AC-04 |

## 4. AC traceability

- **AC-01:** a synthetic accepted result containing `S02-new` asserts list aggregate and selected board change in the same update, with exactly one returned re-arm command; an injected discover function proves no second resolver call.
- **AC-02:** due/result messages with duplicate and older generations prove only one discovery command is issued and stale delivery changes neither values nor errors; existing live/log tick forwarding tests are extended with refresh messages.
- **AC-03:** insertion/reorder fixtures assert release and slice identity restoration; removal fixtures assert deterministic clamp, board clear, and `viewBoard → viewReleases` without panic.
- **AC-04:** an error result asserts byte-for-byte-equivalent last-good model values and visible `Error: ` output plus one re-arm; the next successful result clears only the refresh error and installs the new snapshot.

## 5. Risks / trade-offs

- **R1: Bubble Tea command concurrency.** `tea.Batch` can execute commands concurrently, so overlap prevention must live in model state before a discovery command is returned, not inside the command itself.
- **R2: pointer aliasing across snapshots.** `ReleaseInfo.Catalog` values must point to stable per-record copies; conversion tests should ensure later loop iterations or mutations do not alias entries.
- **R3: board hydration side effects.** `LoadBoardFromCatalog` currently also reads active-merge decorations and resets gate state. Atomic apply must explicitly preserve non-catalog presentation state and must not introduce another catalog/status election.
- **R4: disappearing releases in alternate views.** The slice clears stale board identity but deliberately changes navigation only from `viewBoard`; broader live/log lifecycle policy is outside this catalog hotfix.

## 6. Effort/complexity confirmation

The slice remains **high effort / high complexity / beast**: only four production/test files are in scope, but correctness depends on a serial asynchronous protocol, generation rejection, atomic multi-component replacement, identity-preserving navigation, and independent error/tick lifecycles.
