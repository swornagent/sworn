---
title: Slice proof bundle — S03-tui-chrome-rework
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S03-tui-chrome-rework`

Rendered from `proof.json` (proof-v1). Single implementation pass.

## Scope

The sworn TUI now stores the real terminal size from `tea.WindowSizeMsg`
(previously discarded) and uses it to size the releases pane, board pane,
header, and help bar responsively; it renders a header (version + selected
release), a full-width help bar, and ellipsis-truncates long release names so
nothing wraps illegibly or overflows the terminal (the VS Code viewport root
cause).

## Files changed

```
$ git diff --name-only 0439b11740c5c1eb93ac3cfab0cfe68bc653fa0f
cmd/sworn/main.go
cmd/sworn/top.go
docs/release/2026-07-01-render-drift-reconciliation/S03-tui-chrome-rework/journal.md
docs/release/2026-07-01-render-drift-reconciliation/S03-tui-chrome-rework/status.json
docs/release/2026-07-01-render-drift-reconciliation/index.md
go.mod
internal/tui/model.go
internal/tui/releases.go
internal/tui/styles.go
internal/tui/tui.go
internal/tui/tui_test.go
```

`proof.json`, `proof.md` and `reachability-tui-capture.txt` land with this
bundle commit (they are the bundle itself). `journal.md`, `status.json` and the
re-rendered `index.md` also carry the earlier `in_progress` transition commit.

## Test results

### Go

```
$ go build ./...
(no output, exit 0)

$ go vet ./internal/tui/...
(no output, exit 0)

$ gofmt -l internal/tui/model.go internal/tui/styles.go internal/tui/releases.go \
         internal/tui/tui.go internal/tui/tui_test.go cmd/sworn/main.go cmd/sworn/top.go
(empty — all touched Go files gofmt-clean)

