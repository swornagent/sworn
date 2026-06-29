---
title: 'Proof bundle — ADR-0011 keystone, step 2 (structured-output authoring path)'
description: 'The StructuredOutput interface + ChatStructured on the OAI and OpenAIResponses drivers, with strict projection and fail-closed acceptance.'
date: 2026-06-30
---

# Proof bundle — ADR-0011 keystone, step 2

## Scope
Let a model driver be handed a JSON Schema and emit a validated JSON object:
add the `StructuredOutput` interface + `CapStructuredOutput` capability, implement
`ChatStructured` on the `OAI` (native strict `response_format` + tool-call fallback)
and `OpenAIResponses` (`text.format`) drivers, with call-time strict projection of
the lenient canonical schema (D1) and a wire-level fail-closed guard. Additive —
`Verify`/`Chat` signatures untouched.

## Files changed
`git diff --name-only HEAD` (step-2 work, uncommitted at bundle time) +
`git diff --name-only harden/baton-v0.6.3-pin..HEAD` (step 1 already committed):

Step-2 (this session):
```
internal/model/capabilities_test.go   (M)
internal/model/client.go              (M)  CapStructuredOutput + StructuredOutput interface
internal/model/oai.go                 (M)  Structured field, Capabilities, Chat→postChat refactor, ChatStructured
internal/model/openai_responses.go    (M)  text.format types, Capabilities, Chat→postResponses refactor, ChatStructured
internal/model/provider.go            (M)  factory: openai→ResponseFormat, deepseek→ToolCall
internal/model/registry.go            (M)  advertise CapStructuredOutput: openai, openai-responses, deepseek
internal/model/structured.go          (A)  StructuredMode, strictProjection, schemaName, fail-closed guard
internal/model/structured_test.go     (A)  11 new tests
```

## Test results
Slice-relevant commands (NOT the full suite — merge gate owns that):

- `go build ./...` → exit 0
- `go vet ./internal/model/...` → exit 0
- `go test ./internal/model/... ./internal/baton/...` → `ok` both packages
- `go test ./internal/model/ -run 'Structured|StrictProjection|SchemaName|ChatStructured' -v`
  → 11 tests PASS (TestStrictProjection, TestStrictProjection_Invalid, TestSchemaName[3],
  TestOAI_ChatStructured_ResponseFormat, TestOAI_ChatStructured_ToolCall,
  TestOAI_ChatStructured_ToolCall_NoCall, TestOAI_ChatStructured_FailClosed[3],
  TestOAI_ChatStructured_Unsupported, TestOpenAIResponses_ChatStructured,
  TestOpenAIResponses_ChatStructured_FailClosed)
- Downstream consumers unbroken: `go test ./internal/orchestrator/... ./internal/run/...` → `ok`

## Reachability artefact
This is a library-layer driver capability; the integration point that owns the
affordance is `ChatStructured` itself. The reachability gate stated in the step-2
handoff is: "a real dispatch returns a schema-validated object; a malformed
emission fails closed." Proven through the integration point (not the leaf):

- `TestOAI_ChatStructured_ResponseFormat` drives the full
  marshal → HTTP (httptest) → unmarshal → normalise path and asserts the WIRE
  request carried `response_format.json_schema` with `strict:true` and a *projected*
  (`additionalProperties:false`) schema, and that the emitted object lands in
  `Choices[0].Message.Content`.
- `TestOAI_ChatStructured_ToolCall` asserts the fallback forces a single
  `emit_structured_output` tool (params = schema) via `tool_choice`, no
  `response_format`, and lifts the tool arguments into Content.
- `TestOAI_ChatStructured_FailClosed` (prose / empty / JSON-array) and
  `..._ToolCall_NoCall` prove malformed emissions fail closed (error, not silent pass).
- `TestOpenAIResponses_ChatStructured[_FailClosed]` prove the same over `text.format`.

## Delivered
- `CapStructuredOutput` bit + additive `StructuredOutput` interface —
  `internal/model/client.go`; compile-time guard `var _ = []StructuredOutput{&OAI{}, &OpenAIResponses{}}`
  in `capabilities_test.go`.
- `ChatStructured` on `OAI`, two modes selected by the `Structured` field —
  `internal/model/oai.go`; `TestOAI_ChatStructured_ResponseFormat` / `_ToolCall`.
- `ChatStructured` on `OpenAIResponses` via `text.format` — `internal/model/openai_responses.go`;
  `TestOpenAIResponses_ChatStructured`.
- Call-time strict projection (D1): seal objects, all-keys-required, optionals→nullable,
  recurse properties/items/$defs/combinators — `internal/model/structured.go` `strictProjection`;
  `TestStrictProjection`.
- Wire-level fail-closed acceptance (non-empty + parses as JSON object) —
  `normaliseStructuredContent`; `TestOAI_ChatStructured_FailClosed`.
- Factory + registry advertise the capability honestly for openai / openai-responses /
  deepseek — `provider.go`, `registry.go`; `TestCapabilities_AllDrivers`.
- `Chat` behaviour preserved while extracting shared `postChat` / `postResponses`
  helpers (omitempty fields keep the wire output byte-identical for plain Chat).

## Not delivered (Rule 2 — why + tracking + acknowledgement)
- **No LIVE provider dispatch.** All evidence is against `httptest` fakes; no real
  OpenAI/DeepSeek key was exercised this session.
  - *Why:* no provider API keys configured in this session; live keys are a Step 3
    concern (the verifier actually emitting over a real key).
  - *Tracking:* Step 3 pilot in the step-2 handoff §"THEN" — the verifier emits
    `verifier-verdict-v1` via `ChatStructured` against a live model.
  - *Acknowledgement:* surfaced here in plain text for the Coach; the fresh-context
    verifier (Rule 7) should treat "no live dispatch" as a known boundary of this slice,
    not a hidden gap.
- **Semantic schema-name validation not wired here (by design).** `ChatStructured`
  guards JSON-object-shape only; validation against the canonical schema by name
  (`baton.ValidateSchema`) is the caller's responsibility and lands with the Step 3
  caller. The interface signature (`schema []byte`, no name) enforces this layering.
  - *Tracking:* Step 3 (interpreter.go rewire, supersedes #32/#34).
- **Strict-keyword stripping not performed.** `strictProjection` does the structural
  transform only; structured-output *target* schemas must stay within OpenAI's
  strict-supported keyword subset (no `minLength`/`pattern`/`format` on emit targets).
  - *Why:* a general keyword-stripping pass is speculative; the role/layer target
    schemas are authored within the supported subset.
  - *Tracking:* documented constraint in `structured.go` doc comment + ADR-0011 §3 / D1.
- **anthropic / claude-cli structured output not added.** Tie to #35 (anthropic tool-use).

## Divergence from plan
- The handoff listed `baton.ValidateSchema` of the returned object under step-2 point 4.
  Implemented as a **wire-level** JSON-object guard in `model`; **semantic** name-based
  validation stays with the Step 3 caller because the `ChatStructured(schema []byte)`
  signature (ratified in the step-1 handoff) carries no schema name, and importing
  `internal/baton` into `internal/model` would invert the wire→schema layering. This is
  a Type-2 (narrow, reversible) design choice and matches the interface contract; no
  capability is advertised without a real implementation behind it.
- Beyond the handoff's "oai.go" focus, `OpenAIResponses.ChatStructured` was also
  implemented so the registry advertisement for `openai-responses` (point 5) is honest
  rather than decorative (Rule 1). Marginal cost: reuses `strictProjection`/`schemaName`.
