<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Clean design — 3 mechanical pins, all addressable inline. One critical (state transition guard). The design correctly isolates proof generation from model narration and keeps the interface narrow.

1. **Implementer prompt in agentic loop.** The embedded `prompt.Implementer()` includes human-orchestration instructions (track worktree discovery, server lifecycle, "tell the human") that may not apply in a machine-driven tool loop. Confirm via smoke test that a real model handles this gracefully with the full prompt. If it gets stuck, a trimmed-down variant goes on the backlog — no need to pre-empt.

2. **State transition guard.** The `Run` function transitions status.json to `implemented` post-hoc. If the model errors before doing `design_review → in_progress` (per the implementer prompt), the package would attempt an illegal `design_review → implemented` transition. Add a guard: read current state first; if still `design_review`, transition to `in_progress`, then `implemented`. Or do the `design_review → in_progress` transition inside `Run` before launching the agentic loop.

3. **Test spec fixture.** Inline constant is fine. Proceed.

Flags (not pins): (a) `agent.Run` errors should leave the slice in a non-`implemented` state — standard `Run` semantics cover this, just be explicit. (b) The `Run` function returns `error` not `verdict.Result` — the asymmetric return from `verify.Run` is fine, just note it.

§2 decisions all ack. §6 question ack (inline constant).

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: 3 mechanical pins, all apply-inline during implementation; no spec deviations, no memory conflicts, no authority calls
-->
