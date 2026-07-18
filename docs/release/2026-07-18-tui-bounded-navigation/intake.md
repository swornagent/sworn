---
title: 'Release intake: TUI bounded navigation'
description: 'Planning record for lazy release loading, height-bounded scrolling, resize reflow, and directional pane navigation.'
---

# Release Intake: `2026-07-18-tui-bounded-navigation`

## Release goal

Keep the SwornAgent TUI usable and quick to enter in projects with many release
boards or many slices. A shipped release primes a bounded newest-first subset of
release boards, lets the operator request older releases when needed, constrains
the release and board regions to the current terminal height, and keeps the
highlighted row reachable as the terminal is resized. The operator can move
from the highlighted release into its board with the right arrow and return
with the left arrow, while the existing Enter and Esc gestures remain valid.

## Needs

- N-01: **Bounded release priming.** Startup and background refresh do not
  materialise every release board in a large repository; the operator initially
  receives a small newest-first batch and can request older batches on demand.
- N-02: **Terminal-height ownership.** The complete root TUI frame never
  renders taller than the latest terminal height, including its header, pane
  borders, status or error lines, and help bar.
- N-03: **Independent visible scroll regions.** The release list and board
  slice list each render only the rows that fit their assigned content height,
  scroll around their own cursor, and keep the selected row visible.
- N-04: **Resize-safe reflow.** Every `tea.WindowSizeMsg` recomputes the pane
  heights and visible windows without losing or silently changing the selected
  release or slice.
- N-05: **Directional pane navigation.** From the release list, Right opens and
  focuses the highlighted release board; from the board, Left returns focus to
  the release list. Enter and Esc remain equivalent aliases.

## Source of truth

