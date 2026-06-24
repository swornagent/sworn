---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S12-google-driver`

## Session log

### 2026-07-08 — implementation

**Entry state:** `design_review` (Coach approved; Captain verdict PROCEED, 1 pin):
- Pin 1: add `internal/model/config.go` to `planned_files` — applied before transition.

**Implementation summary:**
- Created `internal/model/google.go`: `Google` struct implementing `Verifier`, with `NewGoogleGemini` (Gemini API, API key auth) and `NewGoogleVertex` (Vertex AI, ADC auth) constructors.
- Created `internal/model/google_test.go`: 10 test functions covering Verify with mock, API error (rate limit, auth), non-HTTP transient errors, cost calculation, unknown model cost=0, routing (google→*Google, vertex→*Google), missing-param guards, and a live integration test (skipped without GOOGLE_API_KEY + SWORN_LIVE_TESTS=1).
- Updated `internal/model/provider.go`: Added `GoogleCloudProject`, `GoogleCloudLocation` to `ProviderConfig`; wired `google/*` → `NewGoogleGemini` and `vertex/*` → `NewGoogleVertex` in `NewClient`; updated `ProviderConfigFromEnv` with `envOrAlias` for `GoogleKey` and new Cloud fields.
- Updated `internal/model/config.go`: Updated `swornProviderConfig` with `envOrAlias` for `GoogleKey` and Cloud fields; added vertex key-gate bypass (ADC, no API key) and google `envOrAlias` key check in `FromEnv`.
- Updated `internal/model/provider_test.go`: Removed `google/gemini-2.5-pro` from native stub test (google is now registered).
- `go.mod`/`go.sum`: Added `google.golang.org/genai` v1.61.0 (+ transient deps).

**Design decisions applied:**
1. **genai SDK error type** — `genai.APIError` is a value type (not pointer) with `.Code` (int) field. Used `errors.As(err, &apiErr)` with value-type target. Direct typed access — no string-parsing heuristic needed (unlike Anthropic driver).
2. **Vertex routing test** — skipped when `GOOGLE_CLOUD_PROJECT` is not set (ADC required for `genai.NewClient` with `BackendVertexAI`).
3. **Pricing** — sourced from `https://ai.google.dev/pricing` 2026-07-08 snapshot: 6 models (2.0 Flash, 2.0 Flash Lite, 2.5 Flash, 2.5 Flash Lite, 2.5 Flash Lite Preview, 2.5 Pro). Unknown models → cost 0.

**Test results:**
- `go test ./internal/model/... -run Google`: 10 PASS, 2 SKIP (live + vertex routing)
- `go test ./internal/model/...`: all model tests PASS
- `go build ./...`: PASS
- `go vet ./...`: PASS

**Open deferrals:** None.

**Pre-existing failure:** `TestCmdRun_Parallel` in `cmd/sworn` fails — unrelated to this slice (no changes to `cmd/sworn`).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.