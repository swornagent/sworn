# Captain review — S14-azure-driver
Date: 2026-07-09
Design commit: b857b19cbf2c041a6ce57ee4a116b96c91b4802f

## Pins

1. [mechanical] §3.config.go — **CRITICAL: config.go missing from status.json planned_files.**
   What I observed: design.md §3 correctly lists `internal/model/config.go` as a file to modify (add `case "azure":` in the `FromEnv()` key gate). But status.json `planned_files` only lists `["internal/model/azure.go", "internal/model/azure_test.go", "internal/model/provider.go"]` — config.go is absent. This is the same pattern that caused S12-google-driver R1 and S13-bedrock-driver to nearly ship broken: the `FromEnv()` key gate in config.go has no `case "azure":`, so `azure/*` falls through to the `default` branch reading `SWORN_AZURE_OPENAI_API_KEY`. If the user sets the canonical `AZURE_OPENAI_API_KEY` (as the spec instructs), the key gate returns empty and `FromEnv()` returns an error before `NewClient()` is ever reached. The design acknowledges this file but status.json doesn't track it.
   What to ask the implementer: Add `internal/model/config.go` to `planned_files` in status.json before transitioning to in_progress. Also add a `case "azure":` branch in the `FromEnv()` key gate that reads `AZURE_OPENAI_API_KEY` (canonical) with `SWORN_AZURE_OPENAI_API_KEY` as fallback, matching the Google pattern (`envOrAlias`). The spec says the user sets `AZURE_OPENAI_API_KEY`; the key gate must accept it.

2. [mechanical] §2.4 / spec Risks — **api-version default: spec AC says preview, spec Risk says GA — design doesn't acknowledge the contradiction.**
   What I observed: Spec AC #2 says `apiVersion defaults to "2024-12-01-preview"`. Spec Risks #1 says "The default should be the most recent GA version at implementation time; check Azure docs." These contradict — `2024-12-01-preview` is a preview, not GA. Design §2.4 picks `2024-12-01-preview` per the AC but doesn't mention the Risk's GA direction. The Risk's mitigation is binding direction, not advisory.
   What to ask the implementer: Check Azure docs for the most recent GA api-version at implementation time. If a GA version exists, use it as the default (and note the spec AC may need amendment via /replan-release). If `2024-12-01-preview` is genuinely the latest available, state that explicitly in the code comment so the contradiction is acknowledged, not silent.

3. [mechanical] §2.5 — **Chat() method is scope expansion beyond spec; no other native driver implements it.**
   What I observed: Design §2.5 adds a `Chat()` method to `AzureOAI`, saying it's "zero marginal cost" since it shares HTTP dispatch. The spec only requires `Verify()`. The `Verifier` interface (client.go) only has `Verify()`. The `Agent` interface (agent.go) requires `Chat()` — but none of the other native drivers (Bedrock, Google, Anthropic) implement `Chat()`. Only `OAI` does (because it's the OAI-compat driver the agent loop uses). Adding `Chat()` to AzureOAI implies it could serve as an `agent.Agent`, but the agent loop dispatches via `model.FromEnv()` → type-assert to `agent.Agent`, and Azure would need to be routed through the OAI-compat path for agent use (or explicitly supported). This is a scope expansion that could mislead future readers.
   What to ask the implementer: Either (a) drop `Chat()` from this slice — it's not in the spec ACs and no other native driver has it; or (b) if keeping it, add a comment explaining AzureOAI is not currently wired as an `agent.Agent` (the agent loop only type-asserts `OAI`), so `Chat()` is a convenience method awaiting future wiring. Don't silently imply AzureOAI can serve as the implementer model.

4. [mechanical] §3.provider.go — **ProviderConfig field naming: spec says `AzureAPIKey`, existing code has `AzureOpenAIKey`.**
   What I observed: Spec says "add `AzureEndpoint`, `AzureAPIKey`, `AzureAPIVersion` fields to `ProviderConfig`". The existing code (landed by S13) already has `AzureOpenAIKey` in `ProviderConfig`. Design §3 says "add `AzureEndpoint` and `AzureAPIVersion` fields" — correctly not mentioning `AzureAPIKey` because it already exists as `AzureOpenAIKey`. This is fine, but the spec's naming (`AzureAPIKey`) doesn't match the codebase (`AzureOpenAIKey`). The design should use the existing field name `AzureOpenAIKey` and note the spec naming divergence.
   What to ask the implementer: Use the existing `AzureOpenAIKey` field name in ProviderConfig (not `AzureAPIKey` as the spec says). Add `AzureEndpoint` and `AzureAPIVersion` as new fields. In `NewClient()`, pass `pcfg.AzureOpenAIKey` to `NewAzureOAI()`. No rename needed — just use what's there.

5. [memory-cited] §2.3 — **Decision 3 (NewProviderError with provider="azure") aligns with provider-error taxonomy memory.**
   What I observed: Design §2.3 says "reuse `NewProviderError` with provider='azure'" — same error taxonomy as OAI/Bedrock/Google/Anthropic. This is the correct pattern.
   What to ask the implementer: No action needed — confirmation only. The typed `*model.Error` with `Provider: "azure"` is the established pattern.
   Citation: [[project_provider_error_taxonomy]]

6. [memory-cited] §4 — **No new go.mod dep aligns with dep policy revision memory.**
   What I observed: Design §4 says "No new go.mod dependencies — Azure OAI uses the same `/chat/completions` JSON format; stdlib `net/http` + `encoding/json` are sufficient." This is correct for Azure OpenAI (which speaks the OAI wire format). The dep policy allows justified deps but doesn't require them when stdlib suffices.
   What to ask the implementer: No action needed — confirmation only. Azure OpenAI's wire format is identical to OAI's `/chat/completions`; no SDK needed.
   Citation: [[project_dep_policy]]

