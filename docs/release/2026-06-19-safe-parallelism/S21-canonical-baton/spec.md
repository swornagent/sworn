---
title: 'S21-canonical-baton — embed full Baton protocol in binary; rewrite sworn init to remove per-repo Baton copy'
description: 'The complete Baton protocol (7 rules + track-mode + session-discipline + role prompts) moves into internal/prompt/baton/ as a go:embed. sworn init stops writing docs/baton/ and stops splicing AGENTS.md; it writes a minimal MCP-pointer AGENTS.md template instead. ADR-0005 documents the architecture. User prompt overrides deferred post-launch.'
---

# Slice: `S21-canonical-baton`

## User outcome

A developer runs `sworn init` on a new repo. When done, the repo has a minimal
`AGENTS.md` that points the developer's AI at `sworn mcp` — no Baton rule content
in the repo, no `docs/baton/` directory. Any AI that connects gets the current
canonical Baton protocol fresh from the binary. The developer never has to run
`sworn init` again to get protocol updates; they just update the `sworn` binary.

## Entry point

`sworn init` (rewritten) and `sworn mcp` (`sworn://baton/*` resources served from
the embed). Verifiable by: running `sworn init` on a clean repo; confirming
`docs/baton/` is NOT created; confirming `AGENTS.md` is the minimal template;
connecting an AI to `sworn mcp` and reading `sworn://baton/rules` to confirm it
returns the embedded content.

## In scope

### `internal/prompt/baton/` — embedded Baton protocol

New subdirectory in `internal/prompt/` containing the complete Baton protocol as
embedded markdown files. These become the single authoritative source; the
`~/.claude/baton/` directory on the developer's machine is no longer the canonical
copy (it continues to work for local slash-command harness use, but sworn itself does
not install from it or depend on it):

```
internal/prompt/baton/
  rules.md           — all 7 rules in one file (or 7 separate files, implementer's choice)
  track-mode.md      — track-mode documentation
  session-discipline.md
  brainstorm-patterns.md
  README.md          — index / overview
```

The existing `internal/prompt/` embed is extended to include the `baton/` subdirectory.
`Baton(name string) (string, error)` added to `internal/prompt/prompt.go` to expose
individual Baton doc files. `BatonAll() map[string]string` returns all files for
MCP resource listing.

These files are populated by copying from the current canonical Baton sources at
`~/.claude/baton/` or `$HOME/.claude/baton/`. The implementer copies them verbatim;
no content editing. The VERSION.txt (current Baton version) is also embedded.

### `sworn init` rewrite — `cmd/sworn/init.go`

**Remove entirely:**
- `adopt.Materialise(repoRoot)` call — no longer writes `docs/baton/`
- `adopt.SpliceAgents(repoRoot, force)` call — no longer splices AGENTS.md/CLAUDE.md
- Scan phase entries for `docs/baton/` and agent file splice
- Print messaging about Baton docs being created/updated

**Keep unchanged:**
- Config file creation (`config.Scaffold`)
- API key prompting
- Design system prompting (`config.PromptDesignSystem`)

**Add:**
- `AGENTS.md` creation from `docs/templates/agents.md`:
  - If `AGENTS.md` does not exist → create from template; note in scan/apply output
  - If `AGENTS.md` exists and is the old splice format (detected by presence of
    Baton rule content marker `<!-- baton:start -->`) → warn: "AGENTS.md contains
    legacy Baton content — run `sworn doctor` to migrate"; do not touch
  - If `AGENTS.md` exists and is not legacy format → skip (no overwrite without --force)
- Final message updated: "Done. Connect your AI to sworn mcp to get the Baton protocol
  and role prompts. Run 'sworn doctor' to verify your setup."

### `docs/templates/agents.md` — minimal MCP-pointer template

Written to `AGENTS.md` by `sworn init`. Contains:

```markdown
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
```

### `docs/adr/0005-canonical-baton.md` — architecture decision record

Documents:
- **Decision**: The sworn binary is the single source of truth for the Baton protocol.
  All role prompts and protocol documentation are embedded in the binary via `go:embed`.
  The MCP server (`sworn://prompts/*`, `sworn://baton/*`) serves them from the embed.
  `sworn init` does not copy Baton content into repos; repos contain only a minimal
  MCP-pointer `AGENTS.md`.
- **Rationale**: Eliminates per-repo drift. Protocol improvements roll out to all
  users on binary update. Customers can report issues against a specific binary version.
  Support cost is bounded: one canonical version, not N per-repo forks.
