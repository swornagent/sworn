---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S14-azure-driver`

## Scope

Implement an Azure OpenAI Service driver (`AzureOAI`) that dispatches verification calls to Azure OpenAI deployments using the `/chat/completions` endpoint with `api-key` auth. Register `azure/*` prefix in the provider router and add `AZURE_OPENAI_API_KEY` / `AZURE_OPENAI_ENDPOINT` / `AZURE_OPENAI_API_VERSION` env var reading.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/proof.md
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/spec.md
docs/release/2026-06-19-safe-parallelism/S14-azure-driver/status.json
docs/release/2026-06-19-safe-parallelism/index.md
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

**`go test ./internal/model/...`** — full model test suite PASS (1.609s).

**`go build ./...`** — BUILD_SUCCESS (no new external deps; azure.go uses only stdlib).

**`go vet ./...`** — clean (no output).

**`gofmt -l`** on changed source files — clean (no output).

## Reachability artefact

- **`TestNewClient_AzureRouted`** — asserts `model.NewClient("azure/gpt-4o", cfg)` returns `*AzureOAI` with correct Deployment, APIKey, APIVersion, and https://-prepended Endpoint.
- **`TestAzureVerify_ReturnsText`** — end-to-end request through `httptest.Server`; asserts `Verify()` returns text content from a valid chat completions response.
- **`TestAzureVerify_CorrectURL`** — asserts the URL matches the Azure pattern including `/openai/deployments/gpt-4o/chat/completions` and `api-version=2024-12-01-preview`.
- Live integration test gated on `SWORN_LIVE_TESTS=1` + `AZURE_OPENAI_API_KEY` + `AZURE_OPENAI_ENDPOINT` — not run (no live Azure credentials in this session).

## Delivered

- [x] `go build ./...` succeeds with no new external deps (azure.go uses only stdlib `net/http` + `encoding/json`)
- [x] `NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com", key, "")` returns non-nil `*AzureOAI` with no error; api-version defaults to `"2024-12-01-preview"` (matches spec AC #2 exactly)
- [x] `model.NewClient("azure/gpt-4o", cfg)` returns non-nil `*AzureOAI` Verifier; registered via `case "azure":` in `NewClient()`
- [x] The HTTP request produced by `Verify()` uses the URL `https://<endpoint>/openai/deployments/gpt-4o/chat/completions?api-version=<version>` and the header `api-key: <key>`
- [x] The HTTP request does NOT include an `Authorization` header (Azure uses `api-key`, not `Bearer`)
- [x] `go test ./internal/model/... -run Azure` passes with zero failures (9 tests)
- [x] All prior model tests still pass (full suite 1.609s, zero regressions)
- [x] `case "azure":` added to `FromEnv()` key gate in config.go (reads `AZURE_OPENAI_API_KEY` canonical, `SWORN_AZURE_OPENAI_API_KEY` fallback)
- [x] `AzureAPIKey`, `AzureEndpoint`, and `AzureAPIVersion` fields added to `ProviderConfig`, `ProviderConfigFromEnv()`, and `swornProviderConfig()` — field name `AzureAPIKey` matches spec AC exactly
- [x] Azure stub in `NewClient()` replaced with `NewAzureOAI()` call using correct param order `(model, endpoint, apiKey, apiVersion)`
- [x] Endpoint normalisation: strips trailing slashes, prepends `https://` when no scheme present
- [x] Standalone `AzureOAI` struct (not embedding `*OAI`) — Azure replaces URL construction and auth header entirely
- [x] `gofmt -l` clean on all changed source files (formatting violations from prior round fixed)
- [x] Comment typo "environment// variables" fixed in provider.go → "environment variables"
- [x] spec.md "Planned touchpoints" updated to include `config.go` and `provider_test.go`

## Not delivered

- **Azure AD / Entra ID token auth** — deferred per spec. **Why**: Entra token flow requires MSAL and azure-sdk-go; adds a significant dependency for an edge case (api-key covers the primary enterprise use case). **Tracking**: post-R3 GitHub issue. **Acknowledged**: planning session 2026-06-20, Coach-approved design review 2026-07-09.
- **`Chat()` method** — intentionally excluded. AzureOAI implements only `Verifier` (`Verify()`). No other native driver (Anthropic, Google, Bedrock) implements `Chat()` — only `OAI` does (for the agent loop). AzureOAI is not wired as an `agent.Agent`.
- **Azure cost modelling** — `Verify()` returns 0 for cost. Azure pricing varies by deployment tier, region, and commitment; not modelled. The caller still receives a verdict. **Acknowledged**: Coach design review ack 2026-07-09.

## Divergence from plan

- spec.md "Planned touchpoints" updated to include `config.go` (FromEnv azure key gate) and `provider_test.go` (azure stub removal). These were legitimately touched in the original implementation but omitted from the spec; now recorded.
- All other divergences from prior verifier round (gofmt formatting, comment typo, indentation fusion) are resolved — this round is gofmt-clean and go-vet-clean.

## First-pass script output

```
release-verify.sh
  slice:       S14-azure-driver
  slice dir:   docs/release/2026-06-19-safe-parallelism/S14-azure-driver
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit e6c92d5
  PASS  11 file(s) changed vs diff base
  (first 20)
    docs/release/2026-06-19-safe-parallelism/S14-azure-driver/approved-ack.md
    docs/release/2026-06-19-safe-parallelism/S14-azure-driver/journal.md
    docs/release/2026-06-19-safe-parallelism/S14-azure-driver/proof.md
    docs/release/2026-06-19-safe-parallelism/S14-azure-driver/spec.md
    docs/release/2026-06-19-safe-parallelism/S14-azure-driver/status.json
    docs/release/2026-06-19-safe-parallelism/index.md
    internal/model/azure.go
    internal/model/azure_test.go
    internal/model/config.go
    internal/model/provider.go
    internal/model/provider_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~11) consistent with diff vs start_commit (11)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```
