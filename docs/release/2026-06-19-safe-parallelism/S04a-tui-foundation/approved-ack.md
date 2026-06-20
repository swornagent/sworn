Clean design, 5 pins all apply-inline:

1. **go.mod/go.sum missing from §3.** Add both to §3 and `planned_files` in `status.json` before writing any code. bubbletea and lipgloss will appear in `git diff --name-only` and must be explained.
2. **ADR required for bubbletea + lipgloss.** Per dep policy, each new dep needs an ADR commit before `go.mod` changes. Write a brief `docs/adr/000X-tui-deps.md` (or confirm ADR-0004 covers TUI deps if general-purpose) as the first commit of implementation — before any `go get`.
3. **tui.Run() location must be declared.** Either add `internal/tui/tui.go` to §3 + `planned_files` (preferred — matches touchpoint matrix), or put Run() in `model.go` and note Divergence from plan + correct the matrix annotation.
4. **design_decisions in status.json.** Transcribe all 5 §2 decisions (all Type-2) into the `design_decisions` field and run `sworn designfit 2026-06-19-safe-parallelism` before writing production code.
5. **§6 Q1 is answered by spec.** `sworn top` (no args) → TUI; `sworn top <release>` → existing evidence surface. Your proposed resolution is correct. No Coach decision needed.

Flags: (a) §6 Q2 self-decided — no action; (b) model.go should expose clean extension points for S04b/S04c.

§2 decisions D1–D5 all Type-2 — ack. §6 Q1 spec-answered, Q2 self-decided — ack.

Address pins 1–4 inline at implementation start (pre-code steps), then proceed to in_progress.
