# Design TL;DR: S39-openai-responses-provider

## §1. User-visible change

A user can now configure an OpenAI reasoning model (e.g. `openai/gpt-5.5`,
`openai/gpt-5.5-pro`) as a sworn role — it works with reasoning (`reasoning_effort`)
and tool calls, dispatched through OpenAI's `/v1/responses` endpoint instead of
`/chat/completions`. The agent tool set also gains a `web_fetch` function-tool
available to all chat/completions providers (deepseek, groq, etc.), giving
non-OpenAI models a lightweight way to fetch web content. OpenAI models using the
responses provider also get OpenAI's built-in `web_search` tool as a provider-native
option.

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
   default is acceptable. The existing `/chat/completions` client likewise omits
   it (the `chatRequest` struct has no temperature field).

3. **WebFetch only for cross-provider tool**: The spec mentions "WebSearch
   (and/or WebFetch)." To keep scope bounded per the spec's own note ("may
   split into its own slice"), this slice ships `web_fetch` — a simple HTTP GET
   tool that fetches a URL and returns truncated text. WebSearch (search-engine
   integration) is deferred as tool-scope of its own.

4. **No dedicated `Agent` interface for responses**: Both `Verify` and `Chat`
   live on the same `OpenAIResponses` struct, matching the existing OAI pattern.
   The OAI struct's `Chat` is not touched — the responses provider is a separate
   type registered in `NewClient` under a new provider prefix.

5. **Pricing added for reasoning models**: `gpt-5.5`, `gpt-5.5-pro`, and
   `gpt-5.3-codex` pricing entries are added to the existing `modelPricing`
   table in `oai.go` (that table serves both the chat and responses paths). If
   the responses API reports a `reasoning_tokens` field in usage, it is summed
   into completion tokens for cost calculation.

## §3. Files I'll touch grouped by purpose

- **New provider**: `internal/model/openai_responses.go` — the `/v1/responses`
  provider struct, `Verify`, and `Chat`. `internal/model/openai_responses_test.go`
  — httptest unit tests covering request shape, response parsing, and tool-call
  round-trips.

- **Provider registration**: `internal/model/provider.go` — add `"openai-responses"`
  (or `openai/responses`) dispatch in `NewClient`. The `openai` prefix continues
  to route to `/chat/completions`; a new prefix isolates the responses path.

- **Cross-provider tool**: `internal/agent/tools.go` — add `webFetchToolSchema()`
  to `allToolDefs()` and `runWebFetch` to the executor. `internal/agent/tools_test.go`
  — schema presence + stubbed fetch test.

- **Pricing**: `internal/model/oai.go` — add gpt-5.x entries to `modelPricing`.

- **Forward-compat for responses Chat interface**: `internal/model/oai.go`
  — no changes. The existing `Chat` method on OAI remains `/chat/completions`.
  The agent loop selects the appropriate provider at dispatch time; the responses
  provider satisfies the same `Agent` interface via its own `Chat` method.

## §4. Things I'm NOT doing

- **Streaming** — deferred per spec (needs its own slice; the responses API
  streaming format differs significantly from chat/completions SSE).
- **WebSearch search-engine integration** — deferred. The `web_fetch` tool
  provides a portable HTTP fetch; full search (e.g. SerpAPI backing) is a
  separate slice.
- **Multi-turn conversation with `previous_response_id`** — each `Chat` call
  sends the full history as input items. Stateful conversation (using the
  responses API's `previous_response_id` for lower latency) is a follow-up
  optimization.
- **Migrating existing `openai/` prefix to responses** — the existing `/chat/completions`
  path remains the default for `openai/gpt-4o` etc. The responses path is opt-in
  via a distinct model ID prefix (`openai-responses/gpt-5.5`).
- **Legacy bash coach drivers** — out of scope per spec.

## §5. Reachability plan

- **Unit**: `go test ./internal/model/...` — `openai_responses_test.go` against
  an httptest server mimicking `/v1/responses` with a reasoning response + tool
  call, asserting correct request shape (reasoning_effort present, temperature
  absent) and correct output parsing.
- **Integration**: `go test ./internal/agent/...` — `tools_test.go` asserting
  `web_fetch` tool schema is in `allToolDefs()` and the executor fetches from a
  local httptest URL.
- **Full loop**: Run the test suite at session end, capture output in proof.md.

## §6. Open questions for the Coach

None — straightforward translation layer with well-defined boundaries.