$ go test ./internal/tui/... -count=1
ok  github.com/swornagent/sworn/internal/tui   0.954s
```

New S03 tests (all PASS), driven through the `Model.Update` / `Model.View`
integration point (Rule 1), not leaf styles in isolation:

- `TestWindowSizeMsgStoresDimensions` — AC-01: `tea.WindowSizeMsg` stores
  width AND height on the Model.
- `TestPaneWidthsReserveBorderColumns` — AC-01 + review pin 2: `paneWidths`
  reserves the 4 border columns (`left+right+4 <= total`) for 80/100/120/220;
  legacy `(30,80)` fallback for `paneWidths(0)`.
- `TestPaneWidthsLeftFloor` — AC-02 (Coach): left pane held at/above
  `minLeftPane` at 80 cols.
- `TestTwoPaneRenderFitsTerminalWidth` — AC-01/AC-05 + pin 2: the full
  `Model.View()` frame width `<= m.Width` at 80/100/120/220.
- `TestReleasesListNoWrapAtTypicalWidth` — AC-02: `<40`-char name at a wide
  pane is one line, untruncated.
- `TestReleasesListTruncatesLongNameAtNarrowPane` — AC-02 (Coach): a long
  name at an 80-col left pane is `…`-truncated on one line; line width `<=`
  pane.
- `TestHeaderShowsVersionAndNoReleaseSelected` — AC-03: initial screen shows
  version + "no release selected".
- `TestHeaderShowsSelectedRelease` — AC-03: navigated state shows version +
  release name.
- `TestViewRendersHeader` — AC-03: header reached through `Model.View`.
- `TestHelpBarSpansFullWidth` — AC-04: `lipgloss.Width(renderHelp()) ==
  m.Width` (and `== 110` fallback).

Full suite (AC-06), no regression:

```
$ go test ./... -count=1 -timeout 550s
ok  github.com/swornagent/sworn/cmd/sworn        39.913s
ok  github.com/swornagent/sworn/internal/account 10.144s
... (all packages ok, 0 failures) ...
ok  github.com/swornagent/sworn/internal/tui     1.392s
```

`cmd/sworn` (39.9s) exercises the `tui.Run(version)` signature change end to
end.

## Reachability artefact

`reachability-tui-capture.txt` — the real `bin/sworn`, built from this track's
HEAD, run on a genuine pseudo-terminal (Python `pty`) whose window size was set
via `TIOCSWINSZ`. bubbletea's `checkResize()` turns that into the
`tea.WindowSizeMsg` the Model now stores; the alt-screen output stream was
replayed through a `pyte` terminal emulator to reconstruct the final on-screen
frame. Captured at BOTH 80 columns (the reported-symptom width) and 200
columns. The frames show:

- the header bar `sworn 0.0.0-dev  •  no release selected` on the initial
  screen (AC-03);
- the releases pane, board pane, and help bar all sizing to the real width —
  the 80-vs-200 contrast is the responsiveness proof (AC-01);
- long release names ellipsis-truncated onto a single line each — no wrapping
  (AC-02, Coach pin 1);
- a full-width help bar (AC-04);
- no line exceeding the terminal width at either size (AC-05 mechanism).

**AC-05 human smoke step (accepted AC-05 form, review pin 4 — the sandbox
cannot drive a real VS Code window):** in VS Code's integrated terminal run
`go build -o bin/sworn ./cmd/sworn && ./bin/sworn`; observe (a) the top row is
the `sworn <version>  •  no release selected` header with no content rendered
above it or scrolled above the visible viewport top; (b) release names do not
wrap; (c) the help bar spans the full width. Compare against the before
screenshot `docs/release/2026-07-01-render-drift-reconciliation/screenshots/2026-07-01-tui-current-state.png`.

## Delivered

- **AC-01** — `Model.Width/Height` stored from `WindowSizeMsg`; hardcoded pane
  `.Width(...)` removed from `styles.go`; `paneWidths` + `Model.View`
  `.Copy().Width(...)`. Tests: `TestWindowSizeMsgStoresDimensions`,
  `TestPaneWidthsReserveBorderColumns`.
- **AC-02** (Coach option b) — `minLeftPane` floor in `paneWidths`;
  `ReleasesList.View` ellipsis-truncation via `x/ansi.Truncate`. Tests:
  `TestReleasesListNoWrapAtTypicalWidth`,
  `TestReleasesListTruncatesLongNameAtNarrowPane`; 80-col reachability frame.
- **AC-03** — `Model.Version`, `renderHeader()`, `tui.Run(version)`. Tests:
  `TestHeaderShowsVersionAndNoReleaseSelected`, `TestHeaderShowsSelectedRelease`,
  `TestViewRendersHeader`; both reachability frames.
- **AC-04** — `renderHelp()` uses `m.Width`. Test: `TestHelpBarSpansFullWidth`;
  both reachability frames.
- **AC-05** — consequence of AC-01. Tests: `TestTwoPaneRenderFitsTerminalWidth`;
  reachability capture + human VS-Code smoke step.
- **AC-06** — `go build ./...` 0; `go test ./internal/tui/...` pass; full
  `go test ./...` pass; vet/gofmt clean.

## Not delivered

None. (`proof.json.not_delivered` is empty.)

## Divergence from plan

- **Touchpoint expansion (all pre-flagged, now realised):**
  `internal/tui/releases.go` (Coach pin-1 truncation, where the label is
  built); `cmd/sworn/main.go` + `cmd/sworn/top.go` (design DC-2 —
  `tui.Run(version)` call sites); `go.mod` (`go mod tidy` promoted
  `github.com/charmbracelet/x/ansi` from `// indirect` to direct because
  `releases.go` now imports `ansi.Truncate`). The x/ansi change is **not a new
  dependency** — it is already compiled in transitively via lipgloss (`go.sum`
  unchanged, no new module downloaded); only its direct/indirect marker
  changed. No ADR required.
- **`m.Height` stored but unused for sizing (review pin 3):** width alone does
  all sizing in this slice; Height is stored per AC-01's text and retained for
  tracked future vertical pagination (no pagination exists before or after this
  slice). Stated in the `Model.Height` code comment and `proof.json` so a fresh
  verifier does not read it as dead code.
- **AC-05 evidence form (review pin 4):** PTY-capture + emulation of the real
  binary PLUS the explicit human VS-Code smoke step; neither is silently
  substituted for the other.
- **DC-3 (as designed):** the selected release persists in the header across
  `esc` back to the list; "no release selected" is scoped to the initial,
  never-navigated screen.
