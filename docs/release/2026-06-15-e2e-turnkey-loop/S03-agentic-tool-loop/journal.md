# Journal — S03-agentic-tool-loop

## State transitions

- **2026-06-16**: Entered `design_review` with approved-ack.md. Captain PROCEED verdict, 6 mechanical pins.
- **2026-06-16**: Transitioned to `in_progress`. Implementation begins.

## Captain pins addressed

1. **AC2 error-return path** — Added error-path test case to reachability plan.
2. **Document sandbox boundary** — Package doc comment in `internal/agent/tools.go` covers confinement logic.
3. **Agent logging discipline** — Security comment carried forward from `internal/model/oai.go`.
4. **Error-path reachability** — Added second FakeAgent sequence: tool error → model adapts → terminates.
5. **Tool-to-JSON-schema ownership** — `ToolDef` struct in `internal/model/` is the wire format; each tool in `internal/agent/tools.go` provides `Schema() model.ToolDef`. Agent imports from model; no drift surface.
6. **Verify regression** — Ran `TestOAI_Verify` after struct extension. All existing cases pass unchanged.

## Design decisions

- ToolDef lives in `internal/model/` (wire format), agent tools implement `Schema() model.ToolDef` — Captain pin 5 resolved as option (a) variant.
- Chat method added to OAI, not a separate client — keeps one HTTP client, one pricing table.
- Tool calls executed sequentially within a turn (no parallel execution) per design §4.
- Workspace confinement is path-prefix enforcement with `filepath.Clean` + prefix check, documented in package doc.
## Implementation summary

- Extended `internal/model/oai.go`: exported ChatMessage, ChatResponse, ToolCall, FunctionCall, UsageBlock, ToolDef. Added `Chat()` method.
- Created `internal/agent/`: Agent interface, Run loop, six tools (Read/Write/Edit/Bash/Grep/Glob), workspace confinement.
- 5 unit tests covering success path, error path (AC2), turn cap (AC4), absolute path rejection, traversal rejection (AC3).
- Pin 6: TestOAI_Verify passes unchanged after struct extension — backward compatible.
