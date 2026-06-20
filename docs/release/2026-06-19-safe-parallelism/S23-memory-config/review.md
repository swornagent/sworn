# Captain review — S23-memory-config
Date: 2026-06-21
Design commit: 9cd540dded207501d8c2a6b7c70b01546adb12c9

## Pins

1. [mechanical] §2.D2 / Spec Risk 1 — Path-encoding test prescribed by spec Risk mitigation is absent from planned tests.
   What I observed: Spec Risk 1 mitigation reads "add a test with a known path → encoded mapping." Verified: `captain-memory-search.py` line 91 uses `str(project_dir).replace("/", "-")` — design Decision 2 replicates this correctly in Go via `strings.ReplaceAll(path, "/", "-")`. However, the planned tests (`TestLoadMerge`, `TestDefaultsAutoDetect`, `TestUnknownHarness`, `TestAPIKeyEnvNotLeaked`) contain no test asserting a fixed known-pair encoding, e.g. `/home/user/myproject` → `-home-user-myproject`. `TestDefaultsAutoDetect` verifies the auto-detect logic (path on disk) but not the encoding algorithm's correctness against a reference value.
   What to ask the implementer: Add `TestEncodeProjectPath` (or equivalent) to `internal/memory/config_test.go` asserting at least one known path → encoded string pair (e.g., `/home/brad/projects/sworn` → `-home-brad-projects-sworn`). This test is the spec Risk 1 mitigation; without it the verifier will flag Gate 3 (required tests missing).

2. [mechanical] §2 / Step 2b — `design_decisions` absent from status.json; designfit gate passes trivially (silently unchecked).
   What I observed: `status.json` has no `design_decisions` field. Checked `internal/designfit/designfit.go` line 126: `if len(st.DesignDecisions) == 0 { // No design decisions means no design-fit gate to enforce. }` — the gate passes for S23 without evaluating anything. Two of the five §2 decisions are architecturally significant (Type-1 per Rule 9): D1 (arrays-replaced merge semantic — S24/S25 are built on this contract, hard to reverse) and D3 (embedding config placement in `MemoryConfig.Embedding` — S24 imports this struct directly). Without `design_decisions` populated, the design-fit gate provides no coverage.
   What to ask the implementer: Populate `design_decisions` in `status.json` before transitioning to `in_progress`. D1 and D3 must be classified Type-1 with a recorded human decision (this design-review ack constitutes the human decision; cite it in the `decision_record` field). D2, D4, D5 may be classified Type-2. Format per the `state.DesignDecision` struct in `internal/designfit/designfit_test.go` lines 48/84.

3. [mechanical] §5 — `cmd/sworn/memory_test.go` mentioned in reachability plan but not in planned_files.
   What I observed: Design §5 item 4 says "Integration test: `TestCmdMemory_Status` in `cmd/sworn/memory_test.go` (or inline in memory.go's test file)." `status.json.planned_files` lists `internal/memory/config_test.go` but not `cmd/sworn/memory_test.go`. The spec's Required Tests section also doesn't list this file. The "or inline" qualifier leaves the file's existence ambiguous.
   What to ask the implementer: Resolve the ambiguity before writing code: if `TestCmdMemory_Status` will be a separate file, add `cmd/sworn/memory_test.go` to `status.json.planned_files`. If it will be co-located inline in `internal/memory/config_test.go` (or skipped in favour of the smoke-step reachability artefact in §5 items 1–3), remove the `cmd/sworn/memory_test.go` reference from §5 so the verifier has an unambiguous file list.

4. [mechanical] §3 / Step 6 — `cmd/sworn/main.go` cross-track collision unacknowledged.
   What I observed: S06a-sworn-login-auth (T3-commercial, state: design_review, planned_files includes `cmd/sworn/main.go`) and S08a-mcp-transport (T4-mcp, state: design_review, planned_files includes `cmd/sworn/main.go`) both plan additive switch cases on the same file. All three slices (S23, S06a, S08a) add independent additive cases to the switch — no semantic conflict — but the release-wt merge coordinator will face a 3-way switch-case merge when these tracks land. The design does not acknowledge this.
   What to ask the implementer: Add a note to `status.json.open_deferrals` (or as a comment in the design) flagging `cmd/sworn/main.go` as a cross-track merge touchpoint with T3 and T4. No code change needed; the merge is mechanical. This is advisory so the merge coordinator doesn't treat the conflict as unexpected.

## Summary

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: Pin 1 (spec-required test missing — verifier will fail Gate 3), Pin 2 (designfit coverage gap — Type-1 decisions go unreviewed by the gate)

## Smaller flags (not pins, worth one-line ack)

(a) **`usage()` does not list `memory`** — by spec design ("additive dispatch only"), so `sworn --help` won't mention `memory`. Users must already know the subcommand. Acceptable given spec scope; worth noting for a future discoverability pass.

(b) **`<set>`/`<not set>` sentinel notation** — angle brackets in CLI output can be misread as template placeholders. Consider `(set)`/`(not set)` for shell-cleanliness. Cosmetic only; not blocking.

## Suggested ack reply
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
