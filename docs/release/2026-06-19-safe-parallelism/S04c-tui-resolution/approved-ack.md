<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Sound design overall; 5 pins. Coach has resolved the two that needed authority (Pins 3 and 5); the rest are apply-inline.

1. **BLOCKED-state detection.** Check BOTH `state == "failed_verification"` AND `state == "implemented" && verification.result == "blocked"` when deciding to show the blocked panel. A slice with a BLOCKED verifier verdict stays at `implemented` in the JSON — it is not `failed_verification`.

2. **Board cursor (CRITICAL).** Before wiring the Enter key transition, add slice-level cursor navigation to the board view: (a) `Cursor int` field on `BoardView`; (b) up/down key handling in `handleBoardKey`; (c) visual selection indicator in `board.go` `View()`. Without this, the Enter entry point does not exist. Wire it as part of this slice.

3. **Dep policy — bubbles/textinput → COACH DECISION: add the dep.** Add `github.com/charmbracelet/bubbles` and use `bubbles/textinput` for the deferral reason input. Write an ADR entry first (per [[project_dep_policy]], analogous to ADR-0004) in the same slice, BEFORE the dep appears in go.mod. Add `go.mod` and `go.sum` to `planned_files` in status.json before verify (Gate 2). Do not implement the inline rune-buffer alternative.

4. **Proof.md format audit.** Run `grep -r "^## Violations" docs/release/` before implementing the parser. Expect zero results (R3 uses `## Not delivered`). Record the audit in the design or proof. The spec mandated it.

5. **Auto-fix [1] UX → COACH DECISION: use the spec-permitted stub.** On `[1]`, show an inline message — `to re-run: sworn run --slice <id> --release <name>` — and return to the panel immediately. Do NOT use `tea.ExecProcess` for `[1]` (no TUI suspend, no subprocess for the auto-fix path). `tea.ExecProcess` remains the choice for `[2]`/`[3]` (AI-tool launch) per design §2.3 — that part is unchanged. Update design §2.3/§6 to reflect the stub decision for [1].

Flags: (a) `tea.ExecProcess` confirmed present in bubbletea v1.3.10 — valid for [2]/[3]; (b) no cross-track file collisions on the `internal/tui/` files; (c) once bubbles is added, `go.mod`/`go.sum` MUST be in `planned_files` before verify.

§2 decisions (violation heuristic, context file format, intake.md append) ack. §2 Decision 4 (deferral input) now resolved per Pin 3 → bubbles/textinput + ADR. §6 question resolved per Pin 5 → stub.

Address Pins 1, 2, 4 inline during implementation; Pins 3 and 5 are Coach-decided above — implement as specified. Then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Coach resolved the two escalations — Pin 5 (auto-fix [1] = spec-permitted stub, not tea.ExecProcess) and Pin 3 (deferral input = add charmbracelet/bubbles + ADR, go.mod/go.sum into planned_files). Remaining pins (1, 2, 4) are apply-inline mechanical. Coach ack 2026-06-21.
-->
