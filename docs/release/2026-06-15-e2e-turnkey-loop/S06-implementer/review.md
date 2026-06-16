# Captain review — S06-implementer
Date: 2026-06-16
Captain version: 0.1
Design TL;DR commit: 8a64a228a47359abd57b4dec98834dabcb4c9a47

## Pins

1. [mechanical] §2.4 — Embedded implementer prompt carries human-orchestration instructions into the agentic loop
   What I observed: `prompt.Implementer()` (embedded at `internal/prompt/implementer.md`) was authored for a human-pasted, single-slice workflow. It includes ~150 lines of instructions about track worktree auto-discovery, server lifecycle (`baton-server-start.sh`), "output to the human" wrap-up prose, and references to files (`docs/baton/track-mode.md`, `index.md`) that may not exist in the target workspace. When used as the system prompt for `agent.Run()`, the model may waste turns attempting to follow these instructions.
   What to ask the implementer: Confirm via a smoke test with a real model (after S06 lands) that the agent successfully implements a trivial spec against the full implementer prompt. The model should handle missing-file errors gracefully (standard tool-loop resilience). If it gets stuck on human-orchestration instructions, a trimmed-down variant may be warranted.

2. [mechanical] §3 vs state machine — Illegal transition risk: `design_review → implemented`
   What I observed: The design says the S06 `Run` function "updates `status.json` to `implemented`" after the agent loop completes. The embedded implementer prompt instructs the model to transition `design_review → in_progress` as its first step. But if the model errors before doing that transition (or the workspace lacks a `status.json`), the S06 package would attempt `design_review → implemented`, which is an illegal state transition per `internal/state/state.go` (only `in_progress → implemented` is allowed). The `state.Write` call would fail.
   What to ask the implementer: Add a guard before the final transition — read current state, and if it's `design_review` (model didn't transition), transition to `in_progress` first, then to `implemented`. Or transition to `in_progress` in the S06 package itself before launching the agentic loop.

3. [mechanical] §6 — Test fixture: inline constant vs testdata directory
   What I observed: The implementer asks whether to inline a minimal spec as a test constant or use a `testdata/specs/` directory. Either works for a trivial spec fixture. The spec says "a fake model scripted to implement a trivial spec in a temp repo" but doesn't prescribe where the trivial spec lives.
   What to ask the implementer: Inline constant is fine for a single trivial spec. If future slices need reusable spec fixtures, move to `testdata/` then. Proceed with inline constant.

## Summary

Pins: 3 total — 3 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): Pin 2 — if the state transition fails, the slice won't reach `implemented`. Address before code.

## Smaller flags (not pins, worth one-line ack)

- The design doesn't mention handling `agent.Run` returning an error (turn cap, Chat failure). A failure should leave the slice in a non-`implemented` state, not silently write a partial proof. (Standard `Run` semantics — return error, don't transition status.)
- Decision 2 says the `Run` signature mirrors `verify.Run`, but `verify.Run` returns a `verdict.Result` while this returns `error`. The pattern is the same shape (single blocking call) but the return types differ. Not a problem — just note the asymmetry.

## Suggested ack reply

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