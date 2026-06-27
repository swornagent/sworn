---
title: Design TL;DR — S11-anthropic-driver
description: Implementation plan for the native Anthropic Messages API driver
---

# Design TL;DR: S11-anthropic-driver

## §1. User-visible change

A developer sets `ANTHROPIC_API_KEY` in `~/.sworn/.env` and `verifier.model =
"anthropic/claude-sonnet-4-6"` in config.json; `sworn run` dispatches
verification calls to the Anthropic Messages API using the official
`anthropic-sdk-go`. The model returns PASS/FAIL verdicts with accurate per-token
costing. The `anthropic/*` prefix now resolves to a live driver instead of
`ErrDriverNotRegistered`.

## §2. Design decisions not in spec

1. **Official SDK chosen.** The spec calls for `github.com/anthropics/anthropic-sdk-go`. This is justified by ADR-0007 (S10) which allows "minimal, justified deps". The SDK handles auth headers, JSON wire format, and error response parsing — none of which should be reimplemented from scratch. Pinned to v1.51.1 (minor-version lock, per spec Risk 1). Pin 2 satisfied.

2. **Pricing table mirroring OAI pattern.** Spec says to follow `internal/model/oai.go`'s pricing table pattern. An `anthropicPricing` map with `{inputPricePerM, outputPricePerM}` for the three named models (opus-4-8: $15/$75, sonnet-4-6: $3/$15, haiku-4-5: $1/$5 per 1M tokens). Unknown `claude-*` models get zero cost (same behaviour as OAI's unknown model path). Pricing sourced from Anthropic's public pricing page (2026-06-23 snapshot).

3. **Error handling via the existing `model.Error` taxonomy.** HTTP-level errors from the SDK are classified through `NewProviderError` so caller retry logic (`IsTerminal`/`IsTransient`) works unchanged. The SDK's error type (`*apierror.Error`) is in an internal package and cannot be imported directly; the implementation extracts the HTTP status code from the formatted error string. Pin 3 satisfied — a comment names the internal type being unwrapped.

4. **First text block only.** The Messages API response can contain multiple content blocks (text, tool_use, etc.). SwornAgent uses single-shot verify calls — there are no tools. Extract the first `ContentBlockTypeText` block. If none is present, return an error (same posture as OAI's empty-choices error).

5. **No `Chat` method.** The spec doesn't require it. The Anthropic driver only implements `Verify()`. If/when SwornAgent needs multi-turn agentic chat against Anthropic, that's a separate slice.

## §3. Files I'll touch grouped by purpose

- **New driver** — `internal/model/anthropic.go`: `Anthropic` struct, `NewAnthropic` constructor, `Verify` method, `computeAnthropicCost`, `anthropicPricing` table, `anthropicStatusCode` error extractor. OAI-import segregation enforced (Pin 1): imports only `anthropic-sdk-go` types, never `internal/model/oai.go` or OAI struct types.
- **New tests** — `internal/model/anthropic_test.go`: four unit tests (text block, multi-block, API error with `KindRateLimit` assertion per Pin 4, routing) plus the live integration test (skipped without `ANTHROPIC_API_KEY`).
- **Router registration** — `internal/model/provider.go`: add `case "anthropic":` in `NewClient` that calls `NewAnthropic(model, pcfg.AnthropicKey)`.
- **Dependencies** — `go.mod`, `go.sum`: add `github.com/anthropics/anthropic-sdk-go` at v1.51.1.

## §4. Things I'm NOT doing

- **No Chat() method.** Only `Verify()`. The spec lists streaming, tool use, and extended thinking as out of scope.
- **No bedrock routing.** Bedrock-hosted Anthropic models route via `bedrock/*` (S13), not `anthropic/*`.
- **No default-client fallback for empty `AnthropicKey`.** `NewAnthropic` returns an error on empty key; `NewClient` still constructs the driver if the key is set.
- **No OAI import.** `anthropic.go` imports only Anthropic SDK types — never `internal/model/oai.go` or any OAI struct types (Pin 1 — satisfies spec Risk 2).

## §5. Reachability plan

- **Unit tests** (`go test ./internal/model/... -run Anthropic`): pass with a local `httptest`-based round-tripper injected into the SDK client. No API key needed. Includes `KindRateLimit` assertion on API error (Pin 4).
- **Router test**: `model.NewClient("anthropic/claude-sonnet-4-6", cfg)` returns an `*Anthropic`, not `ErrDriverNotRegistered`. Type-assert in the test.
- **Live integration test** (skipped by default): set `ANTHROPIC_API_KEY` + `SWORN_LIVE_TESTS=1`, run `go test ./internal/model/... -run Anthropic`, verify the live driver returns a verdict from the real API.
- **Build**: `go build ./...` succeeds with the new dep.

## §6. Open questions for the Coach

None.