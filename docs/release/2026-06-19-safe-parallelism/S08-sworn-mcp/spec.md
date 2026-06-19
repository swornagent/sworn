---
title: 'S08-sworn-mcp — MCP server (planning + operations + resources)'
description: 'sworn mcp starts an MCP server over stdio. Any MCP client (Claude Code, Codex, Cursor, Windsurf, Gemini CLI) can plan releases, query board state, surface blocked slices with full context, trigger reruns, and approve merges.'
---

# Slice: `S08-sworn-mcp`

## User outcome

A developer configures `sworn mcp` as a local MCP server in their AI tool of choice
(Claude Code, Codex, Cursor, Windsurf, Gemini CLI), and their AI assistant can plan
releases, query board state, get blocked slices with full context pre-assembled, trigger
reruns, and approve merges — without the developer manually navigating worktrees or
assembling context.

## Entry point

`sworn mcp` subcommand — starts an MCP 2024-11 compliant server reading JSON-RPC 2.0
requests from stdin and writing responses to stdout (STDIO transport). Runs until the
client closes the connection (stdin EOF).

## In scope

**MCP transport**: JSON-RPC 2.0 over stdio. `initialize` / `initialized` handshake.
Protocol version: `2024-11-05`. Capabilities advertised: `tools`, `resources`, `prompts`.

**Operations tools** (allow the AI to query and act on running releases):
- `get_board(release?: string)` → board state for all releases (or specified one):
  tracks, slices, states, last_updated_at
- `get_blocked()` → all slices in `failed_verification` or `BLOCKED` state; for each:
  release, track, slice_id, violations list (from proof.md), worktree_path
- `get_slice_context(release: string, slice_id: string)` → assembled context:
  spec.md content, violations list (from proof.md), git diff (worktree vs start_commit),
  journal.md content; the full package an AI needs to propose a fix
- `rerun_slice(release: string, slice_id: string)` → resets slice state to in_progress
  and triggers `sworn run` on the specific slice; returns the run ID
- `patch_slice(release: string, slice_id: string, instructions: string)` → writes a
  `PATCH_INSTRUCTIONS.md` file to the slice worktree directory and calls rerun_slice;
  the implementer agent reads this file as additional context
- `approve_merge(release: string, track_id: string)` → validates all track slices are
  verified; runs the merge-track logic; returns success or error
- `defer_slice(release: string, slice_id: string, reason: string)` → writes state
  `deferred` to status.json and appends a Rule 2 deferral entry to intake.md
- `get_credits()` → credit balance from `~/.config/sworn/credits.json` cache

**Planning tools** (allow the AI to create and update release artefacts):
- `create_release(name: string, goal: string, tracking_issue?: string)` → creates
  `docs/release/<name>/` directory, `intake.md` (from template, with goal filled in),
  and `index.md` (from template); returns the paths created
- `create_slice(release: string, slice_id: string, spec_content: string, track_id: string)`
  → creates `docs/release/<release>/<slice_id>/` directory, writes `spec.md` with the
  provided content, writes `status.json` with state=planned; returns the path
- `set_track(release: string, track_id: string, slices: string[], depends_on?: string)`
  → updates `index.md` frontmatter tracks list and the Tracks table; validates that
  all slice_ids exist under the release before writing
- `update_intake(release: string, section: string, content: string)` → appends `content`
  to the named section heading in `intake.md`; section must match an existing `## ` heading

**MCP Resources**:
- `sworn://prompts/plan` → full content of embedded `baton/role-prompts/planner.md`
- `sworn://prompts/implement` → full content of embedded `baton/role-prompts/implementer.md`
- `sworn://prompts/verify` → full content of embedded `baton/role-prompts/verifier.md`
- `sworn://release/{name}/board` → content of `docs/release/<name>/index.md`
- `sworn://release/{name}/intake` → content of `docs/release/<name>/intake.md`
- `sworn://release/{name}/{slice}/spec` → content of `docs/release/<name>/<slice>/spec.md`
- `sworn://release/{name}/{slice}/proof` → content of `docs/release/<name>/<slice>/proof.md`

**Claude Code config example** (written to `docs/mcp-setup.md`):
```json
{
  "mcpServers": {
    "sworn": {
      "command": "sworn",
      "args": ["mcp"]
    }
  }
}
```

## Out of scope

- HTTP transport (streamable HTTP MCP) — post-R3
- Authentication of MCP clients (STDIO trust model — local process only)
- Remote MCP server hosted at SwornAgent cloud — post-R3
- `sworn mcp` daemon mode (persistent background server) — post-R3; R3 is per-session

