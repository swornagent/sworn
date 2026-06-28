# Captain review — S05-merge-gate-oracle
Date: 2026-06-28
Design commit: 383c6a413f739aa99eb8be1b990421bca31c72aa

## Pins

1. [mechanical] design.md (body) — design.md is corrupted with leaked model control tokens and a duplicated block.
   What I observed: lines 122–137 contain `<｜end▁of▁thinking｜>` and a `<｜｜DSML｜｜parameter name="new_string" ...>` token, followed by a verbatim re-paste of the "New test files" and "Risks / pins for reviewer" sections that already appear at lines 109–122. The substantive content (Approach, §1–3, Files touched, AC traceability) is all present; the middle is a generation glitch.
   What to ask the implementer: strip the two leaked control tokens and delete the duplicated block before this design.md is committed to the durable record. Pure doc cleanup — no design change.

2. [mechanical] §2 / spec AC6 — spec names MCP tools `merge_track` / `merge_release` that do not exist; the real merge-gate tool is `approve_merge`.
   What I observed: spec "In scope" and AC6 say "fix the existing `merge_track` and `merge_release` MCP tools". `internal/mcp/tools_ops.go:126` registers exactly one merge-gate tool — `approve_merge` (`handleApproveMerge`, tools_ops.go:383). The design correctly targets `handleApproveMerge` and `readReleaseBoard` (tools_ops.go:207), so the design is right; the spec text is stale.
   What to ask the implementer: satisfy AC6 against `approve_merge` (the design already does). Spec text naming is a smaller flag (see Flags) — fix inline in proof/AC mapping; a spec rewrite via `/replan-release` is optional, not required.

3. [mechanical] Files touched — design adds `internal/git/git.go` (new `MergeDryRun`/`ResetMerge`/`MergeAbort`) but git.go is not in the spec Planned touchpoints or status.json `planned_files`.
   What I observed: spec Planned touchpoints and status.json `planned_files` list only `cmd/sworn/merge.go`, `internal/mcp/tools_ops.go`, `internal/router/router.go`. The design's "Files touched" table adds a fourth file, `internal/git/git.go`. git.go currently has only `Merge` (git.go:92) and `IsAncestor` (git.go:153); the three dry-run helpers are genuinely new. git.go was last touched by S21 (verified, merged) — additive, no collision.
   What to ask the implementer: add `internal/git/git.go` to status.json `planned_files`/`actual_files` so the touchpoint is declared (Rule 2 / Rule 6 fidelity). No scope question — the methods are needed; just declare them.

4. [mechanical] §1 / spec deferral — the journey-gate deferral premise is now stale: `.sworn/journeys.json` exists and is ratified.
   What I observed: spec "Deferrals allowed?" and design §1 both assume "journeys.json does not yet exist (S17 not yet shipped)" and that the journey gate "may be stubbed." Live state contradicts this: S17-journeys-declare is `verified`, T4 is `merged`, this track is 0 commits behind release-wt, and `.sworn/journeys.json` exists with `ratification.is_ratified: true`. `journey.Check(projectRoot)` (internal/journey/journey.go:242) is real and returns `CheckPass`/`CheckMissing`/`CheckUnratified`.
   What to ask the implementer: wire the REAL `journey.Check` (the design already calls it and fails closed — keep that, do not hard-stub "always block"). AC4 is now testable against the real artefact: real `journey.Check` returns `CheckPass` today, so merge-release should proceed; an absent/unratified artefact must block with the spec's exact message (see Flag c). The open_deferral in status.json can be closed.

