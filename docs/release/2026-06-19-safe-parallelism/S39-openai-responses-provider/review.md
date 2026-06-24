# Captain review — S39-openai-responses-provider
Date: 2026-07-12
Design commit: d9d1ae8c5997e8df7cd22168bff252cdcd65efc3

## Pins

1. **[mechanical] §3 — `internal/model/config.go` missing from file plan; provider will fail at runtime on key resolution.**
   What I observed: The design registers the provider under prefix `openai-responses` in `NewClient` (provider.go), but `FromEnv` (config.go) derives the key env var from the prefix: `strings.ToUpper(strings.ReplaceAll("openai-responses", "-", "_"))` → `OPENAI_RESPONSES` → checks `SWORN_OPENAI_RESPONSES_API_KEY`. The spec says "reading `OPENAI_API_KEY` (existing config path)." Users have `OPENAI_API_KEY` or `SWORN_OPENAI_API_KEY` set, not `SWORN_OPENAI_RESPONSES_API_KEY`. Without either (a) special-casing `openai-responses` in `FromEnv`'s key switch to reuse `pcfg.OpenAIKey`, or (b) adding a `ProviderConfig` field for the responses key, the provider will error "SWORN_OPENAI_RESPONSES_API_KEY not set" at runtime.
   What to ask the implementer: Add `internal/model/config.go` to the file plan. In `FromEnv`'s key-derivation switch, route `case "openai-responses": key = envOrAlias("OPENAI_API_KEY", "SWORN_OPENAI_API_KEY")` (same as `openai`). In `NewClient`, pass `pcfg.OpenAIKey` to the responses provider. Add a unit test asserting `openai-responses/gpt-5.5` resolves with `OPENAI_API_KEY` set and `SWORN_OPENAI_RESPONSES_API_KEY` unset.

2. **[mechanical] §3 — `internal/model/provider.go` and `internal/model/oai.go` missing from status.json `planned_files`.**
   What I observed: Design §3 lists `internal/model/provider.go` (NewClient dispatch) and `internal/model/oai.go` (modelPricing entries) as files to touch, but `status.json` `planned_files` only lists 4 files: `openai_responses.go`, `openai_responses_test.go`, `tools.go`, `tools_test.go`. The lint/touchpoint gates (S30, verified) check `planned_files` against actual diffs.
   What to ask the implementer: Add `internal/model/provider.go`, `internal/model/oai.go`, and `internal/model/config.go` to `planned_files` in status.json before transitioning to in_progress.

3. **[mechanical] §3 — `openai/responses` prefix alternative is unworkable with `parseModelID`.**
   What I observed: Design §3 says the prefix will be `"openai-responses" (or openai/responses)`. `parseModelID` splits on the first `/`, so `openai/responses/gpt-5.5` → provider=`openai`, model=`responses/gpt-5.5` → routes to the existing `/chat/completions` OAI client, not the responses provider. The `openai/responses` alternative is a dead end.
   What to ask the implementer: Commit to `openai-responses` as the sole prefix. Remove the `openai/responses` alternative from the design to avoid confusion.

4. **[mechanical] §1 vs §4 — model ID prefix inconsistency.**
   What I observed: §1 says `openai/gpt-5.5` as an example model ID, but §4 says the opt-in prefix is `openai-responses/gpt-5.5`. The `openai/` prefix routes to `/chat/completions`, so `openai/gpt-5.5` would not use the responses provider. The §1 example is misleading.
   What to ask the implementer: Fix §1 to use `openai-responses/gpt-5.5` as the example model ID, consistent with §4.

5. **[escalate] §2.3 + §4 — spec Risk #2 mitigation narrowed without acknowledgement.**
   What I observed: Spec Risk #2 says "the function-tool WebSearch (item 3) is the portable fallback" for web_search gating. Design §2.3 ships `web_fetch` only and defers WebSearch (search-engine integration) to a separate slice. The spec's prescribed fallback for web_search gating was WebSearch, not WebFetch — WebFetch is a URL fetcher, not a search engine. The design narrows the spec's fallback without acknowledging the deviation from the Risk #2 mitigation.
   What to ask the implementer: Coach, the design ships WebFetch (HTTP GET) instead of WebSearch (search-engine query) as the cross-provider tool. Spec Risk #2 names WebSearch as the portable fallback for web_search gating. WebFetch is a different tool (URL fetch vs search). Either (a) ack the narrowing — WebFetch is sufficient as the cross-provider web tool and WebSearch is a separate slice, or (b) require WebSearch be implemented in this slice per the spec's Risk #2 mitigation. If (a), the spec's Risk #2 should be amended via /replan-release to reference WebFetch as the fallback.

6. **[mechanical] §3 — AC3 (web_search selectable) has no implementation file or mechanism described.**
   What I observed: §1 mentions "OpenAI models using the responses provider also get OpenAI's built-in `web_search` tool as a provider-native option." AC3 requires "OpenAI built-in `web_search` is selectable and reaches the model as a tool." But §3 lists no file or mechanism for how web_search is wired into the responses provider request. The design doesn't describe how the tool is sent (e.g., as a `{"type": "web_search"}` entry in the responses API tools array) or how it's made opt-in per role/config.
   What to ask the implementer: Add to §3 a description of how `web_search` is wired: which file adds it to the responses request, what the tool entry looks like in the responses API format, and how it's made opt-in (config flag, role setting, or always-on for the responses provider). Add a test asserting the tool reaches the model.

