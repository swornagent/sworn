# Design TL;DR — S23-memory-config

## §1. User-visible change

A developer running `sworn memory status` sees a human-readable table showing which AI harnesses are configured (Claude Code, Gemini CLI, OpenCode, Cursor, Windsurf, Codex, custom), which specific memory paths exist on disk, which embedding provider (`voyage`/`oai-compat`/`ollama`) is active, and where the SQLite index will be stored. The command works with no config file (auto-detects Claude Code memory path and reports "using defaults"), with a global config at `~/.config/sworn/memory.json`, or with a per-project override at `.sworn/memory.json`. The API key's env var name is shown but the key value is never printed or logged. Unknown harness IDs in config produce a clear error listing the valid harnesses.

## §2. Design decisions not in spec (max 5)

1. **Config merge: arrays are replaced (not appended).** The spec flags this as a Risk. If the project config lists `"harnesses": ["cursor"]`, it starts fresh — the global `["claude-code", "gemini-cli"]` is not merged in. Rationale: an explicit project override should be a complete declaration, not an append. S24/S25 consumers read the merged result; an accidental global leak is worse than a missing harness.

2. **Claude Code memory path encoding is simple `/` → `-` substitution.** The spec references `captain-memory-search.py` as the authority. That script uses `str(project_path).replace("/", "-")` to derive the encoded project directory under `~/.claude/projects/`. I replicate this exactly in Go via `strings.ReplaceAll(path, "/", "-")`. Cross-platform: Windows `\` is normalised to `/` via `filepath.ToSlash` before encoding, matching Go's OS-agnostic `filepath.Join`.

3. **Embedding provider config lives in the memory config, not the main sworn config.** The spec places `embedding.provider`, `embedding.model`, `embedding.api_key_env`, `embedding.base_url` in `memory.json`. Rationale: orthogonal to model selection for verification (which lives in the main `config.json`). S24 reads the embedding config from `MemoryConfig.Embedding` directly.

4. **Harness paths report existence-on-disk as a boolean, not size/mtime.** `sworn memory status` runs `os.Stat()` on each path and prints `✓ (exists)` or `✗ (not found)`. Rationale: this is a pre-build check — S23 is about "is my config ready to build?", not statistical analysis. S24 reads the same paths.

5. **The `api_key_env` sentinel is `"<set>"` / `"<not set>"`, never the raw value.** When displaying the embedding config, the env var name is shown verbatim; the resolved value column shows only whether the env var is set (non-empty string). Rationale: prevents accidental key exposure in shell history, screenshots, or log capture. The verifier AC-4 explicitly gates on this.

## §3. Files I'll touch grouped by purpose

| Group | Files | Why |
|---|---|---|
| **Config model** | `internal/memory/config.go` (new) | `MemoryConfig` struct with `Load()`, `Defaults()`, JSON marshal/unmarshal; merge logic (global → project override, arrays replaced) |
| **Harness discovery** | `internal/memory/harness.go` (new) | `ListHarnesses()` returns all known harnesses with their canonical paths; `HarnessMemoryPath()` maps harness ID → path; existence check via `os.Stat`; error type `ErrUnknownHarness` |
| **CLI entry point** | `cmd/sworn/memory.go` (new) | `sworn memory` root command with `status` subcommand; prints formatted table; additive dispatch from main.go |
| **Additive dispatch** | `cmd/sworn/main.go` (additive) | Add `case "memory": os.Exit(cmdMemory(os.Args[2:]))` — additive only, no existing case changed |
| **Tests** | `internal/memory/config_test.go` (new) | `TestLoadMerge`, `TestDefaultsAutoDetect`, `TestUnknownHarness`, `TestAPIKeyEnvNotLeaked` — as specified in ACs |

## §4. Things I'm NOT doing

- Building or querying the embedding index (S24 — `sworn memory build`)
- Migration from `captain-memory-search.py` (S25 — `sworn memory search`)
- TUI settings panel for memory config (post-R3)
- Auto-installing Claude Code memory path if it doesn't exist (detection only)
- Validating that the index path is writable (S24 will fail at build time)
- Adding `sworn memory` to the usage string in `usage()` — the `default` case in `main.go` already handles this (unknown commands print "unknown command" + usage), and the subcommand `help` case only special-cases `run`; `sworn memory --help` will work via the flag set. Additive dispatch in the switch means no usage() update needed.

## §5. Reachability plan

Concrete artefact path (Rule 1):

1. **With no config**: `cd /tmp/test-empty && /path/to/sworn memory status` — output captured showing "using defaults" + auto-detect result.
2. **With global config only**: Write a temp `~/.config/sworn/memory.json`, run `sworn memory status` — output shows configured harnesses, embedding provider, index path.
3. **With project override**: Write `.sworn/memory.json` in the same dir, run `sworn memory status` — output shows merged values with project taking precedence.
4. **Integration test**: `TestCmdMemory_Status` in `cmd/sworn/memory_test.go` (or inline in memory.go's test file) exercises `cmdMemory()` with args, asserting exit code 0 and expected output substrings.

## §6. Open questions for the Coach

None — all design decisions are covered in spec or §2 above.