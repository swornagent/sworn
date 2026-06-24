---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S14-azure-driver`

## Session log

### 2026-07-09 — Implementation (state: design_review → in_progress → implemented)

Coach approved design with 5 active pins + 4 flags. All pins addressed:

1. **config.go added to planned_files + FromEnv() key gate** — Added `case "azure":` to `FromEnv()` key gate reading `AZURE_OPENAI_API_KEY` (canonical) with `SWORN_AZURE_OPENAI_API_KEY` fallback via `envOrAlias`.
2. **api-version default: 2024-10-21 (GA)** — Checked Azure REST API specs on GitHub; latest stable version is 2024-10-21. Used this as default instead of 2024-12-01-preview. Code comment documents the GA choice and lists all stable versions.
3. **No Chat() method** — AzureOAI implements only `Verifier` (Verify()). No other native driver implements Chat().
4. **Used existing `AzureOpenAIKey` field name** — Added `AzureEndpoint` and `AzureAPIVersion` to ProviderConfig, ProviderConfigFromEnv(), and swornProviderConfig().
5. **Replaced azure stub** — `case "azure":` now calls `NewAzureOAI()` instead of returning `ErrDriverNotRegistered`.

Structural decisions:
- Standalone `AzureOAI` struct (not embedding *OAI) — Azure replaces URL construction and auth header entirely.
- Endpoint normalisation: strips trailing slashes, prepends `https://` when no scheme present.
- Error handling via `NewProviderError` with provider="azure" — same taxonomy as OAI/Anthropic/Google/Bedrock.
- Azure cost returns 0 (pricing varies by deployment tier, region, commitment; not modelled).
- Removed `azure/gpt-4o` from `TestNewClient_NativeStub` since the driver is now registered.

Tests: all 9 Azure-specific tests pass (CorrectURL, APIKeyHeader, AuthorizationHeaderAbsent, DefaultAPIVersion, ReturnsText, NewClient_AzureRouted, NewAzureOAI_Errors, EndpointNormalisation, ErrorResponse). Full model test suite passes (no regressions).

Skeptic panel: skipped — runtime does not support parallel subagent dispatch.

## Open questions

None.

## Deferrals surfaced

- Azure AD / Entra ID token auth — deferred per spec. Tracked: post-R3 GitHub issue. Acknowledged: planning session 2026-06-20.
- Azure cost modelling — returns 0. Tracked: S52-ledger-projection may add this. Acknowledged: Coach design review ack 2026-07-09.

## Verifier verdicts received

*(None yet — slice is implemented, awaiting fresh-context verification.)*
### 2026-07-09 — Verifier verdict — FAIL

FAIL

Slice: S14-azure-driver

Violations:
1. Gate 6 — Claimed scope matches implemented scope
   Evidence: spec.md AC #2: `NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com", key, "")` with default "2024-12-01-preview"; code in azure.go has different param order (deployment, apiKey, endpoint, apiVersion) and default "2024-10-21"; field names AzureAPIKey vs AzureOpenAIKey in provider.go/config.go
2. Gate 6 — "Delivered" list claims ACs satisfied but they are not literally (proof acknowledges divergences but does not update spec ACs)

Required to address:
1. Make implementation match spec ACs (param order, default api-version, field names) or update spec ACs to match implementation (latter requires planner if changing binding ACs).

Next step: re-open `/implement-slice S14-azure-driver 2026-06-19-safe-parallelism` in a fresh session to address the violations.

### 2026-07-09 — Re-implementation (state: failed_verification → in_progress → implemented)

Verifier FAIL violations addressed:

1. **Param order** — `NewAzureOAI(deployment, endpoint, apiKey, apiVersion)` now matches spec AC #2 exactly (`NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com", key, "")`).
2. **Default api-version** — Changed from `"2024-10-21"` (GA) back to `"2024-12-01-preview"` to match spec AC #2. The spec AC is binding; the risk section's GA-direction advice is advisory and doesn't override the literal AC.
3. **Field name** — Renamed `AzureOpenAIKey` → `AzureAPIKey` in `ProviderConfig`, `ProviderConfigFromEnv()`, and `swornProviderConfig()`. Matches spec "In scope" which says "add `AzureEndpoint`, `AzureAPIKey`, `AzureAPIVersion` fields to `ProviderConfig`". The pre-existing `AzureOpenAIKey` (from S13) is replaced — all references are within this slice's touchpoints.

All 9 Azure tests pass. Full model test suite passes (zero regressions). Build and vet clean.

Formatting issues: fused newlines (`}}` on one line, `case "oci":` fused onto prior line) were pre-existing from round 1; fixed in this round.

Skeptic panel: skipped — runtime does not support parallel subagent dispatch.