---
title: 'S11-anthropic-driver — native Anthropic Messages API driver'
description: 'Implements the model.Verifier interface using the Anthropic Messages API via the official anthropic-sdk-go. Registers the anthropic/* prefix in the provider router.'
---

# Slice: `S11-anthropic-driver`

## User outcome

A developer sets `ANTHROPIC_API_KEY` in `~/.sworn/.env` and `verifier.model =
"anthropic/claude-opus-4-8"` in config.json; `sworn run` dispatches verification calls
to the Anthropic Messages API and returns PASS/FAIL verdicts. Anthropic models are
available as both verifier and implementer.

## Entry point

`sworn run` model resolution path → `model.NewClient("anthropic/claude-opus-4-8", cfg)`
→ `*Anthropic` driver → `Verify()` call to Anthropic Messages API.

## In scope

- `internal/model/anthropic.go`:
  - `type Anthropic struct` with fields: `Client *anthropic.Client`, `Model string`
  - `NewAnthropic(modelID, apiKey string) (*Anthropic, error)` constructor
  - `Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error)`
    — calls `client.Messages.New(ctx, anthropic.MessageNewParams{...})` with the system
    prompt as a system message and userPayload as the first human turn; returns the first
    text block from the response
  - Cost calculation: `inputTokens * inputPricePerM / 1e6 + outputTokens * outputPricePerM / 1e6`
    using the known per-model pricing from `internal/model/oai.go`'s pricing table pattern
  - Supported model IDs: `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-haiku-4-5`,
    and any future `claude-*` (pattern match, not exhaustive list)
  - Max tokens: 8192 default (configurable via `MaxTokens` field)
- `internal/model/anthropic_test.go`:
  - Mock server or `anthropic.NewClient` with a test transport
- `internal/model/provider.go` update: register `anthropic/*` → `NewAnthropic()`
  instead of `ErrDriverNotRegistered`
- `go.mod`: add `github.com/anthropics/anthropic-sdk-go`

## Out of scope

- Streaming responses (sworn uses single-shot verify calls, not streaming chat)
- Tool use / function calling
- Extended thinking mode (future, if needed for harder specs)
- Bedrock-hosted Anthropic models (those route via S13-bedrock-driver)
- Vision / image inputs

## Planned touchpoints

- `internal/model/anthropic.go` (new)
- `internal/model/anthropic_test.go` (new)
- `internal/model/provider.go` (modify — register anthropic/* prefix)
- `go.mod`, `go.sum` (modify — add anthropic-sdk-go)

## Acceptance checks

- [ ] `go mod tidy` with `github.com/anthropics/anthropic-sdk-go` in go.mod; `go build
  ./...` succeeds
- [ ] `NewAnthropic("claude-sonnet-4-6", key)` returns non-nil `*Anthropic` with no error
- [ ] `model.NewClient("anthropic/claude-sonnet-4-6", cfg)` returns a non-nil Verifier
  (router now dispatches instead of returning `ErrDriverNotRegistered`)
- [ ] `Verify()` with a test HTTP transport returns the text block from the first content
  item in the Anthropic response without error
- [ ] Cost calculation returns a non-zero float for a response with non-zero token counts
- [ ] `go test ./internal/model/... -run Anthropic` passes with zero failures (no live
  API key required — use a local test transport)
- [ ] `go test ./internal/model/...` (all model tests) still passes — no regression to
  OAI tests

## Required tests

- **Unit** `internal/model/anthropic_test.go`:
  - `TestAnthropicVerify_ReturnsTextBlock`: mock response with one text content block;
    assert Verify returns that text and a non-negative cost
  - `TestAnthropicVerify_MultiBlock`: mock response with two content blocks; assert first
    text block returned (sworn uses first meaningful block)
  - `TestAnthropicVerify_APIError`: mock returns 429; assert Verify returns non-nil error
  - `TestAnthropicNewClient_RoutedCorrectly`: `model.NewClient("anthropic/claude-opus-4-8",
    cfg)` returns an `*Anthropic`, not an OAI client — type assertion in test
- **Reachability artefact**: live integration test (skipped in CI unless
  `ANTHROPIC_API_KEY` is set and `SWORN_LIVE_TESTS=1`): call `Verify()` with a simple
  "Reply with PASS." system prompt; assert the returned text contains "PASS".

## Risks

- `anthropic-sdk-go` is an official Anthropic library but may have breaking changes
  before 1.0. Pin to a specific minor version in go.mod.
- The Messages API has a different response structure from the OAI chat completions
  response. The implementer must not confuse the two; `anthropic.go` must import only the
  Anthropic SDK types, not any OAI types.

## Deferrals allowed?

No blocking deferrals. The live integration test may be marked `t.Skip` when
`ANTHROPIC_API_KEY` is absent — that is acceptable and is not a deferral.
