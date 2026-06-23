# Captain review — S36-captain-resolve-dirty-worktree
Date: 2026-07-04
Design commit: bec3d685cf55534130737e2ab6146198a04547df

## Pins

1. [mechanical] §1B/Risk2 — Detector contract deviates from spec Risk 2 mitigation without acknowledgement
   What I observed: Spec Risk 2 says "The detector must not fire on benign untracked build artefacts (e.g. a stray `sworn` binary) — scope the dirty-check to tracked changes + intended touchpoints." Design Decision 3 specifies the gate contract as "`git status --porcelain` at the gate point, non-empty → dispatch Captain's `resolve-dirty-worktree`" — this is unfiltered and WILL fire on a stray `sworn` binary or other untracked artefact. Design Decision 2 handles build artefacts as a *discard* case in the Captain's resolution, not at the detection layer. The spec says prevent firing; the design delegates classification to Captain.
   What to ask the implementer: Resolve inline by either (a) adding filter logic to the captain.md detector contract (e.g. "dispatch Captain only when `git status --porcelain` shows modifications to tracked files, or untracked files within the slice's declared touchpoints"), or (b) adding an explicit acknowledgement + rationale to Decision 3 documenting why broad detection is preferred over the spec's scope-at-detection approach. Option (a) aligns with spec; option (b) requires acknowledgement of the deviation.

2. [mechanical] §2b — `design_decisions` absent from status.json; `sworn designfit` will fail
   What I observed: S36's `status.json` has no `design_decisions` field. The design.md §2 lists four decisions. S35 (the immediately preceding T12 verified slice) established this pattern: its status.json includes structured `design_decisions` entries with `choice`, `stake_class`, `options`, and `rationale`. Without `design_decisions`, `sworn designfit 2026-06-19-safe-parallelism` will fail at merge. S35's trial-log entry recorded the exact same pin.
   What to ask the implementer: Add the four §2 decisions as structured `design_decisions` entries to status.json (following S35's schema: `choice`, `stake_class`, `options`, `rationale`) before transitioning to `in_progress`. All four decisions appear Type-2; confirm the classification matches the design intent.

3. [mechanical] §3 — `internal/prompt/prompt_test.go` absent from `status.json` planned_files
   What I observed: Design §3 explicitly plans to touch `internal/prompt/prompt_test.go` ("add a test verifying `Captain()` contains the `resolve-dirty-worktree` function name and its commit-by-default rule"). The `status.json` planned_files lists only `"internal/prompt/captain.md"`. The test file is a named file the design commits to changing; it must be declared.
   What to ask the implementer: Add `"internal/prompt/prompt_test.go"` to status.json `planned_files` before writing code.

4. [memory-cited] §2.D2 — Commit-by-default bias aligns with [[project_coach_loop_worktree_hygiene]]
   What I observed: Design Decision 2's commit-by-default rule ("Bias to commit; let Rule 7 catch bad work downstream") directly matches the memory's recorded root cause and durable-fix direction. The memory records: "dominant failure class was dirty worktrees: implementer workers exit a dispatch without committing... durable fix: ship S35-mutation-guard + S36-captain-resolve-dirty-worktree." It also records the exact failure instances (T3/S06a left account_test.go uncommitted; T8/S24 left 21 dirty files) that motivated the commit-by-default bias. The memory also confirms scope: "sworn's run loop already commits after implement (slice.go:148-153), so it won't have the dirty-worktree bug" — consistent with Design Decision 3 scoping the contract to the bash harness.
   Citation: [[project_coach_loop_worktree_hygiene]]
   What to ask the implementer: Decision 2 aligns — ack confirms the citation.

## Summary

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none — no pin would cause the slice to ship broken; pins 1–3 are gate-hygiene catches the Verifier would surface.

## Smaller flags (not pins, worth one-line ack)

- S27-public-readiness-scrub (planned, T10) also declares both `captain.md` and `prompt_test.go` in its planned_files. T10 depends_on T12 per the board oracle — merge ordering is guaranteed; no collision, but worth noting the downstream touch so S27's captain step is aware of what S36 added.
- The trial log shows S35 hit the identical `design_decisions`-missing pin. Consider a process note: the `sworn designfit` check could be surfaced earlier (at design-review time, before implement-slice opens the session) to reduce this recurring class.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is AC-aligned and memory-cited; 3 mechanical gate-hygiene fixes before code. 3 pins + 1 memory-cited flag:

1. **Detector scope vs. spec Risk 2.** Spec Risk 2 says scope the dirty-check to tracked changes + intended touchpoints (don't fire on a stray `sworn` binary). Design Decision 3 uses `git status --porcelain` unfiltered. Resolve inline: either (a) add filter logic to captain.md's detector contract — "dispatch Captain only when `git status --porcelain` shows tracked changes, or untracked files within the slice's declared touchpoints" — or (b) acknowledge the deviation explicitly in Decision 3 with rationale (e.g., "broad detection preferred because Captain provides a classification record").
2. **Add design_decisions to status.json.** Add the four §2 decisions as structured entries (`choice`, `stake_class`, `options`, `rationale`) following S35's schema. All four are Type-2. Do this before opening in_progress.
3. **Add prompt_test.go to planned_files.** Add `"internal/prompt/prompt_test.go"` to status.json `planned_files` — the test file is an explicit §3 touchpoint.

Decision 2 [memory-cited]: commit-by-default bias aligns with [[project_coach_loop_worktree_hygiene]] — ack confirmed.
§6 open questions: none declared — ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are mechanical gate-hygiene fixes (scope-note, status.json fields, planned_files entry) the implementer applies inline; no design re-review needed; Verifier backstops.
-->
