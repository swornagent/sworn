# Design TL;DR — S03-tui-chrome-rework

**Slice state at authoring:** `planned` → this doc gates entry to `design_review` (Rule 9: design review before code).

## User outcome (from spec.json)

The sworn TUI announces itself with a header (version + currently-selected
release), sizes its panes to the real terminal width so release names don't
wrap illegibly, renders its help bar as a full-width styled bar instead of
floating text with black gaps at either edge, and no longer renders its first
row(s) above the visible viewport top in terminals like VS Code's integrated
terminal.

## Root cause (from spec.json rationale, confirmed by reading the code)

`Model.Update`'s `case tea.WindowSizeMsg: return m, nil`
(`internal/tui/model.go:68-69`) discards the terminal's real width/height —
`Model` has no `Width`/`Height` field at all. Every pane style in
`internal/tui/styles.go` hardcodes its own width instead
(`ReleaseListStyle` `Width(30)`, `BoardStyle` `Width(80)`, `HelpBar`
`Width(110)`), completely disconnected from the actual terminal. That
explains the narrow-pane wrapping (confirmed visually in S02's own
`reachability-tui-capture.txt`, captured at a 220-col tmux pane — several
release labels still wrap across 3+ lines against the current fixed
`Width(30)`), the help bar not spanning the real terminal edge-to-edge, and
per spec's own rationale is "the most likely cause" of the reported VS Code
viewport-fit bug: a TUI rendering panes wider than the real terminal forces
the terminal itself to line-wrap output, producing more physical rows than
Bubble Tea's internal cursor-diffing model expects — a well-known trigger for
alt-screen cursor-position drift on startup/resize in xterm.js-based
emulators (VS Code's integrated terminal).

## Approach

Store the real terminal size on `Model` from `tea.WindowSizeMsg`, compute
pane/bar widths from it at render time (removing the hardcoded `Width(...)`
calls from the `styles.go` var block), add a header render function, and
restyle the help bar to use the real width instead of a fixed `110`. No new
dependency — same `bubbletea`/`lipgloss` already in use.

### AC-by-AC design

- **AC-01 (store + use real width/height):** add `Width int` / `Height int`
  fields to `Model`. `Update`'s `tea.WindowSizeMsg` case sets
  `m.Width, m.Height = msg.Width, msg.Height` (was `return m, nil`,
  discarding both). `styles.go`'s `ReleaseListStyle`, `BoardStyle`, `HelpBar`
  vars **drop their `.Width(...)` chain** — they become unsized base styles
  (border/padding/colour only). A new `paneWidths(total int) (left, right
  int)` function in `styles.go` computes the two pane widths from `m.Width`;
  `Model.View()` applies them via `ReleaseListStyle.Copy().Width(left)` /
  `BoardStyle.Copy().Width(right)`. **Fallback:** when `m.Width == 0` (no
  `WindowSizeMsg` received yet — true for every existing test that
  constructs `&Model{}` directly without driving it through
  `tea.NewProgram`), `paneWidths` returns the legacy constants `(30, 80)` so
  every pre-existing test keeps passing unchanged. Real usage always has
  `Width` set before the first `View()` — Bubble Tea sends the initial
  `WindowSizeMsg` before the first render when attached to a real TTY (AC-05
  text itself: "bubbletea's normal startup sequence").
- **AC-02 (no wrap for typical names at typical widths):** decouple "pane
  gets a width from the terminal" (AC-01, above) from "given adequate width,
  content doesn't wrap" (AC-02) — see DC-1 below for why. The AC-02 test
  renders `ReleaseListStyle.Copy().Width(100).Render(...)` directly (a
  `ReleasesList` with one release whose name is <40 chars) and asserts the
  rendered output has exactly the expected line count (title + one line per
  release, no extra wrapped lines) — proving the fix is "the hardcoded
  `Width(30)` is gone," not asserting a specific total-terminal-to-pane-width
  ratio.
- **AC-03 (header: version + selected release):** `tui.Run` gains a
  `version string` parameter (see DC-2 — this is the one place the change
  reaches outside `internal/tui`, flagged explicitly). `Model` gains a
  `Version string` field, set once in `tui.Run`. New `Model.renderHeader()`
  renders `sworn <version>  •  <release-label>` through a new `HeaderStyle`
  in `styles.go` (same full-width background-bar treatment as `HelpBar`, for
  visual symmetry top/bottom). `<release-label>` is `"no release selected"`
  when `m.state == viewReleases && m.Board.ReleaseName == ""` (the initial
  screen, never navigated), else `m.Board.ReleaseName` (the release the user
  last navigated into — matches the intake's ratified decision "TUI's own
  selection state," `docs/release/.../intake.md` §"TUI header sources active
  release from its own selection state," 2026-07-01). The header renders only
  in `Model.View()`'s two-pane branch (`viewReleases`/`viewBoard`) — the
  `viewLive`/`viewBlocked`/`viewSettings` full-screen branches are untouched
  (they're not spec touchpoints; out of scope for this slice).
- **AC-04 (full-width help bar):** `Model.renderHelp()` applies
  `HelpBar.Copy().Width(m.Width)` (falls back to the legacy `110` when
  `m.Width == 0`, same pattern as AC-01) instead of the style's own
  hardcoded `Width(110)`. `HelpBar` already sets `Background(colHelpBg)`, so
  once its width tracks the real terminal, lipgloss's background-fill
  behaviour makes it span edge-to-edge with no gap, on any terminal width —
  fixing both "too narrow, gap at edges on wide terminals" and "not
  calibrated to a narrow terminal" in one change.
- **AC-05 (viewport-fit / VS Code integrated terminal):** no new code path —
  this is the direct consequence of AC-01. Once pane/help-bar widths track
  the real reported width instead of hardcoded values that can exceed it
  (`30+80` before border/padding overhead already meets/exceeds a common
  80-col terminal), the terminal stops being forced to line-wrap Bubble
  Tea's output, which removes the row-count mismatch that was the
  spec-identified most-likely cause. Verified via reachability artefact
  (below), not a unit test — this AC is explicitly visual/terminal-emulator-
  specific per its own text ("recorded/screenshotted session").
- **AC-06 (build + tests green):** `go build ./...` and
  `go test ./internal/tui/...`.

## Files to touch

Matches spec touchpoints, plus one flagged mechanical addition (DC-2):

- `internal/tui/model.go` — `Width`/`Height`/`Version` fields; `WindowSizeMsg`
  case stores them; `renderHeader()`; `View()` wires header + computed pane
  widths; `renderHelp()` uses real width.
- `internal/tui/styles.go` — drop hardcoded `.Width(...)` from
  `ReleaseListStyle`/`BoardStyle`/`HelpBar`; add `paneWidths(total int)
  (left, right int)`; add `HeaderStyle`.
- `internal/tui/tui.go` — `Run(version string) error`; pass `version` into
  the constructed `Model`.
- `internal/tui/tui_test.go` — new tests for AC-01 (`WindowSizeMsg` stores
  width/height), AC-02 (no-wrap at width 100), AC-03 (header content across
  both release-label states), AC-04 (help bar rendered width == `m.Width`).
- **`cmd/sworn/main.go` + `cmd/sworn/top.go`** (not in spec's touchpoints
  list — see DC-2): both call sites become `tui.Run(version)` instead of
  `tui.Run()`. One-line, mechanical, no other change in either file.

## Design choices for reviewer

- **DC-1 (Type-2, local/reversible) — decouple AC-02's wrap-proof from
  AC-01's proportion algorithm.** I considered making AC-02's test drive the
  *whole* two-pane split at a total terminal width of 100 (i.e., assert
  `paneWidths(100)` leaves the left pane wide enough for a 40-char name).
  The arithmetic doesn't work: the rendered release-item label is
  `"▸ " + name + "  ─ (" + trackCount + " tracks, " + state + ")"`, which for
  a worst-case 39-char name plus a state like `verified` is ~65-70 chars —
  meaning the release-list pane alone would need roughly two-thirds of a
  100-col terminal, leaving the board pane unusably narrow (~25-30 cols) at
  that same width. Forcing that ratio to satisfy one literal AC number would
  make the *split itself* worse for the common case (both panes need to be
  usable at once). Instead, `paneWidths` uses a proportional split tuned for
  real/typical terminals (evidence: S02's own reachability capture used a
  220-col pane and still showed wrapped release labels against the current
  fixed `Width(30)`), and AC-02 is proven as a standalone rendering fact —
  "given the pane the width it needs, it does not wrap" — decoupled from
  what specific total-terminal-width produces that pane width. This is the
  behavioural bug fix (removing the hardcoded `Width(30)` ceiling that wraps
  *regardless* of how wide the terminal actually is); the exact split ratio
  is a separate, freely-tunable cosmetic parameter. Flagging in case the
  reviewer wants AC-02 read more literally (whole-model render at total=100).
- **DC-2 (Type-2, local/mechanical) — `tui.Run` gains a `version` parameter,
  touching 2 files outside the spec's touchpoint list.** AC-03 requires the
  header to show "the same value `sworn --version` reports"
  (`cmd/sworn/main.go`'s `version` package var, injected via
  `-ldflags -X main.version=...` in `Makefile`/`.goreleaser.yaml`). That
  value has no existing path into `internal/tui` — `tui.Run()` currently
  takes no arguments. Alternatives considered: (a) a package-level
  `tui.Version` var set by ldflags directly (`-X
  .../internal/tui.Version=...`) — rejected: requires editing
  `Makefile`/`.goreleaser.yaml` too (an even wider, and easier to silently
  desync, touchpoint expansion — two independently-set build-time constants
  that could drift apart); (b) a shared `internal/version` package — rejected
  as disproportionate (a new package for one string, still requires touching
  `main.go`). Chose (c): `tui.Run(version string) error`, with
  `cmd/sworn/main.go:63` and `cmd/sworn/top.go:31` updated to
  `tui.Run(version)` — the minimal, explicit, one-line-per-call-site glue
  that the Reachability Gate (Rule 1) requires to make the affordance
  actually reachable from the real entry point. Flagging because it's a
  scope-matrix addition, even though it's mechanical.
- **DC-3 (Type-2, local/reversible) — "currently selected release" persists
  across `esc` back to the list.** `handleBoardKey`'s `esc` returns to
  `viewReleases` but does not clear `m.Board.ReleaseName`. I read AC-03's "or
  an explicit 'no release selected' state on the initial releases-list
  screen" as scoped to the *initial* screen (never navigated), not every
  return to the list — so the header keeps showing the last-navigated
  release even while browsing back to pick another one, until a new `enter`
  changes it. This matches the ratified intake decision (TUI's own selection
  state is a durable "what's active" answer, not reset by transient
  navigation) and needs no new state tracking beyond the existing
  `Board.ReleaseName`.

## Design-level risks

- No vertical pagination exists for the releases list or board panes today,
  before or after this slice — `m.Height` is stored (AC-01) but not yet used
  to truncate/paginate content. A release list or board longer than the
  terminal's reported height can still overflow vertically. This is
  pre-existing behaviour, not introduced by this slice, and AC-05 only
  requires the *initial frame* not to render above the visible top (the
  width-mismatch root cause) — not general-purpose scroll/pagination. Not a
  new deferral (nothing changes here), but noted so the reviewer doesn't read
  AC-05 as a full vertical-overflow guarantee.
- `HeaderStyle`'s exact visual treatment (colour/weight) is a cosmetic
  Type-2 default — any reasonable full-width bar distinct from the pane
  borders satisfies AC-03's literal requirement (version + release text
  present, rendered above the two-pane layout).
- AC-05's reachability evidence is inherently terminal-emulator-specific
  (VS Code's integrated terminal, specifically). This sandboxed environment
  cannot drive a real VS Code window; the reachability artefact will be (a)
  a `tmux capture-pane` session (same mechanism as S02's artefact) proving
  the mechanism — pane/help-bar widths tracking a real reported terminal
  size instead of fixed constants — plus (b) an explicit manual smoke-step
  description in `proof.md` for a human to run `sworn` inside VS Code's
  actual integrated terminal and confirm no content renders above the
  visible top, before/after. Flagging now, per Rule 1, rather than silently
  substituting (a) for (b) at proof time.

## Test plan

- Unit: `tea.WindowSizeMsg{Width, Height}` through `Model.Update` sets
  `m.Width`/`m.Height` (AC-01).
- Unit: `paneWidths(0)` returns the legacy `(30, 80)` fallback; `paneWidths(n)`
  for a realistic wide `n` (e.g. 220, matching S02's capture width) returns
  two positive widths that sum (plus border/padding overhead) to `<= n`
  (AC-01).
- Unit: `ReleaseListStyle.Copy().Width(100).Render(...)` for a <40-char
  release name — assert no extra wrapped line (AC-02).
- Unit: `Model.View()`/`renderHeader()` with `Version` set and
  `Board.ReleaseName == ""` on `viewReleases` → contains "no release
  selected"; with `Board.ReleaseName` set → header contains both the version
  string and the release name (AC-03).
- Unit: `renderHelp()` with `m.Width` set to a realistic value — assert
  `lipgloss.Width(rendered) == m.Width` (AC-04).
- Reachability: `tmux capture-pane` session (mirrors S02's
  `reachability-tui-capture.txt`) showing pane widths responding to a real
  reported terminal size, saved alongside `proof.md`; plus an explicit
  manual VS-Code-integrated-terminal smoke-step description for AC-05 (see
  design-level risks above).
- `go build ./...` && `go test ./internal/tui/...` (AC-06).

## Traceability

| AC | Change | Test |
|----|--------|------|
| AC-01 | `Model.Width`/`Height` from `WindowSizeMsg`; `paneWidths()` | new `Update`/`paneWidths` unit tests |
| AC-02 | `styles.go` drops hardcoded pane `Width(...)` | new no-wrap-at-100 render test |
| AC-03 | `Model.Version`, `renderHeader()`, `tui.Run(version)` | new header-content tests (both release-label states) |
| AC-04 | `renderHelp()` uses `m.Width` | new full-width render test |
| AC-05 | Consequence of AC-01 (no new code path) | tmux reachability capture + manual VS Code smoke step |
| AC-06 | build + tui tests | `go build ./...`, `go test ./internal/tui/...` |
