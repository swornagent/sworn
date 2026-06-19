---
title: 'S16-ollama-driver — native Ollama API driver'
description: 'Implements model.Verifier using Ollama''s native /api/chat endpoint (not the OAI-compat shim). Replaces the existing OAI-compat ollama/* preset in the provider router with a proper Ollama driver. No new SDK dep — stdlib HTTP only.'
---

# Slice: `S16-ollama-driver`

## User outcome

A developer running Ollama locally sets `verifier.model = "ollama/llama3.2"` in
config.json; `sworn run` dispatches to the Ollama native API at `http://localhost:11434`
and returns a PASS/FAIL verdict. The native endpoint exposes Ollama-specific features
(model name format without version tags) and eliminates the OAI-compat shim overhead.

## Entry point

`sworn run` → `model.NewClient("ollama/llama3.2", cfg)` → `*Ollama` driver → `Verify()`
call to Ollama `POST /api/chat`.

## In scope

- `internal/model/ollama.go` (stdlib `net/http` + `encoding/json` only, no new dep):
  - Ollama `/api/chat` request format:
    ```json
    {"model": "llama3.2", "stream": false,
     "messages": [{"role": "system", "content": "..."}, {"role": "user", "content": "..."}]}
    ```
  - Response format: `{"message": {"role": "assistant", "content": "..."}, "done": true,
    "prompt_eval_count": N, "eval_count": N}`
  - `type Ollama struct` with fields: `Host string` (default `http://localhost:11434`),
    `Model string`, `Client *http.Client`
  - `NewOllama(modelID, host string) *Ollama` — host defaults to `http://localhost:11434`
    if empty; reads `$OLLAMA_HOST` if set
  - `Verify(ctx, systemPrompt, userPayload string) (string, float64, error)` — POSTs to
    `<host>/api/chat`; parses response; extracts `message.content`; cost is 0.0 (Ollama
    is free, no token pricing)
  - Error on non-200 status or `"error"` field in response
- `internal/model/ollama_test.go` — uses `httptest.Server` to capture and mock requests
- `internal/model/provider.go` update: replace the existing `ollama/*` OAI-compat preset
  with `NewOllama()`; `OLLAMA_HOST` env var used for host override (already in
  `ProviderConfigFromEnv`)
  - The replacement does not break existing users: the OAI-compat shim was an
    internal preset, not a documented interface. The native driver uses the same
    `ollama/model-name` ID prefix.

## Out of scope

- Ollama model pull / push / list APIs
- Ollama multimodal (image) inputs
- Ollama streaming
- Ollama `keep_alive` parameter
- Ollama `options` (temperature, top_p, etc.) — sworn uses model defaults

## Planned touchpoints

- `internal/model/ollama.go` (new)
- `internal/model/ollama_test.go` (new)
- `internal/model/provider.go` (modify — replace OAI-compat ollama preset with native
  driver)

## Acceptance checks

- [ ] `go build ./...` succeeds with no new external deps
- [ ] `NewOllama("llama3.2", "")` returns `*Ollama` with Host = `"http://localhost:11434"`
- [ ] `NewOllama("llama3.2", "http://myserver:11434")` uses the provided host
- [ ] `$OLLAMA_HOST` env var sets the host when `NewOllama` host param is empty
- [ ] `model.NewClient("ollama/llama3.2", cfg)` returns `*Ollama` (not the old `*OAI`
  preset) — type assertion in test confirms it
- [ ] `Verify()` with a mock `/api/chat` server returns `message.content` from the
  response
- [ ] `Verify()` returns a non-nil error when the mock server returns a JSON body with
  an `"error"` field
- [ ] Cost is always 0.0 (no token pricing for local Ollama)
- [ ] `go test ./internal/model/... -run Ollama` passes with zero failures (no live
  Ollama required)
- [ ] All prior model tests still pass

## Required tests

- **Unit** `internal/model/ollama_test.go` (all using `httptest.Server`):
  - `TestOllamaVerify_ReturnsContent`: mock `/api/chat`; assert Verify returns
    `message.content`
  - `TestOllamaVerify_ErrorField`: mock returns `{"error": "model not found"}`;
    assert non-nil error
  - `TestOllamaVerify_NonOKStatus`: mock returns 503; assert non-nil error
  - `TestOllamaDefaultHost`: NewOllama("m", "") → Host is localhost:11434
  - `TestOllamaHostFromEnv`: set `OLLAMA_HOST`; NewOllama("m", "") → Host matches
  - `TestOllamaRequestFormat`: capture the POST body; assert `stream: false` and
    system message included
  - `TestNewClient_OllamaIsNative`: `model.NewClient("ollama/llama3.2", cfg)` is
    `*Ollama`, not `*OAI`
- **Reachability artefact**: live integration test (skipped unless Ollama is running
  locally and `SWORN_LIVE_TESTS=1`): call Verify with "Reply with PASS.";
  assert "PASS" returned.

## Risks

- The native Ollama API format (`/api/chat`) differs from the OAI-compat format
  (`/v1/chat/completions`). Tests against the real Ollama server must run against the
  native endpoint, not the OAI shim.
- Replacing the existing OAI preset: if any user was calling `model.NewClient("ollama/...")`
  and relying on the OAI-compat response format, the native driver may behave slightly
  differently (e.g., no `usage` field, different error shapes). The unit tests cover this.

## Deferrals allowed?

Model pull / list: deferred post-R3 (these are operational features, not inference).
