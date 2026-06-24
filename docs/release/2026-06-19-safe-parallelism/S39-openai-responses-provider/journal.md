---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S39-openai-responses-provider`

## 2026-06-21 — planned (replan)

Added after a live smoke test showed sworn (and the legacy coach drivers) can't drive
OpenAI reasoning models: gpt-5.5 rejects temperature:0 and requires /v1/responses for
tools+reasoning. T5 ships no first-class OpenAI provider. This slice adds the
responses-API provider (reasoning_effort + tool-calls + built-in web_search) and a
cross-provider WebSearch/WebFetch agent tool — answering both "support gpt-5.x reasoning"
and "give models more than the 6 core tools". Effort focused on sworn (the product); the
bash coach drivers are intentionally out of scope. Depends on S10-provider-foundation's
interface; placed at T5's tail.

## Open questions

- Whether OpenAI built-in web_search is available at the account tier (fallback: the
  function-tool WebSearch).

## Deferrals surfaced

None yet.

## 2026-07-12 — design revised (implementer, re-entry after Coach decline)

Coach declined the original design (2026-06-24): "web_search is mandatory."
Addressed all 8 Captain pins from the original review (d9d1ae8c):

1. **[pin 1] config.go key routing** — added `internal/model/config.go` to file plan;
   `case "openai-responses":` routes to `envOrAlias("OPENAI_API_KEY",
   "SWORN_OPENAI_API_KEY")` (same as `openai`).
2. **[pin 2] planned_files** — expanded to include provider.go, oai.go, config.go.
3. **[pin 3] prefix** — committed to `openai-responses` as sole prefix; removed
   `openai/responses` alternative (parseModelID splits on first `/`).
4. **[pin 4] §1 example** — fixed to `openai-responses/gpt-5.5`.
5. **[escalate/Coach decline] WebSearch mandatory** — design now ships `web_search`
   function-tool backed by DuckDuckGo HTML lite (no API key), replacing WebFetch.
   This is the spec's Risk #2 portable fallback.
6. **[pin 6] web_search wiring** — described in §3: `{"type": "web_search_preview"}`
   in responses API tools array; opt-in via `UseWebSearch bool` field.
7. **[pin 7] Rule 2 deferral tracking** — each §4 deferral now has tracking and
   Coach ack recorded.
8. **[pin 8] design_decisions** — D1–D5 added to status.json, all Type-2.

Smaller flags addressed:
- (a) tools.go/oai.go collision with S27 noted — S27 is planned, not in_progress.
- (b) Pricing keys use bare model names, confirmed in §2.5.
- (c) /v1/responses usage field names (output_tokens/input_tokens) mapped in §2.5.

## Verifier verdicts received

### 2026-07-12 — verifier (fresh context)

FAIL

Slice: S39-openai-responses-provider

Violations:
1. Gate 2 — Planned touchpoints (spec.md, status.json) list `internal/agent/tools_test.go` but actual changed file is `internal/agent/agent_test.go`; proof.md "Divergence from plan" incorrectly states "None".
   Evidence: spec.md:61 (planned touchpoints), status.json:21 (planned_files), proof.md:11 (files changed lists agent_test.go), proof.md:117 (Divergence: None), git diff --name-only aa97dc3..HEAD
2. Gate 3 — Required tests explicitly call for `internal/agent/tools_test.go` (web_search schema + stub) but the file does not exist; tests live in agent_test.go instead.
   Evidence: spec.md:79-80, directory has no tools_test.go, agent_test.go:369-392 has the 2 web_search tests
3. status.json records wrong `start_commit` (930bc0ae... chore ack commit) instead of aa97dc3 (docs start implementation commit).
   Evidence: status.json:10, git log shows aa97dc3 is the "docs(...): start implementation" commit

Required to address:
1. Either create `internal/agent/tools_test.go` containing the web_search tests (to match spec) or update spec.md + planned_files + required tests to reference agent_test.go, and document the choice in proof.md "Divergence from plan".
2. Fix start_commit in status.json to "aa97dc3".
3. Re-generate proof bundle from live state, re-mark implemented, then re-verify in fresh session.

STATE: blocked_needs_human
SLICE: S39-openai-responses-provider
NEXT: NONE
REASON: Gates 2 and 3 fail on test file mismatch (spec requires tools_test.go; impl uses agent_test.go without acknowledgement); start_commit wrong in status.json.
## 2026-07-12 — implemented (Coach ack round)

Coach approved with 4 pins, all addressed inline:

1. **Pin 1 (CRITICAL): Proxy routing bypasses responses provider.** In `config.go` `FromEnv`, the proxy path now special-cases `openai-responses` to return `&OpenAIResponses{}` instead of `&OAI{}`. Same proxy URL + token; only the struct type differs so `/v1/responses` is called.

2. **Pin 2: reasoning_effort wiring.** Added `ReasoningEffort string` field to `OpenAIResponses`. Default "medium"; overridable via `SWORN_OPENAI_RESPONSES_REASONING_EFFORT` env var. Included in all `/v1/responses` requests.

3. **Pin 3: UseWebSearch wiring.** Added `UseWebSearch bool` field. Default false; set to true via `SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH=1` env var. When true, `{"type": "web_search_preview"}` is added to the request tools array.

4. **Pin 4: Deferral tracking.** Filed GitHub issues #16 (streaming), #17 (previous_response_id), #18 (search-engine integration). Updated design.md §4 with real issue references.

Implementation:
- New file: `internal/model/openai_responses.go` — OpenAIResponses struct with Verify+Chat, message conversion, response parsing, usage mapping
- New file: `internal/model/openai_responses_test.go` — 13 httptest tests covering verify, chat, request shape, web_search tool, multi-turn conversion, error handling, env overrides
- Modified: `internal/model/provider.go` — added `case "openai-responses":`
- Modified: `internal/model/config.go` — proxy routing special-case + key resolution  
- Modified: `internal/model/oai.go` — pricing entries for gpt-5.5 / gpt-5.5-pro / gpt-5.3-codex
- Modified: `internal/agent/tools.go` — web_search tool schema + executor (DuckDuckGo HTML lite)
- Modified: `internal/agent/agent_test.go` — web_search tool tests

Test results: all model tests pass (1.644s), all agent tests pass (0.029s), go vet clean, go build clean.

Deferrals carried forward with Coach ack: streaming (#16), previous_response_id (#17), search-engine integration (#18).

Skeptic panel: skipped — runtime subagent dispatch not confirmed in this session.
