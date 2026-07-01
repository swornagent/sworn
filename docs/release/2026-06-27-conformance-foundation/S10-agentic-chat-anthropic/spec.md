---
title: 'S10 — Agentic Chat for native Anthropic driver + keyless claude-cli + cost fix'
description: 'Add Chat() to the Anthropic driver using the Messages API multi-turn path; add Chat() to the keyless CLI driver using message-stacking; fix cost=0 in Chat dispatches by wiring actual token counts to the pricing map.'
---

# Slice: `S10-agentic-chat-anthropic`

## User outcome

An implementer configured with an Anthropic API key (or a keyless subscription via `claude -p`) can run the agentic implementer role via the Chat path, with a correct USD cost populated in the Dispatch record rather than always 0.

## Entry point

`sworn run --implementer-model anthropic/claude-sonnet-4-6` — the model package resolves to the Anthropic driver, capability check (S08) passes (after this slice adds CapChat), and the implementer dispatches via `Chat()`.

## In scope

- `internal/model/anthropic.go`: add `Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error)` using the Anthropic SDK `Messages.New()` with the messages array mapped to `[]anthropic.MessageParam`
- Update `internal/model/anthropic.go` `Capabilities()` method (added in S08) to return `CapVerify | CapChat`
- Cost fix: Anthropic `Chat()` and `Verify()` must populate `cost_usd` from actual `InputTokens * inputPricePerToken + OutputTokens * outputPricePerToken` using the pricing map in `internal/model/` (or a new `internal/model/pricing.go`); no longer always 0
- `internal/model/cli.go`: add `Chat()` that stacks the message history as a single concatenated prompt (the `claude -p` binary does not natively support multi-turn tool calls, so full multi-turn Chat is a formal deferral); the Chat() method calls the existing subprocess path with the messages collapsed to `[system][turn1][turn2]...` as plaintext; acceptable for single-agent non-tool-use patterns
- `cli.go` `Capabilities()` update: return `CapVerify | CapChat` after adding Chat()
- Pricing map: `internal/model/pricing.go` (new) — static map of `modelID → {InputPricePerMillionTokens, OutputPricePerMillionTokens}`; seeded with Claude Sonnet 4.6 and Claude Haiku 4.5 rates; other models default to 0 (not estimated)

## Out of scope

- Tool-use/function-calling support in the Anthropic Chat driver (Anthropic tool-use is a different API shape; full tool calling is a later slice)
- Real multi-turn tool-loop for the CLI driver (the stacked-prompt approach is sufficient for the implementer's linear-conversation pattern; interactive tool calls are deferred)
- The full Dispatch enrichment (duration, token split in status.json) — that is S24; this slice only fixes the in-memory cost calculation on the Chat() return value

## Planned touchpoints

- `internal/model/anthropic.go` (add Chat(), fix cost in Verify() + Chat())
- `internal/model/cli.go` (add Chat() stub)
- `internal/model/pricing.go` (new — static pricing map)

## Acceptance checks

- [ ] `(*Anthropic).Chat()` compiles and satisfies the same `Chat(ctx, []ChatMessage, []ToolDef) (*ChatResponse, error)` signature as `(*OAI).Chat()`
- [ ] WHEN `(*Anthropic).Chat()` is called with a 2-message history (user + assistant turn), THE SYSTEM SHALL map them to `[]anthropic.MessageParam` and pass to `Messages.New()`
- [ ] WHEN `(*Anthropic).Chat()` succeeds, the returned `ChatResponse` has `Usage.InputTokens > 0` (from the SDK response) and `CostUSD` calculated as `inputTokens * inputPrice + outputTokens * outputPrice` using the pricing map (not 0)
- [ ] WHEN `(*Anthropic).Verify()` is called, `cost_usd` in the returned cost float is computed from actual token counts (not always 0)
- [ ] `(*cliDriver).Chat()` compiles and invokes the claude subprocess with the messages collapsed to a single prompt; it returns a ChatResponse with the subprocess stdout as content
- [ ] `Anthropic.Capabilities()` returns a value with `CapChat` set after S10
- [ ] `pricing_test.go`: Sonnet 4.6 and Haiku 4.5 model IDs return non-zero input and output prices; unknown model IDs return 0

## Required tests

- **Unit**: `internal/model/pricing_test.go` (new) — pricing map spot checks
- **Unit**: `internal/model/anthropic_test.go` — add Chat() round-trip test (can mock the SDK or use test-only client)
- **Reachability artefact**: `go test ./internal/model/... -v -run TestAnthropicChat` exits 0; `go test ./internal/model/... -v -run TestPricing` exits 0

## Risks

- The Anthropic SDK's `MessageParam` type structure may differ from the flat `ChatMessage` struct — the implementer must map carefully, especially for the `role` field (`"user"` vs `"human"` — Anthropic API uses `"user"`/`"assistant"`)
- CLI Chat stacking is a lossy approach for long conversations; deferred to a formal ADR if the context window limit is hit in practice

## Deferrals allowed?

Yes — keyless CLI Chat can be deferred if the subprocess stacking approach turns out to be incompatible with the implementer's tool-call pattern. Rule 2: Why = claude -p does not natively support multi-turn tool-call history injection; Tracking = S10 status.json open_deferrals; Acknowledged = Brad, 2026-06-27.
