# Captain review — S03-agentic-tool-loop
Date: 2026-06-15T18:50:49Z
Captain version: 0.1
Design TL;DR commit: 65eaf1794941f7fbe79baebeb679b2c17e7ff33b

## Pins

1. [mechanical] §1 — AC2 "tool errors returned to model" not addressed in design
   What I observed: AC2 requires "Tool errors are returned to the model (not fatal); the loop continues." The design §1 describes tool execution and the turn cap but never mentions the error path. The §5 reachability plan tests only the success path (Write → Bash → text termination). An error-return case is absent from both.
   What to ask the implementer: Add one error-path test case to the §5 reachability plan — a tool returning an error → model receives it → loop continues and eventually terminates. Confirm AC2 is covered before code.

2. [mechanical] §2.4 / spec Risk #2 — "document the sandbox boundary" not listed as deliverable
   What I observed: Spec Risk #2 mitigation explicitly says "workspace confinement; document the sandbox boundary." Design §2.4 implements path-prefix enforcement (rejecting `..`, absolute paths, symlink traversal) but doesn't list a documentation deliverable. The spec's "document" verb is distinct from "implement."
   What to ask the implementer: Confirm whether source-code comments in the confinement logic satisfy the spec's documentation requirement, or whether a separate docs entry (e.g. package doc or ADR fragment) is expected. If code comments are sufficient, note it in journal.md.

3. [mechanical] §3 / AGENTS.md Security — logging discipline for agent message history
   What I observed: S02's OAI client carries a security comment: "No logging of API keys, request bodies, or response payloads — per project AGENTS.md Security rule." The agent loop will accumulate multi-turn message history containing file contents and command output fed back to the model. No equivalent logging discipline is stated for the agent loop.
   What to ask the implementer: Confirm the agent loop honours the same logging discipline as the OAI client — no logging of message history, file contents, or tool outputs. Add the equivalent security comment to the agent package.

4. [mechanical] §5 — reachability plan tests success path only; error path untested
   What I observed: The §5 reachability plan's FakeAgent scripts "Write → Bash → text termination." This is a good success-path test. But with AC2 (tool errors returned to model) in play and no error-path test case, the spec's error-handling requirement has no reachability artefact.
   What to ask the implementer: Add a second FakeAgent sequence to the reachability plan: a tool call that returns an error → model receives the error → model issues a different tool call → text termination. This proves the loop continues after tool errors (AC2) and terminates (AC4).

5. [mechanical] §2.1/§3 — cross-package type boundary between model and agent
   What I observed: The design puts `Tool`/`ToolCall`/`ChatMessage` structs (wire format) in `internal/model/oai.go` and tool execution (`Execute` methods) in `internal/agent/tools.go`. Tool definitions — name, description, parameters JSON schema — need to exist somewhere. If defined in `internal/agent/` and serialized in `internal/model/`, the mapping crosses a package boundary with no stated sync mechanism.
   What to ask the implementer: Clarify where tool-to-JSON-schema mapping lives. Options: (a) define tool schemas in `internal/model/` and have agent import them, (b) define in `internal/agent/` and export for model to serialize, (c) a shared `tool.Schema()` method on each tool. Pick one and state it in design §2. Avoid hand-editing the JSON schema on both sides — that's a drift surface.

6. [mechanical] §3 — Verify backward compatibility after struct extension
   What I observed: `internal/model/oai.go` (S02, commit `1e12201`) has `chatMessage` (Role + Content) and `chatResponse` (Choices with Message.Content). S03 will add `Chat` method and extend structs with tool-call fields. The design says "The existing `Verify` method is preserved unchanged." If the shared structs get optional `ToolCalls`/`ToolCallID` fields, `Verify` should stay compatible (nil/empty omitted by JSON). But this is an inference, not tested.
   What to ask the implementer: After adding tool-call fields to `chatMessage`/`chatResponse`, run the existing `TestOAI_Verify` cases to confirm no regression. If `Verify` needs zero changes, note in journal.md. If the extension requires a `Verify` tweak, update §2.1.

## Summary
Pins: 6 total — 6 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: None. The most impactful is pin 1+4 (AC2 error-path coverage gap) — untested error handling could allow a tool error to silently terminate the loop.

## Smaller flags (not pins, worth one-line ack)

(a) §6 is empty ("Open questions for the Coach" heading with no content). Confirm the implementer has no open questions — if questions were intended but lost, re-surface them before code.

(b) Design §4 lists four NOT-doing items with rationales. All are reasonable scope cuts for MVP. No silent deferrals detected — each is acknowledged with a why.

(c) No project memory exists for the sworn repo yet (first Captain run). The memory cross-reference (Step 2) used AGENTS.md non-negotiables as the fallback. As S03–S10 land and feedback accumulates, memory entries will tighten future reviews.

## Suggested ack reply

TL;DR Clean design, well-scoped. 6 mechanical pins + 3 flags:

1. **AC2 error-return path.** Add one error-path test case to the §5 reachability plan — a tool returning an error → model receives it → loop continues. AC2 requires it.
2. **Document the sandbox boundary.** Spec Risk #2 says "document." If code comments in the confinement logic cover it, note in journal.md. Otherwise add a package doc comment.
3. **Agent logging discipline.** Carry forward S02's "no logging of API keys, request bodies, or response payloads" security comment into the agent package. The message history contains file contents and command output — same discipline applies.
4. **Error-path reachability.** FakeAgent test currently scripts success only (Write → Bash → text). Add a second scripted sequence covering tool error → model adapts → terminates.
5. **Tool-to-JSON-schema ownership.** Clarify which package owns the tool schema mapping (name/description/parameters JSON). Avoid hand-editing on both sides of the model/agent boundary. Pick one owner and state it.
6. **Verify regression after struct extension.** After adding tool-call fields to chatMessage/chatResponse, run TestOAI_Verify to confirm backward compatibility. Note result in journal.md.

Flags (not pins): (a) Confirm empty §6 is intentional; (b) NOT-doing items look clean — no silent deferrals; (c) first Captain run on sworn — no project memory yet, future reviews tighten.

§2 decisions 1–5 all ack — clear rationales, good MVP scoping. §6 empty — ack (or re-surface if questions were lost).

Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: six apply-inline mechanical pins (error-path test coverage, sandbox docs, logging discipline, type ownership, regression check) — none require design re-review; no spec deviation, no memory conflict, no authority call
-->