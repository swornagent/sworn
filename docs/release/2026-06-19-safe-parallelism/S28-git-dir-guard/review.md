# Captain review — S28-git-dir-guard
Date: 2026-06-21
Design commit: c511a13b2bb8a5d0f9dc43a01c5b71b9c75664fe

## Pins

1. [mechanical] §2.D3 — `os.Chdir` in `TestEmptyDirDoesNotTouchCwd` should be replaced with `t.Chdir()`
   What I observed: Design Decision 3 says the test "creates a temp git repo, `os.Chdir`s into it." `os.Chdir` is process-wide state. In the rare case a future test in the same package calls `t.Parallel()`, or if `os.Chdir` is not restored via `defer os.Chdir(origDir)`, subsequent tests see a changed cwd and flake.
   What to ask the implementer: The project is on Go 1.26 (≥ 1.24), so `t.Chdir(tempDir)` is available — it saves and restores cwd automatically and marks the test parallel-unsafe. Use `t.Chdir(tempDir)` instead of `os.Chdir(tempDir)`. If for any reason `os.Chdir` is used directly, add `origDir, _ := os.Getwd(); defer os.Chdir(origDir)` and confirm the test does not call `t.Parallel()`.

2. [mechanical] Step 2b — `design_decisions` absent from `status.json`; `sworn designfit` gate cannot evaluate
   What I observed: `status.json` has no `design_decisions` field. Five §2 decisions are present in design.md, each requiring a Type-1 / Type-2 classification before the design-fit gate can pass. Without the field, `sworn designfit 2026-06-19-safe-parallelism` returns no data for this slice.
   What to ask the implementer: Before transitioning to `in_progress`, add `"design_decisions"` to `status.json` with each §2 decision classified as `"type_1"` (architecturally significant, hard to reverse) or `"type_2"` (local, reversible). Suggested classification: D1 (guard in `run()` not `New()`) → Type-2 (single-function, obvious, reversible); D2 (error includes git args) → Type-2; D3 (test uses `t.Chdir`) → Type-2; D4 (no callers need fixing) → Type-2; D5 (error not panic) → Type-2. All five are Type-2, which is correct — no Coach ack needed under Rule 9.

## Summary

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none (no AC would ship broken; both pins prevent test flakiness / process gate failure)

## Smaller flags (not pins, worth one-line ack)

(a) Design §1 enumerates 8 methods ("Checkout, Branch, Commit, Merge, Stage, RevParse, DiffRange, Init") but the live `git.go` also has `DiffRangeStat` (a 9th method). Since the guard is in `run()`, `DiffRangeStat` is covered automatically — no action required, but proof.md should note all 9 methods are guarded rather than 8.

(b) AC1 specifies `.Checkout("main")`, `.Branch("x")`, `.Commit("m")` as the tested methods. The `Commit()` method uses `--allow-empty` internally, but the guard fires before the git binary is invoked, so this flag does not affect the test. Confirm `TestRunRejectsEmptyDir` also exercises `.Commit()` to satisfy AC1 completely.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean, focused design — 2 mechanical pins to apply inline:

1. **`t.Chdir()` in cwd test.** Replace `os.Chdir(tempDir)` with `t.Chdir(tempDir)` in `TestEmptyDirDoesNotTouchCwd`. Go 1.26 supports it; it restores cwd automatically and blocks parallel misuse. No `defer os.Chdir` needed.
2. **`design_decisions` in status.json.** Before transitioning to `in_progress`, add the field. All five §2 decisions are Type-2 (local, reversible). Suggested JSON: `"design_decisions": [{"id": "D1", "type": "type_2", "summary": "Guard in run() not New()"}, {"id": "D2", "type": "type_2", "summary": "Error includes git args"}, {"id": "D3", "type": "type_2", "summary": "Test uses t.Chdir"}, {"id": "D4", "type": "type_2", "summary": "No callers need fixing"}, {"id": "D5", "type": "type_2", "summary": "Error not panic"}]`

Flags (not pins): (a) `DiffRangeStat` is a 9th method covered by the run() guard — note it in proof.md; (b) confirm `TestRunRejectsEmptyDir` exercises `.Commit()` to complete AC1.

§2 decisions D1–D5 all Type-2 ack. §6 no open questions ack.

Address pins 1–2 inline before transitioning to in_progress, then proceed.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline mechanical corrections (swap os.Chdir → t.Chdir; add design_decisions to status.json); no design re-check needed before code is safe.
-->
