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
