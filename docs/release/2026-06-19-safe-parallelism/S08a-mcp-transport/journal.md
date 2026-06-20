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