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

## Verifier verdicts received

*(None yet.)*
