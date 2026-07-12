---
title: "Dogfood findings — T4-loop-fidelity engine run (gpt-5.6/gpt-4o/o3), Baton v0.11.0 integration release"
date: 2026-07-13
author: Claude (orchestrator) + Brad (Coach)
release: 2026-07-11-contract-edge-gates
driver: openai/gpt-5.6 (implementer+verifier, medium effort), escalation → openai/gpt-4o → openai/o3, SWORN_DIRECT=1
---

# Loop dogfood — 2026-07-13 (T4-loop-fidelity, gpt-5.6)

## Scope

After Phase-4 specced the 12-slice Baton-v0.11.0 integration release, the Coach
directed an **engine dogfood**: run `sworn loop --parallel` with `openai/gpt-5.6`
(medium) to build the loop-fidelity track (T4) first, via a `depends_on`
re-plan (T1/T2/T3/T5 depends_on T4). Goal: sharpen the axe (harden the loop)
before the loop builds the tracks that stress it harder.

## Outcome

**The loop could NOT autonomously build S07-resume-worktree-reset.** It correctly
blocked on the Definition-of-Ready gate (my spec gap), then hit a cascade of
engine/model bugs while escalating gpt-5.6 → gpt-4o → o3, and finally produced a
**hollow S07** — `status.json` flipped to `implemented`, a 2336-line `proof.md`,
but **zero production code** (`internal/run/parallel.go`/`slice.go` untouched).
Verification stayed INCONCLUSIVE (a verdict-schema bug), so nothing reached
`verified` — the state machine held. Stopped manually to halt o3 spend.

Board integrity preserved: S08/S09/S10 still `planned`; S07 is `implemented`
(hollow — to be discarded); T4 `in_progress`. Mirrors the 2026-06-28 eval verdict:
**engine, not model, is the bottleneck** — verified for zero slices.

## The board-read fix (landed this session)

Before the loop could even see the plan, it read the board from the **working
tree** (integration branch) instead of the **release-wt ref** where /replan-release
commits it — so the 12-slice widening was invisible (it saw the stale 3-slice
board). Fixed: `resolveReleaseBoard()` reads the board from the release-wt ref via
`repo.Show` (mirroring `internal/board` oracle.readTrackInfos + the reference
coach-loop), falling back to the working tree on cold start. Mutation-proven,
committed **`880af26` on release/v0.1.0**, installed. Validated by the loop then
loading **5 tracks / 12 slices, T4 first**.

## Findings (triaged)

| # | Finding | Owner | Fix |
|---|---------|-------|-----|
| A | Loop read board from working tree, not release-wt ref → replanned release invisible | engine → sworn | **FIXED `880af26`** |
| B | Run from *inside* the release worktree → derives `<cwd>-worktrees/...` nested path → `git worktree add` fatal (branch already checked out); no guardrail asserting cwd is the primary repo | engine → sworn | new (Rule-11 target-assertion) |
| C | `--base` flag is a dead no-op (`run.go:40 _ = base`); integration branch comes from board.json | engine → sworn | new (remove or wire) |
| D | Cost telemetry blind: no pricing entry for gpt-5.6 / gpt-4o / o3 → CostSource=unknown, $0 (finding 3 reproduced, not grok-specific) | engine → sworn | **S04 provider-registry** |
| E | Autonomous mode runs a gpt-5.6 **captain** design-review, does not halt for the human Coach (finding 6 reproduced) | protocol/engine | **S10 autonomous-design-authority** |
| F | **DoR gate correctly blocks** planned→in_progress: the T4 specs have (1) no AC `test_refs` (orphaned_ac_no_test), (2) no human-ratified `validation` record (positive/negative scenarios + benefit hypothesis). Rule 8 working; my Phase-4 left DoR incomplete. | planning (mine) | complete DoR: add test_refs + reqvalidate records |
| G | `orphaned_ac_no_test` chicken-and-egg: DoR (planned→in_progress) requires AC test_refs, but tests don't exist until implementation (which DoR gates). Planner must name *intended* tests in test_refs, or the check must allow named-but-absent tests pre-implementation. | protocol/engine | clarify Rule 8 / DoR |
| H | sworn sends `reasoning.effort` to **every** openai/ model, incl. gpt-4o which rejects it (`Unsupported parameter: 'reasoning.effort'`) → the default escalation model (gpt-4o) fails immediately at turn 0, poisoning the escalation chain | engine → sworn | send reasoning.effort only to reasoning-capable models (capability-aware, S04/ADR-0013) |
| I | Responses API agent-loop bug: `agent: turn 2: Missing required parameter: 'input[6].output'` (gpt-5.6) — a tool-result item is dropped from the Responses `input` array mid-loop | engine → sworn | new (Responses tool-call sequencing) |
| J | `verifier-verdict-v1` emission: verifier emits `violations[].proposed_amendment: null`, schema wants string → verdict fails validation → INCONCLUSIVE (blocks verification even of good work) | engine → sworn | new (nullable field / emission fix) |
| K | **Triage misescalation**: a DoR block (requirements), an API param mismatch (H), and a verdict-schema failure (J) all triaged as `escalate_model` → burned gpt-5.6, gpt-4o, **o3** against non-model-solvable failures. Triage must classify these as non-model and page/stop, not escalate. | engine → sworn | new (triage classifier) |
| L | **Hollow implementation**: S07 → `implemented` with a 2336-line `proof.md` but ZERO production code (Rule 1 reachability + Rule 6 proof-bundle violation). The `implemented` checkpoint didn't detect "no touchpoint code changed." | engine → sworn | new (reachability/dark-code gate at `implemented`) |

## Verdict + recommendation

**The loop cannot build its own fixes** — the bootstrapping trap is confirmed.
Findings H/I/J/K/L block autonomous slice-building with gpt-5.6/gpt-4o/o3, and
several of them ARE the loop-fidelity slices (T4/T5). So the loop-hardening fixes
must be **hand-implemented** (the findings are the spec), then the loop can be
re-dogfooded on the remaining tracks.

Cheap high-impact first: **H** (reasoning.effort capability guard — unblocks the
escalation chain), **K** (triage: don't escalate models on DoR/API/schema), **J**
(verdict nullable field). Then **F** (complete DoR on the specs: test_refs +
validation records). **I** (Responses agent-loop) and **L** (hollow-impl gate) are
deeper. **D**/**E**/**G** are already-planned slices (S04/S10) or protocol notes.

## Cleanup pending

- Discard hollow S07: reset the T4 track worktree + revert S07 status to `planned`.
- The board-read fix (`880af26`) stays — it is validated and correct.
