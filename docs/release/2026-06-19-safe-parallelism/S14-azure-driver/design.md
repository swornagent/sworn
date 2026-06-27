# Design TL;DR: `S14-azure-driver`

## §1. User-visible change

A developer adds `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_ENDPOINT`, and optionally
`AZURE_OPENAI_API_VERSION` to `~/.sworn/.env` and sets `verifier.model =
"azure/gpt-4o"` in config.json. `sworn run` constructs an Azure-specific HTTP
client that sends `/chat/completions` requests to the Azure OpenAI endpoint
using `api-key` auth (not Bearer), and returns verdicts from the deployment.

## §2. Design decisions not in spec (max 5)

1. **Struct design — standalone `AzureOAI`, not embedding `*OAI`.** OAI embeds
   `BaseURL`, `Model`, `APIKey` and constructs URLs as `BaseURL+/chat/completions`.
   Azure replaces all three: URL is
   `https://<endpoint>/openai/deployments/<deployment>/chat/completions?api-version=<version>`
   and auth is `api-key` not `Bearer`. Embedding would require overriding three
   fields and two methods — a standalone struct with its own `Verify()` and
   `Chat()` is clearer and avoids the impression that `BaseURL` or `Authorization`
   are meaningful for Azure.
2. **Endpoint normalisation — strip trailing slashes and prepend `https://`
   only if missing.** The spec says "if endpoint does not include the scheme,
   prepend `https://`". I'll also strip trailing slashes so
   `myendpoint.openai.azure.com/` doesn't produce double slashes. The endpoint
   path `/openai/deployments/...` is hardcoded; users provide only the host.
3. **Error handling — reuse `NewProviderError` with provider="azure".** Same
   taxonomy as OAI: HTTP status → ErrorKind classification → typed `*model.Error`.
   Azure returns standard HTTP status codes and occasionally a JSON error body
   with `error.message`; `providerErrorMessage()` already handles that shape.
4. **api-version default — `"2024-12-01-preview"`** per spec. This is the
   most recent preview as of implementation time. Overridable via
   `AZURE_OPENAI_API_VERSION` env var.
5. **Chat() method included.** The spec focuses on `Verify()` but the OAI
   driver provides both `Verify()` and `Chat()`. I'll implement `Chat()` for
   AzureOAI following the same pattern to keep the interface surface consistent
   — this is zero marginal cost since it shares the same HTTP dispatch logic
   as `Verify()`.

## §3. Files I'll touch grouped by purpose

- **`internal/model/azure.go` (new)** — `AzureOAI` struct with `Endpoint`,
  `Deployment`, `APIKey`, `APIVersion`, `Client`; `NewAzureOAI()` constructor
  with endpoint normalisation and default api-version; `Verify()` and `Chat()`
  methods with Azure-specific URL construction and `api-key` auth header.
- **`internal/model/azure_test.go` (new)** — table-driven tests using
  `httptest.Server` to assert URL structure, `api-key` header presence,
  `Authorization` header absence, default api-version, and text response.
  Live test gated on `SWORN_LIVE_TESTS=1` + `AZURE_OPENAI_API_KEY` +
  `AZURE_OPENAI_ENDPOINT`.
- **`internal/model/provider.go` (modify)** — register `azure/*` case in
  `NewClient()` calling `NewAzureOAI()`; add `AzureEndpoint` and
  `AzureAPIVersion` fields to `ProviderConfig` and both `ProviderConfigFromEnv()`
  and `swornProviderConfig()`.
- **`internal/model/config.go` (modify)** — add `case "azure":` in `FromEnv()`
  key gate to read `AZURE_OPENAI_API_KEY` (canonical) with
  `SWORN_AZURE_OPENAI_API_KEY` fallback, matching the pattern used for Google.

## §4. Things I'm NOT doing

- Azure AD / Entra ID token auth (spec defers this; I'll document the deferral
  with a Rule 2 card in proof.md).
- Azure OpenAI assistants / embeddings / DALL-E endpoints.
- Managed identity / workload identity auth.
- Any new go.mod dependencies — Azure OAI uses the same `/chat/completions`
  JSON format; stdlib `net/http` + `encoding/json` are sufficient.

## §5. Reachability plan

- **Unit tests** via `httptest.Server` capture request URL + headers to prove
  Azure-specific URL structure and `api-key` auth without a live key.
- **Live integration test** (`TestAzureVerify_Live`) skipped unless
  `SWORN_LIVE_TESTS=1`, `AZURE_OPENAI_API_KEY`, and `AZURE_OPENAI_ENDPOINT`
  are all set; calls a real Azure deployment and asserts the response contains
  "PASS". This is the Rule 1 reachability artefact.

## §6. Open questions for the Coach

None.