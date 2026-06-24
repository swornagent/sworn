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
