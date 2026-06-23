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

1. **Official SDK chosen.** The spec calls for `github.com/anthropics/anthropic-sdk-go`. This is justified by ADR-0007 (S10) which allows "minimal, justified deps". The SDK handles auth headers, JSON wire format, and error response parsing — none of which should be reimplemented from scratch. The dep is pinned to a specific minor version to guard against pre-1.0 breakage (per spec Risk).

2. **Pricing table mirroring OAI pattern.** Spec says to follow `internal/model/oai.go`'s pricing table pattern. I'll add an `anthropicPricing` map with `{inputPricePerM, outputPricePerM}` for the three named models (opus-4-8, sonnet-4-6, haiku-4-5). Unknown `claude-*` models get zero cost (same behaviour as OAI's unknown model path). Pricing sourced from Anthropic's public pricing page.

3. **Error handling via the existing `model.Error` taxonomy.** Rather than wrapping Anthropic SDK errors opaquely, HTTP-level errors from the SDK are classified through the existing `ClassifyHTTP`/`NewProviderError` path so caller retry logic (`IsTerminal`/`IsTransient`) works unchanged. This is the same pattern OAI uses.

4. **First text block only.** The Messages API response can contain multiple content blocks (text, tool_use, etc.). SwornAgent uses single-shot verify calls — there are no tools. I'll extract the first `ContentBlockTypeText` block. If none is present, return an error (same posture as OAI's empty-choices error).

5. **No `Chat` method.** The spec doesn't require it. The Anthropic driver only implements `Verify()`. If/when SwornAgent needs multi-turn agentic chat against Anthropic, that's a separate slice.

## §3. Files I'll touch grouped by purpose

- **New driver** — `internal/model/anthropic.go`: `Anthropic` struct, `NewAnthropic` constructor, `Verify` method, `computeAnthropicCost`, `anthropicPricing` table.
- **New tests** — `internal/model/anthropic_test.go`: four unit tests (text block, multi-block, API error, routing) plus the live integration test (skipped without `ANTHROPIC_API_KEY`).
- **Router registration** — `internal/model/provider.go`: add `case "anthropic":` in `NewClient` that calls `NewAnthropic(model, pcfg.AnthropicKey)`.
- **Dependencies** — `go.mod`, `go.sum`: add `github.com/anthropics/anthropic-sdk-go` with a pinned minor version.

## §4. Things I'm NOT doing

- **No Chat() method.** Only `Verify()`. The spec lists streaming, tool use, and extended thinking as out of scope — all of which would need `Chat()`.
- **No bedrock routing.** Bedrock-hosted Anthropic models route via `bedrock/*` (S13), not `anthropic/*`. This driver only handles direct Anthropic API access.
- **No default-client fallback for `ProviderConfig.AnthropicKey` emptiness.** If the key is empty, `NewClient` still constructs the driver — the API call will fail with an auth error, which the existing error taxonomy handles. This matches OAI's behaviour.

## §5. Reachability plan

- **Unit tests** (`go test ./internal/model/... -run Anthropic`) pass with a local `httptest`-based round-tripper injected into the SDK client. No API key needed.
- **Router test**: `model.NewClient("anthropic/claude-sonnet-4-6", cfg)` returns an `*Anthropic`, not `ErrDriverNotRegistered`. Type-assert in the test.
- **Live integration test** (skipped by default): set `ANTHROPIC_API_KEY` + `SWORN_LIVE_TESTS=1`, run `go test ./internal/model/... -run Anthropic`, verify the live driver returns a verdict from the real API.
- **End-to-end**: `go build ./...` succeeds with the new dep; `sworn run` with an Anthropic model ID dispatches to the driver (verified by the existing run-loop tests in `internal/run/`).

## §6. Open questions for the Coach

None.