---
title: 'S43-agent-loop-natural-stop — terminate the agent loop on the model''s natural stop, not the turn cap'
description: 'The agent tool loop only returns cleanly on a turn with text content AND no tool calls; a model that finishes its work then stops with empty content + no tool calls (gpt-oss-class) spins to MaxTurns and errors, discarding completed work. S43 treats "no tool calls" as the terminal signal regardless of content, letting proof-from-diff + the verifier judge the actual result — so good work flows to verification instead of being thrown away.'
---

# Slice: `S43-agent-loop-natural-stop`

## User outcome

When an implementer model completes its tool calls and then stops **without a final text
message** (empty content, no tool calls — observed on reasoning models like gpt-oss-120b),
`sworn run` proceeds to verify the work it actually did, instead of spinning the agent loop to
`MaxTurns` and erroring `"turn cap reached with no text response"` (which discards the diff and
forces a blind model escalation).

## Entry point

`internal/agent/agent.go` `Run` loop — the per-turn termination decision. Reached on every
`sworn run` implement attempt.

## Background

The loop returns cleanly only when `msg.Content != "" && len(msg.ToolCalls) == 0`
(`agent.go:111`). A model that stops with empty content and no tool calls matches neither the
return branch nor the tool-call branch, so the loop continues to `MaxTurns` (25) and returns the
turn-cap error — even though the model already produced a complete, correct diff. This is the
in-product analogue of the coach-loop's empty-`result_text` retry storm. sworn judges ground
truth (proof.md is built from `git diff` + tests in `implement.go`, not the agent's prose), so
the agent's final message is not needed for correctness — only the loop's termination logic
assumes it.

## In scope

- In `agent.go` `Run`, treat **a turn with no tool calls as terminal**, returning the
  accumulated content (which may be empty). The "did it actually do the work" judgment is
  deferred to `verify.Run` over the diff, consistent with sworn's ground-truth design.
- Keep the `MaxTurns` cap as the *upper* bound (a model that keeps emitting tool calls without
  finishing still hits the cap and errors → escalates). Only the **empty-content + no-tool-calls**
  case changes from "spin to cap" to "return".
- Confirm `implement.Run` tolerates an empty agent-return string (it builds proof.md from `git
  diff` + test output, not the return value); add a guard/comment if it does not.

## Out of scope

- Synthesizing a summary from the model (the coach-loop's "force-summary") — unnecessary here,
  because nothing downstream consumes the agent's prose; the diff is the artifact.
- Detecting mid-task stalls (tool-call loops with no progress) — bounded by `MaxTurns` today;
  wall-clock hangs are bounded by S42.

## Planned touchpoints

- `internal/agent/agent.go` (terminate on no-tool-calls)
- `internal/agent/agent_test.go` (empty-stop-after-toolcalls returns, does not error)

## Acceptance checks

- [ ] A fake agent that issues tool calls for N turns then returns empty content + no tool calls
  causes `Run` to **return nil error** (not the turn-cap error), with the tool side effects intact
- [ ] A fake agent that returns text + no tool calls still returns that text (happy path unchanged)
- [ ] A fake agent that only ever emits tool calls still hits `MaxTurns` and errors (cap intact)
- [ ] `go test -race ./internal/agent/... ./internal/implement/...` passes; an empty-return
  implement attempt proceeds to proof generation rather than erroring

## Required tests

- **Unit**: `internal/agent/agent_test.go` — `TestRunReturnsOnEmptyStopAfterToolCalls`,
  `TestRunStillCapsOnEndlessToolCalls`, plus the existing happy-path test must stay green.
- **Reachability artefact**: paste the test output in `proof.md` showing the empty-stop case
  returning cleanly and the cap case still erroring.

## Risks

- A model that transiently returns empty *before* doing the work would now terminate early with a
  thin/empty diff — acceptable, because `verify.Run` will FAIL it and the escalation loop advances
  (no worse than today, and no wasted 25-turn spin). Note this trade-off in the design.

## Deferrals allowed?

No deferrals expected — bounded change to one loop's termination condition.
