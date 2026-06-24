# Design TL;DR: `S16-ollama-driver`

## §1. User-visible change

A developer running Ollama locally sets `verifier.model = "ollama/llama3.2"` in
config.json; `sworn run` dispatches verification calls to Ollama's native
`POST /api/chat` endpoint at `http://localhost:11434` (or `$OLLAMA_HOST`). The
native endpoint eliminates the OAI-compat shim overhead and uses Ollama's own
`/api/chat` request/response format. The same `ollama/` model-ID prefix works
unchanged — the existing OAI-compat preset in `internal/model/provider.go` is
replaced with a native `*Ollama` driver that implements `Verifier` via stdlib
`net/http`.

## §2. Design decisions not in spec (max 5)

1. **No `OllamaConfig struct` — just two constructor params.** The spec says
   `NewOllama(modelID, host string) *Ollama`. That is one parameter fewer than the
   provider-config pattern used by Azure/Anthropic. Follow the spec literally:
   two params + empty-host → `$OLLAMA_HOST` fallback logic inside the constructor.
2. **`OllamaHost` field on `ProviderConfig` flips from OAI-compat base URL to raw
   host.** Currently `ProviderConfig OllamaHost` is the `/v1` OAI-compat base.
   The native driver will consume it as the raw host (no `/v1` append). The env
   var `OLLAMA_HOST` semantics change from "URL base for OAI compat" to "native
   endpoint host:port". This is the spec's stated intent — the OAI-compat shim was
   an internal preset, not a documented interface.
3. **No separate `ollamaHost()` helper — `ollamaHost()` already exists in
   provider.go and returns the host.** Reuse that function in `NewOllama` when the
   host param is empty — pass `pcfg.OllamaHost` from `NewClient`.
4. **Response parsing: decode into anonymous struct.** Same pattern as the OAI
   driver — `json.NewDecoder` into a struct with only the fields we need
   (`message.content`, `error`, `done`). Unknown fields silently ignored.
5. **Error on `"error"` field: use `fmt.Errorf` with the error string.** The
   Ollama response may contain `{"error": "model not found", ...}`. When present,
   return `fmt.Errorf("ollama: %s", ollamaErr.Error)` — not a `*model.Error`
   (unlike the Anthropic/Google drivers which map to provider error taxonomies),
   because Ollama is a local free tool with no rate limits, auth errors, or
   billing states to classify.

## §3. Files I'll touch grouped by purpose

- **New driver** — `internal/model/ollama.go`: `Ollama` struct, `NewOllama`,
  `Verify` (stdlib `net/http` + `encoding/json`, no new deps). Follows the Azure
  driver's standalone struct pattern (no OAI embedding).
- **Unit tests** — `internal/model/ollama_test.go`: all tests use `httptest.Server`
  for mocked `/api/chat`; plus a live integration test gated on
  `SWORN_LIVE_TESTS=1`.
- **Provider dispatch** — `internal/model/provider.go`: replace the `case "ollama"`
  block that constructs an `&OAI{}` with a call to `NewOllama(model,
  pcfg.OllamaHost)`.

## §4. Things I'm NOT doing

- Not touching the `ProviderConfig.OllamaHost` field name or its env-var reader —
  the field stays, only the consumer changes.
- Not adding Ollama-specific error taxonomy (`*model.Error` kinds) — Ollama is
  local/free, no auth/rate-limit/billing errors to classify.
- Not removing the `cloudflare` or `github` OAI-compat presets (those are
  different providers; the scope is *only* replacing the ollama preset).
- Not adding `keep_alive`, `options`, or any Ollama configuration beyond host
  — spec explicitly out of scope.

## §5. Reachability plan

- **Unit reachability**: `TestNewClient_OllamaIsNative` — calls
  `model.NewClient("ollama/llama3.2", cfg)` and type-asserts the return is
  `*Ollama`, not `*OAI`. This proves the dispatch path.
- **Live reachability**: `TestOllamaVerify_Live` — gated on
  `SWORN_LIVE_TESTS=1`, calls Verify against a real Ollama instance with
  "Reply with PASS." and asserts "PASS" in the response.

## §6. Open questions for the Coach

None.