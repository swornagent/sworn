# Captain review — S33-spec-template-hardening
Date: 2026-06-22
Design commit: 494c669a6b5cecc34cee15c0f9f84e4cbcada137

## Drift gate note
`git rev-list --count track/..release-wt/` returned 2. The two drift commits are `959b717 plan: hold T3-commercial on T15-cli-registry` and `64619c2 chore: materialise T15-cli-registry worktree` — neither touches T12 or S33 artefacts. The spec is not stale; review proceeds. Forward-merge at implementer entry per baton drift protocol.

## Pins

1. [mechanical] §2.D2 + AC(c) — `[[feedback_worktree_devserver_cors_port]]` memory file does not exist; AC(c) cannot be satisfied as written
   What I observed: Design §2 D2 says to place "a staleness note … adjacent to the dynamic-CORS rule" for `[[feedback_worktree_devserver_cors_port]]`. AC(c) says the planner.md change must "mark the `[[feedback_worktree_devserver_cors_port]]` memory stale". `ls ~/.claude/projects/-home-brad-projects-sworn/memory/` shows no such file — every memory in MEMORY.md was enumerated; this slug is absent.
   What to ask the implementer: Confirm whether (a) the inline note in planner.md IS the staleness marking (self-contained, no memory file needed), which would mean AC(c)'s "marks stale" language is fulfilled by the comment text alone, or (b) the memory file must be created first and then annotated stale — in which case the implementer should create the file as part of this slice and update planned_files. Resolve before writing code; the AC is ambiguous without this answer.

2. [escalate] §1 vs §4 — §1 implies WATCHER cleanup will happen; §4 explicitly says NOT editing implementer.md; task (d) scope is unresolved
   What I observed: §1 states "the WATCHER comment-wrapper is cleaned from the end-of-turn status block in the embedded prompt files — though investigation reveals it exists in `implementer.md` (not `verifier.md` as the spec claims)." This reads as a commitment: WATCHER will be cleaned. §4 states "NOT editing `internal/prompt/implementer.md` — its WATCHER block at line 183 is outside this slice's planned touchpoints." These are contradictory. WATCHER at implementer.md:183 is confirmed by grep. Verifier.md has no WATCHER (also confirmed). The spec's planned_files includes verifier.md but that file will not change. Task (d) as specced (clean verifier.md) is trivially satisfied because verifier.md never had WATCHER. The real question is whether WATCHER should also be cleaned from implementer.md.
   What to ask the Coach: Option (a): in-scope — add `internal/prompt/implementer.md` to planned_files and clean the WATCHER block at line 183; §1 wording stands. Option (b): out of scope — §1 must be revised to not imply WATCHER cleanup will happen in this slice; track implementer.md cleanup as a separate slice. Option (b) is the lower-risk choice (no planned_files expansion); option (a) bundles a small, clean change with no Go impact and closes the open WATCHER debt.

3. [escalate] §6.Q2 — external spec template Rule-2 deferral needs explicit Coach ack before code
   What I observed: The spec's touchpoint note "(FLAG FOR HUMAN)" says: "If the rules must also live in the shipped/external template file, add that as a second acknowledged touchpoint before implementation." Rule 2 requires Why + Tracking + Acknowledgement. Why and Tracking are present in the spec; Acknowledgement ("flagged to the human in the replan summary") has not occurred — this design review IS the acknowledgement gate.
   What to ask the Coach: Option (a): explicitly defer — the three rules in `internal/prompt/planner.md` are sufficient for now; the external template is a separate, future concern (close Rule-2 gate with this ack). Option (b): in-scope addition — add `$HOME/.claude/baton/release-mode-template/spec.md` as a second acknowledged touchpoint; the implementer edits both files. One of these options must be on record before the implementer opens the diff.

4. [mechanical] §3 / status.json — planned_files includes verifier.md but design §4 says NOT editing it
   What I observed: `status.json` lists `"internal/prompt/verifier.md"` in `planned_files`. Design §4 says "NOT editing `internal/prompt/verifier.md`". At verification time the Verifier compares `planned_files` vs `actual_files`; verifier.md will not appear in `actual_files`, creating a mismatch the Verifier must explicitly resolve.
   What to ask the implementer: Remove `internal/prompt/verifier.md` from `planned_files` in status.json before starting implementation. This is a one-line JSON edit; do it at the same time as the PIN 2 resolution (add implementer.md if option (a) is chosen, or simply remove verifier.md if option (b) is chosen).

