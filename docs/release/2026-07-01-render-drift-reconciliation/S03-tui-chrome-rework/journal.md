# Journal — S03-tui-chrome-rework

## 2026-07-02 — Implementer session start

`state`: `design_review` → `in_progress`. `start_commit` was `null`; set to
`0439b11740c5c1eb93ac3cfab0cfe68bc653fa0f` (current branch HEAD, the Coach
pin-1 resolution commit) on this transition — never overwritten thereafter.

Design review (`review.md`, commit `740e7e7`) surfaced 6 pins (3 mechanical,
2 memory-cited, 1 escalate). The escalate pin (pin 1, AC-02 at 80 cols) was
resolved by the Coach (`0439b11`): **option (b) — a left-pane minimum-width
floor plus ellipsis truncation of long release names**, delivering AC-02's
legibility outcome at a real 80-col terminal, not only at the width the tests
render. This binding decision supersedes design.md's pure-proportional-split
DC-1 where they conflict. No `approved-ack.md` marker convention exists in
this repo; per S01/S02 precedent this journal is the durable acknowledgement
that the Coach dispatched this implementer session against the reviewed
design + resolved pin.

Applying the 6 pins during implementation:

1. **[escalate → Coach-resolved] AC-02 at 80 cols.** `paneWidths` gives the
   left pane a minimum-width floor (`minLeftPane`); `ReleasesList.View`
   ellipsis-truncates each release label to the pane's content width
   (ANSI-aware, via `x/ansi.Truncate`) so a long name is truncated with `…`
   instead of wrapping. Proven at 80 cols, not just at 100.
2. **[mechanical] paneWidths reserves the 4 border columns.** `paneWidths`
   subtracts `borderCols=4` (2 rounded-border cols × 2 panes; `JoinHorizontal`
   adds no gap — verified live against lipgloss v1.1.0 in the review) before
   splitting, so `left+right+4 <= total`. The AC-01 test asserts both
   `left+right+4 <= n` and that the joined two-pane render width `<= m.Width`.
3. **[mechanical] Height stored but unused for sizing.** `m.Height` is stored
   from `WindowSizeMsg` (AC-01 requires storing height) but **width alone
   performs all sizing in this slice**; height is retained for tracked future
   vertical pagination (design-level risk in design.md — no pagination exists
   before or after this slice). Stated explicitly in code comment + proof so a
   fresh verifier does not read the unused field as dead code.
4. **[mechanical] AC-05 reachability substitution.** proof.md carries BOTH a
   `tmux capture-pane` artefact (mechanism proof: pane/help-bar widths track a
   real reported terminal size) AND an explicit human VS-Code-integrated-
   terminal smoke step, and states that the manual step is the accepted AC-05
   form (the sandbox cannot drive a real VS Code window).
5. **[memory-cited] Render-drift guard is fail-closed (S01).** Every
   status.json state transition is followed by `sworn render` and the
   re-rendered index.md committed together.
6. **[memory-cited] Newline-eating edit corruption (3× on 2026-06-27, incl.
   tui files).** styles.go/model.go are comment-dense; after editing I grep
   the diff for fused comment+code lines and run the full `go test ./...`
   (-timeout 600s) before trusting green.

Plan (TDD, Rule 1 — drive through `Model.View`/`Model.Update`, the real
integration point that owns the affordance, not leaf styles):

- Add `Width`/`Height`/`Version` fields to `Model`; `WindowSizeMsg` stores
  width+height (was `return m, nil`, discarding both).
- `styles.go`: drop hardcoded `.Width(...)` from `ReleaseListStyle`/
  `BoardStyle`/`HelpBar`; add `paneWidths(total)` (border-reserving,
  floor-clamped); add `HeaderStyle`.
- `releases.go`: `View` truncates each label to the pane content width with
  `…` (Coach pin-1 truncation) when a real width is set.
- `model.go`: `renderHeader()` (version + selected release), `View()` wires
  header + computed pane widths + width-sized help bar; `renderHelp()` uses
  `m.Width`.
- `tui.go`: `Run(version string)`; `cmd/sworn/main.go` + `cmd/sworn/top.go`
  call `tui.Run(version)` (DC-2 flagged, mechanical glue for Rule 1
  reachability).

## 2026-07-02 — Implementation complete

TDD executed per the plan. First wrote 10 new tests in `tui_test.go` driving
the affordance through `Model.Update`/`Model.View` (Rule 1); confirmed the
correct TDD red (compile failure — `Model` had no `Width`/`Height`/`Version`,
no `paneWidths`/`minLeftPane`). Then implemented:

- `styles.go`: dropped hardcoded `.Width(30)`/`.Width(80)`/`.Width(110)` from
  `ReleaseListStyle`/`BoardStyle`/`HelpBar`; added `HeaderStyle`; added the
  `legacyLeftWidth`/`legacyRightWidth`/`legacyHelpWidth`/`minLeftPane`/
  `borderCols` constants and `paneWidths(total)` — reserves the 4 border
  columns (pin 2) and floors the left pane at `minLeftPane=26` (Coach pin 1),
  legacy `(30,80)` when `total <= 0`.
- `model.go`: added `Width`/`Height`/`Version` fields; `WindowSizeMsg` stores
  width+height; `renderHeader()` (`sworn <version>  •  <label>`, label = "no
  release selected" when `Board.ReleaseName==""`); `View()` computes pane
  widths, sets `Releases.Width`, prepends the header; `renderHelp()` widths
  the bar to `m.Width` (fallback 110).
- `releases.go`: `ReleasesList.Width` field; `View()` ellipsis-truncates each
  label to `Width-6` via `x/ansi.Truncate` (ANSI-aware — the label carries a
  styled Divider) when `Width>0`, else untruncated (legacy path).
- `tui.go`/`cmd/sworn`: `Run(version string)`, both call sites updated.

`go mod tidy` promoted `x/ansi` from indirect to direct (already compiled in
via lipgloss; `go.sum` unchanged — not a new dependency, no ADR needed).

Verification (all live): `go build ./...` 0; `go vet ./internal/tui/...` 0;
`gofmt -l` empty on all 7 touched Go files; `go test ./internal/tui/...`
green (10 new tests pass); full `go test ./...` (`-timeout 550s`) green, no
regression (`cmd/sworn` exercises `tui.Run(version)`).

Pin 6 (newline-eating corruption): grepped the diff for fused comment+code
lines — none; the comment-dense `styles.go`/`model.go` edits are clean; full
suite green (not just `internal/tui`).

Reachability (AC-05, pin 4): the STALE pre-feature `bin/sworn` (built during
the in_progress transition step) initially produced a legacy-width, no-header
capture — a false negative caught by inspecting the artefact rather than
trusting it. Rebuilt `bin/sworn` from HEAD and re-captured the REAL binary on
a Python `pty` (window size set via `TIOCSWINSZ`), replayed through a `pyte`
emulator, at 80 AND 200 cols. Both frames show the header, responsive panes,
`…`-truncated single-line release names, and a full-width help bar, with no
line exceeding the terminal width — saved as `reachability-tui-capture.txt`.
Only the initial releases screen was used (no `enter`), so `board.ReadBoard`'s
lazy-migration side effect never fired; `git status` shows no stray
`board.json`. Plus the explicit human VS-Code smoke step in `proof.md`.

Wrote `proof.json` + `proof.md` (matching S02's shape) from live repo state.

State -> `implemented`. Stopping here per role boundaries — no verifier prompt
in this session; `/verify-slice S03-tui-chrome-rework` in a fresh session is
next. NEVER self-certify to `verified`.
