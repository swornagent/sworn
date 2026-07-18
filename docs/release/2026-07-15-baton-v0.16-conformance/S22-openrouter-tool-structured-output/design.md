# S22 design: native proof receipt recovery

## Material re-scope

The earlier S22 design and Captain PROCEED covered the direct OpenRouter
forced-tool transport only. They do not authorize implementation after the
proof-receipt recovery policy below. This document supersedes that design for
the resumed slice and requires a fresh Captain design review before any source
or provider action.

The historical direct invocation is represented by the committed
receipts/attempt-1.json metadata-only record. Its raw output was destroyed. The
record is a receipt_failure with UNPARSEABLE result, not a model verdict, and
is the only historical input to the new retry policy.

The 2026-07-18 narrow safety replan adds three design obligations that were not
covered by the superseded Captain review: stable payload-free MCP provider
errors, fail-closed recovery when finalization and reservation restoration both
fail, and a mechanical S21 evidence gate before any proof dispatch. This
revision incorporates those obligations and requires a new Captain decision.

## Boundary

- The direct-only OpenRouter forced-function route remains unchanged in
  principle: proxy-routed OpenRouter, Ollama, generic OAI-compatible
  endpoints, and every unprofiled provider remain structured-output
  unsupported.
- The canonical Baton report and S04 requested/emitted identity check remain
  the semantic authority. The receipt is separate from LLMCheckReport and
  never substitutes for canonical validation.
- Existing RunLLMCheck semantics remain unchanged outside the explicit native
  proof-receipt mode. MCP keeps its existing error/non-success control flow, but
  provider/model failures are rendered publicly as exactly
  `llm_check: provider request failed`; the underlying provider-derived text is
  never exposed.

## Native receipt lifecycle

The public command gains an explicit proof-receipt mode bound to the selected
check, model, release, slice, and immutable status.start_commit. It does not
accept a shell-redirection receipt convention.

1. Before any provider request, validate the fixed release, slice, check,
   model, and immutable-start identity and all existing receipt bindings.
   Refuse a mismatch in any one of those fields without consuming or reusing
   S22 retry budget, as well as a final prior verdict, opaque or untrusted
   historical record, exhausted two-attempt budget, or invalid receipt
   location.
2. Create a metadata-only reservation for the selected ordinal, fsync it,
   close it, atomically rename it to the private receipt path, then fsync the
   containing directory. A reservation failure causes zero provider dispatch.
3. Dispatch exactly once for that ordinal. Finalize the same receipt through
   the same durable atomic sequence. If finalization fails after rename, restore
   the reservation atomically. If restoration also fails, overwrite or surface
   only a durable receipt_failure/UNPARSEABLE record with unavailable exit
   semantics; never trust the renamed final model verdict.
4. Render only the strict receipt fields. Normal proof-mode output, generic
   JSON, errors, journals, and proof material must not expose endpoints,
   headers, request/response bodies, findings, raw errors, prompts, diffs,
   credentials, or keys.

The receipt schema allows only schema/version, release, slice ID, check type,
model ID, immutable start commit, attempt, class, result, and process-exit
semantics. It has no payload or explanatory text field by design.

## Retry decision table

| Attempt outcome | May dispatch attempt 2? | Meaning |
| --- | --- | --- |
| Locally validated PASS, FAIL, or BLOCKED | No | Final model verdict |
| rate_limit, upstream, transient, network, deadline, runner_failure, receipt_failure | Yes, only when this is attempt 1 | Explicit environmental/recovery class |
| HTTP 400, 401, 402, other unclassified HTTP/local error | No | Client or unknown failure |
| parse, schema, identity, malformed-tool, opaque, or untrusted binding | No | Never infer a verdict or retry eligibility |
| Any attempt-2 outcome | No | Surface two sanitized receipts; never dispatch a third request |

The proof classifier is narrow and must not reuse legacy broad transient
handling that treats an unknown error as retryable. Error-message text and raw
response content are not classifier inputs.

## Required deterministic evidence

- Built-command local fake-endpoint reachability for a valid PASS and stable
  final FAIL/BLOCKED semantics.
- Atomic reservation/finalization coverage, including a preflight fault with
  zero dispatch, a post-call receipt-write fault that has no inferred verdict,
  and a post-rename plus restoration double fault that cannot leave a trusted
  final verdict.
- Exact dispatch-count coverage for 429/5xx, normalized network/deadline,
  runner, and receipt failure; and no retry for 400/401/402, unknown,
  parse/schema/identity/malformed-tool, opaque, or untrusted outcomes.
- Mismatched release, slice, check, model, and immutable-start receipt
  bindings reject before dispatch and cannot consume or reuse the S22 retry
  budget.
- Leak-canary coverage over receipt files, stdout, stderr, generic JSON,
  MCP public errors, journals, and Git-visible artifacts. It must prove generic
  raw_response is not serialized, malformed generic output uses a stable
  non-raw diagnostic, and registered `sworn.llm_check` reachability emits
  exactly `llm_check: provider request failed` for provider/model errors.

## Acceptance trace

- AC-01/02/03/04/05/07/08 retain the already-delivered direct-only OpenRouter
  forced-tool route, canonical validation, binary reachability, endpoint
  isolation, and malformed-tool rejection tests without changing provider
  authority.
- AC-06 is implemented by the bound atomic reservation/finalization state
  machine, including the post-rename restoration double-fault invariant.
- AC-09/10 are implemented by an explicit typed retry classifier and a strict
  two-attempt state machine; error-message text and opaque output never decide
  retry eligibility.
- AC-11 is implemented at the receipt renderer, generic JSON renderer, and
  registered MCP adapter boundary with payload/key canaries.
- AC-12 is implemented as a zero-dispatch preflight that reads the declared S21
  authoritative status reference and checks slice/release identity, immutable
  start, verified/PASS state, non-empty verdict time, and fresh-context flag
  before the command may reserve or dispatch attempt 2.

## Release gate

S21-openai-structured-envelope is the immediately preceding serial T1 slice.
Before reservation or dispatch, S22 resolves the declared authoritative status
reference and mechanically proves S21's slice/release identity, immutable start
ed0badf68673f0af84834458f07be0792555484f, verified/PASS state, non-empty
verdict time, and `verifier_was_fresh_context: true`. Any absent, stale, or
mismatched fact produces zero provider dispatches and consumes no retry budget.

Only after that preserved S21 gate, a fresh Captain PROCEED and acknowledgement, the revised
implementation, deterministic evidence, full suite, vet, build, and proof
bundle may the command consider policy-authorized attempt 2. A resulting PASS
still requires a fresh artifact-only S22 verifier PASS before the separately
blocked S20 lifecycle can resume.
