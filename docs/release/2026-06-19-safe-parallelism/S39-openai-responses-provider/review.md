# Captain review — S39-openai-responses-provider
Date: 2026-07-12
Design commit: 66e6a4b03c282511004b4da117e484f4f644883c
Review round: R2 (re-review after Coach decline: "web_search is mandatory")

## R1 pin resolution check

All 8 R1 pins resolved in the revised design:

- R1-1 (config.go key routing CRITICAL): RESOLVED. §3 now includes config.go with `case "openai-responses":` routing to `envOrAlias("OPENAI_API_KEY", "SWORN_OPENAI_API_KEY")`. status.json planned_files includes config.go.
- R1-2 (planned_files incomplete): RESOLVED. status.json now lists all 7 files.
- R1-3 (openai/responses prefix alternative): RESOLVED. Design commits to `openai-responses` as sole prefix.
- R1-4 (§1 model ID example): RESOLVED. §1 uses `openai-responses/gpt-5.5`.
- R1-5 (WebSearch vs WebFetch ESCALATE): RESOLVED. Design now ships WebSearch (DuckDuckGo), matching spec Risk #2's prescribed fallback. The Coach decline mandated this.
- R1-6 (web_search wiring): RESOLVED. §3 describes tool entry format `{"type": "web_search_preview"}`, opt-in via UseWebSearch bool, test asserting presence/absence.
- R1-7 (Rule 2 deferral tracking): PARTIALLY RESOLVED — see Pin 4 below. §4 now has tracking+ack lines, but "follow-up slice TBD" is placeholder tracking.
- R1-8 (design_decisions absent): RESOLVED. status.json has 5 design_decisions, all Type-2 with rationale.

## Pins

1. **[mechanical] §3 — CRITICAL: proxy routing path in FromEnv bypasses NewClient, returns &OAI{} for all providers.**
   What I observed: `FromEnv` (config.go lines 52-66) has a proxy routing path that fires when sworn credentials are present and `SWORN_DIRECT` is not set. It returns `&OAI{BaseURL: proxyURL, Model: model, APIKey: creds.Token}` for ALL model IDs — bypassing `NewClient` entirely. This means `openai-responses/gpt-5.5` would get `&OAI{}` (chat/completions via proxy), NOT `OpenAIResponses` (responses API). gpt-5.5 rejects /chat/completions ("use /v1/responses instead"). The design does not mention the proxy routing path at all. Any user with sworn credentials (the default on-ramp) would get chat/completions, not responses.
   What to ask the implementer: In `FromEnv`'s proxy routing block (config.go ~line 57), add a special case: when `provider == "openai-responses"`, construct an `OpenAIResponses` with the proxy URL as BaseURL instead of `&OAI{}`. Alternatively, if the SwornAgent proxy does not yet support `/v1/responses` forwarding, document that the responses provider requires `SWORN_DIRECT=1` and defer proxy support as a Rule 2 deferral (with real tracking). Either way, the proxy path must not silently route a responses-provider model to /chat/completions.

2. **[mechanical] §2/§3 — reasoning_effort has no field, no config mechanism, no wiring path.**
   What I observed: The spec says "reasoning_effort configurable per role (default medium)" and AC1 requires "reasoning_effort sent." The design mentions reasoning_effort in §1 and §5 (test assertion) but never describes: (a) a `ReasoningEffort` field on `OpenAIResponses`, (b) how it's configured per role (config.json? env var? hardcoded default?), (c) how it's passed through `NewClient(modelID, pcfg ProviderConfig)` which has no parameter for it. `internal/config/config.go` (ModelSetting) is not in the file plan. The design §3 says NewClient constructs OpenAIResponses with "BaseURL, pcfg.OpenAIKey, and the model name" — no reasoning_effort.
   What to ask the implementer: Add a `ReasoningEffort string` field to `OpenAIResponses` (default "medium"). Either add it to `ProviderConfig` or read from an env var (e.g. `SWORN_OPENAI_RESPONSES_REASONING_EFFORT`). If per-role config via `ModelSetting` is intended, add `internal/config/config.go` to the file plan. At minimum, hardcode "medium" as the default and send it in the request — the spec AC requires it to be sent, and "configurable" can be satisfied by an env var override.

