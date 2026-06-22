# Captain review — S18-consideration-catalog
Date: 2026-06-23
Design commit: 78e9d37d7647429d2dd9da3f77382462af0b37e8

## Pins

1. **[mechanical]** §3.planned_files — `docs/templates/decisions.md` missing from status.json `planned_files`
   What I observed: Design §3 explicitly lists `docs/templates/decisions.md` as a new template file ("empty decision registry with documented entry format and three example entries"). It is absent from status.json `planned_files` (`docs/templates/considerations.md` is listed but `decisions.md` is not). S35 mutation-guard and the verifier both scope to `planned_files`; an absent entry lets the file ship unchecked.
   What to ask the implementer: Add `docs/templates/decisions.md` to `planned_files` in status.json before writing any code.

2. **[mechanical] CRITICAL** §5.test_commands — both test filter flags miss the spec's test names → false green at verification
   What I observed: status.json carries two broken test_commands:
   - `go test ./cmd/sworn/... -run Catalog` — spec's Required Tests section names the tests `TestInitCreatesBothTemplates`, `TestInitSkipsBoth`, `TestInitOverwriteGuard`. None contain "Catalog". Go exits 0 when no tests match `-run`, so the verifier would see a green output covering zero actual tests.
   - `go test ./internal/prompt/... -run PlannerPrompt` — spec names the tests `TestPlannerHasPhase2b` and `TestPlannerPhase2bDRYGate`. Neither contains "PlannerPrompt"; same false-green risk. The spec's AC explicitly requires `-run Planner`.
   What to ask the implementer: Update status.json test_commands to match the test names from spec. Correct commands: `go test ./cmd/sworn/... -run TestInit` (matches all three `TestInit*` tests) and `go test ./internal/prompt/... -run Planner` (matches `TestPlannerHasPhase2b` and `TestPlannerPhase2bDRYGate`, per AC[8]).

3. **[mechanical]** §3.planned_files — test files absent from status.json `planned_files`
   What I observed: Design §3 lists `internal/prompt/prompt_test.go` (extend) and `cmd/sworn/init_test.go` (new) as files the implementer will touch. Neither appears in status.json `planned_files`. The `cmd/sworn/init_test.go` is a new file and particularly load-bearing (S21-canonical-baton also lists it in its `planned_files` — see Pin 6). Leaving it untracked removes the verifier's visibility into whether the new test file was actually created.
   What to ask the implementer: Add `internal/prompt/prompt_test.go` and `cmd/sworn/init_test.go` to `planned_files` in status.json.

4. **[mechanical]** §1 vs Spec Risk 1 — fast-path for missing catalog files not acknowledged in design
   What I observed: Spec Risk 1 reads: "Phase 2b in planner.md must keep the 'file not found → one note, don't block' branch as the fast path for projects without a catalog." This is load-bearing binding direction — the planner must be usable without a fully configured catalog. Design §1 and §2 describe how the planner checks `docs/decisions.md` and runs consultations but make no mention of what happens when the catalog files are absent. Design §4 lists deferrals but doesn't note the fast-path guard as a required behaviour.
   What to ask the implementer: Confirm that the Phase 2b text written into `internal/prompt/planner.md` explicitly handles the "file not found" case with a single note rather than a blocking gate. Add a NOT-doing or implementation note in design §4 ("Phase 2b does not block when catalog files are absent — it notes their absence and proceeds") so the verifier knows to check this path.

5. **[memory-cited]** §2.3 — Decision 3 (raw markdown, no template engine) aligns with dep policy and dep justification test
   What I observed: Decision 3 says: "Templates are raw markdown, not Go templates... Adding a template engine now would be speculative complexity." This is the correct call for a one-shot verbatim copy. The templates are `sworn`-authored, static, and not user-variable at copy time — the same profile as the yaml.v3 call in S08c that was correctly rejected.
   Citation: [[project_dep_policy]], [[feedback_dep_justification_test]]
   What to ask the implementer: Ack confirms the citation — `os.ReadFile` + `os.WriteFile` is correct; no template engine dep warranted.

6. **[mechanical]** §6.inter-slice — S21-canonical-baton has `cmd/sworn/init.go` and `cmd/sworn/init_test.go` in its `planned_files`; same files S18 creates/modifies
   What I observed: S21-canonical-baton (state: `planned`, same T3-commercial track) lists `cmd/sworn/init.go` and `cmd/sworn/init_test.go` in its `planned_files`. S18 both modifies `init.go` and creates `init_test.go` (new). Both slices are serial in T3-commercial; S18 lands first. Since the track is serial-owned, merge ordering resolves itself — S21 must confine its `init.go` hunk to its own additions (baton-vendor section) and re-run the shared init tests after its changes.
   What to ask the implementer: Note in journal.md what the init.go catalog section looks like post-S18 (which line range, what the new prompt block is named). S21's implementer will need that anchor to confine their hunk cleanly.

## Summary
Pins: 6 total — 5 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: Pin 2 (false green at verification — both test_commands miss the spec's test names; verifier would run 0 tests and see exit 0)

## Smaller flags (not pins, worth one-line ack)

(a) A stale `proof.md` (8 lines) already exists in the slice directory despite state being `design_review`. The verifier must regenerate proof from live repo state per Rule 6; the stale file should be overwritten or deleted before proof bundle creation.

(b) Design §2.2 names the insertion anchor as "the last paragraph of Phase 2 (the schema-vs-spec audit note)." Verified: the "Schema-vs-spec audit" paragraph is at planner.md line 98, immediately before Phase 3's heading at line 100. The insertion point is correct; no action needed.

(c) The `docs/templates/` directory does not yet exist in the worktree. The implementer must create it; `os.WriteFile` won't fail since the spec-side copy logic uses `os.MkdirAll` or similar, but this should be explicit in the implementation.

## Suggested ack reply

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
