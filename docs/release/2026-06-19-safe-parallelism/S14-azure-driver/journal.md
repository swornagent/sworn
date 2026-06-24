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