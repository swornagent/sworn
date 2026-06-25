<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design looks clean — 1 mechanical pin + 2 minor flags:

1. **Populate `design_decisions` in status.json.** Add three Type-2 entries matching design.md §2 decisions (rule-clause placement → 11-process-global-mutation.md; Step 7 insertion; four-pattern scope). Confirm with `sworn designfit 2026-06-19-safe-parallelism` passing.

Flags (not pins): (a) S36 (T12, planned) also touches captain.md — sequential in T12, no conflict, name the handoff in proof.md; (b) §4 stale planned_files reference to 02-no-silent-deferrals.md — ignore, NOT-doing item is correct.

§2 decisions (rule-clause placement, Step 7 location, four-pattern scope) ack — all Type-2, well-motivated, consistent with spec Risks mitigation. §6 open questions: none to ack.

Address pin 1 inline at implementation start, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Single mechanical pin (missing design_decisions field) is apply-inline; design is otherwise sound and AC-aligned.
-->
