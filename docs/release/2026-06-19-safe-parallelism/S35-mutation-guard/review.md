# Captain review — S35-mutation-guard
Date: 2026-07-03
Design commit: 4a74b638f56c5833e3622abc049ccc31e0644b4c

## Pins

1. [mechanical] §2b — `design_decisions` absent from status.json
   What I observed: status.json has no `design_decisions` field. design.md §2 presents three decisions (rule-clause placement, Step 7 placement, four-pattern scope). All appear Type-2, but `sworn designfit` requires the field to be populated for the gate to pass.
   What to ask the implementer: Populate `design_decisions` in status.json before or at the start of implementation — three entries, all stake_class Type-2, with the human_decision and rationale from §2. Confirmed by `sworn designfit 2026-06-19-safe-parallelism` passing.

## Summary

Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none — the slice does not ship broken without the design_decisions field, but the Verifier will FAIL at the designfit gate.

## Smaller flags (not pins, worth one-line ack)

(a) S36-captain-resolve-dirty-worktree (T12, state: planned) lists `internal/prompt/captain.md` in its planned_files. S35 lands first in T12's serial order — no current collision — but the Step 7 insertion point (between Step 6 and `## Output`) should be stable enough that S36's future edit lands cleanly after it without touching Step 7's content. No action required now; worth naming in the proof bundle.

(b) design.md §4 states "status.json's `planned_files` lists it [02-no-silent-deferrals.md] only as a placeholder pending the implementer's rules-dir read." Current status.json planned_files does not list 02-no-silent-deferrals.md — this rationale cites a prior state of the file. The NOT-doing item itself is correct (don't touch Rule 2); only the supporting rationale is stale. No functional impact.

(c) [[project_coach_loop_worktree_hygiene]] confirms S35 is the process-side durable fix for the dirty-worktree bug class (memory explicitly names S35-mutation-guard + S36-captain-resolve-dirty-worktree as the durable fix). Design scope — Captain check + Baton rule — is consistent with that framing.

## Suggested ack reply
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
