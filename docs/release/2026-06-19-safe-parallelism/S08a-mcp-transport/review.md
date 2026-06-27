# Captain review — S08a-mcp-transport
Date: 2026-06-21
Design commit: 04a497ab0335caf95335e75c9c601d39a933c687

## Pins

1. [mechanical] §2 D1 — `bufio.Scanner` default 64KB token limit is an implicit size ceiling
   What I observed: Decision 1 picks `bufio.Scanner` with default `ScanLines`. Go's `bufio.Scanner` caps single-line tokens at 64KB. The release goal for `sworn mcp` is AI-driven planning and operations (S08b/S08c carry full file contents, diffs, and spec text as tool arguments); a `tools/call` request carrying any of these will routinely exceed 64KB. When it does, `scanner.Err()` returns `bufio.ErrTooLong` and the server must drop the connection — it cannot return a well-formed JSON-RPC error because the request was never fully parsed.
   What to ask the implementer: Before the read loop in `server.go`, call `scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)` (or another reasoned ceiling). Alternatively, document 64KB as a deliberate hard constraint in §4 with explicit rationale for why no planned tool argument will exceed it — but given S08b/S08c's stated scope, that argument is hard to make.

2. [mechanical] §2 D4 — `ToolResult` struct fields not defined in design
   What I observed: D4 declares `ToolHandler` as `func(ctx context.Context, params json.RawMessage) (*ToolResult, error)` but does not define `ToolResult`'s fields. S08b (`tools_ops.go`) and S08c (`tools_plan.go`) both implement `ToolHandler` and must return `*ToolResult`. If the struct is defined without the correct MCP 2024-11-05 wire fields, the JSON output will be non-conformant. MCP 2024-11-05 `tools/call` response shape: `{isError: bool, content: [{type: string, text: string, ...}]}`. The spec already names this shape in §In scope (`{isError: true, content: [{type: "text", text: "not implemented"}]}`), but the design doesn't document the Go struct layout.
   What to ask the implementer: Add a note to design §2 D4 (or confirm inline) that `ToolResult` maps to `{IsError bool; Content []ContentItem}` where `ContentItem` is `{Type string; Text string}`. This is the handoff anchor for S08b and S08c.

## Summary

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none — Pin 1 is a latent size-limit bug (caught at runtime once S08b/S08c supply large tool args), Pin 2 is a completeness gap caught by AC#5 testing.

## Smaller flags (not pins, worth one-line ack)

(a) Design adds "plus a round-trip smoke test" beyond the 5 spec-named tests — benign scope addition, no concern.
(b) `usage()` in `main.go` should include a `sworn mcp` entry — not in spec ACs but user-visible; minor omission the implementer should catch while editing `main.go`.
(c) Spec Risk says "Document this limitation" for batch-request rejection — a code comment at the batch-rejection site in `server.go` (alongside the logged warning) satisfies this; no separate documentation file is needed.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is clean — 2 mechanical pins + 3 flags:

1. **Scanner buffer.** Call `scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)` before the read loop. Default 64KB limit will break on large tool-call payloads (file contents, diffs) that S08b/S08c will carry. If you choose a different ceiling, document the constraint explicitly in §4.
2. **ToolResult layout.** Add the field definitions to design §2 D4 before writing code: `ToolResult{IsError bool; Content []ContentItem}`, `ContentItem{Type string; Text string}`. This is the handoff contract S08b and S08c implement against.

Flags (no action required, but worth a glance): (a) round-trip smoke test beyond spec — fine; (b) add `sworn mcp` entry to `usage()` when editing `main.go`; (c) code comment at the batch-rejection site satisfies spec Risk's "Document this limitation" — no separate doc needed.

§2 decisions 1–5 ack (all Type-2, spec-prescribed or standard Go patterns). §6 empty ack.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins have unambiguous fixes the implementer applies inline; no design re-check or Coach authority needed.
-->
