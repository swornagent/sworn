# Design TL;DR — S01-tui-bounded-navigation

**Slice:** S01-tui-bounded-navigation · **Track:** T1-tui-bounded-navigation · **Release:** 2026-07-18-tui-bounded-navigation  
**State:** design_review (Rule 9 gate — no production code written)  
**User outcome:** A SwornAgent operator can launch the TUI in a project with many releases, load only the recent catalog depth they need, scroll the release and slice panes inside the current terminal height, resize without losing selection, and move between panes with Right/Left or Enter/Esc.

## 1. Approach

1. Bound catalog materialisation in `internal/board/discovery.go` without changing the existing complete API:
   - Introduce a bounded result containing `Records []CatalogRecord` and `HasOlder bool`, with a positive newest-release limit.
   - Keep ref/path enumeration global so release IDs can be ranked deterministically, but select the newest IDs by the existing bytewise order before assembling object reads or validating/electing topology and status objects.
   - Route bounded and unbounded callers through one ranking/election core; `DiscoverCatalog` remains the complete CLI/automation contract and preserves ascending output.
2. Extend the root catalog transaction in `internal/tui/model.go` and startup in `internal/tui/tui.go`:
   - Prime exactly 10 newest releases and retain `desiredCatalogLimit`, accepted limit, `HasOlder`, and the existing `uint64` refresh generation as request/result identity.
   - Treat lowercase `o` in release focus as a non-blocking request to grow the desired depth by 10. Never overlap discovery: an in-flight transaction records the larger pending depth and completion immediately services it before the five-second refresh is re-armed.
   - Accept a result only when its generation is current and its positive limit is not smaller than the retained desired depth. Successful results atomically replace list and selected board from one snapshot while restoring release and slice IDs; failures preserve the last good snapshot and requested depth.
3. Add cursor-relative render windows to `ReleasesList` and `BoardView`:
   - Give both components the same non-negative content height derived by the root from measured header, help, error, separator, border, and padding rows.
   - `ReleasesList` renders its title, the rows surrounding `Cursor`, and one stable footer state (`o older`, `loading older`, or `all releases loaded`) when height permits.
   - `BoardView` builds a display-line model from `displayTracks`, including track headers, maps `orderedSlices[Cursor]` to its line, and renders the cursor-relative body window without changing declaration/dependency ordering, gates, or durability markers.
4. Make root rendering and resize handling height-safe:
   - Preserve the established height-zero legacy fallback.
   - For every positive height, measure fixed chrome with Lip Gloss, clamp the pane allocation and style dimensions, and clip alternate views/final assembly where needed so `lipgloss.Height(Model.View())` cannot exceed the terminal height.
   - Recompute windows from the unchanged release ID, slice ID, desired depth, and focus state after each `tea.WindowSizeMsg`.
5. Unify spatial and existing keyboard transitions:
   - Route Right and Enter through one selected-release async-open helper; route Left and Esc through one board-to-releases helper.
   - Select accent versus neutral pane borders from focus state and render state-specific help. Board-focus `o` continues to call the existing `handleBoardKey` / `BoardView.ToggleSort` path.

## 2. Design choices and review pins

- **PIN-1 — bounded means bounded object reads.** Selecting ten rows after complete topology/status materialisation is forbidden; the release-name window must be fixed before `ReadObjects`, parsing, validation, or election for excluded releases.
- **PIN-2 — one discovery core.** Bounded and unbounded APIs share candidate ranking and election logic so the aggregate CLI contract cannot drift from the TUI window.
- **PIN-3 — monotonic desired depth.** A smaller completed request may never replace a newer desired depth. Generation and positive limit travel together, with at most one discovery command active.
- **PIN-4 — atomic snapshot identity.** Releases and the selected board are installed from the same bounded catalog result, and selections are restored by release/slice ID rather than numeric index.
- **PIN-5 — rendered-line board window.** Track headers participate in height calculations but not logical slice order; the selected slice-to-line mapping is rebuilt from the current `displayTracks` order after sorting or resize.
- **PIN-6 — measured height budget.** Root chrome is measured from rendered strings, not hardcoded deductions. All derived dimensions are clamped before they reach Lip Gloss.
- **PIN-7 — shared transitions.** Arrow aliases call the same transition helpers as Enter/Esc; no parallel loading, staleness, live-view, or cursor semantics are introduced.
- **PIN-8 — tiny-height priority.** For extremely small positive heights, preserve boundedness and the focused selected row when possible; clip non-essential help/error/header chrome before allowing a pane dimension to become negative or the frame to exceed the terminal.

