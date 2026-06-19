---
title: 'S14-azure-driver — Azure OpenAI Service driver'
description: 'Implements model.Verifier for Azure OpenAI deployments by extending the existing OAI HTTP client with Azure-specific auth (api-key header) and URL structure (endpoint + deployment + api-version). Registers azure/* prefix in the provider router. No new SDK dep.'
---

# Slice: `S14-azure-driver`

## User outcome

A developer sets `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_ENDPOINT`, and optionally
`AZURE_OPENAI_API_VERSION` in `~/.sworn/.env`, and sets
`verifier.model = "azure/gpt-4o"` in config.json; `sworn run` dispatches to their
Azure OpenAI deployment and returns a PASS/FAIL verdict.

## Entry point

`sworn run` → `model.NewClient("azure/gpt-4o", cfg)` → `*AzureOAI` driver (or extended
`OAI` struct) → `Verify()` using Azure endpoint URL and `api-key` header.

## In scope

- `internal/model/azure.go`:
  - Azure OpenAI uses the same `/chat/completions` request body as OpenAI; it differs in:
    1. URL structure: `https://<endpoint>/openai/deployments/<deployment>/chat/completions?api-version=<version>`
       where `deployment` = the model ID after `azure/` (e.g. `gpt-4o`)
    2. Auth header: `api-key: <key>` instead of `Authorization: Bearer <key>`
  - `type AzureOAI struct` — wraps `*OAI` with endpoint, deployment, and api-version
    fields; overrides `buildRequest()` or equivalent to construct the Azure URL and
    set the `api-key` header
  - `NewAzureOAI(deployment, endpoint, apiKey, apiVersion string) (*AzureOAI, error)`
    constructor; `apiVersion` defaults to `"2024-12-01-preview"` if empty
  - `Verify()` reuses the parent OAI HTTP logic; only the URL and auth header differ
  - Endpoint format: if `endpoint` does not include the scheme, prepend `https://`
- `internal/model/azure_test.go`
- `internal/model/provider.go` update: register `azure/*` → `NewAzureOAI()` using
  `cfg.AzureEndpoint`, `cfg.AzureAPIKey`, `cfg.AzureAPIVersion`
- `internal/model/provider.go`: add `AzureEndpoint`, `AzureAPIKey`, `AzureAPIVersion`
  fields to `ProviderConfig` (read from `AZURE_OPENAI_ENDPOINT`, `AZURE_OPENAI_API_KEY`,
  `AZURE_OPENAI_API_VERSION`)
- No new go.mod dep — Azure OAI uses the same `/chat/completions` JSON format; the
  existing `net/http` + `encoding/json` HTTP client is sufficient

## Out of scope

- Azure AD / Entra ID token auth (only api-key auth in this slice)
- Azure OpenAI assistants API
- Azure OpenAI embeddings
- Managed identity / workload identity auth
- DALL-E / image generation endpoints

## Planned touchpoints

- `internal/model/azure.go` (new)
- `internal/model/azure_test.go` (new)
- `internal/model/provider.go` (modify — register azure/* prefix, add Azure fields to
  ProviderConfig)

## Acceptance checks

- [ ] `go build ./...` succeeds with no new external deps (azure.go uses only stdlib)
- [ ] `NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com", key, "")` returns non-nil
  `*AzureOAI` with no error; api-version defaults to `"2024-12-01-preview"`
- [ ] `model.NewClient("azure/gpt-4o", cfg)` returns non-nil Verifier
- [ ] The HTTP request produced by `Verify()` uses the URL
  `https://<endpoint>/openai/deployments/gpt-4o/chat/completions?api-version=<version>`
  and the header `api-key: <key>` (verified via test server)
- [ ] The HTTP request does NOT include an `Authorization` header (Azure uses `api-key`,
  not `Bearer`)
- [ ] `go test ./internal/model/... -run Azure` passes with zero failures (no live key)
- [ ] All prior model tests still pass

## Required tests

- **Unit** `internal/model/azure_test.go` (uses an `httptest.Server` to capture requests):
  - `TestAzureVerify_CorrectURL`: assert the request URL matches the Azure pattern
    including deployment name and api-version query param
  - `TestAzureVerify_APIKeyHeader`: assert `api-key` header is set and `Authorization`
    header is absent
  - `TestAzureVerify_DefaultAPIVersion`: assert default `2024-12-01-preview` when version
    is empty
  - `TestAzureVerify_ReturnsText`: mock a valid chat completions response; assert
    Verify returns the content field text
  - `TestNewClient_AzureRouted`: `model.NewClient("azure/gpt-4o", cfg)` returns `*AzureOAI`
- **Reachability artefact**: live integration test (skipped unless `AZURE_OPENAI_API_KEY`
  and `AZURE_OPENAI_ENDPOINT` and `SWORN_LIVE_TESTS=1`): call Verify; assert text returned.

## Risks

- Azure OpenAI api-version changes frequently; `2024-12-01-preview` may be superseded.
  The default should be the most recent GA version at implementation time; check
  Azure docs. The default is overridable via `AZURE_OPENAI_API_VERSION`.
- Azure endpoint format varies: some tenants include trailing slashes or the path prefix;
  normalise in the constructor.

## Deferrals allowed?

Azure AD / Entra ID auth deferred; api-key auth covers the primary enterprise use case.
Document the deferral with Rule 2 card in proof.md: Why: Entra token flow requires
MSAL and azure-sdk-go; adds a significant dep for edge cases. Tracking: post-R3 issue.
Acknowledged: planning session 2026-06-20.
