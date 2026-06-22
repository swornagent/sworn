<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Clean design on a well-scoped new-file slice. 5 pins — all apply-inline:

1. **Add `design_decisions` to status.json.** §2 lists 5 decisions; all appear Type-2. Add them to the `design_decisions` array in `status.json` before coding starts and run `sworn designfit 2026-06-19-safe-parallelism` to confirm the gate passes.
2. **Fix Decision 1 framing.** Change "Same stdlib-only approach" → "stdlib is sufficient for this slice's text-file ops; no new dependency, no ADR required." The dep policy is now [[project_dep_policy]] "minimal justified deps + ADR," and "stdlib-only" echoes the deprecated rule.
3. **Drop "request-time" from §4.** `prompt.go` loads embedded files at `init()`. The conclusion (no code change needed) is correct — S19's prompt changes are confirmed on the T7 branch. Do not carry "at request time" into any code comment for this slice.
4. **Resolve `create_release` in `intake.md`.** Either update the two references to `plan_release`, or declare `intake.md` exempt as historical context with a Rule-2 tracking note (why + tracking + ack). Silent skip is blocked by Risk 1's explicit requirement.
5. **Guard AC2's `slice_count: 24` in proof.md.** Confirm `TestPlanReleaseExisting` asserts against the fixture count (not literally 24). Note in `proof.md` that the real release count exceeds 24 and AC2 passes on the fixture.

Flags (not pins): (a) `plan_release` Returns schema in spec omits `slice_count`/`release_worktree_branch` — implement from the descriptive text, not just the Returns line; (b) create the screenshots directory before writing the proof.md reachability artefact.

§2 decisions 1–5 ack (subject to Pin 1 + 2 corrections). §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline corrections — missing status.json field, framing fix, comment guard, intake.md triage, AC fixture confirmation. Design is structurally sound with correct registration pattern, file plan, and stdlib choice. No redesign needed before code.
-->
