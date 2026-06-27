# Design TL;DR: `S12-google-driver` (revised — addresses Captain review pins 1–7)

## §1. User-visible change

A developer sets `GOOGLE_API_KEY` (or `SWORN_GOOGLE_API_KEY`) in `~/.sworn/.env` and `verifier.model = "google/gemini-2.0-flash"`; `sworn run` dispatches to the Gemini API. A GCP user sets `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`, and uses `GOOGLE_APPLICATION_CREDENTIALS` (or ADC) with `verifier.model = "vertex/gemini-2.0-flash"`; `sworn run` dispatches to Vertex AI instead — no API key required (ADC handles auth). Both paths return the first text candidate's text, compute cost from token usage, and map errors through the existing model.Error taxonomy.

## §2. Design decisions not in spec (max 5)

1. **`google.golang.org/genai` SDK version** — will use the latest stable release; the SDK is the official Go SDK from Google. The version will be pinned in `go.mod` after `go get`. Pre-ratified by ADR-0007 (§"Provider SDKs"). The commit that adds the dep will carry a `Co-Authored-By:` trailer per ADR-0007.

2. **ProviderConfig and config.go changes** — `GoogleCloudProject` and `GoogleCloudLocation` fields are added to `ProviderConfig`, `ProviderConfigFromEnv()` (reads `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`), and `swornProviderConfig()` (reads env vars). `swornProviderConfig()`'s `GoogleKey` field uses `envOrAlias("SWORN_GOOGLE_API_KEY", "GOOGLE_API_KEY")` so both the SWORN_* backward-compat key and the canonical `GOOGLE_API_KEY` work — matching the spec's user-facing contract. The `FromEnv()` key-gate for the `google` provider uses the same `envOrAlias` pattern so either env var satisfies the key check.

3. **Error mapping — genai SDK error type verified at implementation time.** The Anthropic driver string-parses `err.Error()` for HTTP status codes (a brittle heuristic specific to anthropic-sdk-go). The genai SDK is a different package. At implementation time, the implementer will check the genai SDK's error type for a direct status field (e.g. `.HTTPStatusCode`, `.Code`) and use it directly if available, falling back to string-parse only if the SDK provides no typed accessor. This is explicitly called out in spec Risk #1.

4. **Gemini pricing** — prices sourced from Google's public pricing page at implementation time (Gemini 2.0 Flash, 2.5 Flash, 2.5 Pro). Unknown models return cost 0 (same posture as OAI and Anthropic). The `UsageMetadata` field names (`PromptTokenCount`, `CandidatesTokenCount`) will be verified against the actual SDK at implementation time per spec Risk #1.

5. **Constructor signatures** — `NewGoogleGemini(modelID, apiKey)` and `NewGoogleVertex(modelID, project, location)`, both returning `(*Google, error)`. The provider router calls the right constructor based on prefix. `NewGoogleGemini` requires a non-empty `apiKey`; `NewGoogleVertex` requires non-empty `project` and `location` but accepts no API key (ADC handles auth).

6. **Key-gate bypass for `vertex/*` (ADC, no API key).** `FromEnv()` line 73-76 currently requires `SWORN_<PREFIX>_API_KEY` for every provider before dispatching. For `vertex`, prefix=`VERTEX`, but Vertex AI uses Application Default Credentials — there is no API key. A bypass is added in `FromEnv()`: when `provider == "vertex"`, the key-gate is skipped (a placeholder `"adc"` is used so the empty check passes). This is scoped to `vertex` only — a general keyless-provider mechanism is deferred to the Ollama driver (S16), which has the same gap but is out of scope for S12.

## §3. Files I'll touch grouped by purpose

| Group | Files | Why |
|---|---|---|
| **New driver** | `internal/model/google.go` | The `Google` struct, constructors, `Verify()`, cost computation, error extraction |
| **New tests** | `internal/model/google_test.go` | Mocked Gemini API response tests, routing tests, error taxonomy tests |
| **Provider routing** | `internal/model/provider.go` | Replace `google` case from `ErrDriverNotRegistered` to `NewGoogleGemini`; add `vertex` case → `NewGoogleVertex`; add `GoogleCloudProject`, `GoogleCloudLocation` to `ProviderConfig` and `ProviderConfigFromEnv` |
| **Production dispatch** | `internal/model/config.go` | Add `GoogleCloudProject`/`GoogleCloudLocation` to `swornProviderConfig()`; add `envOrAlias` fallback for `GoogleKey`; add key-gate bypass for `vertex/*` in `FromEnv()`; add `envOrAlias` for `google` key-gate in `FromEnv()` |
| **Dependencies** | `go.mod`, `go.sum` | Add `google.golang.org/genai` |

## §4. Things I'm NOT doing

- **Grounding / Google Search tool use** — out of scope per spec
- **Streaming** — sworn uses single-shot calls; nothing to do
- **Image/audio/video inputs** — single text-only Verify call
- **Live integration test** — skipped unless `GOOGLE_API_KEY` and `SWORN_LIVE_TESTS=1` (documented conditional skip per spec Risks)
- **Vertex live test** — skipped without `GOOGLE_CLOUD_PROJECT` (per spec Risks)
- **General keyless-provider mechanism** — the vertex bypass is scoped to `vertex` only. A general mechanism for keyless providers (Ollama, future) is out of scope for S12.
- **Changing key-gate for providers other than google/vertex** — existing providers (OpenAI, Anthropic, etc.) are unchanged.
- **`parseModelID` for `vertex/*`** — already works correctly (splits on first `/`). No changes needed.
- **`design_decisions` array in `status.json`** — the spec does not require it and the Captain flag notes it as a recurring pattern, not a pin. The decisions here are all Type-2 (implementation details following an established pattern). Not adding to `status.json` in this slice.

## §5. Reachability plan

- **Unit test reachability** (no live key needed): `go test ./internal/model/... -run Google` — all tests pass with mocked HTTP transport. Verify genai SDK exposes an injectable HTTP client (e.g. `option.WithHTTPClient`) before implementing mock tests — if not, use `httptest.NewServer` and construct the client with the test server URL.
- **Routing reachability**: `model.NewClient("google/gemini-2.0-flash", cfg)` returns `*Google`; `model.NewClient("vertex/gemini-2.0-flash", cfg)` returns `*Google`
- **Production dispatch reachability**: `FromEnv("google/gemini-2.0-flash")` succeeds when `GOOGLE_API_KEY` (or `SWORN_GOOGLE_API_KEY`) is set; `FromEnv("vertex/gemini-2.0-flash")` succeeds when `GOOGLE_CLOUD_PROJECT` and `GOOGLE_CLOUD_LOCATION` are set (no API key needed)
- **Binary-reachable regression gate**: `go build ./...` succeeds, `go test ./internal/model/...` (all model tests) passes — existing Anthropic and OAI tests must not break

## §6. Open questions for the Coach

*(None — all Captain pins are addressed in the revised design.)*