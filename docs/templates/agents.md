# AGENTS.md

This repository uses [sworn](https://swornagent.com) for autonomous release
management under the Baton protocol.

## For AI agents and developers

The canonical Baton protocol and all role prompts are served by the sworn MCP
server. Do not rely on any Baton content that may exist in this repo's git
history or in docs/baton/ — always fetch the current protocol from the binary.

## Connect the MCP server

Add to your AI tool's MCP config (Claude Code, Codex, Cursor, Gemini CLI, etc.):

    {
      "mcpServers": {
        "sworn": { "command": "sworn", "args": ["mcp"] }
      }
    }

## Start here

| What you need | MCP resource / tool |
|---|---|
| Full Baton protocol | `sworn://baton/rules` |
| Planner role prompt | `sworn://prompts/plan` |
| Implementer role prompt | `sworn://prompts/implement` |
| Verifier role prompt | `sworn://prompts/verify` |
| Current release board | `get_board` tool |

## Consideration catalog

- `docs/considerations.md` — read before planning any slice
- `docs/decisions.md` — search before asking any design question

## Non-negotiables

- Never claim a slice `verified` from the same session that implemented it
- Never edit `status.json`, `proof.md`, or `journal.md` directly — use sworn verbs
- Fail closed: on any ambiguity, BLOCK rather than guess