# S45-design-tldr — Implementation Journal

## 2026-07-20 — Implementation session

Session opened. Slice state: planned. Fresh track worktree materialised.

**Design decisions ratified during implementation:**
- Dedicated tool-less model call (single-shot `agent.Agent.Chat()`) — not folded into agent loop
- Same implementer model as resolved for the slice
- Timeout: bounded by the same implementTimeout that wraps the agent loop (S42)

## 2026-07-20 — State transition: planned → in_progress → implemented

**Decisions made during implementation:**
- `design.Generate` uses `agent.Agent.Chat()` (single tool-less call) rather than `model.Verifier.Verify()` — this avoids state-sharing between the design step and the verification step when test fakes return the same instance. In production, both paths resolve to the same model.
- Design step runs BEFORE the implement loop in `RunSlice`, using the first escalation model from the list.
- On timeout or model error, the design step warns and proceeds without `design.md` — the TL;DR is a nice-to-have artefact, not a hard gate.
- Test fixes: `TestRun_PassPath_Merges` and `TestRunSlice` factories updated to return fresh agent instances per call (matching production behavior where each `NewAgent` call creates a new model client).

**Trade-offs:**
- The design step adds one extra model call per slice (cost). Mitigated by using the cheapest model in the escalation list and a single-shot call (no tool loop overhead).
- If the model returns tool calls in the design response, they are ignored (the step only reads `Message.Content`). The six-section check catches truly empty responses.

**Subagent dispatches:** None — all implementation in single session.

**State:** implemented → ready for fresh-context verification.