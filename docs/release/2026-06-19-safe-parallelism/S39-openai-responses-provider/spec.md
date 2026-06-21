---
title: 'S39-openai-responses-provider — first-class OpenAI provider via /v1/responses (reasoning + tools incl. web_search)'
description: 'sworn has no first-class OpenAI provider — internal/model/oai.go is /chat/completions-only, which OpenAI reasoning models (gpt-5.5, gpt-5.5-pro, gpt-5.3-codex) reject for tools+reasoning ("use /v1/responses instead"; temperature must be default). Add an OpenAI responses-API provider supporting reasoning_effort, tool-calls, and OpenAI built-in tools (web_search) — and add a cross-provider WebSearch/WebFetch agent tool so non-OpenAI tool-calling providers get more than the 6 core tools.'
---

# Slice: `S39-openai-responses-provider`

## User outcome

A user can configure an OpenAI **reasoning** model (e.g. `gpt-5.5`, `gpt-5.5-pro`) as a
sworn role and it works — with thinking on (`reasoning_effort`) and tools. And a
capable model can look things up online: the agent tool set gains a first-class
`WebSearch`/`WebFetch` tool (OpenAI's built-in `web_search` for the responses provider;
a function-tool for the chat/completions providers), rather than only the 6 core tools.

## Why

Verified 2026-06-21: `gpt-5.5` returns clean output on a minimal request, but the
`/chat/completions` path the existing client uses fails on reasoning models —
`temperature: 0` is rejected ("only default 1"), and "function tools with
reasoning_effort are not supported in /v1/chat/completions — use /v1/responses." So
sworn cannot drive gpt-5.x reasoning agents today. T5-providers ships anthropic/google/
bedrock/azure/oci/ollama but **no first-class OpenAI provider** — this fills that gap.

## In scope

### 1. OpenAI responses-API provider (`internal/model/`)

A new provider implementing the S10-provider-foundation interface against
`POST /v1/responses`:
- request shape: `input` items (not `messages`), `reasoning: {effort: low|medium|high}`,
  tools as responses-API function tools, **omit `temperature`** (reasoning models reject
  non-default) — or only send it for non-reasoning OpenAI models;
- parse tool calls + final text + `usage` (incl. reasoning tokens) from the responses
  output items;
- `reasoning_effort` configurable per role (default medium); selectable model id
  (`gpt-5.5`, `gpt-5.5-pro`, dated snapshots, `gpt-5.3-codex`).

### 2. OpenAI built-in `web_search` tool

Expose OpenAI's built-in `web_search` tool through the responses provider (opt-in per
role/config), so a model can "find details online" natively.

### 3. Cross-provider WebSearch/WebFetch agent tool (`internal/agent/tools.go`)

Add a `WebSearch` (and/or `WebFetch`) tool to the agent tool set — a function-tool
available to the chat/completions providers (deepseek/gemini/groq) too, so the answer to
"more than 6 tools" isn't OpenAI-only. Back it with a simple HTTP search/fetch
implementation. (If this grows large, it may split into its own slice — note it.)

## Out of scope

- The legacy bash coach-loop drivers (focus is the sworn product).
- Streaming for the responses API (a follow-up; first land non-streaming correctness).
- Provider auth/key management beyond reading `OPENAI_API_KEY` (existing config path).

## Planned touchpoints

- `internal/model/openai_responses.go` (new — the /v1/responses provider)
- `internal/model/` provider registration/selection (wire it into the S10 interface)
- `internal/agent/tools.go` (add WebSearch/WebFetch to the tool set)
- corresponding `_test.go` files

## Acceptance checks

- [ ] an OpenAI reasoning model (`gpt-5.5`) round-trips through the responses provider:
  a tool-call turn + a final-text turn, with `reasoning_effort` sent and `temperature` omitted
- [ ] the provider parses tool calls and final text from `/v1/responses` output items
- [ ] OpenAI built-in `web_search` is selectable and reaches the model as a tool
- [ ] a `WebSearch`/`WebFetch` function-tool is registered in the agent tool set and is
  offered to a chat/completions provider (e.g. a deepseek/groq dispatch)
- [ ] `go build ./...` + `go vet` + the new tests pass; existing `internal/model` tests unaffected

## Required tests

- **Unit** `internal/model/openai_responses_test.go`: against an `httptest` server mimicking
  `/v1/responses`, assert the request omits `temperature`, includes `reasoning.effort`, and
  the response's output items are parsed into tool calls + text + usage.
- **Unit** `internal/agent/tools_test.go`: `WebSearch`/`WebFetch` tool schema present in the
  tool set; a stubbed search returns results to the model.
- **Reachability artefact**: (network-permitting) a real `gpt-5.5` round-trip via the
  responses provider firing a tool call — or, if the env blocks live calls, the httptest
  round-trip — captured in proof.md. No-mock boundary (Rule 10): if a live OpenAI call is
  required and unreachable, surface as blocked, do not mock the model at the boundary
  beyond the declared httptest unit.

## Risks

- The `/v1/responses` output shape differs materially from `/chat/completions` (output
  items, reasoning blocks). Parse defensively; cover with the httptest fixture.
- Built-in `web_search` may have account/tier gating — if unavailable, the function-tool
  WebSearch (item 3) is the portable fallback; note any gating as a Rule-2 deferral.

## Deferrals allowed?

Streaming for the responses API may be deferred (with why + tracking + ack). The
cross-provider web tool (item 3) may split into its own slice if it outgrows this one —
surface the split explicitly rather than dropping it.