5. [mechanical] Rule 9 design-fit — status.json carries no `design_decisions`; the invariant-4 source-of-truth choice and the oracle-injection signature change are unrecorded.
   What I observed: status.json has no `design_decisions` field. The design makes at least two non-trivial choices: (a) invariant-4 shared-file source of truth = parse `index.md` touchpoint matrix with a `status.json` `planned_files` fallback (design Risk #1) — this gates whole-release merges; (b) injecting `*git.Repo` into `RegisterOpsTools`, changing its signature (2 callers: cmd/sworn/mcp.go:34, internal/mcp/tools_test.go:177). Choice (a) is plausibly Type-1 (it shapes a release-blocking gate); the design does present two options with trade-offs in Risk #1, which is the Rule 9 evidence, but it is not recorded in status.json.
   What to ask the implementer: populate `design_decisions` and classify the invariant-4 source-of-truth choice (Type-1 vs Type-2) so the design-fit gate passes; record the index.md-matrix-with-fallback rationale already written in Risk #1.

6. [mechanical] Rule 11 process-global mutation — design restores well and asserts a clean tree, but does not state a fail-closed assertion that git ops target the EXPECTED worktree/branch.
   What I observed: the invariant-4 classifier runs `git merge --no-commit --no-ff` (mutates the working tree) and `cmdMergeTrack` runs the real `repo.Merge`. The design covers Rule 11's restore arm (abort/`reset --merge` in every path, §3 steps 3/5/6) and one assertion (working-tree clean, §3 step 1). It does not state the Rule 11 "fail-closed target assertion" — assert the operating worktree/branch is the intended one before mutating — nor a reachability artefact proving the guard fires.
   What to ask the implementer: before any merge/dry-run, assert the target worktree+branch is the expected one (Rule 11), and add a test proving the guard fires (e.g. wrong-directory / wrong-branch → blocked, tree untouched). "A git op in an unexpected directory can corrupt branch state."

7. [mechanical] Rule 1 reachability — planned tests are mock-oracle + mock-git table tests; ensure the reachability artefact exercises the real `sworn merge-track` entry point.
   What I observed: design "New test files" / §1–3 lean on "mock oracle + mock git" table tests. Rule 1 requires the proof to render through the integration point that owns the affordance (the `sworn merge-track` CLI dispatch → oracle → git), not only leaf functions behind mocks. The spec already prescribes the smoke step (`sworn merge-track --dry-run` on a real board).
   What to ask the implementer: capture the spec's reachability artefact — a real `sworn merge-track --dry-run` (or `--dry-run` equivalent) against the live release board, exit code observed — in proof.md, in addition to the mock-backed unit tests.

## Summary

Pins: 7 total — 7 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none would ship the slice broken if unaddressed; pins 4 and 6 carry the most correctness weight (real journey gate behaviour; Rule 11 target assertion).

## Smaller flags (not pins, worth one-line acknowledgement)

(a) Spec AC5 names `board.Oracle.ReadSliceState()`; the actual method is `ReadSliceStatus()` (internal/board/oracle.go:282/562) — the design uses the correct name, so this is a spec-text nit only.
(b) Touchpoint-matrix parser fragility (design Risk #1, self-flagged): the index.md markdown-table parser must tolerate the current format; add a parser unit test against the live `index.md` (the design's 6-file hardcoded list — oai.go, run/slice.go, verify.go, openai_responses.go, verify_test.go, state.go — matches index.md exactly today, so the test has a known-good fixture).
(c) AC4 message string: emit the spec's exact text `BLOCK: no ratified journeys.json — Rule 10 gate`; the design currently says "BLOCK message (per Rule 10)" generically.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Solid, spec-aligned design — anchors verified, documented-shared list matches index.md, Rule 11 mostly covered. 7 pins, all apply-inline; 3 flags. Proceed.

1. **Clean design.md.** Strip the leaked control tokens (`<｜end▁of▁thinking｜>`, the `<｜｜DSML｜｜...>` token) and delete the duplicated "New test files / Risks" block at lines ~122–137 before committing the design record.
2. **AC6 targets `approve_merge`.** Spec names MCP tools `merge_track`/`merge_release` that don't exist — the real tool is `approve_merge` (`handleApproveMerge`). Your design already targets it correctly; just map AC6 → `approve_merge` in proof.
3. **Declare git.go.** Add `internal/git/git.go` to status.json `planned_files`/`actual_files` — your new `MergeDryRun`/`ResetMerge`/`MergeAbort` make it a real (additive, no-collision) touchpoint not in the spec's list.
4. **Journey gate is live, not stubbed.** `.sworn/journeys.json` now exists and is ratified (S17 verified, T4 merged). Keep the real `journey.Check` call (don't hard-stub); AC4 is testable for real — `CheckPass` today → merge-release proceeds; absent/unratified → block. Close the status.json open_deferral.
5. **Record design_decisions.** Populate status.json `design_decisions`: classify the invariant-4 shared-file source-of-truth choice (index.md matrix + status.json fallback — likely Type-1, it gates release merges) and the `RegisterOpsTools(*git.Repo)` signature change, with the rationale already in Risk #1.
6. **Rule 11 target assertion.** You restore well (abort in all paths) and assert a clean tree — also assert the operating worktree+branch is the expected one before any merge/dry-run, and add a test proving the guard fires (wrong dir/branch → blocked, tree untouched).
7. **Real reachability artefact.** Beyond the mock-oracle/mock-git unit tests, capture the spec's smoke step — `sworn merge-track --dry-run` against the live board, exit code observed — in proof.md (Rule 1: render through the CLI entry point, not only leaf mocks).

Flags (not pins): (a) spec AC5 says `ReadSliceState()` but the real method is `ReadSliceStatus()` — your design uses the right name; (b) add a parser unit test for the index.md touchpoint-matrix reader (your 6-file list matches index.md today — good fixture); (c) emit AC4's exact message `BLOCK: no ratified journeys.json — Rule 10 gate`.

§2 decisions (CLI registry pattern, oracle-backed reads, invariant-4 classifier placement in router.go per spec, idempotent merges) acknowledged — all spec-aligned. No project memory entries apply (project memory is empty). Design "Risks / pins for reviewer" #1–3 acknowledged and folded into pins 5/6 and flag (b).

Address pins 1–7 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 7 pins are apply-inline (doc cleanup, touchpoint/decision declaration, stale-deferral close, Rule 11/Rule 1 guard-and-artefact); none changes the design enough to need re-review before code.
-->
