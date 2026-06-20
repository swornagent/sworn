# Design TL;DR — S08a-mcp-transport

## §1. User-visible change

A developer runs `sworn mcp`, which starts a JSON-RPC 2.0 server over stdio (line-by-line
JSON over stdin/stdout) implementing the MCP 2024-11 protocol. The initialise handshake
negotiates protocol version `2024-11-05` and declares empty capabilities (tools, resources,
prompts). The server reads indefinitely until stdin closes. MCP clients (Claude Code, etc.)
can connect; `tools/list` returns an empty array (no implemented tools yet); unknown methods
return error code -32601; `tools/call` for any tool returns `isError: true, text: "not
implemented"`.

## §2. Design decisions not in spec (max 5)

1. **Single-request-per-line parsing, no batching.** The spec's risk section says batch
   requests should be logged and skipped with a clear error. I'll use `bufio.Scanner`
   (default `ScanLines` → split on newlines), parse each line as a single JSON-RPC object,
   and error on arrays. This is the simplest implementation that matches >99% of real MCP
   client behaviour (no known MCP client sends batches over stdio).

2. **Channel-based request dispatch with context support.** The server loop reads requests
   and dispatches them via a method lookup map (not a big switch). Each handler receives
   `context.Context` and the full JSON-RPC request, returns a response. The `RegisterTool()`
   method on Server allows S08b and S08c to register handlers without touching server.go.

3. **`io.Pipe` for test isolation.** The spec requires in-process testing. Tests create
   `io.Pipe()` pairs: the test goroutine writes requests to the write end of stdin; the
   server reads from the read end of stdin, writes to the write end of stdout. The test
   reads responses from the stdout read end. This avoids starting a real subprocess for
   every test.

4. **Tool handler signature.** `RegisterTool(name string, inputSchema json.RawMessage, handler ToolHandler)`
   where `ToolHandler` is `func(ctx context.Context, params json.RawMessage) (*ToolResult, error)`.
   `ToolResult` maps to MCP 2024-11-05 wire shape: `ToolResult{IsError bool; Content []ContentItem}`,
   `ContentItem{Type string; Text string}`. The handler map is `map[string]ToolHandler`. For now
   no tools are registered; S08b and S08c supply them.
5. **Non-exported server type.** `New()` constructor returns a pointer to an unexported
   `server` struct. Only `Run(ctx)` and `RegisterTool(...)` are the public API. Keeps the
   surface small for a package with two downstream consumers (S08b, S08c).

## §3. Files I'll touch grouped by purpose

- **`internal/mcp/server.go`** (new) — Core MCP server: struct, constructor `New()`, `Run(ctx)`
  read loop, method dispatch, handler registration, JSON-RPC response building
- **`internal/mcp/server_test.go`** (new) — All 6 acceptance tests using `io.Pipe`:
  `TestInitializeHandshake`, `TestToolsListEmpty`, `TestUnknownMethod`, `TestUnregisteredToolCall`,
  `TestRegisteredToolStub`, plus a round-trip smoke test
- **`cmd/sworn/mcp.go`** (new) — `cmdMcp()` function: creates server with `mcp.New()`, calls
  `server.Run(ctx)`
- **`cmd/sworn/main.go`** (touch) — Add `case "mcp": os.Exit(cmdMcp(os.Args[2:]))` to the
  subcommand switch

## §4. Things I'm NOT doing

- **No batch JSON-RPC support.** Per spec risk: log a warning and return an error for batch
  arrays.
- **No HTTP/SSE transport.** Out of scope per spec (post-R3).
- **No tool implementations.** S08b and S08c own these.
- **No resource/prompt content.** S08c owns resources and prompts.
- **No logging to stdout.** MCP uses stdout for protocol; all logging goes to stderr.
- **No graceful shutdown beyond context cancellation.** `Run(ctx)` watches `ctx.Done()` and
  stops the read loop (stdin close is the primary shutdown signal).
- **No `mcp.json` or `claude_desktop_config.json` writing.** Setup docs in S08c's `mcp-setup.md`.

## §5. Reachability plan

**Reachability artefact**: configure `sworn mcp` in Claude Code's MCP settings (via repo-level
`.mcp.json` or Claude Desktop config). Open Claude Code, verify no connection error on
startup. Ask "what tools do you have available?" and observe Claude calling `tools/list`
returning an empty array. Capture screenshot or terminal log snippet.

## §6. Open questions for the Coach

None. The spec is clear and self-contained.