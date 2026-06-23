---
title: Slice proof bundle
description: Rule 6 proof bundle. Generated from live repo state.
---

# Proof Bundle: `S11-anthropic-driver`

## Scope

Implement the native Anthropic Messages API driver (`anthropic/*`) using the official `anthropic-sdk-go`, so `sworn run` dispatches verification calls to Anthropic models.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/status.json
go.mod
go.sum
internal/model/anthropic.go
internal/model/anthropic_test.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### `go test ./internal/model/... -run Anthropic`

```
=== RUN   TestAnthropicVerify_ReturnsTextBlock
--- PASS: TestAnthropicVerify_ReturnsTextBlock (0.00s)
=== RUN   TestAnthropicVerify_MultiBlock
--- PASS: TestAnthropicVerify_MultiBlock (0.00s)
=== RUN   TestAnthropicVerify_APIError
--- PASS: TestAnthropicVerify_APIError (1.38s)
=== RUN   TestAnthropicNewClient_RoutedCorrectly
--- PASS: TestAnthropicNewClient_RoutedCorrectly (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	1.385s
```

### `go test ./internal/model/...` (all model tests — no regression)

```
ok  	github.com/swornagent/sworn/internal/model	1.595s
```

### `go build ./...`

```
BUILD OK
```

## Reachability artefact

- **Unit tests** (`go test ./internal/model/... -run Anthropic`): all four tests pass with a local `httptest`-based server, exercising the full `Verify()` path through the SDK.
- **Router test**: `TestAnthropicNewClient_RoutedCorrectly` confirms `model.NewClient("anthropic/claude-opus-4-8", cfg)` returns a non-nil `*Anthropic`.

## Delivered

- [x] `go mod tidy` with `github.com/anthropics/anthropic-sdk-go` in go.mod; `go build ./...` succeeds
- [x] `NewAnthropic("claude-sonnet-4-6", key)` returns non-nil `*Anthropic` with no error
- [x] `model.NewClient("anthropic/claude-sonnet-4-6", cfg)` returns a non-nil Verifier (router dispatches instead of returning `ErrDriverNotRegistered`)
- [x] `Verify()` with a test HTTP transport returns the text block from the first content item in the Anthropic response without error
- [x] Cost calculation returns a non-zero float for a response with non-zero token counts
- [x] `go test ./internal/model/... -run Anthropic` passes with zero failures (no live API key required)
- [x] `go test ./internal/model/...` (all model tests) still passes — no regression to OAI tests

## Not delivered

- Live integration test (`SWORN_LIVE_TESTS=1` with `ANTHROPIC_API_KEY`): not run — no API key available in this session. The test is structured as `t.Skip` when the env vars are absent, which the spec explicitly allows ("Deferrals allowed?" section: "The live integration test may be marked `t.Skip` when `ANTHROPIC_API_KEY` is absent — that is acceptable and is not a deferral.")

## Divergence from plan

- **SDK error extraction (Pin 3):** The Captain's pin assumed `anthropic.APIStatusError` with a `StatusCode` field. In practice, the SDK's HTTP error type is `*apierror.Error` in an internal package (`internal/apierror`), which cannot be imported. The implementation extracts the HTTP status code from the formatted error string instead, documented with a comment naming the internal type. This preserves the existing `ClassifyHTTP`/`NewProviderError` taxonomy without reflection or import hacks.

## First-pass script output

```
== First-pass verdict ==
  checks passed: 21
  checks failed: 1
FIRST-PASS FAIL
```

The single failure is `state is 'in_progress'` — expected, as this proof bundle is generated before the final state transition to `implemented`. Transitioning now.