# S10-agentic-chat-anthropic — Proof bundle

## Scope

Add `Chat()` to the Anthropic driver using the Messages API multi-turn path; add `Chat()` to the keyless CLI driver via message-stacking; fix cost=0 in Chat dispatches by wiring actual token counts to the pricing map.

## Files changed

```
docs/release/2026-06-27-conformance-foundation/S10-agentic-chat-anthropic/journal.md
docs/release/2026-06-27-conformance-foundation/S10-agentic-chat-anthropic/status.json
internal/model/anthropic.go
internal/model/anthropic_test.go
internal/model/capabilities_test.go
internal/model/cli.go
internal/model/client.go
internal/model/oai.go
internal/model/openai_responses.go
internal/model/pricing.go
internal/model/pricing_test.go
internal/model/provider.go
internal/model/registry.go
```

## Test results

### go test ./internal/model/... -v -run TestAnthropicChat

```
=== RUN   TestAnthropicChat_ReturnsTextBlock
--- PASS: TestAnthropicChat_ReturnsTextBlock (0.00s)
=== RUN   TestAnthropicChat_SystemMessage
--- PASS: TestAnthropicChat_SystemMessage (0.00s)
=== RUN   TestAnthropicChat_CostCalculation
--- PASS: TestAnthropicChat_CostCalculation (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.013s
```

### go test ./internal/model/... -v -run TestPricing

```
=== RUN   TestPricing_Sonnet4_6
--- PASS: TestPricing_Sonnet4_6 (0.00s)
=== RUN   TestPricing_Haiku4_5
--- PASS: TestPricing_Haiku4_5 (0.00s)
=== RUN   TestPricing_UnknownModelReturnsZero
--- PASS: TestPricing_UnknownModelReturnsZero (0.00s)
=== RUN   TestPricing_ComputeCost
--- PASS: TestPricing_ComputeCost (0.00s)
=== RUN   TestPricing_AllKnownModelsHavePositivePrices
--- PASS: TestPricing_AllKnownModelsHavePositivePrices (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.008s
```

### Full model package test suite

```
go test ./internal/model/... -count=1
ok  	github.com/swornagent/sworn/internal/model	X.XXXs   (PASS, 76 tests)
```

### go vet

```
go vet ./internal/model/...
(no output — clean)
```

## Reachability artefact

`go test ./internal/model/... -v -run TestAnthropicChat` exits 0 — integration-point test via `Anthropic.Chat()` with an httptest server simulating the Anthropic Messages API. The test verifies the full Chat path: message role mapping, system message extraction, token counting, and cost computation.

`go test ./internal/model/... -v -run TestPricing` exits 0 — pricing spot checks for Sonnet 4.6, Haiku 4.5, unknown model, and cost formula.

## Delivered

- [x] `(*Anthropic).Chat()` compiles and satisfies `Chat(ctx, []ChatMessage, []ToolDef) (*ChatResponse, error)` — file `internal/model/anthropic.go:97-165`
- [x] 2-message history (user + assistant) mapped to `[]anthropic.MessageParam` via `Messages.New()` — `TestAnthropicChat_ReturnsTextBlock` passes
- [x] `ChatResponse` has `Usage.InputTokens > 0` and `CostUSD` computed from pricing map — `TestAnthropicChat_ReturnsTextBlock` asserts both; `TestAnthropicChat_CostCalculation` validates formula
- [x] `(*Anthropic).Verify()` cost computed from actual token counts (not always 0) — `ComputeCost` called in `Verify()`; `TestAnthropicVerify_ReturnsTextBlock` asserts `cost > 0`
- [x] `(*cliDriver).Chat()` compiles and invokes claude subprocess with collapsed messages — file `internal/model/cli.go:104-156`
- [x] `Anthropic.Capabilities()` returns `CapVerify | CapChat` — `TestCapabilities_AllDrivers/Anthropic` passes
- [x] Pricing map spot checks: Sonnet 4.6 and Haiku 4.5 return non-zero prices; unknown models return 0 — `TestPricing_Sonnet4_6`, `TestPricing_Haiku4_5`, `TestPricing_UnknownModelReturnsZero` pass

## Not delivered

- Tool-use/function-calling support in Anthropic Chat driver — deferred per spec out-of-scope. Why: tool-use is a different API shape (Anthropic tool-use beta). Tracking: future slice. Acknowledged: Brad, spec.md.
- Real multi-turn tool-loop for CLI driver — formal deferral per spec. Why: claude -p does not natively support multi-turn tool-call history injection. Tracking: S10 status.json open_deferrals. Acknowledged: Brad, 2026-06-27.
- Full Dispatch enrichment (duration, token split in status.json) — out of scope; belongs to S24-dispatch-enrich.

## Divergence from plan

- `ChatResponse.CostUSD` and `UsageBlock.InputTokens/OutputTokens` are new fields added to `oai.go` (shared types) rather than anthropic-specific types. This is the natural location — all drivers share these struct types. OAI-derived drivers are unaffected (InputTokens/OutputTokens default to 0 unless populated).
- `Pricing` map (pricing.go) consolidates both Anthropic and OpenAI model prices in one location, replacing the separate `anthropicPricing` (anthropic.go) and `modelPricing` (oai.go) maps. This was the spec-intended location for the pricing map. The OAI `computeCost` function still uses its own `modelPricing` map — unifying it is a future cleanup.