Design is clean — 2 mechanical pins + 3 flags:

1. **Scanner buffer.** Call `scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)` before the read loop. Default 64KB limit will break on large tool-call payloads (file contents, diffs) that S08b/S08c will carry. If you choose a different ceiling, document the constraint explicitly in §4.
2. **ToolResult layout.** Add the field definitions to design §2 D4 before writing code: `ToolResult{IsError bool; Content []ContentItem}`, `ContentItem{Type string; Text string}`. This is the handoff contract S08b and S08c implement against.

Flags (no action required, but worth a glance): (a) round-trip smoke test beyond spec — fine; (b) add `sworn mcp` entry to `usage()` when editing `main.go`; (c) code comment at the batch-rejection site satisfies spec Risk's "Document this limitation" — no separate doc needed.

§2 decisions 1–5 ack (all Type-2, spec-prescribed or standard Go patterns). §6 empty ack.

Address pins 1–2 inline during implementation, then proceed to in_progress.
