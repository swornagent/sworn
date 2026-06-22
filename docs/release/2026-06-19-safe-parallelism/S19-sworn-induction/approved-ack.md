<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Solid design, clear scope. 5 pins to apply inline before code:

1. **main.go out of planned_files.** Remove `"cmd/sworn/main.go"` from `planned_files` in status.json — spec Risk 3 forbids editing it; having it there causes Gate 2 FAIL at verify.
2. **Add design_decisions to status.json.** Five entries, one per §2 decision, each with `type` (Type-2 for Decisions 1–3 and 5; type for Decision 4 after Pin 3 resolution) and `choice`.
3. **Fix idempotent trigger.** Decision 4: change update-mode detection from `design_system.location` non-empty to `architecture.patterns` non-empty (or file exists with content). This is what AC5 tests against.
4. **No YAML library — ack.** Decision 3 stdlib-only approach confirmed per [[feedback_dep_justification_test]] precedent (same call as S08c).
5. **Tighten test_commands.** Add `go test ./internal/prompt/... -run TestImplementerHasDeviationCheck` as a third prompt test command, or ensure proof.md explicitly names the three new test functions as evidence.

Flags (not pins): (a) verifier.md merge collision with T12 slices — scope your hunk to additions only; (b) frontmatter vs markdown-body parse boundary for `patterns:` vs `project_pinned:` — use different anchors for each; (c) add the three missing test functions before claiming done.

§2 Decision 3 [memory-cited: [[feedback_dep_justification_test]]] ack. §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are unambiguous inline corrections (remove main.go from planned_files, add design_decisions, fix idempotent trigger to match AC5, ack memory citation, tighten test commands) — no design change required; Verifier backstops.
-->
