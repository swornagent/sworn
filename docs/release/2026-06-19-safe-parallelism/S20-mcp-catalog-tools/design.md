---
title: Design TL;DR for S20-mcp-catalog-tools
description: Design decisions and reachability plan for the catalog MCP tools.
---

# Design TL;DR: S20-mcp-catalog-tools

## §1. User-visible change

An AI connected to `sworn mcp` gains eight new tools: `plan_release` (unified create-or-read for releases), `get_induction_status` (checks catalog population), `get_considerations` (reads design dimensions), `search_decisions` (keyword search on decisions registry), `record_decision` (appends ADR entries), `check_design_system` (looks up components), `update_design_system` (writes design system config), and `record_architecture_pattern` (idempotent pattern registration). The AI can now drive conversational induction and design consultation, reading and writing catalog state through these tools.

## §2. Design decisions not in spec

1. **Tool registration pattern** — Follow the existing `RegisterPlanTools(s *Server, repoRoot string)` convention. Create `RegisterCatalogTools` as a separate function in `internal/mcp/catalog.go`, registered in `cmd/sworn/mcp.go` alongside the other three registrations. Stdlib is sufficient for this slice's text-file ops; no new dependency, no ADR required.2. **`plan_release` reuse of `CreateRelease`** — The `CreateRelease` helper already exists in `tools_plan.go` (S08c's internal function). `plan_release` calls it for new releases, then returns structured JSON. No duplication.
3. **`docs/considerations.md` and `docs/decisions.md` format** — These files don't exist yet in the repo. The tools create them from scratch using plain Markdown section structures: `## design_system`, `## architecture.patterns`, `## [type]` dimensions for considerations; `### <TYPE>: <title>` entries for decisions. No YAML frontmatter needed — just parseable section headers for the status/dimension tools and full-text for search.
4. **`check_design_system` options scaffold** — The spec says the tool generates an `options` array with Reuse/Extend/Build-new as a scaffold. This is a static three-option template emitted by the handler; the AI enriches it conversationally. No model call needed in the tool.
5. **`record_decision` overrides convention** — Follow the spec's append-only design: when overriding, record a new entry with `overrides: <prior-decision-date>` rather than editing. The `overrides` field is recorded in the decision entry body; no schema enforcement.

## §3. Files I'll touch grouped by purpose

- **New: `internal/mcp/catalog.go`** — Eight tool handlers + `RegisterCatalogTools` registration function. Core deliverable.
- **New: `internal/mcp/catalog_test.go`** — Twelve table-driven tests using temp dirs with fixture catalog files.
- **Edit: `cmd/sworn/mcp.go`** — Add `mcp.RegisterCatalogTools(server, ".")` call after existing `RegisterPlanTools`.

## §4. Things I'm NOT doing

- Not creating `docs/considerations.md` or `docs/decisions.md` as part of the diff — these are runtime artefacts the tools create on first write.
- Not adding any external dependency — pure stdlib.
- Not changing the prompt resources code — `internal/prompt/prompt.go` loads embedded files at `init()`, and the S19-updated prompts are confirmed present on the T7 branch, so they are automatically served at binary startup without any code change.
- Not removing `CreateRelease` helper from `tools_plan.go` — it was never registered as a tool and remains as the internal implementation behind `plan_release`. The two `create_release` references in `intake.md` are updated to `plan_release` in this slice per Risk 1.
- Not touching `tools_plan.go` or `tools_ops.go`.

## §5. Reachability plan

Connect Claude Code to `sworn mcp`; ask "what's the induction status of this repo?"; observe the AI calls `get_induction_status`; confirm the tool returns `{catalog_exists: false}` (no catalog yet on a fresh repo). Screenshot in proof.md at `docs/release/2026-06-19-safe-parallelism/screenshots/S20-mcp-catalog-tools-induction-status.png`.

## §6. Open questions for the Coach

None.