## Planned touchpoints

- `internal/mcp/server.go` (new — JSON-RPC 2.0 transport, request routing, capabilities)
- `internal/mcp/tools_ops.go` (new — operations tools implementation)
- `internal/mcp/tools_plan.go` (new — planning tools implementation)
- `internal/mcp/resources.go` (new — resource URI handlers)
- `internal/mcp/prompts.go` (new — prompt resource handlers, reads embedded role prompts)
- `internal/mcp/context.go` (new — get_slice_context assembler: spec + violations + diff + journal)
- `internal/mcp/server_test.go` (new — JSON-RPC roundtrip tests, in-process server)
- `internal/mcp/tools_test.go` (new — tool call tests with fixture releases)
- `cmd/sworn/mcp.go` (new — `sworn mcp` subcommand)
- `cmd/sworn/main.go` (touch — dispatch `mcp` subcommand)
- `docs/mcp-setup.md` (new — setup instructions for Claude Code, Codex, Cursor, Windsurf)

## Acceptance checks

- [ ] `sworn mcp` starts and reads a JSON-RPC `initialize` request from stdin; responds
  with `capabilities: {tools: {}, resources: {}, prompts: {}}` and protocol version
  `2024-11-05`
- [ ] `tools/list` response includes all 9 operations tools + 4 planning tools with
  correct JSON Schema input definitions
- [ ] `tools/call get_board` on a fixture release returns a JSON object with the
  correct track + slice structure matching the fixture's `index.md`
- [ ] `tools/call get_blocked` on a fixture release with one `failed_verification`
  slice returns that slice's violations list (extracted from a fixture `proof.md`)
- [ ] `tools/call get_slice_context` returns assembled spec content, violations, diff,
  and journal for a fixture slice with known content
- [ ] `tools/call create_release` with `name="test-release-mcp"` creates the expected
  directory structure with populated `intake.md` and `index.md`; cleans up after test
- [ ] `tools/call create_slice` creates `spec.md` and `status.json` at the correct path
- [ ] `resources/read sworn://prompts/plan` returns the full embedded planner.md content
- [ ] `resources/read sworn://release/{name}/board` returns the content of `index.md`
- [ ] `docs/mcp-setup.md` exists and contains the Claude Code JSON config block
- [ ] `go test ./internal/mcp/...` passes (in-process server, no external process needed)

## Required tests

- **Unit / integration**: `internal/mcp/server_test.go`
  — `TestInitialize`: send `initialize` request; assert correct capabilities response
  — `TestToolsList`: send `tools/list`; assert all 13 tools present with correct names
  — `TestGetBoardRoundtrip`: fixture release; call get_board; assert response structure
  — `TestGetBlockedExtractsViolations`: fixture with known proof.md; assert violations
    returned correctly
  — `TestCreateReleaseWritesFiles`: call create_release; assert files exist; cleanup
  — `TestResourceRead`: assert sworn://prompts/plan returns non-empty content matching
    the embedded planner.md
- **Reachability artefact**: configure `sworn mcp` in a local Claude Code instance;
  open Claude Code; type "what releases does sworn have?"; confirm AI calls `get_board`
  via MCP and returns the board content. Screenshot or session transcript in proof.md.

## Risks

- MCP protocol is evolving. This slice targets the `2024-11-05` version. If Claude Code
  or Codex ships a breaking change to the protocol before R3 ships, the server may need
  an update. Mitigation: pin the protocol version in the `initialize` handshake; the
  client negotiates down.
- The `patch_slice` tool writes a file to the worktree and triggers a rerun. If the
  implementer agent does not check for `PATCH_INSTRUCTIONS.md`, the patch is silently
  ignored. This is an implementer convention, not a protocol guarantee. Document in
  `docs/mcp-setup.md`.
- `diff` in `get_slice_context` requires the worktree to be at a known `start_commit`.
  If the worktree has been manually modified since the last commit, the diff may include
  unintended changes. Documented limitation; diff is best-effort context, not authoritative.

## Deferrals allowed?

Yes, with Rule 2 compliance:
- HTTP/SSE transport: deferred post-R3. Why: STDIO covers all local AI tool use cases;
  HTTP needed for remote (cloud-hosted) sworn server. Tracking: TBD. Ack: now.
- Daemon mode (persistent background MCP server): deferred post-R3. Why: per-session
  STDIO is sufficient for R3; daemon mode requires IPC or socket management. Tracking: TBD.
