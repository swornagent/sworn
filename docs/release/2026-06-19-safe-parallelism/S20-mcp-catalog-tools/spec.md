---
title: 'S20-mcp-catalog-tools — MCP tools for catalog management, decision registry, and unified release planning'
description: 'Registers eight MCP tools giving any connected AI assistant full read/write access to the consideration catalog and decision registry, plus a unified plan_release tool replacing create_release. The AI drives the conversational induction, design consultation, and decision capture; the MCP tools handle persistence. Also updates sworn://prompts/* resources to serve the S19-updated role prompts.'
---

# Slice: `S20-mcp-catalog-tools`

## User outcome

A developer opens their AI interface (Claude Code, Codex, Cursor, Gemini CLI), connects
to `sworn mcp`, and says "let's onboard this repo." The AI calls `get_induction_status`,
sees the catalog is empty, and walks them through induction conversationally — calling
`update_design_system`, `record_architecture_pattern`, and `set_nfr_stance` to build
`docs/considerations.md` piece by piece. When planning a release, the AI calls
`search_decisions` before proposing any design option, and `record_decision` after any
choice is made. Nothing is asked twice.

## Entry point

`tools/call` on the MCP server — any AI connected to `sworn mcp` can call these tools.
Verifiable by: connecting Claude Code to `sworn mcp`; asking the AI to check the
catalog; observing it call `get_induction_status`; confirming it reads and reports
the catalog state.

## In scope

Eight new MCP tools registered in `internal/mcp/catalog.go`:

### 1. `plan_release(name, goal?, tracking_issue?)`

Replaces S08c's `create_release`. Detects whether a release already exists:
- **New release**: creates `docs/release/<name>/` with `intake.md` + `index.md` from
  templates (same logic as the old `create_release`).
- **Existing release**: reads `index.md` via the board oracle; returns `{exists: true,
  slice_count, state_summary, release_worktree_branch}` so the AI knows the current
  state and can continue the planning conversation.

Returns: `{exists: bool, created_paths?: [], state_summary?: {planned, in_progress, verified}}`

### 2. `get_induction_status()`

Returns whether `docs/considerations.md` exists and how populated it is:
```json
{
  "catalog_exists": true,
  "design_system_configured": true,
  "architecture_patterns_count": 3,
  "enabled_dimensions": ["security", "api", "data", "observability"],
  "decisions_count": 7
}
```
The AI uses this to decide whether to run induction or proceed to planning.

### 3. `get_considerations(slice_type?)`

Reads `docs/considerations.md` and returns the applicable dimensions for `slice_type`
(`"ui"` | `"api"` | `"data"` | `"all"`). Returns the full dimension sections for the
applicable types plus the `design_system` and `architecture.patterns` blocks. If the
catalog does not exist, returns `{catalog_missing: true}` — not an error.

### 4. `search_decisions(keywords)`

Searches `docs/decisions.md` for entries matching any keyword (case-insensitive, matches
against title + decision + applies_to fields). Returns matching entries in full so the AI
can surface them before asking a design question. Returns empty array if no matches or if
`docs/decisions.md` does not exist.

### 5. `record_decision(type, title, decision, rationale, applies_to, release?, slice?, overrides?)`

Appends a new entry to `docs/decisions.md` in the standard format. `type` is one of
`design | architecture | data | flow | deviation | resolution`. Creates
`docs/decisions.md` if it does not exist. Returns the written entry.

### 6. `check_design_system(component_description)`

Reads `design_system` from `docs/considerations.md`. Returns:
```json
{
  "status": "exists" | "close" | "missing" | "unconfigured",
  "matched_component": "Card",       // if exists or close
  "gap_description": "...",          // if close or missing
  "options": [                        // if close or missing
    {"label": "Reuse as-is", "description": "..."},
    {"label": "Extend with variant", "description": "..."},
    {"label": "Build new", "description": "..."}
  ],
  "recommendation": "Extend with variant"
}
```
If `design_system.location` is blank, returns `{status: "unconfigured"}`. The AI
uses this to drive the design consultation from Phase 2b-ii.

### 7. `update_design_system(location, framework, version?, component_library?)`

Writes the `design_system` section in `docs/considerations.md`. Creates the file from
the template if it does not exist. Used by the AI during conversational induction.

### 8. `record_architecture_pattern(pattern, location, intent)`

Appends an entry to `architecture.patterns` in `docs/considerations.md`. Creates the
file from the template if it does not exist. Used by the AI during conversational
induction. Idempotent: does not add duplicate patterns (matched by `pattern` string).

### Resource update

`sworn://prompts/plan`, `sworn://prompts/implement`, `sworn://prompts/verify` already
registered by S08c. S20 updates the resource handler to serve the S19-updated prompt
files (which now include Phase 2b, the deviation check, and the catalog conformance
check). No code change needed if the resource handler reads the embedded prompts at
request time rather than init time — just ensure it reads from `internal/prompt/`
embed, which was already updated by S18 and S19.

