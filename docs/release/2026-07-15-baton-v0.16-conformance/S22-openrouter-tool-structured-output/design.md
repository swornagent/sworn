# S22 Design TL;DR — configured recovery attempt 3

## §1 User-visible change

A release operator can run the native S22 proof-receipt command in an explicit
`--configured-recovery` mode after immutable historical attempts 1 and 2. The
command accepts no `--model` value; it loads the operator's existing Sworn
configuration and resolves `verifier.model` through
`config.Load` plus `ResolveVerifierModel("", cfg)`. It validates the exact
historical receipt bindings, the fresh S21 gate, model configuration and
structured-output capability before reserving or dispatching exactly attempt
3. The strict v2 receipt records only the resolved model ID and the existing
metadata allowlist. Every attempt-3 result or error is terminal: there is no
fallback, provider/model switch, or attempt 4.

The direct-only OpenRouter forced-tool transport, local canonical validation,
S04 requested/emitted identity authority, ordinary `RunLLMCheck` behavior,
stable MCP diagnostic, and v1 attempts 1–2 remain unchanged. In particular,
AC-05 keeps endpoint overrides isolated to direct OpenRouter with zero fallback;
AC-07 keeps JSON `null` tool arguments fail-closed after one dispatch; and AC-08
keeps non-`function` tool calls fail-closed after one dispatch.

## §2 Design decisions

1. **Keep direct OpenRouter structured transport explicit and isolated
   (Type-2).** Only the existing direct `openrouter/` forced-function route may
   expose `StructuredToolCall`; proxy OpenRouter, Ollama, generic OAI-compatible
   endpoints, and unprofiled providers remain unsupported. This preserves the
   already-ratified transport boundary without extending provider authority.

2. **Keep proof evidence in the native atomic metadata-only receipt lifecycle
   (Type-1, Coach-ratified 2026-07-17).** Historical attempts 1 and 2 stay
   byte-for-byte v1 records. Reservation precedes dispatch; finalization uses
   the existing durable trust guard; receipt failures never infer a model
   verdict. The alternative shell-redirection, generic-report, and broad-retry
   designs remain rejected because they cannot preserve atomic proof identity
   or payload confidentiality.

3. **Authorize one config-only administrative recovery outside the typed retry
   classifier (Type-1, Coach-ratified 2026-07-18).** Attempt-2 `opaque` remains
   terminal and non-retryable under the unchanged classifier. Separately,
   configured-recovery-v2 may authorize only attempt 3 after the exact history,
   S21, Captain-review, deterministic-test, build, and proof gates pass. The
   selected model comes solely from the customer's current verifier config,
   with no CLI override, default, config mutation, or fallback. This applies
   `[[capability-based-model-selection-ratified]]`: capability is a floor;
   Sworn does not choose a provider/model for the customer.

The alternatives for Decision 3 were to leave S22 permanently blocked, retry
or silently replace GLM, or delete/reset the receipts. The Coach selected the
bounded config-only path because it preserves history and customer choice while
making the exception explicit and unrepeatable.

## §3 File plan

Configured-recovery semantic implementation is confined to:

- `cmd/sworn/llmcheck.go` — parse `--configured-recovery`, reject any non-empty
  `--model`, resolve the configured verifier model through the single config
  authority, enforce capability/preflight gates, and select terminal attempt 3.
- `cmd/sworn/llmcheck_test.go` — built-command coverage for config-only
  resolution, override/unconfigured/incapable/history/S21/attempt-4
  zero-dispatch rejection, terminal attempt 3, public-output leak canaries, and
  explicit preservation of AC-05 endpoint isolation plus AC-07/AC-08 malformed
  tool-call rejection.
- `internal/gate/llmcheck_receipt.go` — validate immutable v1 history, select
  the v2 attempt-3 schema/version, and preserve terminal exhaustion without
  changing the typed retry classifier.
- `internal/gate/llmcheck_receipt_test.go` — exact v1/v2 binding, history
  preservation, classifier separation, and terminal attempt-3 tests.

The Planner-owned
`docs/release/2026-07-15-baton-v0.16-conformance/S22-openrouter-tool-structured-output/llm-check-proof-receipt-v2.schema.json`
is the strict attempt-3 record authority and is consumed unchanged.

The remaining declared S22 touchpoints are preservation surfaces. Existing
tests must continue to prove the direct/proxy provider boundary, canonical
report validation, typed error classes, atomic double-fault distrust, generic
JSON sanitization, and exact MCP diagnostic. They are not implementation
targets unless deterministic evidence exposes a spec violation; any required
new path or ownership boundary stops for replanning.

Commit `d02899f6` is an unapproved implementation candidate already present in
the live tree. This design does not certify it. After Captain PROCEED and Coach
acknowledgement, the Implementer must assess that candidate against this file
and the current spec, repair only declared touchpoints, and leave certification
to the fresh Verifier.

## §4 Explicitly not doing

- No provider/model dispatch occurs before fresh Captain PROCEED, Coach
  acknowledgement, deterministic tests, full suite, vet, build, regenerated
  proof, and the exact AC-12 preflight all pass.
- No `--model` override, inferred default, hard-coded substitute, fallback,
  provider/model switch, config mutation, or attempt 4.
- No change to the retry classification of `opaque`, parse/schema/identity,
  malformed-tool, authentication/credits/client, unknown, or untrusted results.
- No changes to canonical Baton schemas/prompts, S04 validation, S21's OpenAI
  envelope, hosted-proxy routing, provider catalogue/pricing, real Codex or
  Claude homes, S20 source/evidence, or commercial policy.
- No raw config content/path, endpoint, header, request/response body, finding,
  prompt, diff, credential, key, or provider-derived error enters a receipt,
  normal output, journal, proof, test failure, or Git-visible artefact.

## §5 Reachability and acceptance evidence

- **AC-01–05, AC-07–08:** retain the existing direct OpenRouter construction,
  exact forced tool, built-command fake-endpoint reachability, endpoint
  isolation, malformed-tool guards, and canonical requested/emitted checks.
- **AC-06:** prove v1 attempts 1–2 are immutable, attempt 3 uses only v2,
  reservation precedes dispatch, binding faults dispatch zero calls, and the
  post-rename/restoration double fault cannot leave a trusted verdict.
- **AC-09–10:** table-drive unchanged typed classification separately from the
  administrative gate; exact immutable history plus all gates permits attempt
  3 once, while altered history, existing attempt 3, or any fourth invocation
  dispatches zero calls.
- **AC-11:** retain receipt/CLI/generic-JSON/MCP leak canaries and registered
  `sworn.llm_check` reachability with exactly
  `llm_check: provider request failed` for provider/model errors.
- **AC-12:** built-command tests prove `config.Load` plus
  `ResolveVerifierModel("", cfg)`, no model flag, no config mutation, structured
  capability, exact S21 evidence, and zero-dispatch rejection. Only after the
  full deterministic and proof gates pass may the native command make terminal
  attempt 3 against immutable start
  `a09b0e46df465862d00469d4aef2a997442b3d5b`. A resulting PASS still requires
  fresh artefact-only S22 verification before S20 may resume.

The reachability artefact remains the built `sworn llm-check` process against
local fake endpoints with synthetic keys. The credentialed configured recovery
is policy-governed evidence, not a substitute for deterministic reachability or
fresh verification.

## §6 Open questions

None. The Coach has already ratified the Type-1 configured-recovery decision,
and the Planner has reconciled R-05 with AC-06/09/10/12. Fresh Captain review
must now confirm this design before implementation or provider action resumes.
