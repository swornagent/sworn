# Design TL;DR: S39-openai-responses-provider

## §1. User-visible change

A user can now configure an OpenAI reasoning model (e.g. `openai-responses/gpt-5.5`,
`openai-responses/gpt-5.5-pro`) as a sworn role — it works with reasoning
(`reasoning_effort`) and tool calls, dispatched through OpenAI's `/v1/responses`
endpoint instead of `/chat/completions`. The agent tool set also gains a
`web_search` function-tool available to all chat/completions providers (deepseek,
groq, etc.), giving non-OpenAI models a portable way to search the web. OpenAI
models using the responses provider also get OpenAI's built-in `web_search` tool
as a provider-native option.

## §2. Design decisions not in spec

1. **ChatMessage → responses input mapping**: The `Chat` method converts the
   existing `[]model.ChatMessage` history into the responses API `input` items
   format internally — `ChatMessage`/`ChatResponse` remain the canonical types.
   This keeps the agent loop (`internal/agent`) unchanged. The converter handles:
   system messages → `instructions`; user/assistant messages → `message` input
   items; tool calls → `function_call` items; tool results → `function_call_output`
   items.

2. **Temperature omitted**: The responses provider never sends `temperature`.
   Reasoning models reject non-default values, and for non-reasoning models the
   default is acceptable.

3. **WebSearch cross-provider tool**: The spec names WebSearch as the portable
   fallback for web_search gating (Risk #2). This slice ships `web_search` as a
   function-tool backed by DuckDuckGo's HTML lite search (no API key required).
   The tool takes a query string, fetches `https://lite.duckduckgo.com/lite/?q=…`,
   and returns truncated results text. This satisfies the spec's "simple HTTP
   search" requirement without adding a search-API dependency.

4. **No dedicated `Agent` interface for responses**: Both `Verify` and `Chat`
   live on the same `OpenAIResponses` struct, matching the existing OAI pattern.
   The OAI struct's `Chat` is not touched — the responses provider is a separate
   type registered in `NewClient` under the `openai-responses` prefix.

5. **Pricing added for reasoning models**: `gpt-5.5`, `gpt-5.5-pro`, and
   `gpt-5.3-codex` pricing entries are added to the existing `modelPricing`
   table in `oai.go`. Pricing keys use the bare model name (`gpt-5.5`, not
   `openai-responses/gpt-5.5`) — `computeCost` looks up by model name, not full
   model ID. If the responses API reports `output_tokens` and `input_tokens`
   (the /v1/responses field names), they are mapped to `CompletionTokens` and
   `PromptTokens` in the `UsageBlock` for cost calculation.

## §3. Files I'll touch grouped by purpose

- **New provider**: `internal/model/openai_responses.go` — the `/v1/responses`
  provider struct (`OpenAIResponses`), `Verify`, and `Chat`.
  `internal/model/openai_responses_test.go` — httptest unit tests covering
  request shape, response parsing, and tool-call round-trips.

- **Provider registration**: `internal/model/provider.go` — add `case
  "openai-responses":` dispatch in `NewClient`, constructing an `OpenAIResponses`
  with `BaseURL=https://api.openai.com/v1`, `pcfg.OpenAIKey`, and the model name.

- **Key resolution**: `internal/model/config.go` — add `case "openai-responses":`
  to the `FromEnv` key switch, resolving to `envOrAlias("OPENAI_API_KEY",
  "SWORN_OPENAI_API_KEY")` (same path as `openai`). The user's existing
  `OPENAI_API_KEY` or `SWORN_OPENAI_API_KEY` works for both providers.

- **Cross-provider tool**: `internal/agent/tools.go` — add `webSearchToolSchema()`
  to `allToolDefs()` and `runWebSearch` to the executor. The executor fetches
  `https://lite.duckduckgo.com/lite/?q=<url-encoded-query>` via `net/http`,
  parses the HTML for result snippets, and returns truncated text.
  `internal/agent/tools_test.go` — schema presence + stubbed search test.

- **OpenAI built-in web_search**: `internal/model/openai_responses.go` — the
  responses provider includes the built-in `web_search` tool in the request's
  `tools` array when opted in. The tool entry format follows the responses API
  spec: `{"type": "web_search_preview"}`. Opt-in is per-role via a boolean
  config field (`UseWebSearch bool`) on the `OpenAIResponses` struct — by default
  it's off; the caller (agent loop or sworn run) sets it per the role's config.
  AC3 is satisfied by an httptest asserting the tool entry appears in the
  request when opted in, and is absent when not.

- **Pricing**: `internal/model/oai.go` — add `gpt-5.5`, `gpt-5.5-pro`, and
  `gpt-5.3-codex` entries to `modelPricing`.

## §4. Things I'm NOT doing

- **Streaming** — deferred per spec (needs its own slice; the responses API
  streaming format differs significantly from chat/completions SSE).
  Tracking: follow-up slice TBD. Ack: Coach acked streaming deferral in
  design-review decline (2026-06-24).

- **Multi-turn conversation with `previous_response_id`** — each `Chat` call
  sends the full history as input items. Stateful conversation (using the
  responses API's `previous_response_id` for lower latency) is a follow-up
  optimization.
  Tracking: follow-up slice TBD. Ack: Coach acked in design review.

- **Migrating existing `openai/` prefix to responses** — the existing `/chat/completions`
  path remains the default for `openai/gpt-4o` etc. The responses path is opt-in
  via the distinct prefix `openai-responses/`.

- **Legacy bash coach drivers** — out of scope per spec.

- **Separate search-engine integration** — the cross-provider `web_search` tool
  uses DuckDuckGo HTML lite (no API key). Full search-engine integration (SerpAPI,
  Brave Search, etc.) is a separate slice.
  Tracking: follow-up slice TBD. Ack: Coach mandated web_search is mandatory;
  DuckDuckGo lite satisfies the spec's "simple HTTP search" scope.

## §5. Reachability plan

- **Unit**: `go test ./internal/model/...` — `openai_responses_test.go` against
  an httptest server mimicking `/v1/responses` with a reasoning response + tool
  call, asserting correct request shape (reasoning_effort present, temperature
  absent) and correct output parsing. Additional test: `openai-responses` prefix
  resolves with `OPENAI_API_KEY` set and `SWORN_OPENAI_RESPONSES_API_KEY` unset.

- **Unit**: `go test ./internal/model/...` — `openai_responses_test.go` asserting
  built-in `web_search` tool entry appears in the request when opted in.

- **Integration**: `go test ./internal/agent/...` — `tools_test.go` asserting
  `web_search` tool schema is in `allToolDefs()` and the executor fetches from
  a local httptest URL.

- **Full loop**: `go build ./...`, `go vet ./...`, and the new tests pass.

## §6. Open questions for the Coach

- WebSearch cross-provider tool: is DuckDuckGo HTML lite (no API key) acceptable
  as the "simple HTTP search" backing, or does the Coach prefer a specific search
  API? The spec says "simple HTTP search/fetch implementation" — DDG lite satisfies
  that minimalist scope.
- Pricing for gpt-5.x reasoning models: what are the actual per-1M-token prices?
  The entries will use placeholder costs until confirmed; note as preliminary.