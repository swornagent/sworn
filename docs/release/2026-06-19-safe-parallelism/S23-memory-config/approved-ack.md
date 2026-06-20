<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound; all spec ACs addressed; Risk mitigations correctly cited. 4 mechanical pins to apply inline before coding:

1. **Path-encoding test.** Add `TestEncodeProjectPath` (or similar) to `internal/memory/config_test.go` asserting at least one fixed path → encoded-string pair (e.g., `/home/brad/projects/sworn` → `-home-brad-projects-sworn`). Required by Spec Risk 1 mitigation; verifier will check for it.
2. **Populate `design_decisions` in status.json.** Classify D1 (array-replace merge) and D3 (embedding config in memory.json) as Type-1 with `decision_record` citing this design-review ack. D2, D4, D5 may be Type-2. Format per `state.DesignDecision` struct (see `internal/designfit/designfit_test.go` lines 48–84 for field names). Required before `sworn designfit` provides meaningful coverage.
3. **Resolve `cmd/sworn/memory_test.go` ambiguity.** Either add `cmd/sworn/memory_test.go` to `status.json.planned_files` (if `TestCmdMemory_Status` will be a separate file) or remove the reference from §5 (if co-located inline). Pick one before writing code.
4. **Acknowledge cross-track main.go merge.** Add a one-line note to `status.json.open_deferrals` flagging `cmd/sworn/main.go` as a 3-way additive merge touchpoint with T3 (S06a) and T4 (S08a). Mechanical note for the merge coordinator, no code change.

Flags (not pins): (a) `usage()` won't list `memory` — acceptable per spec scope; (b) `<set>`/`<not set>` sentinel uses angle brackets — cosmetic only.

§2 decisions (D1–D5) ack; all well-reasoned. §6 empty — no open questions. Proceed to `in_progress` after applying pins 1–4 inline.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are mechanical apply-inline fixes (missing test, status.json field, file list clarification, cross-track note) — none require a design revision; Verifier (Rule 7) backstops.
-->
