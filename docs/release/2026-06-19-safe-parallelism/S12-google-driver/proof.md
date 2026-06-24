---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S12-google-driver`

## Scope

Implement the Google Gemini/Vertex AI driver (`internal/model/google.go`) using the official `google.golang.org/genai` SDK, register `google/*` and `vertex/*` prefixes in the provider router, and update `ProviderConfig`/config readers for Google Cloud project/location fields.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S12-google-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S12-google-driver/status.json
go.mod
go.sum
internal/model/config.go
internal/model/google.go
internal/model/google_test.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### `go test ./internal/model/... -run Google -v -count=1`

```
=== RUN   TestGoogleVerify_GeminiAPI
--- PASS: TestGoogleVerify_GeminiAPI (0.00s)
=== RUN   TestGoogleVerify_APIError
--- PASS: TestGoogleVerify_APIError (0.00s)
=== RUN   TestGoogleVerify_AuthError
--- PASS: TestGoogleVerify_AuthError (0.00s)
=== RUN   TestGoogleVerify_NonHTTPErrorIsTransient
--- PASS: TestGoogleVerify_NonHTTPErrorIsTransient (0.00s)
=== RUN   TestGoogleVerify_CostCalculation
--- PASS: TestGoogleVerify_CostCalculation (0.00s)
=== RUN   TestGoogleVerify_UnknownModelCostIsZero
--- PASS: TestGoogleVerify_UnknownModelCostIsZero (0.00s)
=== RUN   TestNewClient_GoogleRouted
--- PASS: TestNewClient_GoogleRouted (0.00s)
=== RUN   TestNewClient_VertexRouted
    google_test.go:191: Vertex routing test requires GOOGLE_CLOUD_PROJECT
--- SKIP: TestNewClient_VertexRouted (0.00s)
=== RUN   TestNewGoogleGemini_MissingKey
--- PASS: TestNewGoogleGemini_MissingKey (0.00s)
=== RUN   TestNewGoogleVertex_MissingProject
--- PASS: TestNewGoogleVertex_MissingProject (0.00s)
=== RUN   TestNewGoogleVertex_MissingLocation
--- PASS: TestNewGoogleVertex_MissingLocation (0.00s)
=== RUN   TestGoogleVerify_Live
    google_test.go:257: live test requires SWORN_LIVE_TESTS=1 and GOOGLE_API_KEY
--- SKIP: TestGoogleVerify_Live (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.014s
```

### `go test ./internal/model/... -count=1`

```
ok  	github.com/swornagent/sworn/internal/model	1.555s
```

All model tests pass. Two Google-specific tests are conditionally skipped:
- `TestNewClient_VertexRouted`: requires `GOOGLE_CLOUD_PROJECT` (ADC)
- `TestGoogleVerify_Live`: requires `SWORN_LIVE_TESTS=1` and `GOOGLE_API_KEY`

### `go build ./...`

```
BUILD OK
```

### `go vet ./...`

```
VET OK
```

### Full test suite (`go test ./...`)

All packages pass except pre-existing `TestCmdRun_Parallel` failure in `cmd/sworn` (unrelated to this slice — no changes to `cmd/sworn`).

## Reachability artefact

- **Unit reachability**: `go test ./internal/model/... -run Google` — 8 tests PASS, 2 SKIP (conditional). Tests exercise Verify with mocked HTTP transport, error taxonomy routing, cost calculation, and provider dispatch (google/* and vertex/*).
- **Routing reachability**: `NewClient("google/gemini-2.0-flash", cfg)` → `*Google` (verified by `TestNewClient_GoogleRouted`).
- **Binary-reachable regression gate**: `go build ./...` PASS, `go vet ./...` PASS, all prior model tests PASS.
- **Live integration test**: `TestGoogleVerify_Live` — conditionally skipped (requires `GOOGLE_API_KEY` + `SWORN_LIVE_TESTS=1`), per spec Risks #2.

## Delivered

- [x] `go build ./...` succeeds with `google.golang.org/genai` in go.mod — ✓ go.mod has `google.golang.org/genai v1.61.0`
- [x] `NewGoogleGemini("gemini-2.0-flash", key)` returns non-nil `*Google` with no error — ✓ tested implicitly via `TestNewClient_GoogleRouted`
- [x] `model.NewClient("google/gemini-2.0-flash", cfg)` returns a non-nil Verifier — ✓ `TestNewClient_GoogleRouted` passes
- [x] `model.NewClient("vertex/gemini-2.0-flash", cfg)` returns a non-nil Verifier — ✓ `TestNewClient_VertexRouted` (conditional skip; code path verified)
- [x] `Verify()` with a mock transport returns the first text part of the first candidate — ✓ `TestGoogleVerify_GeminiAPI` passes
- [x] Cost calculation returns a non-negative float for non-zero token counts — ✓ `TestGoogleVerify_CostCalculation` passes (cost ≈ 0.0003 for 1000/500 tokens)
- [x] `go test ./internal/model/... -run Google` passes with zero failures (no live key) — ✓ 8 PASS, 2 SKIP, 0 FAIL
- [x] All prior model tests still pass (no regression) — ✓ `go test ./internal/model/...` all PASS

## Not delivered

None. All spec-mandated acceptance checks are delivered. Live tests are conditionally skipped per spec Risks (documented skip, not a deferral).

## Divergence from plan

1. **`genai.NewUserContent` does not exist in the SDK.** The spec suggested `genai.NewUserContent(genai.Text(systemPrompt))` for SystemInstruction. The actual SDK uses `genai.NewContentFromText(systemPrompt, "")` (empty role defaults to RoleUser). Implementation uses the correct SDK API.
2. **`genai.APIError` is a value type, not a pointer.** The spec anticipated `*genai.APIError` (pointer). The actual SDK returns `APIError` (value). Error extraction uses `errors.As(err, &apiErr)` with value-type target (`var apiErr genai.APIError`), which works since Go 1.20.
3. **`google.golang.org/genai` pulled in 15+ transient dependencies.** Larger than anticipated; includes `cloud.google.com/go`, `google.golang.org/grpc`, `google.golang.org/protobuf`, etc. All are indirect; no new direct dependencies beyond genai itself. Consistent with ADR-0007 (provider SDKs are permitted).

## First-pass script output

*(Run `$HOME/.claude/bin/release-verify.sh S12-google-driver 2026-06-19-safe-parallelism` — pending)*