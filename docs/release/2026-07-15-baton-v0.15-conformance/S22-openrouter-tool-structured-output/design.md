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

## Boundary

- The direct-only OpenRouter forced-function route remains unchanged in
  principle: proxy-routed OpenRouter, Ollama, generic OAI-compatible
  endpoints, and every unprofiled provider remain structured-output
  unsupported.
- The canonical Baton report and S04 requested/emitted identity check remain
  the semantic authority. The receipt is separate from LLMCheckReport and
  never substitutes for canonical validation.
- Existing RunLLMCheck and MCP semantics remain unchanged outside the explicit
  native proof-receipt mode.

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
   the same durable atomic sequence. A finalization fault remains a durable
   receipt_failure and must not infer a model verdict.
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
  zero dispatch and a post-call receipt-write fault that has no inferred
  verdict.
- Exact dispatch-count coverage for 429/5xx, normalized network/deadline,
  runner, and receipt failure; and no retry for 400/401/402, unknown,
  parse/schema/identity/malformed-tool, opaque, or untrusted outcomes.
- Mismatched release, slice, check, model, and immutable-start receipt
  bindings reject before dispatch and cannot consume or reuse the S22 retry
  budget.
- Leak-canary coverage over receipt files, stdout, stderr, generic JSON,
  journals, and Git-visible artifacts. It must prove generic raw_response is
  not serialized and malformed generic output uses a stable non-raw
  diagnostic.

## Release gate

S21-openai-structured-envelope is the immediately preceding serial T1 slice.
Its authoritative T1 status is verified/PASS from immutable start
ed0badf68673f0af84834458f07be0792555484f. S22 must remain in design review
or later gated state if that upstream verified record is not preserved.

Only after that preserved S21 gate, a fresh Captain PROCEED and acknowledgement, the revised
implementation, deterministic evidence, full suite, vet, build, and proof
bundle may the command consider policy-authorized attempt 2. A resulting PASS
still requires a fresh artifact-only S22 verifier PASS before the separately
blocked S20 lifecycle can resume.
