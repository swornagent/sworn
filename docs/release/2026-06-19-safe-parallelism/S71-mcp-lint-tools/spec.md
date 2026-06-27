---
title: 'Slice spec — S71-mcp-lint-tools'
description: 'Expose the gate-engine lint commands as MCP tools so any AI connected via sworn MCP can run mechanical and LLM checks programmatically.'
---

# Slice: S71-mcp-lint-tools

## User outcome

An AI connected to sworn via MCP calls `sworn.lint`, `sworn.llm_check`, and other gate tools to programmatically verify release quality. Results are returned as structured JSON through the MCP transport. This completes the MCP surface parity with the CLI surface.

## Entry point

New `internal/mcp/lint.go` — registers gate tools on the MCP server. Extends S08a-mcp-transport (T4). CLI registration via `internal/command` registry.

## In scope

- Register MCP tools:
  - `sworn.lint`: run all mechanical lint checks (wraps `sworn lint` composite)
  - `sworn.lint_trace`: RTM + EARS traceability check
  - `sworn.lint_coverage`: AC → test coverage mapping
  - `sworn.lint_design`: design conformance + architecture rules
  - `sworn.lint_mock`: mock boundary enforcement
  - `sworn.llm_check`: invoke LLM quality check
- Each tool accepts the same args as the CLI counterpart
- Returns structured JSON results
- Error handling: tool not available → descriptive error

## Out of scope

- Adding new gate types (only exposes existing S65-S70 commands)
- Modifying MCP transport (S08a owns that)

## Planned touchpoints

- `internal/mcp/lint.go` (new)
- `internal/mcp/lint_test.go` (new)
- `cmd/sworn/mcp.go` (extend — register new tools)

## Acceptance checks

- [ ] All 6 MCP tools registered and discoverable via `tools/list`
- [ ] `sworn.lint` returns unified report with all mechanical check results
- [ ] `sworn.llm_check` accepts `--type` parameter and returns structured verdict
- [ ] Tools return appropriate error when underlying command is not available
- [ ] Tools work over STDIO transport (JSON-RPC)

## Required tests

- **Unit**: `internal/mcp/lint_test.go` — tool registration, argument parsing, error cases
- **Integration**: MCP handshake + tool invocation against fixture release
- **Reachability artefact**: `sworn mcp` output showing `tools/list` includes lint tools
- **E2E gate type**: local
