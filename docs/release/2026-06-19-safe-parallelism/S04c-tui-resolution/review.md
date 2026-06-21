# Captain review — S04c-tui-resolution
Date: 2026-06-21
Design commit: c757906a494e74dcbdd060ff7f9d22a549631a92

## Pins

1. **[mechanical]** §1 / spec entry point — BLOCKED state is not the same as `state: "failed_verification"`
   What I observed: Spec entry point says "slice in `failed_verification` or `BLOCKED` state." In the harness schema, FAIL verdicts transition slices to `state: "failed_verification"`, but BLOCKED verdicts leave the slice at `state: "implemented"` with `verification.result: "blocked"` (confirmed from S01-process-ownership's BLOCKED journey in the index.md activity log). The design §1 says "blocked or failed slice" without addressing the detection difference.
   What to ask the implementer: Confirm the implementation checks BOTH `state == "failed_verification"` AND `state == "implemented" && verification.result == "blocked"` when deciding whether to transition to the blocked panel. If only checking state, BLOCKED slices are silently excluded from the feature.

2. **[mechanical] CRITICAL** §3 / model.go integration — board view has no slice cursor; Enter has no target
   What I observed: `BoardView` struct has no cursor field and no `SelectedSlice` field (confirmed by reading board.go). `handleBoardKey` in model.go handles `"esc"` and `"l"` only — no `"enter"` and no up/down navigation within slices. The board renders `for _, sliceID := range track.Slices` as plain lines with no selection indicator. There is currently no mechanism for the user to select a specific slice within the board view. Without this, the "press Enter on a blocked slice" entry point does not physically exist.
   What to ask the implementer: Before wiring the blocked panel transition, add slice-level cursor navigation to the board view: (a) `Cursor int` (or `SelectedSliceID string`) field to `BoardView`; (b) up/down key handling in `handleBoardKey`; (c) a visual selection indicator in `board.go`'s `View()` render. The S04a implementer pre-wired the model comment ("S04c adds TL;DR overlay") but left cursor navigation for S04c to implement. Confirm this is on scope and designed in §3.

3. **[memory-cited]** §2.4 / dep policy — `charmbracelet/bubbles` not in go.mod; ADR required
   What I observed: Design Decision 4 proposes "a simple text input component (e.g., `bubbles/textinput`)" for the deferral reason prompt. `github.com/charmbracelet/bubbles` is NOT in `go.mod` (grep confirmed: only `bubbletea v1.3.10` and `lipgloss v1.1.0` are present). Adding `charmbracelet/bubbles` is a new dependency.
   What to ask the implementer: Per [[project_dep_policy]], any new dependency requires an ADR entry before it appears in `go.mod`. Either (a) add an ADR for `charmbracelet/bubbles` before importing it, or (b) implement deferral text input without the bubbles package — inline key-event buffering (handle `tea.KeyMsg` runes + backspace into a `string`) is feasible in plain bubbletea and avoids a new dep. Also confirm `go.mod` and `go.sum` are added to `planned_files` if a dep is added (the S04a trial-log flagged this exact omission for bubbletea/lipgloss).
   Citation: [[project_dep_policy]]

4. **[mechanical]** §2.1 / Spec Risk #1 — proof.md format audit not confirmed before picking headings
   What I observed: Spec Risk #1 mitigation says "Check actual proof.md format from any verified R2 slice before implementing the parser." Design §2.1 picks `## Violations` or `## Not delivered` but does not note having performed this audit. Verified R3 proof.md files (S03-verify-under-concurrency confirmed) use `## Not delivered` exclusively. Running `grep -r "^## Violations" docs/release/` finds zero matches.
   What to ask the implementer: Run `grep -r "^## Violations" docs/release/` before implementing the parser. If `## Violations` never appears in practice, document whether it is a forward-compat addition or can be dropped. Either way, the audit must be recorded as a design note; the spec mandated it.

5. **[escalate]** §6 / §2.3 — auto-fix [1] via `tea.ExecProcess` vs spec-approved stub; UX direction call
   What I observed: The spec's `## Deferrals allowed?` section pre-authorizes a stub for [1]: "may be stubbed to a log message if rerunning from within the TUI requires complex subprocess management. Tracking: TBD. Ack: now." The design §2.3 picks `tea.ExecProcess` for both [1] (auto-fix rerun) AND [2]/[3] (AI tool launch). `tea.ExecProcess` is confirmed available in `bubbletea v1.3.10`. Using it for [1] suspends the TUI entirely and shows raw `sworn run` output until the run completes — which may take several minutes for a full slice run. Option (a) `tea.ExecProcess` for [1]: raw terminal output visible, TUI resumes when done. Option (b) stub: pressing [1] shows an inline "to re-run: `sworn run --slice <id> --release <name>`" message and returns immediately. Both are within spec. Coach picks.

## Summary
Pins: 5 total — 3 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins: Pin 2 — board view has no slice cursor; the blocked panel is unreachable without it.

## Smaller flags (not pins, worth one-line ack)

(a) `tea.ExecProcess` confirmed present in `bubbletea v1.3.10` (`exec.go` in module cache) — design §2.3 assumption is valid for [2]/[3] regardless of Coach's decision on [1].
(b) Model comment at model.go line ~27 already says "S04c adds TL;DR overlay" — the S04a implementer deliberately left the extension point open. Board cursor navigation is the missing piece.
(c) `planned_files` in status.json currently lists 4 files but omits `go.mod`/`go.sum`; if bubbles is added, they must appear in `planned_files` before verify (Gate 2).
(d) Touchpoint matrix in index.md assigns `internal/tui/` entirely to T2; no sibling track collision on the 4 files.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Sound design overall; 5 pins. 4 are apply-inline mechanical/memory fixes; 1 needs Coach ack on UX.

1. **BLOCKED-state detection.** Check BOTH `state == "failed_verification"` AND `state == "implemented" && verification.result == "blocked"` when deciding to show the blocked panel. A slice with a BLOCKED verifier verdict stays at `implemented` in the JSON — it is not `failed_verification`.

2. **Board cursor (CRITICAL).** Before wiring the Enter key transition, add slice-level cursor navigation to the board view: (a) `Cursor int` field on `BoardView`; (b) up/down key handling in `handleBoardKey`; (c) visual selection indicator in `board.go` `View()`. Without this, the Enter entry point does not exist. Wire it as part of this slice.

3. **Dep policy — bubbles/textinput.** `charmbracelet/bubbles` is not in `go.mod`. Either (a) write a short ADR for it then add the dep, or (b) implement deferral text input inline (handle rune key events + backspace into a `string` buf — avoids the new dep entirely). If a dep is added, add `go.mod` and `go.sum` to `planned_files` in `status.json`.

4. **Proof.md format audit.** Run `grep -r "^## Violations" docs/release/` before implementing the parser. Expect zero results (R3 uses `## Not delivered`). Record the audit in the design or proof. The spec mandated it.

5. **Auto-fix [1] UX direction.** Coach confirmed: use `tea.ExecProcess` for [1] (TUI suspends, raw `sworn run` output visible) — OR — use the spec-permitted stub (inline message, no subprocess). Coach's call on this §6 question. Design §2.3 currently picks `tea.ExecProcess`; confirm or redirect.

Flags: (a) `tea.ExecProcess` confirmed present in bubbletea v1.3.10 — valid for [2]/[3] regardless of [1] decision; (b) no cross-track file collisions.

§2 decisions (violation heuristic, context file format, deferral input, intake.md append) ack — apply Pin 3 fix during implementation. §6 question ack after Coach resolves Pin 5.

Address Pins 1–4 inline during implementation. Resolve Pin 5 with Coach, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 5 is a product UX direction call (tea.ExecProcess TUI-suspend vs spec-approved stub for auto-fix [1]) that the spec explicitly left open; Coach must pick before code is written.
-->