7. **[mechanical] §4 — deferrals lack tracking and acknowledgement (Rule 2).**
   What I observed: §4 lists three deferrals (streaming, WebSearch, previous_response_id). Each has a why but none has tracking (issue/slice reference) or acknowledgement. Spec "Deferrals allowed" says streaming "may be deferred (with why + tracking + ack)" and the web tool split must be "surfaced explicitly." The design surfaces the split but doesn't link tracking or record acknowledgement.
   What to ask the implementer: For each §4 deferral, add a tracking reference (GitHub issue or slice ID) and record the Coach's acknowledgement. At minimum, note "tracking: follow-up slice" and "ack: Coach acked in design review" for each. The implementer should file issues for WebSearch and streaming if not already filed.

8. **[mechanical] Step 2b — `design_decisions` field absent from status.json.**
   What I observed: status.json has no `design_decisions` field. Sibling slices (S13, S15, S16, S19) carry structured `design_decisions` with id/description/type/rationale. The design-fit gate (S32, verified) expects this field. All 5 §2 decisions are Type-2 (local, reversible) but must be recorded.
   What to ask the implementer: Add a `design_decisions` array to status.json with entries for each §2 decision (D1–D5), classified as Type-2, with rationale. This satisfies the design-fit gate.

## Summary

Pins: 8 total — 7 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins: #1 (provider will fail at runtime without config.go key routing)

## Smaller flags (not pins, worth one-line ack)

- (a) `internal/agent/tools.go` and `internal/model/oai.go` collide with S27 (public-readiness-scrub, state=planned). Low risk since S27 is planned, not in_progress — but if S27 activates before S39 merges, coordinate.
- (b) The design says pricing entries go in `oai.go`'s `modelPricing` table, which serves both chat and responses paths. Confirm the pricing keys match the model IDs the responses provider will use (e.g., `gpt-5.5` not `openai-responses/gpt-5.5` — `computeCost` looks up by model name, not full model ID).
- (c) §2 decision 5 says "if the responses API reports a `reasoning_tokens` field in usage, it is summed into completion tokens for cost calculation." Confirm the responses API usage shape — `usage` in `/v1/responses` may use different field names than `/chat/completions` (e.g., `output_tokens` vs `completion_tokens`).

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Solid translation-layer design with one critical wiring gap and a spec-deviation to ack. 8 pins + 3 flags:

1. **config.go key routing (CRITICAL).** Add `internal/model/config.go` to the file plan. In `FromEnv`'s key-derivation switch, route `case "openai-responses": key = envOrAlias("OPENAI_API_KEY", "SWORN_OPENAI_API_KEY")` (same as `openai`). In `NewClient`, pass `pcfg.OpenAIKey` to the responses provider. Add a unit test asserting `openai-responses/gpt-5.5` resolves with `OPENAI_API_KEY` set and `SWORN_OPENAI_RESPONSES_API_KEY` unset.
2. **planned_files incomplete.** Add `internal/model/provider.go`, `internal/model/oai.go`, and `internal/model/config.go` to `planned_files` in status.json.
3. **Prefix alternative.** Commit to `openai-responses` as the sole prefix. Remove the `openai/responses` alternative — `parseModelID` splits on the first `/`, so it would route to chat/completions.
4. **§1 model ID example.** Fix §1 to use `openai-responses/gpt-5.5` (not `openai/gpt-5.5`) as the example — `openai/` routes to chat/completions.
5. **WebSearch vs WebFetch (Coach ack).** Spec Risk #2 names WebSearch as the portable fallback for web_search gating. Design ships WebFetch (URL fetch) instead. Coach: ack the narrowing (WebFetch is sufficient, WebSearch is a separate slice) or require WebSearch in this slice. If ack, amend spec Risk #2 via /replan-release.
6. **web_search wiring.** Add to §3 a description of how `web_search` is wired into the responses request (file, tool entry format, opt-in mechanism) and a test asserting it reaches the model. AC3 requires it.
7. **Rule 2 deferral tracking.** For each §4 deferral, add a tracking reference (issue or slice ID) and record Coach acknowledgement. File issues for WebSearch and streaming if not already filed.
8. **design_decisions in status.json.** Add a `design_decisions` array to status.json with entries for D1–D5, classified Type-2, with rationale. Satisfies the design-fit gate.

Flags (not pins): (a) tools.go/oai.go collide with S27 (planned, low risk); (b) confirm pricing keys match model names not full IDs; (c) confirm responses API usage field names for reasoning_tokens.

§2 decisions 1–5 ack (Type-2, all local/reversible). §6 questions: none — ack.

Address pins 1–8 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 5 is a spec Risk #2 deviation (WebSearch fallback narrowed to WebFetch) requiring Coach judgement — no single right answer on whether WebFetch satisfies the spec's intent.
-->