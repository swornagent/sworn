# Baton Protocol — Embedded Canonical Set

This directory contains the complete, authoritative Baton protocol embedded in the
`sworn` binary via `go:embed`. These files are served to AI agents through the
`sworn mcp` server (`sworn://baton/*` resources) and surfaced programmatically
via `internal/prompt.Baton()` and `BatonAll()`.

## Files

| File | Content | Source |
|---|---|---|
| `rules.md` | All ten Baton rules (1–10) | `internal/adopt/baton/rules/` — in-repo canonical set |
| `track-mode.md` | Track-mode documentation for safe release parallelism | Pre-existing (S08c-mcp-plan-tools) |
| `session-discipline.md` | Session handoff and discipline rules | Baton protocol |
| `brainstorm-patterns.md` | Brainstorm methodology and patterns | Baton protocol |
| `VERSION.txt` | Vendored Baton protocol version | `internal/prompt/VERSION.txt` |
| `README.md` | This file | — |

## Consumers

- **MCP server** (`sworn mcp`): serves `sworn://baton/rules` etc. from the embed
- **`prompt.Baton(name)`**: programmatic access for any `sworn` subcommand
- **`prompt.BatonAll()`**: returns the full map for MCP resource listing

## Update discipline

When the Baton protocol version bumps, update `rules.md` by re-concatenating from
`internal/adopt/baton/rules/` (the in-repo canonical source) and re-vendor
`session-discipline.md` and `brainstorm-patterns.md` from the upstream Baton
protocol. Bump `VERSION.txt`.