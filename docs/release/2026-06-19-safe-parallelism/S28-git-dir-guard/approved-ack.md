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
