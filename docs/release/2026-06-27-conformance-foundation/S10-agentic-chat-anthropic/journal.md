# S10-agentic-chat-anthropic — Journal

## 2026-07-24 — Session start (implementer)

Entered implementer role. Slice is currently `planned`. Track T2-model-layer, preceded by S08 (verified) and S09 (verified). No blockers.

## 2026-07-24 — Session end (implemented)

State transition: `in_progress` → `implemented`.

### Decisions and trade-offs

1. **Pricing map consolidation**: Created `internal/model/pricing.go` with a unified `Pricing` map containing both Anthropic and OpenAI model prices. The OAI `modelPricing` map in `oai.go` is left in place (used by existing `computeCost`) to avoid touching OAI code in this slice. Future cleanup can unify.

2. **ChatResponse.CostUSD**: Added `CostUSD float64` to the shared `ChatResponse` struct rather than creating an Anthropic-specific wrapper. This keeps the interface uniform — OAI Chat() can populate it later when needed. Currently only Anthropic Chat() sets it.

3. **UsageBlock.InputTokens/OutputTokens**: Added provider-agnostic token count fields to `UsageBlock`. Anthropic SDK natively uses `input_tokens`/`output_tokens`; OAI uses `prompt_tokens`/`completion_tokens`. Both field sets coexist on the struct.

4. **cliDriver Chat() message-stacking**: Messages are collapsed as `[system]\n\nUser: <content>\n\nAssistant: <content>...`. The `--no-session-persistence` flag is preserved. Tools are accepted for interface compatibility but ignored. Multi-turn tool calls remain a formal deferral.

5. **Anthropic Chat() system messages**: System messages are extracted from the `[]ChatMessage` slice and passed via `MessageNewParams.System` (the Anthropic API's canonical location). User/assistant messages go to `Messages`.

### Test coverage

- `TestAnthropicChat_ReturnsTextBlock`: 2-message user+assistant history, verifies content extraction, InputTokens>0, CostUSD>0
- `TestAnthropicChat_SystemMessage`: system+user message history, verifies system handling
- `TestAnthropicChat_CostCalculation`: precise cost formula validation (1M input, 500k output = $10.50 for Sonnet 4.6)
- `TestPricing_Sonnet4_6`, `TestPricing_Haiku4_5`, `TestPricing_UnknownModelReturnsZero`, `TestPricing_ComputeCost`, `TestPricing_AllKnownModelsHavePositivePrices`: pricing map spot checks
- All 76 model-package tests pass; `go vet` clean; `gofmt` clean

### Subagent dispatches

None — entirely in-session.
## Verifier verdicts received

### 2026-07-24 — Verifier session (fresh context, artefact-only)

**PASS**

Slice: `S10-agentic-chat-anthropic`
Verified against: `3620be5ad15fa2ae4b3a641f278de708a9e4f50c`
Verifier session: fresh, artefact-only

All seven gates passed:

- **Gate 1 (User-reachable outcome)**: Entry point `sworn run --implementer-model anthropic/claude-sonnet-4-6` resolves to the Anthropic driver; `run.go:349-358` checks `CapChat` (now `CapVerify|CapChat`); agent loop at `agent.go:97` calls `a.Chat(ctx, history, tools)`.
- **Gate 2 (Planned touchpoints)**: Three planned files changed (`anthropic.go`, `cli.go`, `pricing.go`); additional files (`oai.go`, `registry.go`, `capabilities_test.go`, `pricing_test.go`, `anthropic_test.go`) are natural consequences documented in proof.md "Divergence from plan".
- **Gate 3 (Required tests)**: `TestAnthropicChat` (3 subtests) and `TestPricing` (5 subtests) all pass; full model package (76 tests) green; `go vet` clean.
- **Gate 3b (AC satisfaction)**: LLM check script not available — non-blocking. Manual AC review confirms all 7 acceptance checks satisfied with concrete evidence.
- **Gate 4 (Reachability artefact)**: `go test ./internal/model/... -v -run TestAnthropicChat` exits 0 — integration-point test exercises full Chat() path through httptest server.
- **Gate 4b (Semantic coverage)**: LLM check not configured — non-blocking.
- **Gate 5 (No silent deferrals)**: 8 grep hits in changed Go files; all are either (a) documented in proof.md "Not delivered" with Rule 2 elements, (b) explicit error returns (not silent), or (c) doc drift (stale "Chat is deferred" comment on `cliDriver.Capabilities()` where code correctly returns `CapVerify|CapChat` — minor, non-blocking).
- **Gate 6 (Design conformance)**: Non-UI project (no `design-fidelity.json`) — auto-pass.
- **Gate 7 (Claimed scope)**: All 7 delivered items have evidence references pointing to real, working state.
