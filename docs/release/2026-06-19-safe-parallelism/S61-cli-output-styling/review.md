# Captain review — S61-cli-output-styling
Date: 2026-06-23
Design commit: d47ed997f4416dde7d4d7241b22628ce613571cc

## Pins

1. **[mechanical]** §2 (Step 2b) — `design_decisions` absent from `status.json`
   What I observed: `status.json` has no `design_decisions` field at all. Design.md §2 lists 5 decisions (D1–D5). `cmd/sworn/*.go` files in `planned_files` trigger `impliesType1Work()` in `sworn designfit`; the merge gate hard-fails on an absent or empty `design_decisions[]` array.
   What to ask the implementer: Transcribe all five §2 decisions into `status.json` as `type_2` entries before verify. All five are genuinely Type-2 (spec-directed, reversible, low-stakes). This is the fifth recurrence of this pattern (S23, S24, S21, S60 — per trial log).

2. **[mechanical]** §2 D2 / Spec Risk #1 (Step 1B) — pad-then-style ordering not explicit
   What I observed: Spec Risk #1 says "mitigated by the pad-then-style rule." Design D2 says "every styled output wraps an existing `fmt.Print*` argument in a `style.*()` call, never rephrases the string." This is compatible with the right ordering but doesn't state it explicitly: never pass a styled value as the argument to a width-padded format verb (`fmt.Printf("%-*s", n, style.Accent(val))` → wrong; `style.Accent(fmt.Sprintf("%-*s", n, val))` → right). The spec calls this risk "already hit and fixed once in `init`'s plan table." The renderers (`internal/rtm`, `internal/designfit`, etc.) contain column-aligned table formatters where this matters.
   What to ask the implementer: Add an explicit sub-rule to D2 (or a §4 NOT-doing item): "When a format verb applies width-padding, apply `style.*()` outside the padding, not as the padded argument." Apply it to every renderer table formatter touched during implementation.

3. **[mechanical]** Spec Risk #3 (Step 1B) — stream-mismatch "documented" is unanchored
   What I observed: Spec Risk #3 says "gating on `os.Stdout` while a renderer writes to stderr. Acceptable (single global gate); documented." Design §2 and §4 have no decision or NOT-doing item acknowledging this, and no pointer to where "documented" means (a comment in `style.go`, a §4 item, etc.).
   What to ask the implementer: Add a §4 NOT-doing item: "Single gate on `os.Stdout` is acceptable even when renderers write to stderr (spec-acknowledged); no per-stream gate needed." Add a corresponding comment in `style.go` near `Enabled()`.

4. **[mechanical]** §5 / AC1 (Step 3) — `style_test.go` must be `package style` to test gate logic
   What I observed: The reference branch's `style.go` stores the colour gate in an unexported package-level var `enabled` computed once at init via `detect()`. The spec AC1 requires unit tests to verify NO_COLOR and SWORN_FORCE_COLOR gating. Tests in `package style_test` (external black-box package) can call `Enabled()` but cannot re-invoke `detect()` after setting env vars, because `enabled` is frozen at package init. Only same-package tests (`package style`, not `package style_test`) can directly call `detect()` and temporarily reassign `enabled` to test all three states.
   What to ask the implementer: Declare `style_test.go` as `package style` (same-package). Use `detect()` + temporary `enabled` override to test NO_COLOR / SWORN_FORCE_COLOR / non-TTY gating. If written as `package style_test`, the gating AC is untestable at unit-level and AC1's "Verified by style_test.go" claim is hollow.

---

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (would cause verify to fail or gate to block if unaddressed): **1** (designfit merge gate hard-fails without design_decisions[]), **4** (gate tests untestable → AC1 cannot be verified by style_test.go as spec requires)

## Summary

Design is clean: zero-dep palette is spec-directed and correctly scoped; T18 serialisation against all not-yet-merged tracks (T6, T10, T17) is already established in the touchpoint matrix; `wip/cli-styling-reference` branch exists and `style.go` implements Enabled() correctly. Four mechanical pins, none requiring a design change.

## Smaller flags (not pins, worth one-line ack)

- `Enabled()` returns the frozen `var enabled` (computed once at process start). This is intentional and documented in the reference package comment. Test `detect()` directly to verify gate logic; don't expect `Enabled()` to change within a test process.
- Memory [[project_dep_policy]] aligns: zero new deps, no ADR required. Memory [[feedback_dep_justification_test]] confirms: a pure ANSI escape helper is a narrow one-shot where stdlib is clearly sufficient. No deviation.
- The reference branch (`wip/cli-styling-reference`) is 379 commits behind release-wt, but `internal/style/style.go` has zero deps on other sworn packages — the copy-verbatim direction is safe.
- `cmd/sworn/init.go` carries recent S60 changes (db44c5c) on this branch. S61's styling edits are additive (wrap output strings only) — no semantic conflict with S60's control-flow restructure.

## Suggested ack reply
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
