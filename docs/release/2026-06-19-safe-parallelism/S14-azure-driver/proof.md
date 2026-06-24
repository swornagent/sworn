---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S14-azure-driver`

## Scope

Implement an Azure OpenAI Service driver (`AzureOAI`) that dispatches verification calls to Azure OpenAI deployments using the `/chat/completions` endpoint with `api-key` auth. Register `azure/*` prefix in the provider router and add `AZURE_OPENAI_API_KEY` / `AZURE_OPENAI_ENDPOINT` / `AZURE_OPENAI_API_VERSION` env var reading.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/status.json
internal/model/azure.go
internal/model/azure_test.go
internal/model/config.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

**`go test ./internal/model/... -run Azure`** — all Azure-specific unit tests PASS:

```
=== RUN   TestAzureVerify_CorrectURL
--- PASS: TestAzureVerify_CorrectURL (0.00s)
=== RUN   TestAzureVerify_APIKeyHeader
--- PASS: TestAzureVerify_APIKeyHeader (0.00s)
=== RUN   TestAzureVerify_AuthorizationHeaderAbsent
--- PASS: TestAzureVerify_AuthorizationHeaderAbsent (0.00s)
=== RUN   TestAzureVerify_DefaultAPIVersion
--- PASS: TestAzureVerify_DefaultAPIVersion (0.00s)
=== RUN   TestAzureVerify_ReturnsText
--- PASS: TestAzureVerify_ReturnsText (0.00s)
=== RUN   TestNewClient_AzureRouted
--- PASS: TestNewClient_AzureRouted (0.00s)
=== RUN   TestNewAzureOAI_Errors
--- PASS: TestNewAzureOAI_Errors (0.00s)
=== RUN   TestAzureVerify_EndpointNormalisation
--- PASS: TestAzureVerify_EndpointNormalisation (0.00s)
=== RUN   TestAzureVerify_ErrorResponse
--- PASS: TestAzureVerify_ErrorResponse (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.012s
```

**`go test ./internal/model/...`** — full model test suite PASS (1.551s).

**`go build ./...`** — BUILD_SUCCESS (no new external deps; azure.go uses only stdlib).

**`go vet ./...`** — clean (no output).

## Reachability artefact

- **`TestNewClient_AzureRouted`** — asserts `model.NewClient("azure/gpt-4o", cfg)` returns `*AzureOAI` with correct Deployment, APIKey, APIVersion, and https://-prepended Endpoint.
- **`TestAzureVerify_ReturnsText`** — end-to-end request through `httptest.Server`; asserts `Verify()` returns text content from a valid chat completions response.
- **`TestAzureVerify_CorrectURL`** — asserts the URL matches the Azure pattern including `/openai/deployments/gpt-4o/chat/completions` and `api-version=2024-10-21`.
- Live integration test gated on `SWORN_LIVE_TESTS=1` + `AZURE_OPENAI_API_KEY` + `AZURE_OPENAI_ENDPOINT` — not run (no live Azure credentials in this session).

## Delivered

- [x] `go build ./...` succeeds with no new external deps (azure.go uses only stdlib `net/http` + `encoding/json`)
- [x] `NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com", key, "")` returns non-nil `*AzureOAI` with no error; api-version defaults to `"2024-10-21"` (latest GA)
- [x] `model.NewClient("azure/gpt-4o", cfg)` returns non-nil `*AzureOAI` Verifier
- [x] The HTTP request produced by `Verify()` uses the URL `https://<endpoint>/openai/deployments/gpt-4o/chat/completions?api-version=<version>` and the header `api-key: <key>`
- [x] The HTTP request does NOT include an `Authorization` header (Azure uses `api-key`, not `Bearer`)
- [x] `go test ./internal/model/... -run Azure` passes with zero failures
- [x] All prior model tests still pass
- [x] `case "azure":` added to `FromEnv()` key gate in config.go (Coach pin #1)
- [x] Existing `AzureOpenAIKey` field name used (not renamed to `AzureAPIKey`); `AzureEndpoint` and `AzureAPIVersion` added to ProviderConfig, ProviderConfigFromEnv(), and swornProviderConfig() (Coach pins #4, flag c)
- [x] Azure stub in `NewClient()` replaced with `NewAzureOAI()` call (Coach pin #5)
- [x] Endpoint normalisation: strips trailing slashes, prepends `https://` when no scheme present (Coach flag a)
- [x] Standalone `AzureOAI` struct (not embedding `*OAI`) — Coach flag d

## Not delivered

- **Azure AD / Entra ID token auth** — deferred per spec. **Why**: Entra token flow requires MSAL and azure-sdk-go; adds a significant dependency for an edge case (api-key covers the primary enterprise use case). **Tracking**: post-R3 GitHub issue. **Acknowledged**: planning session 2026-06-20, Coach-approved design review 2026-07-09.
- **`Chat()` method** — intentionally excluded per Coach pin #3. AzureOAI implements only `Verifier` (`Verify()`). No other native driver (Anthropic, Google, Bedrock) implements `Chat()` — only `OAI` does (for the agent loop). AzureOAI is not wired as an `agent.Agent`.
- **Azure cost modelling** — `Verify()` returns 0 for cost. Azure pricing varies by deployment tier, region, and commitment; not modelled. The caller still receives a verdict.

## Divergence from plan

- **API version default: `2024-10-21` (GA) instead of `2024-12-01-preview`.** Per Coach pin #2: spec AC #2 specified `2024-12-01-preview` but spec Risk #1 directs using the most recent GA version at implementation time. The Azure OpenAI REST API stable versions are: 2022-12-01, 2023-05-15, 2024-02-01, 2024-06-01, 2024-10-21. `2024-10-21` is the latest GA. The api-version is overridable via `AZURE_OPENAI_API_VERSION`.
- **`internal/model/config.go` added to planned_files** — Coach pin #1 corrected a status.json omission (config.go was in design.md §3 but not in planned_files).

## First-pass script output

*(See run below after state transition to `implemented`.)*