5. [mechanical] §3 / Step 6 — S18-consideration-catalog (planned, T3-commercial) also targets planner.md on a parallel track
   What I observed: `S18-consideration-catalog` (state: `planned`, track: `T3-commercial`) has `internal/prompt/planner.md` in its planned_files. T3-commercial and T12-harness-hardening are parallel tracks (both depend only on T1-concurrency-core). Both will modify planner.md before merging to release-wt/. The second-lander at merge time must be aware of S33's Phase 4 additions to avoid clobbering hunks.
   What to ask the implementer: Add a `touchpoints` note in status.json recording the planner.md collision with S18 (T3-commercial). The standard baton resolution is: second-lander confines their hunk to non-overlapping lines and re-runs `go build ./...`. No sequencing change is required unless S18 and S33 insert rules at the same Phase 4 line.

## Summary
Pins: 5 total — 3 [mechanical], 0 [memory-cited], 2 [escalate]
Critical pins: PIN 1 (AC(c) cannot be verified without resolving memory-file ambiguity), PIN 2 (§1 vs §4 contradiction leaves the implementer unable to determine task (d) scope)

## Smaller flags (not pins, worth one-line ack)
(a) **Drift:** 2 commits in release-wt not yet forward-merged into T12 (T15-cli-registry materialisation + T3 hold); unrelated to S33; implementer should forward-merge at session entry per baton drift protocol.
(b) **Task (d) AC gap:** task (d) WATCHER cleanup is listed in "In scope" but has no corresponding acceptance check (design §2 D4 notes this). If Coach selects option (a) on PIN 2, the proof.md "Delivered" section must document the WATCHER cleanup against the In Scope description, not a formal AC. The Verifier will need to be briefed on this gap.
(c) **designfit gate:** S33's planned_files (`internal/prompt/planner.md`, `internal/prompt/verifier.md`) do not match the `impliesType1Work` prefix set (`cmd/sworn/`, `internal/state/`, `internal/verdict/`), so an empty `design_decisions` in status.json will not cause a gate violation. No designfit concern for this slice.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Markdown-only slice, clean structure. 5 pins:

1. **Memory file for CORS staleness.** AC(c) says "mark `[[feedback_worktree_devserver_cors_port]]` stale" but no such memory file exists. The inline comment in planner.md IS the staleness marking — no separate memory file needs to be created. AC(c) is satisfied by the in-planner comment alone. Proceed on this basis.
2. **Task (d) WATCHER scope — Coach decides.** §1 implies cleanup; §4 says NOT editing implementer.md. Coach has chosen: **[COACH FILLS: option (a) — add implementer.md to planned_files and clean WATCHER at line 183 / option (b) — remove WATCHER promise from §1, task (d) is no-op for this slice, track separately]**.
3. **External template Rule-2 ack.** Coach has decided: **[COACH FILLS: (a) external template deferred — inline note in planner.md only, Rule-2 gate closed / (b) add external template as acknowledged touchpoint]**.
4. **Remove verifier.md from planned_files.** Edit status.json planned_files before opening the diff: remove `"internal/prompt/verifier.md"` (and add `"internal/prompt/implementer.md"` if option (a) chosen for PIN 2). Apply inline.
5. **S18 collision note.** Add a `touchpoints` entry to status.json recording the planner.md overlap with S18 (T3-commercial); note "second-lander confines hunk". Apply inline.

Flags (not pins): (a) forward-merge release-wt into track at session start (2 unrelated commits); (b) if WATCHER cleanup lands, document in proof.md against In Scope rather than a formal AC.

§2 decisions D1 (rule placement in Phase 4), D3 (WATCHER no-op on verifier.md), D5 (no external template edit) ack. §6 Q1 resolved by Coach (PIN 2). §6 Q2 resolved by Coach (PIN 3).

Address pins 1–5 inline during implementation (1, 4, 5 are immediate JSON/comment edits; 2 and 3 require Coach decisions first), then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: two escalate pins (task-d scope in implementer.md and external-template Rule-2 ack) require human-authority decisions before the implementer can open the diff
-->