- **Supersedes**: The `adopt.Materialise` / `adopt.SpliceAgents` approach from R1.
- **Consequences**:
  - `docs/baton/` in existing repos is now legacy; `sworn doctor` warns and advises removal
  - `~/.claude/baton/` on developer machines continues to work for local slash-command
    harness; it is NOT deprecated, but it is no longer installed by `sworn init`
  - The slash-command harness (`/implement-slice` etc.) reads from `~/.claude/baton/`
    for now; a future release will migrate them to read via `sworn://prompts/*`
- **User prompt overrides**: Deferred post-launch. Why: opening overrides before the
  canonical protocol is stable creates N support surfaces before we can iterate.
  Tracking: post-launch feature. Acknowledged: 2026-06-20 planning session.

## Out of scope

- User prompt overrides / project-level Baton customisation (deferred — see ADR-0005)
- Migration of slash-command harness to read from sworn MCP (post-launch)
- Automatic removal of `docs/baton/` from existing repos (`sworn doctor` warns; removal is manual)
- Deprecating `~/.claude/baton/` (not yet; slash-command harness still reads from it)

## Planned touchpoints

- `internal/prompt/baton/` (new directory + files — embedded Baton protocol content)
- `internal/prompt/prompt.go` (modify — add `Baton(name)` and `BatonAll()` functions)
- `cmd/sworn/init.go` (modify — remove adopt.Materialise + SpliceAgents; add AGENTS.md write)
- `cmd/sworn/init_test.go` (modify — test new init behaviour)
- `docs/templates/agents.md` (new — minimal MCP-pointer template)
- `docs/adr/0005-canonical-baton.md` (new)

## Acceptance checks

- [ ] `internal/prompt/baton/` exists with at minimum `rules.md`, `track-mode.md`,
  and `README.md`; `prompt.Baton("rules.md")` returns non-empty content
- [ ] `go build ./...` passes; `internal/prompt/baton/` is embedded (verified by
  checking `go:embed` directive in prompt.go includes the baton/ path)
- [ ] `sworn init` on a clean directory does NOT create `docs/baton/`
- [ ] `sworn init` on a clean directory DOES create `AGENTS.md` from the template;
  the file contains the string `sworn://baton/rules`
- [ ] `sworn init` on a directory with an existing non-legacy `AGENTS.md` leaves it
  unchanged (no overwrite without --force)
- [ ] `sworn init` on a directory with a legacy Baton-splice `AGENTS.md` (contains
  `<!-- baton:start -->`) prints the migration warning and does NOT overwrite
- [ ] `docs/adr/0005-canonical-baton.md` exists; contains "supersedes" reference to
  `adopt.Materialise`; contains the user-overrides deferral with tracking note
- [ ] `go test ./cmd/sworn/... -run Init` passes; all init scenarios covered
- [ ] `go test ./internal/prompt/... -run Baton` passes

## Required tests

- **Unit** `internal/prompt/prompt_test.go` (extend):
  - `TestBatonRulesNonEmpty`: `Baton("rules.md")` returns string of length > 100
  - `TestBatonAllKeys`: `BatonAll()` map contains "rules.md" and "track-mode.md"
  - `TestBatonMissingFile`: `Baton("nonexistent.md")` returns error (not panic)
- **Unit** `cmd/sworn/init_test.go` (extend):
  - `TestInitCreatesAgentsMD`: fresh dir → AGENTS.md created; contains
    `sworn://baton/rules`; `docs/baton/` NOT created
  - `TestInitSkipsExistingAgentsMD`: AGENTS.md already present (non-legacy) → unchanged
  - `TestInitWarnsLegacyBaton`: AGENTS.md with `<!-- baton:start -->` → warning printed;
    file unchanged; exit 0 (not an error, just a warning)
  - `TestInitDoesNotSpliceClaude`: CLAUDE.md in dir → NOT modified by init
- **Reachability artefact**: run `sworn init` in a temp dir; ls the dir; confirm no
  `docs/baton/`; cat AGENTS.md; confirm MCP config block present. Document in proof.md.

## Risks

- The `adopt` package currently contains the embedded Baton content (the text that gets
  written to `docs/baton/`). After this slice, the canonical content moves to
  `internal/prompt/baton/`. The `adopt` package may become a hollow wrapper or can be
  removed in a follow-up. The implementer must not delete `adopt` if other commands
  depend on it — check all callers before removing.
- AGENTS.md legacy detection: the `<!-- baton:start -->` marker must reliably identify
  old-style splice content. Check the actual marker string used by `adopt.SpliceAgents`
  in the existing code before hardcoding the detection string.
- The `~/.claude/baton/` directory continues to work for developers using slash commands.
  Do NOT modify or delete anything at that path; only change what `sworn init` does.

## Deferrals allowed?

User prompt overrides: explicitly deferred per ADR-0005. No other deferrals.
