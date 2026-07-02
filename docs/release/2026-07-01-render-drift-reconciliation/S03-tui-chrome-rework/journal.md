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
