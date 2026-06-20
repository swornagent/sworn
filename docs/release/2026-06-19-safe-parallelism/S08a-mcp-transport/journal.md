# Journal — S08a-mcp-transport

## 2026-07-01 — Design TL;DR created

- **Actor**: implementer (first session for this slice)
- **State**: `planned` → `design_review`
- **Decisions**:
  - `bufio.Scanner` for line-delimited JSON parsing (no batching; spec risk)
  - `io.Pipe` for test isolation (in-process, no subprocess)
  - `RegisterTool(name, schema, handler)` public API on server for S08b/S08c injection
  - Server `Run(ctx)` loops until stdin close or context cancellation
  - All stderr for logging; stdout reserved for JSON-RPC protocol
- **Open deferrals**: none
## 2026-07-01 — Implementation complete

- **Actor**: implementer
- **State**: `design_review` → `in_progress` → `implemented`
- **Coach pins addressed**:
  - Pin 1: 4MB scanner buffer (`scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)`)
  - Pin 2: ToolResult/ContentItem struct fields defined in design D4 and code
- **Decisions**:
  - Channel-based read loop for ctx cancellation support
  - `io.Pipe` goroutine model in tests (bufio.Reader, not io.Copy)
  - Added TestServerContextCancellation, TestResourcesList, TestPromptsList beyond spec scope
- **Files created**: `internal/mcp/server.go`, `internal/mcp/server_test.go`, `cmd/sworn/mcp.go`
- **Files touched**: `cmd/sworn/main.go` (mcp dispatch + usage), `docs/release/.../spec.md` (CLI smoke test entry)
- **Tests**: 11/11 PASS (0.005s)
- **First-pass**: 23/23 PASS (FIRST-PASS PASS)
- **Skeptic panel**: skipped — runtime does not support subagent dispatch
- **Open deferrals**: none
