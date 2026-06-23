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
