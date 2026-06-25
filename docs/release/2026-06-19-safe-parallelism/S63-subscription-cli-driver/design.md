# Design TL;DR — S63-subscription-cli-driver

## §1. User-visible change

A developer who pays for Claude Code (Pro/Max) or ChatGPT (Codex) can run `sworn` with **no API key set** — not even a `SWORN_*_API_KEY` env var. They configure a role to use `claude-cli:<model>` or `codex:<model>` in the per-role model config (S09), and sworn dispatches that role by spawning the user's locally-installed `claude -p` or `codex exec`, which authenticate through the CLI's own logged-in session. No provider account, no key rotation — just the subscription they already have.

## §2. Design decisions not in spec (max 5)

1. **Driver struct is `cliDriver` in `internal/model/cli.go`, separate from direct-API drivers.** Keeps the subprocess logic isolated; shares the same `Verifier` interface so no caller changes.
2. **`NewClient()` adds `claude-cli` and `codex` provider prefixes, returning a `*cliDriver` with the user-chosen model.** The driver is a first-class provider alongside `openai`, `anthropic`, etc. — no special-case routing.
3. **`FromEnv()` treats `claude-cli` and `codex` as keyless providers** — no API key check. The switch in the key-gate section (L87-108) adds them alongside `vertex`, `bedrock`, and `oci`.
4. **Timeout is configurable via `SWORN_CLI_TIMEOUT` (default 300s).** Subprocess is bounded by `exec.CommandContext` with a context deadline; a timeout returns a typed `model.Error{Kind: KindTransient}`.
5. **Binary override: `CLAUDE_BIN` / `CODEX_BIN` env vars** for testing. Mirrors the reference driver's `CLAUDE_BIN` convention; absent, defaults to `claude` / `codex` on `PATH`.

## §3. Files I'll touch grouped by purpose

| Group | Files | Why |
|---|---|---|
| Subprocess driver | `internal/model/cli.go` (new) | Implements `Verifier` by spawning `claude -p` / `codex exec` with timeout, capturing stdout as verdict text |
| Driver registration | `internal/model/provider.go` | Adds `claude-cli` and `codex` cases to `NewClient()` switch |
| API key bypass | `internal/model/config.go` | Adds `claude-cli` / `codex` to keyless providers in `FromEnv()` so the `SWORN_*_API_KEY not set` error is skipped |
| Tests | `internal/model/cli_test.go` (new) | Fake `claude`/`codex` binaries on `PATH` via `t.TempDir()`; tests for: normal dispatch, missing binary, auth failure (simulated exit code), timeout, model passthrough |

## §4. Things I'm NOT doing

- **Not implementing the full reference `claude-cli.sh` stream-json parsing or result-line contract.** The Go driver is simpler: capture stdout as the verdict text. No tool loop — `claude -p` with the full role prompt as the arg returns text; the model's output IS the verdict.
- **Not implementing `codex exec` — CLAIM AS RULE 2 DEFERRAL.** The two CLIs have different invocation shapes and output normalisation. `claude-cli` ships first; `codex` is a declared deferral with `// TODO: codex exec support (S63-deferral-1)` in `cli.go` and a GitHub issue.
- **Not changing the orchestration loop / scheduler (T17).** This slice provides a driver the loop *uses*; the loop's driver-selection path (`FromEnv` → `NewClient`) is the integration point, and that's what gets exercised.
- **Not adding auth probing beyond binary presence.** The spec says "return a typed `model.Error{Kind: ...}`" — the driver classifies `exec.ErrNotFound` as `KindTransient` (missing binary) and non-zero exit codes as `KindAuth`. No separate `claude --version` probe; that's premature optimisation.

## §5. Reachability plan

`go test -race ./internal/model/...` exercises `cliDriver.Verify()` against a fake `claude` binary on `PATH` (via `t.TempDir()` + `CLAUDE_BIN=<fake>`). The integration point is `FromEnv("claude-cli/sonnet")` → `NewClient()` → `cliDriver.Verify()`. No end-to-end `sworn` invocation needed — the driver is a leaf implementing `Verifier`, and `NewClient` + `FromEnv` are the integration points already covered by the existing model tests.

## §6. Open questions for the Coach

*(empty)*
