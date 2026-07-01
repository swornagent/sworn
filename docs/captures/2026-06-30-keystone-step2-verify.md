---
title: 'Rule 7 verification — ADR-0011 keystone step 2 (structured-output authoring path)'
description: 'Fresh-context adversarial verdict on commit 4c8a6ad: StructuredOutput interface + ChatStructured on OAI/OpenAIResponses, strict projection, fail-closed wire guard.'
date: 2026-06-30
---

# Rule 7 verification — ADR-0011 keystone step 2

## Verdict

**PASS.** Every Delivered claim is satisfied against live repo state at HEAD == `4c8a6ad`.
Build, vet, and all four named test invocations pass; downstream consumers
(orchestrator, run) are unbroken. The interface, compile-time guard, both OAI
emission modes, the OpenAIResponses `text.format` path, `strictProjection`, the
wire-level fail-closed guard, and the honest factory/registry advertisement are
all present and exercised by tests that assert real behaviour (including
fail-closed errors, not silent passes). The "Not delivered" section is honest.

## Files-changed reconciliation

`git diff --name-only 4c8a6ad^..4c8a6ad`:

```
docs/captures/2026-06-30-keystone-step2-proof.md   (the artefact itself)
internal/model/capabilities_test.go
internal/model/client.go
internal/model/oai.go
internal/model/openai_responses.go
internal/model/provider.go
internal/model/registry.go
internal/model/structured.go
internal/model/structured_test.go
```

The bundle's "Step-2" list names exactly the 8 source/test files; the diff adds
only the proof bundle on top. No file claimed-but-missing; no file
changed-but-unclaimed. Git tree clean (no uncommitted drift). MATCH.

## Test results (regenerated live)

- `go build ./...` → exit 0
- `go vet ./internal/model/...` → exit 0
- `go test ./internal/model/... ./internal/baton/...` → `ok` model, `ok` baton (exit 0)
- `go test ./internal/model/ -run 'Structured|StrictProjection|SchemaName|ChatStructured' -v`
  → 10 top-level tests PASS (TestStrictProjection, TestStrictProjection_Invalid,
  TestSchemaName[3 sub], TestOAI_ChatStructured_ResponseFormat,
  TestOAI_ChatStructured_ToolCall, TestOAI_ChatStructured_ToolCall_NoCall,
  TestOAI_ChatStructured_FailClosed[3 sub], TestOAI_ChatStructured_Unsupported,
  TestOpenAIResponses_ChatStructured, TestOpenAIResponses_ChatStructured_FailClosed) — exit 0
- `go test ./internal/orchestrator/... ./internal/run/...` → `ok` both (exit 0)

(Bundle says "11 tests"; live shows 10 top-level funcs. The discrepancy is
cosmetic — TestSchemaName is one func with 3 subtests; the bundle counted by
listed function names. No missing or failing test.)

## Per-claim findings

1. **CapStructuredOutput + StructuredOutput interface + compile-time guard.**
   `client.go:23` defines `CapStructuredOutput`; `client.go:52-54` defines the
   `StructuredOutput` interface (`ChatStructured(ctx, []ChatMessage, []byte)`).
   `capabilities_test.go:94-97` is the compile-time guard
   `var _ = []StructuredOutput{&OAI{}, &OpenAIResponses{}}`. VERIFIED.

2. **ChatStructured on OAI — both modes, selected by `Structured` field.**
   `oai.go:316-367`: `StructuredResponseFormat` sets `response_format` with a
   strict-projected schema + `strict:true` (oai.go:320-332); `StructuredToolCall`
   forces a single `emit_structured_output` tool via `tool_choice` and lifts the
   tool arguments into Content (oai.go:333-359); zero mode returns an error
   (oai.go:342-343). VERIFIED, tested by `_ResponseFormat`, `_ToolCall`,
   `_Unsupported`.

3. **ChatStructured on OpenAIResponses via text.format.**
   `openai_responses.go:240-266` projects the schema and attaches it under
   `text.format` (`responsesTextFormat` with `strict:true`), then normalises the
   output text with the shared guard. VERIFIED, tested by
   `TestOpenAIResponses_ChatStructured`.

