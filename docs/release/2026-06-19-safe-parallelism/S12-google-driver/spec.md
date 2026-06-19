---
title: 'S12-google-driver — native Google Gemini and Vertex AI driver'
description: 'Implements model.Verifier for Google generative AI models via the official google.golang.org/genai SDK, supporting both Gemini API (AI Studio) and Vertex AI (GCP) backends. Registers google/* and vertex/* prefixes in the provider router.'
---

# Slice: `S12-google-driver`

## User outcome

A developer sets `GOOGLE_API_KEY` in `~/.sworn/.env` and `verifier.model =
"google/gemini-2.0-flash"` in config.json; `sworn run` dispatches to the Gemini API.
A GCP user sets `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`, and uses
`GOOGLE_APPLICATION_CREDENTIALS` (or ADC) with `verifier.model = "vertex/gemini-2.0-flash"`;
`sworn run` dispatches to Vertex AI instead.

## Entry point

`sworn run` → `model.NewClient("google/gemini-2.0-flash", cfg)` → `*Google` driver
→ `Verify()` call to Gemini API (or Vertex AI backend).

## In scope

- `internal/model/google.go`:
  - `type Google struct` with fields: `Client *genai.Client`, `Model string`
  - `NewGoogleGemini(modelID, apiKey string) (*Google, error)` — creates a Gemini API
    backend client with `genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})`
  - `NewGoogleVertex(modelID, project, location string) (*Google, error)` — creates a
    Vertex AI backend client with `genai.NewClient(ctx, &genai.ClientConfig{
    Backend: genai.BackendVertexAI, Project: project, Location: location})`;
    uses Application Default Credentials (no explicit key)
  - `Verify(ctx, systemPrompt, userPayload string) (string, float64, error)` — calls
    `client.Models.GenerateContent(ctx, modelID, genai.Text(userPayload),
    &genai.GenerateContentConfig{SystemInstruction: genai.NewUserContent(genai.Text(systemPrompt))})`;
    returns the first text part of the first candidate
  - Cost: uses `UsageMetadata.PromptTokenCount` + `CandidatesTokenCount` with known
    Gemini pricing (add to existing pricing table pattern)
- `internal/model/google_test.go`
- `internal/model/provider.go` update:
  - `google/*` → `NewGoogleGemini()` using `cfg.GoogleAPIKey`
  - `vertex/*` → `NewGoogleVertex()` using `cfg.GoogleCloudProject`,
    `cfg.GoogleCloudLocation` (read from `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`)
- `go.mod`: add `google.golang.org/genai`

## Out of scope

- Grounding / Google Search tool use
- Streaming (sworn uses single-shot calls)
- Image/audio/video inputs
- Model tuning or batch prediction
- Gemini Code Execution tool

## Planned touchpoints

- `internal/model/google.go` (new)
- `internal/model/google_test.go` (new)
- `internal/model/provider.go` (modify — register google/* and vertex/* prefixes)
- `go.mod`, `go.sum` (modify — add google.golang.org/genai)

## Acceptance checks

- [ ] `go build ./...` succeeds with `google.golang.org/genai` in go.mod
- [ ] `NewGoogleGemini("gemini-2.0-flash", key)` returns non-nil `*Google` with no error
- [ ] `model.NewClient("google/gemini-2.0-flash", cfg)` returns a non-nil Verifier
- [ ] `model.NewClient("vertex/gemini-2.0-flash", cfg)` returns a non-nil Verifier
- [ ] `Verify()` with a mock transport returns the first text part of the first candidate
- [ ] Cost calculation returns a non-negative float for non-zero token counts
- [ ] `go test ./internal/model/... -run Google` passes with zero failures (no live key)
- [ ] All prior model tests still pass (no regression)

## Required tests

- **Unit** `internal/model/google_test.go`:
  - `TestGoogleVerify_GeminiAPI`: mock GenerateContent response; assert Verify returns
    first candidate's first text part
  - `TestGoogleVerify_APIError`: mock error response; assert Verify returns non-nil error
  - `TestNewClient_GoogleRouted`: `model.NewClient("google/gemini-2.0-flash", cfg)` →
    type is `*Google`
  - `TestNewClient_VertexRouted`: `model.NewClient("vertex/gemini-2.0-flash", cfg)` →
    type is `*Google`
- **Reachability artefact**: live integration test (skipped unless `GOOGLE_API_KEY` and
  `SWORN_LIVE_TESTS=1`): call Verify with "Reply with PASS."; assert "PASS" in response.

## Risks

- `google.golang.org/genai` SDK is relatively new. The API for creating a Vertex client
  may differ from the Gemini API client despite sharing the package. Check the SDK docs
  for `genai.ClientConfig.Backend` enumeration at implementation time.
- ADC (Application Default Credentials) for Vertex requires `gcloud auth
  application-default login` in dev or a service account in CI. Document in proof.md
  that CI skips the live vertex test without `GOOGLE_CLOUD_PROJECT` set.

## Deferrals allowed?

Live Vertex AI test may be skipped in CI (requires GCP project). That is not a deferral;
it is documented conditional test skip.
