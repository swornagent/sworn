---
title: 'S23-memory-config — memory system config + harness discovery + sworn memory status'
description: 'Config schema for the sworn memory system: global + per-project override, multi-harness path discovery, and sworn memory status showing what is configured and reachable.'
---

# Slice: `S23-memory-config`

## User outcome

A developer running `sworn memory status` sees which AI harnesses are configured
(Claude Code, Gemini CLI, OpenCode, Cursor, Codex, Windsurf, custom paths), which
memory paths exist on disk, which embedding provider is active, and where the
index will be stored — before running any build. Configuration lives at
`~/.config/sworn/memory.json` (global) with per-project override at
`.sworn/memory.json`. Running without any config file prints sensible defaults
and auto-detects the Claude Code memory path if present.

## Entry point

`sworn memory status` in any directory; reads config, resolves paths, prints
the current memory system configuration. No model call; no index read.

## In scope

- `internal/memory/config.go` — `MemoryConfig` struct, `Load()` (reads global
  then per-project JSON, merges with project taking precedence), `Defaults()`
  (detects Claude Code memory at `~/.claude/projects/<encoded-cwd>/memory/` if
  present), marshal/unmarshal to/from JSON
- `internal/memory/harness.go` — `KnownHarness` enum + `HarnessMemoryPath()`
  mapping each harness ID to its canonical memory file/directory location:
  - `claude-code` → `~/.claude/projects/<encoded-cwd>/memory/`
  - `gemini-cli` → `~/.gemini/GEMINI.md` (global) + `<project>/GEMINI.md`
  - `opencode` → `<project>/AGENTS.md`
  - `cursor` → `<project>/.cursorrules`
  - `windsurf` → `<project>/.windsurfrules`
  - `codex` → (no native memory; path left empty, status shows "no native memory")
  - `custom` → arbitrary path list from config `extra_paths`
- `internal/memory/config_test.go` — unit tests for Load, merge, defaults, path
  resolution
- `cmd/sworn/memory.go` — `sworn memory` command with `status` subcommand;
  prints config table showing harness, configured path, exists-on-disk, and
  embedding provider summary
- `cmd/sworn/main.go` — additive dispatch for `memory` subcommand

### Config schema (`~/.config/sworn/memory.json`)

```json
{
  "harnesses": ["claude-code", "gemini-cli"],
  "extra_paths": [],
  "embedding": {
    "provider": "voyage",
    "model": "voyage-code-3",
    "api_key_env": "VOYAGE_API_KEY",
    "base_url": ""
  },
  "index_path": "~/.sworn/memory.db"
}
```

`provider` values: `"voyage"` | `"oai-compat"` | `"ollama"`. `base_url` overrides
the provider's default endpoint (used for custom OAI-compat providers like
Fireworks/Together/local proxies). `model` is passed verbatim to the provider.
`api_key_env` names the env var holding the API key (never stored in config).

## Out of scope

- Actually building or querying the index (S24, S25)
- Embedding API calls (S24)
- Migration from the existing `captain-memory-search.py` (S25)
- Key rotation or secrets management
- TUI settings for memory (post-R3)

## Planned touchpoints

- `internal/memory/config.go` (new)
- `internal/memory/harness.go` (new)
- `internal/memory/config_test.go` (new)
- `cmd/sworn/memory.go` (new)
- `cmd/sworn/main.go` (additive dispatch only)

## Acceptance checks

- [ ] `sworn memory status` exits 0 with no config file; output shows "using
  defaults" and reports whether Claude Code memory path auto-detected on disk
- [ ] `sworn memory status` reads `~/.config/sworn/memory.json` and prints
  the configured harnesses, embedding provider, and index path
- [ ] Per-project `.sworn/memory.json` overrides global config; `sworn memory
  status` run inside a project with a local config file reflects the merged
  result (project takes precedence)
- [ ] Unknown harness ID in config triggers an error with the list of known
  harness IDs, not a silent ignore
- [ ] `api_key_env` is never printed in `sworn memory status` output; the key
  itself is never logged or surfaced (only the env var name is shown)
- [ ] `go test -race ./internal/memory/...` passes

## Required tests

- **Unit**: `internal/memory/config_test.go`
  - `TestLoadMerge`: global config + project override; project value wins on
    conflict, global value preserved where project doesn't override
  - `TestDefaultsAutoDetect`: with no config, defaults include claude-code path
    if `~/.claude/projects/<encoded-cwd>/memory/` exists on disk; omits it
    if not present
  - `TestUnknownHarness`: config naming an unknown harness returns a named error
  - `TestAPIKeyEnvNotLeaked`: status output contains the env var name but not
    the resolved key value even when the env var is set

- **Reachability artefact**: `sworn memory status` run in this repo's worktree
  with and without a `.sworn/memory.json`; output captured in proof.md.

## Risks

- `<encoded-cwd>` derivation for Claude Code path must match Claude Code's own
  encoding exactly. **Mitigation**: read Claude Code's encoding logic from the
  existing baton scripts (`captain-memory-search.py` already derives this) and
  replicate it in Go; add a test with a known path → encoded mapping.
- Config merge ambiguity (array append vs replace). **Mitigation**: document
  the merge rule explicitly: `harnesses` and `extra_paths` are replaced (not
  appended) by the project config. Add a test confirming this.

## Deferrals allowed?

No blocking deferrals. S24 reads the config struct directly; this slice must
ship a stable API for the `MemoryConfig` type.
