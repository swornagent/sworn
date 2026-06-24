# Journal — S63-subscription-cli-driver

## 2026-07-14 — Implementation

### State: in_progress → implemented

**Session:** Single implementer session. All 9 Coach pins from approved-ack.md addressed inline.

### Design decisions enacted

| # | Decision | Type | Rationale |
|---|---|---|---|
| 1 | `cliDriver` struct in `internal/model/cli.go`, separate from direct-API drivers | Type-2 | Isolates subprocess logic; shares `Verifier` interface |
| 2 | `NewClient()` adds `claude-cli` returning `*cliDriver`; `codex` returns deferral error | Type-2 | Claude-CLI ships first; codex deferred for different invocation shape |
| 3 | `FromEnv()` early return for `claude-cli`/`codex` BEFORE proxy routing block | Type-2 | Subscription-based drivers bypass API-key gate and proxy routing entirely |
| 4 | Timeout via `SWORN_CLI_TIMEOUT` (default 300s) | Type-2 | Bounded subprocess prevents silent hangs |
| 5 | Binary override via `CLAUDE_BIN`/`CODEX_BIN` env vars | Type-2 | Mirrors reference driver convention for testing |

### Coach pins resolved

1. ✅ Added `internal/model/provider.go` to `planned_files` in status.json
2. ✅ Added `design_decisions` array to status.json (all Type-2)
3. ✅ `codex` case in `NewClient` returns a deferral error (wraps `ErrDriverNotRegistered`), not `*cliDriver`
4. ✅ `FromEnv()` early return for `claude-cli`/`codex` BEFORE proxy routing block — bypasses sworn login proxy
5. ✅ Missing binary maps to `KindOther` (terminal) — handled via `*exec.Error` and `*fs.PathError` type assertions (Go 1.26 returns `*fs.PathError` for absolute-path missing binaries)
6. ✅ `claude -p` invocation includes `--no-session-persistence` (Rule 7 fresh-context property) and `--model <model>`
7. ✅ `systemPrompt + "\n\n" + userPayload` concatenated as single prompt arg to `claude -p`
8. ✅ GitHub issue #19 filed for codex deferral; tracking URL in cli.go and provider.go
9. ✅ Slash syntax `claude-cli/sonnet` used consistently (spec colon notation was advisory)

### Open deferrals

| ID | Description | Tracking | Acknowledged |
|---|---|---|---|
| S63-deferral-1 | Codex exec subprocess driver support | GitHub #19 | Coach (flag c in approved-ack.md) |

### Codex deferral (S63-deferral-1)

- **Why:** Different invocation shapes and output normalisation from `claude -p`. Claude-CLI ships first to unblock subscription-based flow.
- **Tracking:** `// TODO: codex exec support (S63-deferral-1)` in `cli.go` and `provider.go`; GitHub issue #19
- **Acknowledgement:** Coach ack'd: "Codex case in NewClient returns error, not cliDriver" (pin 3) and "(c) non-zero exit → KindAuth is coarse but acceptable for v1" (flag c)

### Files changed

- `internal/model/cli.go` (new) — subprocess driver: `cliDriver` struct, `Verify()`, `classifyError()`, constructor helpers
- `internal/model/provider.go` — added `claude-cli` and `codex` cases to `NewClient()`
- `internal/model/config.go` — added early return for `claude-cli`/`codex` before proxy routing block
- `internal/model/cli_test.go` (new) — 11 tests: normal dispatch, missing binary, auth failure, timeout, FromEnv integration, empty model, codex deferral, proxy routing bypass, model passthrough, sentinel guard

### Test results

- `go test -race ./internal/model/...` — PASS (6.357s, all existing + new tests)
- `go build ./...` — PASS
- `go vet ./internal/model/...` — PASS

### Reachability

The integration point is `FromEnv("claude-cli/sonnet")` → `NewClient()` → `cliDriver.Verify()`. Tested via `TestClaudeCLI_FromEnvIntegration` which exercises the full path from config resolution through subprocess dispatch.