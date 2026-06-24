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