4. **strictProjection.** `structured.go:69-157`. Sets `additionalProperties:false`
   (line 112), lists all property keys in `required` deterministically
   (lines 95-111), widens non-required props to nullable via `makeNullable`
   (lines 100-102, 143-157), recurses properties/items/$defs/**definitions**
   (line 118)/anyOf/oneOf/allOf (lines 84-135). Adversarial gaps:
   - Array `items` is handled only in single-schema (map) form (line 115); tuple
     form (`items: [...]`) is not recursed. This is a benign pass-through (sub-schemas
     simply aren't sealed), not an unsound transform, and is outside the D1
     role/layer target-schema profile. Noted, not a violation.
   - `makeNullable` only widens nodes with a string/array `type`; bare `$ref`/enum-only
     nodes are left unchanged — explicitly documented as a known limitation
     (structured.go:138-142) and surfaced in the bundle's deferrals. Honest.
   VERIFIED against `TestStrictProjection` (root seal, all-required, optional→nullable,
   required-not-widened, nested-in-array-items sealed, nested plain object sealed).

5. **Wire-level fail-closed guard.** `normaliseStructuredContent`
   (structured.go:200-210) rejects empty content and any content that does not
   parse into a JSON object (`map[string]any`) — so a JSON array `[1,2,3]` fails
   (it won't unmarshal into a map). The tool-call path additionally errors when no
   tool call is returned (oai.go:355-357). Tests assert ERRORS (not silent pass):
   `TestOAI_ChatStructured_FailClosed` (structured_test.go:238 `err == nil → Fatal`),
   `_ToolCall_NoCall` (line 215), `TestOpenAIResponses_ChatStructured_FailClosed`
   (line 297). VERIFIED.

6. **Registry/factory honesty.** `registry.go` advertises CapStructuredOutput on
   exactly `openai` (l14), `openai-responses` (l15), `deepseek` (l23) — no other
   entry sets the bit. Factory wiring: openai→`StructuredResponseFormat`
   (provider.go:99), deepseek→`StructuredToolCall` (provider.go:107),
   openai-responses→`NewOpenAIResponses` (always advertises the cap,
   openai_responses.go:45-47). groq/mistral/openrouter/cloudflare/github build a
   bare `&OAI{}` (no Structured mode) so their instance `Capabilities()` returns
   `CapVerify|CapChat` only, matching their registry rows. No provider advertises
   the capability without a real implementation behind it. VERIFIED, consistent
   with `TestCapabilities_AllDrivers` (capabilities_test.go:16-19).

## Not-delivered honesty check

- **No live provider dispatch** — genuinely out of scope; all evidence is over
  `httptest`. Tied to Step 3. Honest boundary, not a hidden failure.
- **Semantic schema-name validation deferred to caller** — enforced by the
  interface signature `ChatStructured(schema []byte)` carrying no name; the
  wire-level guard is what's claimed and what's implemented. Honest layering, not
  a gap masquerading as a deferral.
- **Strict-keyword stripping not performed** — matches the code: `strictProjection`
  does the structural transform only and documents the keyword-subset constraint
  (structured.go:62-68). Honest.
- **No anthropic/claude-cli structured output** — registry does not advertise the
  cap for those, so no dishonest claim. Tied to #35. Honest.

All four are real Rule-2 deferrals (why + tracking + acknowledgement), not
disguised failures.

## Reachability

`TestOAI_ChatStructured_ResponseFormat` (structured_test.go:125-162) drives the
public integration point `OAI.ChatStructured` end-to-end through an `httptest`
server, captures the outbound `chatRequest`, and asserts the WIRE request carried
`response_format.type == "json_schema"`, `json_schema.strict == true`, a
*projected* schema (`additionalProperties == false`), the derived schema name
`verifier-verdict-v1`, and that the emitted object landed in
`Choices[0].Message.Content`. This drives the affordance (the method callers will
use), not a leaf helper. Reachability gate met.
