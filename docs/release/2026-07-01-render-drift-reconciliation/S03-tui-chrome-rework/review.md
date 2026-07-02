# Captain design review — S03-tui-chrome-rework

Date: 2026-07-02
Reviewer: Captain (fresh-context design review, Rule 9)
Pins: 6 (3 mechanical, 2 memory-cited, 1 escalate)

## Pins

1. **[escalate] AC-02 at 80-col terminals — RESOLVED (Coach decision, see below).** The design tested AC-02 as an isolated pane at Width(100); at a real 80-col terminal the proportional split leaves the left pane ~26 cols, so a 40-char release name still wraps — tests green, outcome undelivered. Punted to the Coach: (a) isolated-pane reading, (b) minimum-width floor + truncation, (c) rescope AC-02.
2. **[mechanical] paneWidths arithmetic (verified live against lipgloss v1.1.0).** `.Width(n)` absorbs padding, but the rounded border adds +2 cols per pane and JoinHorizontal adds no gap: two panes render at left+right+4. paneWidths(total) MUST reserve those 4 border columns; the AC-01 test should assert left+right+4 <= n AND the joined render <= m.Width.
3. **[mechanical] AC-01 stores Height but nothing uses it.** State explicitly that width performs the sizing and height is stored for a tracked future use, or a literal fresh verifier can bounce the slice.
4. **[mechanical] AC-05 reachability substitution.** The spec asks for a VS Code integrated-terminal recording; the sandbox cannot drive VS Code. proof.md must carry BOTH the tmux capture-pane artefact AND the explicit human VS-Code smoke step, and the verifier must be told the manual step is the accepted AC-05 form.
5. **[memory-cited] S01's render-drift guard is fail-closed on this release.** Every status.json state transition must be followed by `sworn render` with the re-rendered index.md committed together.
6. **[memory-cited] Newline-eating edit corruption (3x on the 2026-06-27 release, incl. tui files).** styles.go/model.go are comment-dense; grep the edits for fused comment+code lines and run full `go test ./...` before trusting green.

## Coach decision (post-review)

Date: 2026-07-02
Decision-maker: Brad (Coach), in conversation following this review

**Pin 1 (AC-02 80-col semantics): Option (b) selected — minimum-width floor + truncation.**

- The left pane gets a minimum-width floor; release names longer than the pane are truncated with an ellipsis instead of wrapping. AC-02's user outcome (legible names) is delivered at 80-col terminals, not just at the width the tests happen to render.
- Options (a) isolated-pane reading and (c) rescope were considered and rejected: (a) leaves the reported symptom live at 80 cols; (c) weakens the spec where a modest implementation delivers it.