## 3. Files to touch

| File | Planned responsibility | ACs |
|---|---|---|
| `internal/board/discovery.go` | Bounded catalog result/API, newest-ID windowing before object reads, shared bounded/unbounded core | AC-01 |
| `internal/board/discovery_test.go` | Real-Git fixtures proving bounded reads, order, deferred excluded validation, and unbounded compatibility | AC-01 |
| `internal/tui/tui.go` | Prime the root with the exact 10-release bounded snapshot | AC-02 |
| `internal/tui/model.go` | Desired-depth protocol, generation+limit staleness, atomic apply/retry, resize height allocation, shared key transitions, focus help/style | AC-02, AC-03, AC-04, AC-05 |
| `internal/tui/releases.go` | `HasOlder`/loading state and cursor-relative release rows/footer | AC-02, AC-04 |
| `internal/tui/board.go` | Cursor-relative rendered-line window across track headers and slices | AC-04, AC-05 |
| `internal/tui/styles.go` | Focus-aware pane borders and clamped height helpers | AC-04, AC-05 |
| `internal/tui/tui_test.go` | Root `Update`/`View` journeys for bounded loading, concurrency, scrolling, resize, height, aliases, help, and sort preservation | AC-02, AC-03, AC-04, AC-05 |

## 4. AC traceability

- **AC-01:** 25-release temporary repositories exercise limits 10 and 20, ascending selected output, `HasOlder`, object-read confinement, deferred malformed excluded topology, included fail-closed validation, and unchanged complete `DiscoverCatalog`/CLI tests.
- **AC-02:** a root constructor fixture proves exactly 10 newest records and `o older`; message-driven `o` tests prove immediate return, `loading older`, atomic 20-record install with ID preservation, and no command once `all releases loaded`.
- **AC-03:** injected deterministic discovery commands prove one in-flight transaction, retained maximum desired depth, rejection/supersession of obsolete smaller results, completion-relative scheduling, and error retention/retry without sleeps.
- **AC-04:** oversized release/track fixtures drive repeated Up/Down and positive `WindowSizeMsg` values through root `Update`/`View`; assertions cover reachability, cross-track movement, cursor visibility, selection/depth/focus preservation, and `lipgloss.Height(view) <= height` including tiny heights.
- **AC-05:** paired Right/Enter and Left/Esc journeys assert equivalent commands/state, focused accent border and exact help labels, while the existing board `o` sort path and full regression commands remain green.

## 5. Risks / trade-offs

- **R1: object-read batching currently happens before release ranking.** The discovery refactor must retain canonical topology-skew semantics for included releases while ensuring excluded releases contribute no topology/status object specs.
- **R2: Bubble Tea commands may execute concurrently.** The model must claim in-flight ownership before returning a command and carry desired depth independently from the dispatched depth.
- **R3: shrinking windows can confuse visual and logical order.** Release cursors index records, board cursors index slices, and only the render window maps those identities to visible lines.
- **R4: ANSI-aware height differs from string line counting.** All frame assertions and allocations use Lip Gloss height; clipping happens at rendered-line boundaries.
- **R5: focus styling can change border geometry.** Accent and neutral styles must differ only in colour, not padding/border shape, so focus changes cannot perturb width or height.
- **R6: alternate views have separate renderers.** The positive-height root guarantee may require final clipping for live/log/blocked/settings states, while new scrolling behavior remains confined to release and board panes as specified.

## 6. Effort/complexity confirmation

The slice remains **low effort / high complexity / puzzle** as planned: the write surface is eight files and one operator journey, but bounded Git-object work, generation-plus-depth concurrency, ANSI-aware frame accounting, and two identity-preserving render windows are tightly coupled correctness constraints.
