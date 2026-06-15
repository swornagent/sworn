# Design TL;DR — S03-agentic-tool-loop

## §1. User-visible change

The engine gains the ability to drive a model through a multi-turn tool loop. A
caller (the implementer role, S06) asks the model to perform work in a workspace.
The model can request tool operations — Read, Write, Edit, Bash, Grep, Glob — and
the engine executes them in a workspace-confined sandbox, feeds results back, and
repeats until the model produces a final text response or a turn cap is hit. The
verifier does NOT use this (it's a single-shot call); the agentic loop is the
implementer's engine exclusively.

## §2. Design decisions not in spec (max 5)

1. **Extend OAI with `Chat`, not a new client.** S02's `OAI` already speaks
   `/chat/completions`. Adding a `Chat(ctx, messages, tools)` method (supporting
   tool_calls in both request and response) keeps one HTTP client and one set of
   pricing/error handling. The existing `Verify` method is preserved unchanged
   for the verifier path. *Rationale: one client, two call patterns.*

2. **Separate `Agent` interface in `internal/agent/`.** The agentic model needs a
   different contract than `model.Verifier` — multi-message, tool-aware. A new
   `Agent` interface in the agent package keeps the model package's `Verifier`
   contract small and stable. The `OAI.Chat` method satisfies it. *Rationale:
   different callers, different contracts; don't bloat Verifier.*

3. **Tool definitions are Go structs, not a runtime DSL.** Read/Write/Edit/Bash/
   Grep/Glob are six concrete tool types, each with an `Execute(ctx, workspaceRoot,
   args)` method. This is compiled, type-safe, and testable with fake models.
   *Rationale: MVP is 6 tools; a pluggable registry is premature.*

4. **Workspace confinement is path-prefix enforcement, not chroot.** Every file
   path is resolved relative to the workspace root; `..` segments, absolute paths,
   and symlink traversal are rejected at the tool-execution layer before any IO.
   *Rationale: cross-platform, no root privileges, sufficient for the threat model
   (accidental escape, not adversarial jailbreak).*

5. **Turn cap and output cap are both configurable and enforced in the loop.**
   `MaxTurns` (default 25) stops the loop; `MaxOutputBytes` (default 100KB) per
   tool response truncates with a marker so the model knows output was capped.
   *Rationale: spec names both caps; making them configurable lets S06 tune per
   task.*

## §3. Files I'll touch grouped by purpose

- `internal/model/oai.go` — extend `OAI` with `Chat` method, add `Tool`/`ToolCall`
  /`ChatMessage` structs for the full chat-completion protocol. *Why: S02 client
  is the transport; agent needs tool-aware calls.*

- `internal/agent/agent.go` — `Agent` interface + `Run` loop. *Why: the core of
  S03 — multi-turn execution with tool dispatch.*

- `internal/agent/tools.go` — six tool definitions (Read, Write, Edit, Bash, Grep,
  Glob) with workspace confinement. *Why: tool execution is separate from the loop
  logic.*

- `internal/agent/agent_test.go` — unit tests with a `FakeAgent` that emits
  scripted tool-call sequences; assert file changes and turn-cap termination.
  *Why: only test command cited in spec.*

- `internal/model/oai_test.go` — add `TestOAI_Chat` cases (tool-call response,
  multi-turn). *Why: transport changes need coverage.*

## §4. Things I'm NOT doing

- Implementer role logic / proof-bundle generation (S06).
- Streaming tool calls (SSE). The MVP loop is request-response; streaming is a
  future optimisation.
- Tool approval / human-in-the-loop gating. Every tool call executes immediately;
  the sandbox is the guardrail.
- Parallel tool calls. The MVP executes tool calls sequentially within a turn;
  the OpenAI API can return multiple tool_calls but serial execution is simpler
  and sufficient.
- The verifier using this loop. The verifier stays single-shot via `model.Verifier`.

## §5. Reachability plan

**Integration test** in `internal/agent/agent_test.go`: a `FakeAgent` that scripts
a Write tool call → Bash tool call → text termination. Assertions:
- The file written by the Write tool exists and contains expected content.
- The Bash tool's stdout appears in the message history.
- The loop terminates after the model returns text (not a tool call).
- A turn cap of N halts the loop at exactly N turns even if the fake keeps
  returning tool calls.

This is a unit/integration test at the package boundary — the entry point is
`agent.Run(ctx, fakeAgent, prompt, workspaceRoot, config)`. No external model,
no HTTP. Reachability is proven by driving the public API surface.

## §6. Open questions for the Coach