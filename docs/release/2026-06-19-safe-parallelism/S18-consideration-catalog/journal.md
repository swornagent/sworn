---
title: Slice journal bundle
description: Implementation log. Append-only.
---

# Journal: `S18-consideration-catalog`

## 2026-07-04 — Implementation session

**State transition:** `design_review` → `in_progress` → `implemented`

**Coach ack (Pin 1):** `go test ./cmd/sworn/... -run Catalog` → `go test ./cmd/sworn/... -run TestInit`; `go test ./internal/prompt/... -run PlannerPrompt` → `go test ./internal/prompt/... -run Planner`. Critical — original values would have shown false green at verification (Go exits 0 when no tests match -run).

**Coach ack (Pin 2):** Added `docs/templates/decisions.md` to `planned_files` in status.json.

**Coach ack (Pin 3):** Added `internal/prompt/prompt_test.go` and `cmd/sworn/init_test.go` to `planned_files` in status.json.

**Coach ack (Pin 4):** Phase 2b text includes "do not block" fast-path guard for missing catalog files. Verified by `TestPlannerPhase2bFastPath`.

**Coach ack (Pin 5):** `os.ReadFile` + `os.WriteFile` for verbatim copy — no template engine dep warranted. Aligns with `project_dep_policy` and `feedback_dep_justification_test`.

**Coach ack (Pin 6):** S21 collision note below.

### Design decisions

**D1: Catalog prompt placement in init.go.** Lines 312-333 (catalog prompt + goto done + materialiseCatalog call). Inserted after the implementer-model prompt block and before the final "Done." line. Uses shared `bufio.Reader` (`in`) created at function top (line 25).

**D2: Phase 2b insertion point.** Inserted in `internal/prompt/planner.md` after the Schema-vs-spec audit paragraph (original line 98) and before Phase 3 heading (original line 100). All four sub-step headings match spec acceptance check strings exactly.

**D3: Raw markdown templates.** `docs/templates/considerations.md` and `docs/templates/decisions.md` are plain markdown, copied verbatim via `os.ReadFile` + `os.WriteFile`. No `text/template` or `html/template`. Acked by Coach.

**D4: Overwrite guard.** Same interactive-read pattern (`in.ReadString('\n')`) as catalog prompt. Defaults to no (skip overwrite). Match existing init.go conventions.

**D5: Verbatim heading strings.** Phase 2b headings use exact spec strings: "Registry check", "Design consultation", "Architecture conformance", "Capture" — verified by `TestPlannerHasPhase2b`.

**D6: Shared `bufio.Reader`.** Two separate `bufio.NewReader(os.Stdin)` instances compete for buffered pipe data, breaking testability. Fixed by creating a single `in := bufio.NewReader(os.Stdin)` at `cmdInit` top and passing it to `materialiseCatalog`. Design decision — same interactive-read pattern, one reader instance.

### Inter-slice collision: S21-canonical-baton

S21's `planned_files` includes `cmd/sworn/init.go` and `cmd/sworn/init_test.go` — the same files S18 creates/modifies. Both slices are serial in T3-commercial (S18 first).

**S21 implementer guidance:**
- `cmd/sworn/init.go` post-S18: catalog prompt at lines 312-333, `materialiseCatalog` function at lines 336-377. S21's baton-vendor additions should go in a separate block after the catalog section and before the existing helper functions (`promptAPIKey`, `writeConfig`).
- `cmd/sworn/init_test.go` post-S18: contains three catalog tests (TestInitCreatesBothTemplates, TestInitSkipsBoth, TestInitOverwriteGuard) plus the helper functions (setupCatalogTemplates, setupMinimalConfig, feedStdinFromString). S21's tests should be added after the existing tests.
- Both files are additive-only — S18 appends, S21 appends further. No shared state or function signature changes.

### Test results

All 7 tests pass (4 planner + 3 init):
- `TestPlannerHasPhase2b` — all four Phase 2b headings present
- `TestPlannerPhase2bDRYGate` — "docs/decisions.md" in planner prompt
- `TestPlannerPhase2bFastPath` — "do not block" guard present
- `TestInitCreatesBothTemplates` — `--yes` creates both files
- `TestInitSkipsBoth` — `n` creates neither
- `TestInitOverwriteGuard` — `n` to overwrite preserves original content
- `go build ./...` — no new dependencies (stdlib only)
### Skeptic panel

skeptic_panel: skipped — runtime does not support subagent dispatch (no Agent/Workflow tool available).
Real verifier (Rule 7) is the backstop.

### First-pass verification

23/24 checks PASS. 1 known false positive: "screenshot" in spec body text (Phase 2b planner description), not in any AC. Documented in proof.md per feedback_release_verify_darkcode_docs_glob.
