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

## 2026-06-21 — Verifier verdict: PASS (round 1)

- **Actor**: verifier (fresh-context session, Rule 7 compliant)
- **State**: `implemented` → `verified`
- **Verified against**: `fb35263ea9eac3fb4a93f618beb99e245409e91a`
- **All six gates passed.**
  - Gate 1: `sworn mcp` subcommand wired in `cmd/sworn/main.go:43-47`; `cmdMcp()` creates `mcp.New()` and calls `server.Run(ctx, os.Stdin, os.Stdout)`. Production code, not a test fixture.
  - Gate 2: All four planned touchpoints present in diff; extra files are baton artefacts only.
  - Gate 3: All 5 spec-named tests present and passing (11/11 live re-run, 0.005s). Tests use `io.Pipe` to exercise the integration point (`Run` loop), not leaf functions.
  - Gate 4: `manual-smoke-step` reachability artefact documented in proof.md with exact user gesture and observed output. CLI-only feature; no browser required.
  - Gate 5: No `TODO`/`FIXME`/`deferred`/`placeholder`/`XXX`/`HACK` markers in changed source files. "later slices" references in comments are forward documentation with explicit slice IDs.
  - Gate 6: All 6 ACs delivered with evidence. Extra tests (TestServerContextCancellation, TestResourcesList, TestPromptsList) disclosed in Divergence from plan — additive, not substituting.
- **Next**: `/implement-slice S08b-mcp-ops-tools 2026-06-19-safe-parallelism` in a fresh session.