- **Human stakeholder**: repository owner / TUI operator
- **Tracking issue**: [sworn#125](https://github.com/swornagent/sworn/issues/125)
- **Preceding release**:
  `docs/release/2026-07-17-ref-aware-board-discovery/`, merged into
  `release/v0.2.0` at `3788a5657bd4134587f03cefb861357fe5bd4195`
- **Related captures**: none; the human supplied the live failure description
  during planning.

## Users and their gestures

- **TUI release operator**: launches `sworn` in a project with many releases,
  sees a bounded recent list promptly, moves with Up/Down, requests older
  releases when needed, presses Right or Enter to focus the selected board,
  moves through its slices with Up/Down, and presses Left or Esc to return.
- **TUI release operator resizing a terminal**: grows or shrinks the terminal
  and sees the frame immediately reflow to the new height while the selected
  release or slice remains selected and visible.

## What's currently broken or missing

- The human reported: "if there is a big list of releases, it renders all of
  them but doesn't fix the size of the viewport". The complete frame grows
  beyond the terminal, so the header is drawn above the reachable screen.
- Startup and periodic catalog refresh materialise every locally available
  release even though the operator normally needs only the newest few.
- `ReleasesList.View` renders every release row and `BoardView.View` renders
  every track and slice row. Neither owns a height or scroll offset.
- `Model.View` uses the latest terminal width for pane sizing, but does not
  allocate the latest terminal height across the header, panes, errors, and
  help bar.
- Enter opens a release board and Esc returns, but the left/right arrows do not
  express the visible two-pane relationship.

## What the human wants

- Prime only the latest five or ten releases rather than loading every release
  in the project.
- Provide an explicit way to load older releases when needed.
- Cap the rendered TUI at the current terminal height and reflow dynamically on
  terminal resize.
- Add scroll regions to both the release list and the board slice list.
- Add Right from the release list into the highlighted board and Left from the
  board back to the release list, alongside the existing Enter and Esc keys.
- Keep this work as one small slice in one track and one new release.

## Constraints and non-negotiables

- Sworn remains a native Go binary with no new runtime dependency.
- The TUI remains a consumer of the shared `board.DiscoverCatalog` authority;
  paging must not create a second topology or state-evidence election rule.
- Existing source-ref identity guards, fail-closed catalog errors, selected-ID
  preservation, and non-overlapping live catalog refresh remain intact.
- All gestures remain keyboard-operable. The selected row and focus location
  must be communicated by the existing textual/highlighted TUI affordances;
  browser-specific ARIA semantics do not apply to this terminal surface.
- No network, credential, personal-data, persistence, legal, or regulatory
  surface is introduced.

## Adjacent / out of scope

- **TUI palette, typography, and width-ratio redesign**: deferred because this
  release fixes bounded navigation and performance, not visual identity.
  **Tracking**: [sworn#125](https://github.com/swornagent/sworn/issues/125).
  **Acknowledged**: repository owner through the one-slice scope direction,
  2026-07-18.
- **Mouse-wheel, click, or touch input**: deferred because the requested and
  existing TUI contract is keyboard navigation. **Tracking**:
  [sworn#125](https://github.com/swornagent/sworn/issues/125).
  **Acknowledged**: repository owner through the one-slice scope direction,
  2026-07-18.
- **Changing aggregate `sworn board` CLI output or discovery correctness**:
  deferred because CLI callers still require the complete catalog; the bounded
  behaviour is a TUI read contract. **Tracking**:
  [sworn#125](https://github.com/swornagent/sworn/issues/125).
  **Acknowledged**: repository owner through the one-slice scope direction,
  2026-07-18.

## Decisions made during planning

### 2026-07-18 — isolate the update in a new single-slice release

- **Context**: the preceding ref-aware discovery release is already merged and
  the new defect concerns the scale and navigation of its TUI consumer.
- **Options considered**: reopen the merged release; make an untracked direct
  patch; create a new small release with one slice and one track.
- **Decision**: plan `2026-07-18-tui-bounded-navigation` with one slice,
  `S01-tui-bounded-navigation`, in `T1-tui-bounded-navigation`.
- **Why**: it preserves the prior release's verified history while keeping the
  tightly coupled catalog paging, viewport allocation, scrolling, and focus
  gestures in one independently verifiable user journey.

### 2026-07-18 — add directional focus gestures without removing aliases

- **Context**: the two-pane layout visually implies horizontal navigation, but
  currently only Enter enters the board and Esc returns to the release list.
- **Options considered**: replace Enter/Esc; retain only Enter/Esc; add
  Right/Left as state-specific aliases.
- **Decision**: Right from the release list follows the same load/open path as
  Enter, and Left from the board follows the same return path as Esc. Existing
  gestures remain supported.
- **Why**: directional navigation matches the spatial model without breaking
  the learned keyboard contract.

### 2026-07-18 — use a growing ten-release catalog window

- **Context**: the human delegated an initial batch of five or ten releases and
  required a way to reach older releases. The existing board `o` binding is
  state-specific and therefore does not conflict on the release-list screen.
- **Options considered**: five releases with automatic loading at the list end;
  ten releases with explicit `o` loading; ten releases with PageDown loading.
- **Decision**: load the ten newest release IDs initially and grow the requested
  catalog depth by ten whenever the operator presses `o` in the release list.
  The release help bar labels the action `o older`; the board continues to label
  the same state-specific key `o order`.
- **Why**: ten gives useful recent context without materialising the project,
  while an explicit, labelled action makes extra Git work predictable.

### 2026-07-18 — preserve requested depth across refresh and measure chrome

- **Context**: the existing five-second refresh chain atomically replaces the
  current catalog, and header/help/error rows change the height left for panes.
- **Options considered**: reset to ten on every refresh; append stale older
  records forever; retain a requested depth and re-query that bounded window.
- **Decision**: retain the current requested depth across background refresh,
  reject a result for an obsolete smaller depth, and continue preserving
  release/slice selection by identity where the refreshed window still contains
  it. Compute available pane height from the actual rendered header, help, error,
  separator, padding, and border rows on every render rather than hardcoding one
  terminal profile.
- **Why**: this preserves S03's one-snapshot authority and non-overlap guarantee,
  avoids reintroducing unbounded retained data, and remains correct when help or
  errors add rows.

### 2026-07-18 — retire S01 through rollback and corrected replacement

- **Context**: fresh verification found that S01's root scrolling journey
  stopped at release 19 of 25 and slice 13 of 18, so it did not prove every
  loaded row was reachable. Its cycle-0 maintainability PASS had already frozen
  semantic scope, making an edit under the same slice ID illegal.
- **Options considered**: reopen the frozen S01 scope; waive the missing endpoint
  evidence; preserve S01 as terminal history, verify an exact rollback, then
  implement a corrected replacement on the clean baseline.
- **Decision**: the repository owner's `/replan-release` invocation ratifies the
  mandatory third option. Mark S01 deferred with terminal
  `re_slice_required`; add `S02-tui-bounded-navigation-rollback`; then add
  `S03-tui-bounded-navigation-replacement` in the same track.
- **Why**: the sequence preserves the append-only review ledger, prevents failed
  bytes from becoming the replacement baseline, and makes the previously absent
  endpoint proof an explicit acceptance criterion.

## Schema-vs-spec audit notes

- `board.CatalogRecord` is the canonical TUI snapshot input. This release may
  add a bounded discovery/query shape, but it must return the same record type
  and must not reinterpret source priority, state evidence, or durability.
- `Model` already stores the latest `tea.WindowSizeMsg` width and height. The
  missing contract is allocation and clipping of that height, not a new terminal
  size source.
- The vendored `spec-v1` on `release/v0.2.0` does not yet admit the newer typed
  `references` property, and the vendored `board-v1` does not admit
  `shared_touchpoints`. This one-track plan therefore omits both properties and
  records no cross-slice wire contract in `contracts.json`.

## Revised slice decomposition (confirmed)

`S01-tui-bounded-navigation`  [terminal history]

- Deferred terminal `re_slice_required` original. It retains its verifier FAIL,
  immutable report ledger, and link to mandatory rollback S02.

`S02-tui-bounded-navigation-rollback`  [████░░░░░░░░░░░░░░░░]  10 paths

- Restore the exact seven Go paths and three terminal-frame paths authored by
  S01 to their immutable start-tree modes, objects, and absences. This slice
  must verify before S03 starts.

`S03-tui-bounded-navigation-replacement`  [████████░░░░░░░░░░░░]  10 paths

- Re-deliver bounded catalog loading and height-safe navigation on the verified
  rollback baseline, with root endpoint proof reaching release 25 of 25 and
  slice 18 of 18 across a track boundary.

Ceiling: one mandatory mechanical rollback plus one replacement user journey,
each with a separate Implementer and fresh Verifier session.

## Track and touchpoint matrix (revised)

| File / surface | T1-tui-bounded-navigation |
|---|---|
| `internal/board/discovery.go` | ✓ S01/S02/S03 serial |
| `internal/board/discovery_test.go` | ✓ S01/S02/S03 serial |
| `internal/tui/board.go` | ✓ S01/S02/S03 serial |
| `internal/tui/model.go` | ✓ S01/S02/S03 serial |
| `internal/tui/releases.go` | ✓ S01/S02/S03 serial |
| `internal/tui/tui.go` | ✓ S01/S02/S03 serial |
| `internal/tui/tui_test.go` | ✓ S01/S02/S03 serial |
| `screenshots/S01-tui-bounded-navigation/*.txt` | ✓ S01/S02 serial |
| `screenshots/S03-tui-bounded-navigation-replacement/*.txt` | ✓ S03 |

One track serialises every overlapping touchpoint in the required
S01 → S02 rollback → S03 replacement order. No shared-touchpoint exception or
cross-track dependency exists.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Is the initial and incremental release batch five or ten records, and which gesture requests another batch? | N-01 | Resolved: ten initially; lowercase `o` requests ten more and is labelled `older` only in release-list help. |
| A-02 | How are header, border, error, and help rows deducted from terminal height, especially at very small sizes? | N-02, N-04 | Resolved: measure actual rendered chrome each frame, allocate a non-negative pane content height, and guarantee the final frame does not exceed a positive reported terminal height. |
| A-03 | Does background catalog refresh preserve the operator's expanded older-release depth? | N-01, N-04 | Resolved: yes; refresh re-queries the retained bounded depth and cannot accept an obsolete smaller-depth result. Selection remains identity-based within the returned window. |

## Screenshots / references

- No screenshot supplied. The issue and this intake preserve the textual repro.
