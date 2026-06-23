# Design TL;DR: `S12-google-driver`

## §1. User-visible change

A developer sets `GOOGLE_API_KEY` in `~/.sworn/.env` and `verifier.model = "google/gemini-2.0-flash"`; `sworn run` dispatches to the Gemini API. A GCP user sets `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`, and uses `GOOGLE_APPLICATION_CREDENTIALS` (or ADC) with `verifier.model = "vertex/gemini-2.0-flash"`; `sworn run` dispatches to Vertex AI instead. Both paths return the first text candidate's text, compute cost from token usage, and map errors through the existing model.Error taxonomy.

## §2. Design decisions not in spec (max 5)

1. **`google.golang.org/genai` SDK version** — will use the latest stable release; the SDK is the official Go SDK from Google. The version will be pinned in `go.mod` after `go get`.
2. **Vertex config fields on ProviderConfig** — `GoogleCloudProject` and `GoogleCloudLocation` are read from env vars and stored on `ProviderConfig`, consistent with how every other provider field works (no special-case env reading inside the driver).
3. **Error mapping** — the genai SDK returns typed API errors. We'll extract the HTTP status code from the SDK error and route through `NewProviderError`, identical to the Anthropic pattern. No string-matching needed.
4. **Gemini pricing** — prices sourced from Google's public pricing page at implementation time (Gemini 2.0 Flash, 2.5 Flash, 2.5 Pro). Unknown models return cost 0 (same posture as OAI and Anthropic).
5. **Constructor signatures match spec exactly** — `NewGoogleGemini(modelID, apiKey)` and `NewGoogleVertex(modelID, project, location)`, both returning `(*Google, error)`. The provider router calls the right constructor based on prefix.

## §3. Files I'll touch grouped by purpose

| Group | Files | Why |
|---|---|---|
| **New driver** | `internal/model/google.go` | The `Google` struct, constructors, `Verify()`, cost computation |
| **New tests** | `internal/model/google_test.go` | Mocked Gemini API response tests, routing tests, error taxonomy tests |
| **Provider routing** | `internal/model/provider.go` | Replace `google` case from `ErrDriverNotRegistered` to `NewGoogleGemini`; add `vertex` case → `NewGoogleVertex`; add `GoogleCloudProject`, `GoogleCloudLocation` to `ProviderConfig` and `ProviderConfigFromEnv` |
| **Dependencies** | `go.mod`, `go.sum` | Add `google.golang.org/genai` |

## §4. Things I'm NOT doing

- **Grounding / Google Search tool use** — out of scope per spec
- **Streaming** — sworn uses single-shot calls; nothing to do
- **Image/audio/video inputs** — single text-only Verify call
- **Live integration test** — skipped unless `GOOGLE_API_KEY` and `SWORN_LIVE_TESTS=1` (documented conditional skip per spec Risks)
- **Vertex live test** — skipped without `GOOGLE_CLOUD_PROJECT` (per spec Risks)
- **`vertex/*` prefix requires new `parseModelID` logic** — the `parseModelID` function splits on first `/`, so `vertex/gemini-2.0-flash` already parses correctly. No changes needed.
- **ProviderConfig changes for Vertex** — adding `GoogleCloudProject` and `GoogleCloudLocation` fields is the minimal delta; no other struct consumers need to change (they're additive fields with zero defaults).

## §5. Reachability plan

- **Unit test reachability** (no live key needed): `go test ./internal/model/... -run Google` — all tests pass with mocked HTTP transport
- **Routing reachability**: `model.NewClient("google/gemini-2.0-flash", cfg)` and `model.NewClient("vertex/gemini-2.0-flash", cfg)` both return `*Google` with no error
- **Binary-reachable regression gate**: `go build ./...` succeeds, `go test ./internal/model/...` (all model tests) passes — existing Anthropic and OAI tests must not break

## §6. Open questions for the Coach

*(None — the design follows the Anthropic pattern exactly; no ambiguities.)*