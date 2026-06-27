<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR clean design, 4 mechanical apply-inline pins:

1. **design_decisions in status.json.** Transcribe D1–D5 from design.md §2 into `status.json` as `type_2` entries before verify. (5th recurrence — same fix every time.)
2. **Pad-then-style ordering explicit.** In D2 (or §4): state the rule "when applying `style.*()` to a value that is also the subject of a `%-*s` width format verb, apply the styling call OUTSIDE the padding — `style.X(fmt.Sprintf("%-*s", n, val))` not `fmt.Sprintf("%-*s", n, style.X(val))`." Apply to every column-aligned formatter in the renderers.
3. **Stream mismatch ack.** Add a §4 NOT-doing item: "Single `os.Stdout` gate is per spec (Risk #3); no per-stream gate needed." Add a corresponding comment in `style.go` near `Enabled()`.
4. **style_test.go package declaration.** Use `package style` (not `package style_test`) so `detect()` and `enabled` are accessible for NO_COLOR/SWORN_FORCE_COLOR gate tests. Document the `t.Cleanup(func() { enabled = old })` idiom for restoring the var.

Flags (not pins): (a) `Enabled()` returns a frozen var — test gating via `detect()` directly; (b) dep-policy memory aligns (no ADR needed); (c) S60 init.go edits are on-branch and additive-compatible.

§2 decisions D1–D5 ack. §6 (none) ack.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline mechanics (status.json field, ordering sub-rule, ack comment, test package declaration); none require re-reviewing the design before code is safe. Verifier backstops pins 2–3 via table tests and AC1 inspection.
-->
