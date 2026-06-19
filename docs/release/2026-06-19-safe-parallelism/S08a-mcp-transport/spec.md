---
title: 'S08a-mcp-transport — MCP JSON-RPC server + initialize handshake'
description: 'sworn mcp starts an MCP 2024-11 compliant JSON-RPC 2.0 server over stdio. Handles initialize/initialized handshake, tools/list, resources/list, and routes tool calls to registered handlers (empty stubs for now).'
---

# Slice: `S08a-mcp-transport`

## User outcome

A developer runs `sworn mcp`, configures it as an MCP server in Claude Code (or any
MCP-compatible client), and the client successfully negotiates the protocol handshake
and receives a tools/list response — even though the tools return stub responses at
this stage. The infrastructure is proven before tools are implemented.

## Entry point

`sworn mcp` subcommand — reads JSON-RPC 2.0 from stdin, writes responses to stdout,
until stdin is closed.

## In scope

- `internal/mcp/server.go`:
  - Read loop: `bufio.Scanner` over stdin, parse each line as a JSON-RPC 2.0 request
    (or batch)
  - Write: JSON-RPC 2.0 response/notification to stdout, one JSON object per line
  - `initialize` request handler: respond with `{protocolVersion: "2024-11-05",
    capabilities: {tools: {}, resources: {listChanged: false}, prompts: {}}}`
  - `initialized` notification: accept and ignore
  - `tools/list` handler: return the registered tool list (names + input schemas) —
    populated from `tools_ops.go` and `tools_plan.go` in later slices; for now
    returns an empty array (so the handshake is provable without tool implementations)
  - `resources/list` handler: returns empty array for now
  - `prompts/list` handler: returns empty array for now
  - Unknown method: return JSON-RPC `{error: {code: -32601, message: "Method not found"}}`
  - Tool call dispatch: `tools/call` routes to a registered handler map; returns
    `{isError: true, content: [{type: "text", text: "not implemented"}]}` for
    unregistered names (safe stub)
- `cmd/sworn/mcp.go`: `sworn mcp` subcommand — creates `mcp.Server`, calls `server.Run(ctx)`
- `cmd/sworn/main.go`: dispatch `mcp` subcommand
- Handler registration pattern: `server.RegisterTool(name, schema, handler)` so
  S08b and S08c can add handlers without touching server.go

## Out of scope

- Actual tool implementations (S08b, S08c)
- Resource content (S08c)
- Prompt content (S08c)
- HTTP/SSE transport (post-R3)
- Authentication of clients (local stdio trust model)

## Planned touchpoints

- `internal/mcp/server.go` (new)
- `internal/mcp/server_test.go` (new)
- `cmd/sworn/mcp.go` (new)
- `cmd/sworn/main.go` (touch — dispatch mcp)

## Acceptance checks

- [ ] `sworn mcp` starts without error and reads from stdin
- [ ] Sending `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}` returns a response with `protocolVersion: "2024-11-05"` and the declared capabilities
- [ ] Sending `tools/list` returns a valid JSON-RPC response with a (possibly empty) `tools` array — no error
- [ ] Sending an unknown method returns `{error: {code: -32601}}`
- [ ] Sending `tools/call` for an unregistered tool returns `{isError: true}` not a crash
- [ ] `go test ./internal/mcp/...` passes; all roundtrips tested in-process (pipe stdin/stdout via `io.Pipe`)

## Required tests

- **Unit**: `internal/mcp/server_test.go`
  — `TestInitializeHandshake`: send initialize; assert protocolVersion in response
  — `TestToolsListEmpty`: send tools/list before any tools registered; assert valid
    empty response (not error)
  — `TestUnknownMethod`: send unknown method; assert -32601 error code
  — `TestUnregisteredToolCall`: tools/call for unknown name; assert isError: true
  — `TestRegisteredToolStub`: register a no-op tool; send tools/call; assert called
    and returns content (not error)
- **Reachability artefact**: configure `sworn mcp` in Claude Code's MCP settings;
  open Claude Code; confirm no connection errors; ask "list sworn tools" and observe
  Claude calls `tools/list` (returns empty or stub list). Screenshot or log in proof.md.

## Risks

- JSON-RPC batching is in the spec but rarely used by MCP clients. Implement single-
  request handling only; log and skip batch requests with a clear error. Document this
  limitation.

## Deferrals allowed?

No. S08b and S08c both register handlers with this server.
