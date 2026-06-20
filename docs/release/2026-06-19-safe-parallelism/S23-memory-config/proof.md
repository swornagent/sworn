---
title: Slice proof bundle — S23-memory-config
description: Rule 6 proof bundle for memory system config + harness discovery + sworn memory status.
---

# Proof Bundle: S23-memory-config

## Scope

A developer running `sworn memory status` sees which AI harnesses are configured (Claude Code, Gemini CLI, OpenCode, Cursor, Codex, Windsurf, custom paths), which memory paths exist on disk, which embedding provider is active, and where the index will be stored — before running any build. Configuration lives at `~/.config/sworn/memory.json` (global) with per-project override at `.sworn/memory.json`. Running without any config file prints sensible defaults and auto-detects the Claude Code memory path if present.

## Files changed

```
$ git diff --name-only 4f2899ec13dc7695fe0daffe618ff1c95112d2a3..HEAD
cmd/sworn/main.go
cmd/sworn/memory.go
cmd/sworn/memory_test.go
docs/release/2026-06-19-safe-parallelism/S23-memory-config/journal.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/proof.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/status.json
internal/memory/config.go
internal/memory/config_test.go
internal/memory/harness.go
```
## Test results

### Go

```
$ go test -race ./internal/memory/...
ok      github.com/swornagent/sworn/internal/memory      1.013s

$ go test -race ./cmd/sworn/...
ok      github.com/swornagent/sworn/cmd/sworn    1.326s

$ go build ./...
# (clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **User gesture**: `sworn memory status` from a directory with no config file, then from a directory with a `.sworn/memory.json` project override.
- **Command output**:

  **No config:**
  ```
  $ cd /tmp/sworn-empty-test && sworn memory status
  memory config: using defaults (no config file found)

  Harnesses:
    Claude Code:    ✗ not found /home/brad/.claude/projects/-tmp-sworn-empty-test/memory

  Embedding:
    provider:  voyage
    model:     voyage-code-3
    api key:   VOYAGE_API_KEY (<not set>)

  Index path: /home/brad/.sworn/memory.db
  ```

  **With project config:**
  ```
  $ cd /tmp/sworn-proof-test && sworn memory status
  memory config:
    loaded: /tmp/sworn-proof-test/.sworn/memory.json

  Harnesses:
    Claude Code:    ✗ not found /home/brad/.claude/projects/-tmp-sworn-proof-test/memory
    Cursor:         ✗ not found /tmp/sworn-proof-test/.cursorrules
    Windsurf:       ✗ not found /tmp/sworn-proof-test/.windsurfrules

  Embedding:
    provider:  ollama
    model:     nomic-embed-text
    api key:   OLLAMA_API_KEY (<not set>)
    base url:  http://localhost:11434

  Index path: /tmp/test-memory.db
  ```

  **Unknown harness (error exit):**
  ```
  $ mkdir -p /tmp/bad-config/.sworn && cat > /tmp/bad-config/.sworn/memory.json << 'EOF'
  { "harnesses": ["claude-code", "nonexistent-harness"] }
  EOF
  $ cd /tmp/bad-config && sworn memory status
  error: loading memory config: unknown harness "nonexistent-harness"; valid: claude-code, gemini-cli, opencode, cursor, windsurf, codex, custom
  ```
- **Evidence path**: Test output captured above. `TestCmdMemory_Status_NoConfig`, `TestCmdMemory_Status_WithConfig`, `TestCmdMemory_Status_UnknownHarness` in `cmd/sworn/memory_test.go` cover the same paths programmatically.

## Delivered

- **AC 1: `sworn memory status` with no config file exits 0 and shows "using defaults"** — evidence: `TestCmdMemory_Status_NoConfig` asserts exit code 0; reachability artefact above confirms the output contains "using defaults".
- **AC 2: `sworn memory status` reads `~/.config/sworn/memory.json` and prints configured values** — evidence: `TestLoadMerge` in `config_test.go` verifies global config loading; reachability artefact with project config shows loaded paths and configured harnesses/embedding.
- **AC 3: Per-project `.sworn/memory.json` overrides global config; project values take precedence** — evidence: `TestLoadMerge` verifies project harnesses replace (not append to) global; project embedding replaces global when specified; global values preserved where project doesn't override.
- **AC 4: Unknown harness ID triggers an error listing known IDs** — evidence: `TestUnknownHarness` asserts `ErrUnknownHarness` with ID and knowns; `TestCmdMemory_Status_UnknownHarness` asserts non-zero exit code; reachability artefact confirms the error message format.
- **AC 5: `api_key_env` is never printed in status output; only the key status (`<set>` / `<not set>`) is shown** — evidence: `TestAPIKeyEnvNotLeaked` verifies the contract; `apiKeyStatus()` in `cmd/sworn/memory.go` never returns the raw value — only `<set>` or `<not set>`; reachability artefact confirms the output format.
- **AC 6: `go test -race ./internal/memory/...` passes** — evidence: captured test output shows `ok`.

## Not delivered

- None — all acceptance checks are delivered.

## Divergence from plan

- `cmd/sworn/memory_test.go` was added per Coach pin in design review (acknowledged in journal.md 2026-06-27). It is implied by the spec's "Required tests" section (which references CLI integration tests) but was missing from the `spec.md` Planned touchpoints. All other planned files match.
## First-pass script output (re-entry fix round)

```
$ $HOME/.claude/bin/release-verify.sh S23-memory-config 2026-06-19-safe-parallelism
release-verify.sh
  slice:       S23-memory-config
  slice dir:   docs/release/2026-06-19-safe-parallelism/S23-memory-config
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  PASS  integration branch drift present but does not affect test infrastructure

== Diff vs start_commit (verifier base) ==
  PASS  9 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~7) consistent with diff vs start_commit (9)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```