7. [mechanical] §3.provider.go — **`NewClient()` azure case currently returns error; design must replace the stub.**
   What I observed: provider.go line 162: `case "azure": return nil, fmt.Errorf("%w: azure driver lands in S14-azure-driver", ErrDriverNotRegistered)`. Design §3 says "register `azure/*` case in `NewClient()` calling `NewAzureOAI()`" — this is the correct replacement. The stub exists and is the right anchor.
   What to ask the implementer: Replace the `case "azure":` stub with `return NewAzureOAI(model, pcfg.AzureOpenAIKey, pcfg.AzureEndpoint, pcfg.AzureAPIVersion)`. Confirm `AzureEndpoint` and `AzureAPIVersion` are read from `AZURE_OPENAI_ENDPOINT` and `AZURE_OPENAI_API_VERSION` env vars in `ProviderConfigFromEnv()` and `swornProviderConfig()`.

## Summary

Pins: 7 total — 5 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: #1 (config.go missing from planned_files — same S12/S13 pattern; would block all azure/* routing through FromEnv)

## Smaller flags (not pins, worth one-line ack)

- Design §2.1 (standalone struct, not embedding *OAI) is the right call — Azure replaces URL, auth header, and URL construction logic. Embedding would create a misleading type relationship. No pin needed.
- Design §2.2 (endpoint normalisation: strip trailing slashes + prepend https://) addresses spec Risk #2 (endpoint format varies). Good.
- The spec's deferral of Azure AD/Entra ID auth (Rule 2 card in proof.md) is correctly acknowledged in design §4. Ensure the proof.md Rule 2 card has all three elements: Why (MSAL/azure-sdk-go dep), Tracking (post-R3 issue), Acknowledgement (planning session 2026-06-20).
- `ProviderConfigFromEnv()` reads `AZURE_OPENAI_API_KEY` (canonical) but `swornProviderConfig()` reads `SWORN_AZURE_OPENAI_API_KEY` (backward-compat). The design should add `AzureEndpoint` and `AzureAPIVersion` to both functions, reading from `AZURE_OPENAI_ENDPOINT`/`AZURE_OPENAI_API_VERSION` in `ProviderConfigFromEnv()` and `SWORN_AZURE_OPENAI_ENDPOINT`/`SWORN_AZURE_OPENAI_API_VERSION` in `swornProviderConfig()`. This dual-path is the established pattern (see Google/Bedrock).

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is sound and follows the established native-driver pattern. 7 pins + 4 flags:

1. **config.go missing from planned_files (CRITICAL).** Add `internal/model/config.go` to `planned_files` in status.json. Add `case "azure":` to the `FromEnv()` key gate in config.go reading `AZURE_OPENAI_API_KEY` (canonical) with `SWORN_AZURE_OPENAI_API_KEY` fallback via `envOrAlias`. Without this, `FromEnv()` blocks azure/* before `NewClient()` is reached — same bug as S12/S13.
2. **api-version default contradiction.** Spec AC says `2024-12-01-preview` but spec Risk #1 says "most recent GA version." Check Azure docs at implementation time. If a GA version exists, use it; if not, add a code comment acknowledging the preview is the latest available so the contradiction is documented, not silent.
3. **Chat() is scope expansion.** Drop `Chat()` from AzureOAI — spec only requires `Verify()`, and no other native driver (Bedrock/Google/Anthropic) implements `Chat()`. If keeping it, add a comment that AzureOAI is not wired as `agent.Agent` (the agent loop only type-asserts `OAI`).
4. **Use existing `AzureOpenAIKey` field name.** ProviderConfig already has `AzureOpenAIKey` (landed by S13). Don't rename to `AzureAPIKey` — use the existing field. Add `AzureEndpoint` and `AzureAPIVersion` as new fields to both `ProviderConfigFromEnv()` and `swornProviderConfig()`.
5. **Replace the azure stub in NewClient().** provider.go line 162 has the `case "azure":` stub returning `ErrDriverNotRegistered`. Replace with `return NewAzureOAI(model, pcfg.AzureOpenAIKey, pcfg.AzureEndpoint, pcfg.AzureAPIVersion)`.
6. **NewProviderError with provider="azure" — confirmed.** Decision 3 aligns with [[project_provider_error_taxonomy]]. No action needed.
7. **No new go.mod dep — confirmed.** Azure OAI uses the same `/chat/completions` JSON format; stdlib suffices. Aligns with [[project_dep_policy]]. No action needed.

Flags (not pins): (a) Endpoint normalisation (strip trailing slashes + prepend https://) correctly addresses spec Risk #2; (b) Ensure proof.md Rule 2 card for Entra ID deferral has all three elements (Why/Tracking/Acknowledgement); (c) Add `AzureEndpoint` + `AzureAPIVersion` to both `ProviderConfigFromEnv()` (canonical env vars) and `swornProviderConfig()` (SWORN_* backward-compat); (d) Standalone struct (not embedding *OAI) is the right call.

§2 decisions 1–5 ack (standalone struct, endpoint normalisation, NewProviderError, api-version default, Chat() — see pin 3). §6 questions: none. Memory citations: [[project_provider_error_taxonomy]], [[project_dep_policy]].

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are apply-inline corrections (config.go planned_files, api-version check, Chat() scope, field naming, stub replacement); no design re-review needed before code.
-->