## Out of scope

- `sworn induction` CLI command — that is S19; S20 provides the MCP tools that enable
  the same flow conversationally
- Real-time catalog sync / change notifications (post-R3)
- Semantic search on decisions.md (keyword search is sufficient; vector search post-R3)

## Planned touchpoints

- `internal/mcp/catalog.go` (new — 8 tool handlers)
- `internal/mcp/catalog_test.go` (new)

## Acceptance checks

- [ ] `plan_release("new-release", "test goal")` creates the release directory and
  returns `{exists: false, created_paths: [...]}`
- [ ] `plan_release("2026-06-19-safe-parallelism")` (existing release) returns
  `{exists: true, slice_count: 24}` without creating any files
- [ ] `get_induction_status()` on a repo with a populated catalog returns
  `architecture_patterns_count > 0` and `design_system_configured: true`
- [ ] `get_induction_status()` on a repo with no `docs/considerations.md` returns
  `{catalog_exists: false}`
- [ ] `get_considerations("ui")` returns the `[ui]` and `[security]` (required_for: all)
  dimension sections plus the `design_system` block
- [ ] `search_decisions("modal")` finds entries whose title or decision contains "modal"
- [ ] `search_decisions("nonexistent")` returns an empty array without error
- [ ] `record_decision("design", "Modal for settings", "Use modal dialogs", "prevents layout shift", "settings surfaces")` appends a correctly-formatted entry to `docs/decisions.md`; a subsequent `search_decisions("modal")` returns it
- [ ] `check_design_system("data table with sortable columns")` returns a `status` of
  `exists`, `close`, `missing`, or `unconfigured` (not a panic) regardless of whether
  the design system is configured
- [ ] `update_design_system("https://storybook.example.com", "storybook")` writes the
  `design_system` block to `docs/considerations.md`; `get_induction_status()` then
  returns `design_system_configured: true`
- [ ] `record_architecture_pattern("interface-first", "internal/model/client.go", "mock injection in tests")` appends to `architecture.patterns`; calling it again with the same pattern is idempotent (no duplicate)
- [ ] `go test ./internal/mcp/... -run Catalog` passes with zero failures
- [ ] `go build ./...` passes; no new external deps

## Required tests

`internal/mcp/catalog_test.go` (all using a temp dir with fixture catalog files):

- `TestPlanReleaseNew`: new release name → files created
- `TestPlanReleaseExisting`: existing release → returns exists=true, no new files
- `TestGetInductionStatus_Empty`: no catalog → catalog_exists=false
- `TestGetInductionStatus_Populated`: catalog with 2 patterns → patterns_count=2
- `TestGetConsiderations_UIType`: returns ui + all-scoped dimensions
- `TestSearchDecisions_Hit`: record a decision; search with matching keyword; assert found
- `TestSearchDecisions_Miss`: search with non-matching keyword; assert empty array
- `TestSearchDecisions_NoCatalog`: no decisions.md → empty array, no error
- `TestRecordDecision_WritesEntry`: assert standard format written; assert section headings
- `TestCheckDesignSystem_Unconfigured`: blank location → unconfigured status
- `TestUpdateDesignSystem_Writes`: call then read back; assert fields match
- `TestRecordArchPattern_Idempotent`: call twice with same pattern; assert only one entry

- **manual-smoke-step** `sworn mcp` → connect Claude Code, call `get_induction_status`. Covers AC1-AC2 (plan_release), AC3-AC4 (induction_status), AC5 (considerations), AC6-AC7 (search_decisions), AC8 (record_decision), AC9-AC10 (check_design_system/update_design_system), AC11 (record_architecture_pattern).

**Reachability artefact**: connect Claude Code to `sworn mcp`; ask "what's the induction
status of this repo?"; observe AI calls `get_induction_status`; confirm it reads and
reports the actual catalog state. Screenshot in proof.md.

## Risks

- `plan_release` replaces `create_release` from S08c. The old `create_release` tool
  name should not be registered; `plan_release` is the canonical name. If any existing
  tests or documentation reference `create_release`, they must be updated in this slice.
- `check_design_system` requires reading and reasoning about design system contents.
  The MCP tool can only check whether the catalog has a `design_system.location` and
  return it; the AI itself reasons about whether a component matches. The tool does not
  need to fetch the design system — it just surfaces what the catalog says and the AI
  applies judgment. The `options` array in the response is generated by the MCP handler
  as a structured scaffold; the AI can enrich it conversationally.
- `docs/decisions.md` is append-only in normal use; `record_decision` never edits
  existing entries. If a decision is overridden, it records a new entry with `overrides:
  <prior-decision-date>` rather than modifying the prior one. This must be documented
  in the tool description so the AI uses it correctly.

## Deferrals allowed?

Semantic / vector search on decisions.md: deferred post-R3 (Why: keyword search covers
the common case; vector search requires an embedding model and significant infra.
Tracking: post-R3 issue. Acknowledged: 2026-06-20).
