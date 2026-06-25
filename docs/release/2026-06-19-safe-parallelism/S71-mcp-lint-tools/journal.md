---
title: 'Journal — S71-mcp-lint-tools'
description: 'Implementation session notes for MCP lint tools.'
---

# Session: 2026-07-20 — Implementer

## State transition: planned → in_progress → implemented

### Decisions

- **Pattern**: followed existing MCP tool registration pattern (RegisterPlanTools, RegisterOpsTools, RegisterCatalogTools). New `RegisterLintTools` mirrors the same signature: `func RegisterLintTools(s *Server, repoRoot string)`.
- **Composite tool design**: `sworn.lint` runs release-level checks (ac, trace, status) always; per-slice checks (coverage, design, mock, deps, touchpoints, symbols) only when `slice_id` provided. This allows both release-wide scanning and targeted slice checking.
- **Error handling**: per-slice tools (lint_coverage, lint_design, lint_mock) validate slice directory existence before calling gate functions, producing "not found" errors that the MCP client can interpret.

### Trade-offs

- `sworn.llm_check` requires `SWORN_MODEL` env var or explicit `model` param. When not configured, returns a descriptive error — same behaviour as the CLI `sworn llm-check`.
- The composite `sworn.lint` aggregates results under `checks` map rather than streaming per-check — this is simpler and matches the existing MCP tool response pattern.

### Findings

- `lint.go` from release-wt had fused lines in `cmd/sworn/mcp.go` (RegisterPrompts + ctx declaration on same line; RegisterResources + RegisterPrompts on same line). These predate this slice. Fixed during implementation.
- `release-verify.sh` has a `PLAYWRIGHT_OPTIN: unbound variable` bug — triggered by "E2E gate type: local" in Required tests section despite no browser screenshots in scope.
## Verifier verdicts received

### Session: 2026-07-20 — Verifier (fresh context)

**Verdict: PASS**

- Gate 1 (User-reachable outcome): PASS — RegisterLintTools wired into sworn mcp, all 6 tools reachable.
- Gate 2 (Touchpoint match): PASS — planned_files match feat commit exactly.
- Gate 3 (Tests exercise integration): PASS — 7 tests pass, build + vet clean, tests exercise MCP server handler interface.
- Gate 4 (Reachability artefact): PASS — live binary tools/list returns all 6 lint tools; sworn.lint_trace returns structured JSON.
- Gate 5 (No silent deferrals): PASS — no TODO/FIXME/HACK in changed files; open_deferrals empty.
- Gate 6 (Scope match): PASS — all 5 acceptance checks confirmed against live code and tests.

**Next step:** /merge-track T21-mcp-lint (sole slice in track — track is complete), then /merge-release 2026-06-19-safe-parallelism once all tracks merged.
