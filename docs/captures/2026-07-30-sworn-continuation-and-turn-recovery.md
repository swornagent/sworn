# Sworn continuation and turn recovery

Date: 2026-07-30

Status: agreed direction; implementation pending approved Baton plan

Target: Sworn v1.0.0 release candidate

## Outcome

Sworn should preserve an Implementer's bounded reasoning context across the
Captain handoff and should recover intelligently when an otherwise healthy
agent turn does not end in the expected submission.

This does not change Baton. Baton already requires a distinct Captain, allows
the Implementer to resume after review, and reserves unconditional clean
context for the Verifier. Sworn currently makes every production dispatch
fresh and therefore implements a stricter, slower flow than the protocol.

The target flow is:

```text
fresh Implementer design
        |
        | suspend private continuation; release workspace authority
        v
fresh Captain review
        |
        | commit exact decision; revalidate authority
        v
resume same Implementer + Captain decision + fresh tool authority
        |
        | discard continuation
        v
fresh read-only Verifier
```

Continuation is a performance cache, never authority. If it is absent,
expired, oversized, unsupported, or bound to stale facts, Sworn discards it and
rehydrates a fresh Implementer from the durable design and Captain receipt.

## Why this is safe

The suspended continuation owns no live process, workspace lease, Git ref,
submission permission, or Baton decision. On resume, Sworn issues a new tool
session only after matching the exact run, release, slice, attempt, plan,
target, design, Captain receipt, driver configuration, profile, model, and tool
contract.

Captain never receives the Implementer continuation. Verifier remains a
separate fresh read-only invocation and cannot accept any prior role's
continuation.

No transcript, reasoning block, prompt, provider response ID, or native session
ID enters Git, the durable journal, WebUI, notification payloads, or OTel.

## One common driver capability

Continuation belongs to the common role-independent driver layer, not to
provider-specific role drivers:

```go
InvokeTurn(ctx, invocation, continuation) -> {
    handoff | yield | recoverable_terminal,
    continuation
}
```

The opaque handle is process-local, non-serializable, bounded, explicitly
closed, and zeroed. Drivers may implement increasingly capable modes:

1. `fresh_rehydrate`, available universally;
2. `transcript_replay`, for standard Chat-style messages and tools;
3. `opaque_replay`, for provider-owned reasoning and signatures;
4. `provider_cursor`, for an explicitly admitted response/session cursor;
5. `native_session`, for a private CLI session store; and
6. `compacted`, after measured context thresholds justify it.

These are capabilities behind one interface, not orchestration branches.

## Provider state

Sworn already preserves most provider-owned state inside a single tool loop,
then destroys it at invocation end:

| Surface | Replayable state |
| --- | --- |
| OpenAI Responses | exact output items, including encrypted reasoning |
| Generic Chat Completions | assistant messages, tool calls, and tool results |
| DeepSeek | Chat transcript plus exact `reasoning_content` |
| Gemini | model parts plus exact `thoughtSignature` |
| Bedrock Runtime | signed and redacted reasoning blocks plus tool use |
| Bedrock Mantle | Chat transcript; response-only reasoning remains inert |
| OpenRouter | requires exact `reasoning_details` support |
| Codex and Claude CLIs | require native session ID plus a private resumable home |

Visible thinking emitted as assistant content is ordinary transcript context.
Separate reasoning fields survive only when the provider returns and accepts
them. Hidden reasoning cannot be reconstructed. Opaque provider state must be
replayed byte-for-byte through the same dialect, never interpreted,
normalized, or translated.

Before suspending an HTTP conversation after a valid `sworn_submit`, Sworn must
append the exact accepted tool result. Resumption then appends the new
implementation envelope and Captain receipt using the newly issued tool set.

Initial Responses support remains stateless with `store:false` and exact item
replay. Server cursors and compaction are later optimizations, not correctness
dependencies.

## Turn recovery controller

The original Coach loop used a cheap interpreter to keep structurally unusual
agent turns from stopping the loop. Sworn restores that essence as a
non-authoritative automation component outside Baton.

Deterministic transport and durable state always outrank model interpretation.
A valid `sworn_submit` follows the normal path without invoking recovery.

The controller may return only:

```text
resume_worker(message)
ask_captain(question)
retry_operationally(reason)
pause_track_for_human(question)
```

It cannot create or accept a submission, mint `PROCEED`, `REVISE`, `PASS`,
`FAIL`, or `BLOCKED`, change scope, execute a shell command, move a ref, or
Merge.

One terminal tool is sufficient:

```json
{
  "name": "sworn_yield",
  "arguments": {
    "kind": "question | blocked",
    "message": "bounded text"
  }
}
```

Recovery order:

1. Return exact tool/schema errors to the same session for at most two bounded
   corrections.
2. For a prose-only terminal response, ask the same worker once to call
   `sworn_submit` or `sworn_yield`.
3. Classify a typed yield or unresolved terminal using the fast configured
   recovery model.
4. Answer only from exact engine facts or quoted current authority.
5. Route genuine design judgment to a distinct advisory Captain that cannot
   issue a gate receipt.
6. Otherwise pause only that track, notify the human, and continue independent
   tracks.

Verifier ambiguity never receives Implementer context. It must produce its
normal typed verdict, yield `blocked`, or pause for human attention.

## Historical correction

The pre-Sworn baseline is Fired's May 29 stateless loop
`5d836ed6da34f600a4ab27cd01ed70d27521c7d9`, board semantics
`2c8ce2417404b051d5a90f92f63c3f2e9551edce`, and the June 10 common driver
boundary `b7654a30942a0f4bc6308fe55e18a7d69bce6a1`.

The original interpreter first appeared at `85442c160b898354af6ed1429e1e136bfcf0ab7c`,
was connected to implementation at `41897467f48337ac97ecc5a5ef7ab2238b8cae68`,
and had false human pages corrected at
`d0267131e72f17c4ab67cebad1444b7e0f29656b`.

It classified `DONE`, `RETRY`, `BLOCKED:<shell>`, or `PAGE:<reason>` from the
last bounded output. It held no resumable model session, did not interpret
Verifier verdicts, and paged every direct question. Sworn retains the useful
classification seam but removes unbounded retry and model-generated shell
execution.

## Delivery boundary

The v1 release candidate should include:

- one bounded continuation handle with fresh fallback;
- HTTP/API Implementer continuation across Captain review;
- exact Responses, Chat, DeepSeek, OpenRouter, Gemini, and Bedrock replay;
- bounded same-session submission correction;
- typed yield, recovery routing, per-track human attention, and notification;
- strict Captain and Verifier isolation;
- continuation and recovery measurements without content capture.

Native Codex and Claude resumption requires a private Sworn-owned session store,
new resume arguments, session-ID capture, cleanup guarantees, and certification
of the Design read-only to Implementation read-write transition. It should be
one focused follow-up slice. Until it passes, those drivers truthfully use
fresh rehydration rather than pretending to resume.

Server-side Responses cursors, provider compaction, durable raw continuation
across Sworn restarts, and model-written transcript summaries are deferred
until measurements show they are needed.

## Measures

Local evaluation and opt-in OTel should report only stable facts:

- continuation mode, reuse, fallback, expiry, and compaction count;
- replay-size and token-count buckets;
- recovery classification and correction-turn counts;
- human escalation and successful-resume rates;
- elapsed time and tokens from design through implementation; and
- false acceptance count, which must remain zero.

No metric label or span attribute contains transcript, reasoning, session ID,
question text, source, diff, credential, or high-cardinality candidate data.
