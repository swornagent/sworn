TL;DR design is sound and insertion points verified; 6 pins, all mechanical — fix status.json and add the fast-path guard note before writing code.

1. **test_commands (CRITICAL).** Fix both test_commands in status.json: change `go test ./cmd/sworn/... -run Catalog` → `go test ./cmd/sworn/... -run TestInit`; change `go test ./internal/prompt/... -run PlannerPrompt` → `go test ./internal/prompt/... -run Planner`. Current values miss all spec-named tests; verifier would see false green.
2. **planned_files — missing decisions.md.** Add `docs/templates/decisions.md` to `planned_files` in status.json.
3. **planned_files — missing test files.** Add `internal/prompt/prompt_test.go` and `cmd/sworn/init_test.go` to `planned_files` in status.json.
4. **Spec Risk 1 fast-path guard.** Phase 2b text in planner.md must include a "file not found → one note, don't block" branch. Add a note to design §4: "Phase 2b does not block when catalog files are absent — it notes absence and proceeds."
5. **Decision 3 dep-policy ack.** `os.ReadFile` + `os.WriteFile` for verbatim copy — aligns with [[project_dep_policy]] and [[feedback_dep_justification_test]]. No template engine dep warranted. Acked.
6. **S21 collision note.** After writing the init.go catalog block, add a journal.md note naming the line range and prompt structure so S21's implementer can confine their hunk cleanly.

Flags (not pins): (a) stale 8-line proof.md in slice dir — overwrite it when writing the real proof bundle; (b) create `docs/templates/` directory explicitly; (c) planner.md Phase 3 insertion point verified at line 100.

§2 decisions D1 (init.go placement), D2 (Phase 2b insertion point after schema-vs-spec audit note, line 98), D3 (raw markdown — [[memory-cited]] dep policy ack), D4 (overwrite guard interactive-read pattern), D5 (verbatim heading strings) ack. §6 open questions: none.

Address pins 1–6 inline during implementation (status.json updates before first test, guard note before planner.md edit, journal note before marking implemented), then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 6 pins are mechanical apply-inline corrections (status.json field updates, one implementation guard note, one journal entry); no design re-architecture required. Critical pin 2 is unambiguous and fixable in status.json before any code is written; Verifier (Rule 7) backstops.
-->