3. **[mechanical] §3 — UseWebSearch bool field has no wiring path through NewClient.**
   What I observed: The design says `UseWebSearch bool` is a field on `OpenAIResponses`, "by default it's off; the caller (agent loop or sworn run) sets it per the role's config." But `NewClient` returns `Verifier`, not `*OpenAIResponses` — the caller would need a type assertion to set the field. The agent loop gets its model via `newAgentFromModel` → `FromEnv` → `NewClient` → returns `Verifier` → type-asserts to `agent.Agent`. There is no step in this chain where the caller can set `UseWebSearch`. No config mechanism (env var, ModelSetting field, ProviderConfig field) is described.
   What to ask the implementer: Either (a) add `UseWebSearch` to `ProviderConfig` and set it in `NewClient` when constructing `OpenAIResponses`, (b) read from an env var (e.g. `SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH`) inside `NewClient`, or (c) default to false and defer per-role configurability. The test in §5 (asserting tool entry appears when opted in) needs a way to set the flag — describe how the test constructs the provider with `UseWebSearch=true`.

4. **[mechanical] §4 — deferrals use "follow-up slice TBD" which is placeholder tracking.**
   What I observed: §4 has three deferrals (streaming, previous_response_id, separate search-engine integration). Each has "Tracking: follow-up slice TBD. Ack: ..." But "TBD" is not a real tracking reference. Per [[feedback_placeholder_tracking_smell]], real tracking is a GitHub issue number (#NNN), an on-disk release folder, or a sibling slice id. The `release-verify.sh` placeholder tracking regex (`tracking:[[:space:]]*(TBD|TODO|pending|none|n/?a)`) will catch this when these deferrals land in proof.md "Not delivered."
   What to ask the implementer: File GitHub issues for each deferral (streaming, previous_response_id, search-engine integration) using `gh issue create` and cite the real issue numbers in §4. Replace "follow-up slice TBD" with "#NNN" references. Do this before transitioning to in_progress.

## Summary

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: #1 (proxy routing path returns &OAI{} for openai-responses, causing runtime failure for sworn-credential users)

## Smaller flags (not pins, worth one-line ack)

- (a) S63-subscription-cli-driver (planned, T5-providers) also touches `internal/model/config.go`. Low risk since S63 is planned, not in_progress — but if S63 activates before S39 merges, coordinate on config.go.
- (b) S27-public-readiness-scrub (planned) touches `internal/agent/tools.go` and `internal/model/oai.go`. Same low-risk caveat.
- (c) DuckDuckGo HTML parsing approach is undescribed. If the implementer promotes `golang.org/x/net/html` from indirect to direct dep, an ADR is required per ADR-0007 and `go.mod` must be added to planned_files. Regexp-based extraction (no new dep) is sufficient for lite.duckduckgo.com's simple HTML.
- (d) Response parsing (output items → tool calls + text) is not described in §2 or §3 — only usage field mapping is covered. This is an implementation detail the httptest tests will exercise; no design-level gap.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Revised design resolves all 8 R1 pins; 4 new mechanical pins (1 critical) from wiring gaps the design didn't cover. 4 pins + 4 flags:

1. **Proxy routing bypasses responses provider (CRITICAL).** FromEnv's proxy path (config.go ~line 57) returns &OAI{} for ALL providers when sworn credentials are present — openai-responses/gpt-5.5 would get /chat/completions, not /v1/responses. Either special-case openai-responses in the proxy block to return OpenAIResponses, or document SWORN_DIRECT=1 requirement as a Rule 2 deferral with real tracking.
2. **reasoning_effort wiring.** Spec requires "configurable per role (default medium)" and AC1 requires it sent. Add a ReasoningEffort field to OpenAIResponses (default "medium"). Wire it via ProviderConfig field or env var (SWORN_OPENAI_RESPONSES_REASONING_EFFORT). At minimum hardcode "medium" so the AC is satisfied.
3. **UseWebSearch wiring.** NewClient returns Verifier, not *OpenAIResponses — no way for the caller to set UseWebSearch. Either add to ProviderConfig, read from env var (SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH) inside NewClient, or default false and defer configurability. Describe how the §5 test sets the flag.
4. **Deferral tracking is placeholder.** "follow-up slice TBD" is not real tracking. File GitHub issues for streaming, previous_response_id, and search-engine integration deferrals. Replace "TBD" with real #NNN references before in_progress.

Flags (not pins): (a) S63 also touches config.go (planned, low risk); (b) S27 also touches tools.go + oai.go (planned, low risk); (c) if DuckDuckGo HTML parsing needs golang.org/x/net/html as direct dep, add ADR + go.mod to file plan — regexp is sufficient; (d) response output-item parsing is undescribed but covered by httptest tests.

§2 decisions 1-5 ack (Type-2, all local/reversible). §6 questions: (1) DuckDuckGo lite is acceptable as "simple HTTP search" — proceed; (2) placeholder pricing is fine — note as preliminary in proof.md. Ack.

Address pins 1-4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline wiring corrections (proxy path special-case, field additions, tracking refs) — the design's approach is sound and no pin requires re-reviewing the design before code is safe.
-->