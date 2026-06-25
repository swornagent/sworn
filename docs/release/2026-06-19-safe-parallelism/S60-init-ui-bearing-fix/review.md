# Captain review — S60-init-ui-bearing-fix
Date: 2026-06-23
Design commit: a148ac4aef9a1de7af13947ba49dd5a90f70be96

## Pins

1. [mechanical] §2b — `design_decisions` missing in status.json
   What I observed: planned_files contains `cmd/sworn/init.go` and `cmd/sworn/init_design_system_test.go`, both of which match the `cmd/sworn/` prefix in `impliesType1Work()` (designfit.go:92). With `design_decisions` absent from status.json, `sworn designfit 2026-06-19-safe-parallelism` emits: "implies Type-1 work (planned_files touch architecturally-significant packages) but design_decisions is empty." This blocks the track merge gate.
   What to ask the implementer: Add a `design_decisions` array to status.json covering the 5 decisions in design.md §2, each with `"type": "type_2"`. All 5 are clearly reversible and scoped; none require human_decision. Apply inline before transitioning to in_progress.

## Summary

Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): Pin 1 blocks the designfit merge gate but does not affect runtime correctness.

## Smaller flags (not pins, worth one-line ack)

(a) S61-cli-output-styling (`planned`, same track T18-cli-polish) also lists `cmd/sworn/init.go` in planned_files. S61 is not yet `in_progress` so no collision now; when S61 is implemented its implementer must account for S60's restructured design-system gate block.

## Suggested ack reply
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
