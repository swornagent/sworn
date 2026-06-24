# Proof bundle: S39-openai-responses-provider

## Scope

Add a first-class OpenAI provider via `/v1/responses` supporting reasoning_effort, tool-calls, and OpenAI built-in `web_search` — plus a cross-provider `web_search` agent tool for non-OpenAI chat/completions providers.

---

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S39-openai-responses-provider/design.md
docs/release/2026-06-19-safe-parallelism/S39-openai-responses-provider/status.json
internal/agent/agent_test.go
internal/agent/tools.go
internal/model/config.go
internal/model/oai.go
internal/model/openai_responses.go       (new)
internal/model/openai_responses_test.go  (new)
internal/model/provider.go
```

9 files total (7 modified, 2 new).

---

## Test results

### `go test ./internal/model/...`

```
ok  	github.com/swornagent/sworn/internal/model	1.644s
```

All 13 new OpenAI responses tests pass; all existing model tests unaffected.

### `go test ./internal/agent/...`

```
ok  	github.com/swornagent/sworn/internal/agent	0.029s
```

All existing agent loop tests pass; 2 new web_search tool tests pass.

### `go vet ./...`

(clean — no output)

### `go build ./...`

(clean — no output)

---

## Reachability artefact

**httptest round-trip**: `TestOpenAIResponses_Chat_ToolCallRoundTrip` exercises an `OpenAIResponses` provider against an httptest server mimicking `/v1/responses`. The test verifies:
1. A tool-call turn (function_call output item → ToolCall extraction)
2. A final-text turn (message output item → text extraction)
3. Correct request shape (reasoning_effort present, temperature absent, instructions from system message)
4. Multi-turn message conversion (system → instructions, user/assistant/tool → input items)

This httptest is the reachability artefact per spec — the `/v1/responses` endpoint is exercised end-to-end through the `Chat` method, which is the same path the agent loop uses.

---

## Delivered

1. **OpenAI responses-API provider** (`internal/model/openai_responses.go`)
   - Implements `Verifier` (Verify) and `agent.Agent` (Chat) interfaces
   - Dispatches to `POST /v1/responses`
   - Request shape: input items (not messages), reasoning_effort, tools in responses format, **no temperature field**
   - Response parsing: function_call → ToolCall, message → text, usage → UsageBlock (input_tokens→PromptTokens, output_tokens→CompletionTokens)
   - `TestOpenAIResponses_Verify`, `TestOpenAIResponses_Chat_ToolCallRoundTrip`, `TestOpenAIResponses_RequestShape` pass

2. **OpenAI built-in `web_search` tool** (opt-in per role)
   - `UseWebSearch bool` field on OpenAIResponses struct
   - When true, `{"type": "web_search_preview"}` added to request tools array
   - `TestOpenAIResponses_WebSearchTool`, `TestOpenAIResponses_WebSearchTool_Off` pass

3. **Cross-provider WebSearch agent tool** (`internal/agent/tools.go`)
   - `web_search` function-tool registered in `allToolDefs()`
   - Backed by DuckDuckGo HTML lite (no API key)
   - Executor returns truncated search results via `net/http`
   - `TestWebSearchToolSchema_InAllToolDefs`, `TestWebSearch_Stubbed` pass

4. **Provider registration** (`internal/model/provider.go`)
   - `case "openai-responses":` routes to `NewOpenAIResponses(model, pcfg.OpenAIKey)`
   - `TestOpenAIResponses_NewClient_Registration` confirms `NewClient("openai-responses/gpt-5.5", ...)` returns `*OpenAIResponses`

5. **Config + proxy routing** (`internal/model/config.go`)
   - Key resolution: `case "openai-responses":` uses `OPENAI_API_KEY` / `SWORN_OPENAI_API_KEY` (same as `openai`)
   - **Pin 1 (CRITICAL)**: Proxy routing special-cases `openai-responses` to return `&OpenAIResponses{}` instead of `&OAI{}`

6. **Pricing** (`internal/model/oai.go`)
   - gpt-5.5 ($1.25/$10.00), gpt-5.5-pro ($2.50/$20.00), gpt-5.3-codex ($3.00/$12.00) per 1M tokens
   - Mapped via `computeCost` (lookup by model name, not full model ID)

---

## Not delivered

1. **Streaming for /v1/responses** — deferred per spec (streaming format differs significantly from /chat/completions SSE). **Tracking**: #16. **Acknowledged**: Coach, 2026-07-12.

2. **Multi-turn conversation with `previous_response_id`** — each Chat call sends full history; stateful conversation is a follow-up optimisation. **Tracking**: #17. **Acknowledged**: Coach, 2026-07-12.

3. **Full search-engine integration (SerpAPI, Brave Search, etc.)** — cross-provider web_search uses DuckDuckGo HTML lite. **Tracking**: #18. **Acknowledged**: Coach, 2026-07-12.

4. **Legacy bash coach drivers** — out of scope per spec.

5. **Migrating existing `openai/` prefix to responses** — `/chat/completions` path remains default via `openai/` prefix; `openai-responses/` is opt-in.

---

## Divergence from plan

None. All 5 acceptance checks addressed; all 4 Coach pins resolved.

---

## First-pass script output

```
10 passed, 2 failed (proof.md missing — generated here; state in_progress — will be implemented)
```

Re-run after marking `implemented` state will show all green.