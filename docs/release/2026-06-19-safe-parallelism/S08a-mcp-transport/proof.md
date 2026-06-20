# Proof Bundle: S08a-mcp-transport

## Scope

A developer runs `sworn mcp`, configures it as an MCP server in Claude Code (or any MCP-compatible client), and the client successfully negotiates the protocol handshake and receives a tools/list response — even though the tools return stub responses at this stage. The infrastructure is proven before tools are implemented.

## Files changed

```
$ git diff --name-only ea456c0..HEAD
cmd/sworn/main.go
cmd/sworn/mcp.go
docs/release/2026-06-19-safe-parallelism/S08a-mcp-transport/status.json
docs/release/2026-06-19-safe-parallelism/S08a-mcp-transport/spec.md
internal/mcp/server.go
internal/mcp/server_test.go
```

New files:
- `internal/mcp/server.go` — Core MCP server: struct, constructor, Run, method dispatch, handler registration, JSON-RPC response building
- `internal/mcp/server_test.go` — All acceptance tests using `io.Pipe` roundtrips
- `cmd/sworn/mcp.go` — `cmdMcp()` subcommand with signal handling

Modified:
- `cmd/sworn/main.go` — Added `mcp` case to the subcommand switch + usage entry
- `docs/release/2026-06-19-safe-parallelism/S08a-mcp-transport/spec.md` — Added CLI smoke test entry to Required tests
- `docs/release/2026-06-19-safe-parallelism/S08a-mcp-transport/status.json` — State transitions and metadata
## Test results

### Go backend

```
$ go test ./internal/mcp/... -v -count=1 -timeout 30s
=== RUN   TestInitializeHandshake
--- PASS: TestInitializeHandshake (0.00s)
=== RUN   TestInitializedNotification
--- PASS: TestInitializedNotification (0.00s)
=== RUN   TestToolsListEmpty
--- PASS: TestToolsListEmpty (0.00s)
=== RUN   TestUnknownMethod
--- PASS: TestUnknownMethod (0.00s)
=== RUN   TestUnregisteredToolCall
--- PASS: TestUnregisteredToolCall (0.00s)
=== RUN   TestRegisteredToolStub
--- PASS: TestRegisteredToolStub (0.00s)
=== RUN   TestResourcesList
--- PASS: TestResourcesList (0.00s)
=== RUN   TestPromptsList
--- PASS: TestPromptsList (0.00s)
=== RUN   TestBatchRejection
--- PASS: TestBatchRejection (0.00s)
=== RUN   TestInvalidJSON
--- PASS: TestInvalidJSON (0.00s)
=== RUN   TestServerContextCancellation
--- PASS: TestServerContextCancellation (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/mcp	0.005s
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**: Run `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' | sworn mcp` and observe the initialize response with `protocolVersion: "2024-11-05"`
- **Evidence**: Verified end-to-end:
  ```
  $ echo '...' | sworn mcp
  {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05",...}}
  ```

## Delivered

- **[AC#1] `sworn mcp` starts without error and reads from stdin** — evidence: `TestInitializeHandshake` passes; end-to-end `echo '...' | sworn mcp` returns valid JSON-RPC response
- **[AC#2] Initialize handshake returns protocolVersion "2024-11-05" and capabilities** — evidence: `TestInitializeHandshake` asserts `protocolVersion` and `serverInfo`
- **[AC#3] tools/list returns a valid JSON-RPC response with tools array (empty)** — evidence: `TestToolsListEmpty` asserts valid response with empty `tools` array
- **[AC#4] Unknown method returns JSON-RPC error code -32601** — evidence: `TestUnknownMethod` asserts error code `-32601`
- **[AC#5] tools/call for unregistered tool returns isError:true not a crash** — evidence: `TestUnregisteredToolCall` asserts `IsError: true` with `not implemented` text
- **[AC#6] `go test ./internal/mcp/...` passes** — evidence: all 11 tests pass (0.005s)

## Not delivered

None. All acceptance checks are delivered.

## Divergence from plan

- Added `TestServerContextCancellation` (beyond the 5 spec-named tests + round-trip smoke test) — verifies the server exits cleanly on context cancellation, which is important for the signal-handling in `cmd/sworn/mcp.go`
- Added `TestResourcesList` and `TestPromptsList` — verify the declared capabilities return well-formed responses (added to match declared capability surface)

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S08a-mcp-transport 2026-06-19-safe-parallelism
```