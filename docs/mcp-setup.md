# SwornAgent MCP Setup Guide

This guide explains how to connect an AI assistant to `sworn mcp` so it can
plan releases, create slices, manage tracks, and read role prompts and release
artefacts — all through the Model Context Protocol (MCP).

## Prerequisites

1. **Build the binary**: `make build` (produces `bin/sworn`)
2. **Verify the server starts**: `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' | bin/sworn mcp`

## Available MCP tools

### Planning tools (S08c)

| Tool | Description |
|------|-------------|
| `create_slice` | Create a new slice under a release with spec.md and status.json |
| `set_track` | Add or update a track entry in a release's index.md frontmatter |
| `update_intake` | Append content under a section heading in intake.md |

### Operations tools (S08b)

| Tool | Description |
|------|-------------|
| `get_board` | Get the release board overview |
| `get_blocked` | List slices with failed verification |
| `get_slice_context` | Get spec, status, and diff for a slice |
| `rerun_slice` | Re-trigger implementation for a failed slice |
| `patch_slice` | Write patch instructions and re-trigger |
| `approve_merge` | Check if a track is ready to merge |
| `defer_slice` | Defer a slice with a Rule 2 reason |
| `get_credits` | Check remaining API credits |
| `list_releases` | List all releases in the repo |

## Available MCP resources

| URI | Description |
|-----|-------------|
| `sworn://prompts/plan` | Baton planner role prompt |
| `sworn://prompts/implement` | Baton implementer role prompt |
| `sworn://prompts/verify` | Baton verifier role prompt |
| `sworn://baton/track-mode` | Baton track-mode protocol document |
| `sworn://baton/version` | Vendored Baton protocol version string |
| `sworn://release/{name}/board` | Release board (index.md content) |
| `sworn://release/{name}/intake` | Release intake document |
| `sworn://release/{name}/{slice}/spec` | Slice spec.md content |
| `sworn://release/{name}/{slice}/proof` | Slice proof.md content (empty string if absent) |

> `sworn://baton/rules` will be available after S21-canonical-baton lands.

## Available MCP prompts

| Name | Description |
|------|-------------|
| `planner` | Baton planner role prompt |
| `implementer` | Baton implementer role prompt |
| `verifier` | Baton verifier role prompt |

## Configuration by tool

### Claude Code

Add to `.claude/settings.json` (project-level) or `~/.claude/settings.json` (global):

```json
{
  "mcpServers": {
    "sworn": {
      "command": "/path/to/bin/sworn",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

After saving, restart Claude Code. The `sworn` MCP server will be available
in the tools panel.

### Codex (OpenAI)

Add to `~/.codex/config.json`:

```json
{
  "mcpServers": {
    "sworn": {
      "command": "/path/to/bin/sworn",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

### Cursor

Add to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "sworn": {
      "command": "/path/to/bin/sworn",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "sworn": {
      "command": "/path/to/bin/sworn",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

### Gemini CLI

Add to `~/.gemini/mcp.json`:

```json
{
  "mcpServers": {
    "sworn": {
      "command": "/path/to/bin/sworn",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

## Example planning workflow

Once configured, you can ask your AI assistant to plan a release:

1. **"Create a slice S01-auth in release 2026-07-01-auth with this spec: ..."**
   - The AI calls `create_slice` with the spec content and track ID.
   - `docs/release/2026-07-01-auth/S01-auth/spec.md` and `status.json` are created.

2. **"Set track T1-core with slices S01-auth, S02-session"**
   - The AI calls `set_track` to register the track in `index.md`.

3. **"Add to the intake: under 'Users and their gestures', add 'Admin: manages users'"**
   - The AI calls `update_intake` to append content.

4. **"Read the planner prompt"**
   - The AI reads `sworn://prompts/plan` to get the Baton planner role prompt.

5. **"Show me the release board"**
   - The AI reads `sworn://release/2026-07-01-auth/board`.

## Notes

- The MCP server communicates over stdio (JSON-RPC 2.0, line-delimited).
- All diagnostic logs go to stderr; stdout is reserved for the protocol.
- The server operates relative to the `cwd` specified in the config (or `.`
  if not specified — the current working directory of the process).
- Role prompts and Baton docs are served from the binary's embedded
  `internal/prompt/` — not from the filesystem. This ensures the binary is
  self-contained with zero runtime dependencies.