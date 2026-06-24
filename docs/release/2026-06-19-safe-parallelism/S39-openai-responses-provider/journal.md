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

*(None yet.)*