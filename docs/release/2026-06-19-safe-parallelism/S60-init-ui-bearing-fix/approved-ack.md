<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design — 1 mechanical pin to apply inline, then proceed:

1. **design_decisions missing.** Add a `design_decisions` array to `docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/status.json` covering the 5 decisions from design.md §2, each with `"type": "type_2"`. No human_decision required for any of them (all reversible / scoped). Apply before transitioning to in_progress.

Flags (not pins): (a) S61-cli-output-styling also touches init.go — when S61 runs, its implementer must account for S60's restructured design-system block.

§2 decisions all Type-2, no memory conflicts — ack. §6 questions none — ack.

Address pin 1 inline during implementation setup, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Single mechanical pin (design_decisions missing in status.json) is an apply-inline fix that doesn't change the design; Verifier backstops correctness.
-->
