# Captain review — S22-openrouter-tool-structured-output

Date: 2026-07-17T11:04:16+10:00
Design commit: ce699ccbb2b04a07ee4d124793b31d57d2ffdd80

## Pins

1. [mechanical] §2.1–2 — Bind the stricter tool-call policy to direct OpenRouter alone, and prove it cannot become a proxy or legacy OAI policy.
   What I observed: `structuredRouteForProvider` currently supplies both direct `NewClient` and `proxyClient`, while the shared `StructuredToolCall` path accepts the first tool call whenever any call exists. The design correctly proposes a construction-time direct-only policy, but its test plan names capability checks without explicitly proving zero dispatch for the closed routes or retaining DeepSeek's existing tool-call semantics.
   What to ask the implementer: add deterministic counter-server coverage showing proxy OpenRouter, unprofiled OAI, and Ollama reject structured output before a request; show the direct route rejects zero, wrong, multiple, and non-object calls after exactly one request; and retain a regression assertion for the existing DeepSeek forced-tool behavior.

2. [mechanical] §2.3 — Test the promised direct base-URL predicate rather than relying on generic URL parsing.
   What I observed: the current `FromEnv` override path uses `url.Parse` alone, whereas the design and AC-05 require an absolute HTTP(S) URL with a host and a pre-dispatch failure. The design calls out an invalid case but not the boundary cases that distinguish this predicate.
   What to ask the implementer: cover relative, hostless, and non-HTTP(S) values with a request counter proving failure before dispatch; also prove the override remains ignored by proxy OpenRouter and another provider.

3. [memory-cited] §2 — Preserve the canonical report and S04 identity gate as the only semantic authority.
   What I observed: the design passes the supplied canonical schema directly as the forced function parameters and routes returned arguments back through the unchanged generic gate. This agrees with the accepted typed ambiguity protocol: report families and reference semantics must remain explicit, deterministic, and fail closed rather than being inferred from provider transport success.
   What to ask the implementer: retain the canonical bytes and requested/emitted-check equality untouched, and ensure every malformed or semantically invalid tool result remains a local non-success without repair or fallback.
   Citation (if [memory-cited]): [[baton_spec_ambiguity_protocol_accepted]]

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): 1

## Summary

The direct-only construction boundary, raw canonical function parameters, and post-emission S04 gate match all six acceptance criteria and the four specified risk mitigations. S21 is verified and acknowledged as an unchanged, incompatible response-format route; S20 remains blocked and is explicitly sequenced after a fresh S22 PASS. All intersecting sibling touchpoints are verified, deferred, blocked, or planned—none is in progress or implemented—so no live handoff collision needs a dependency change.

## Smaller flags (not pins, worth one-line acknowledgement)

- The Captain role prompt normally prescribes `sworn llm-check --check design-review` after a PROCEED. The Coach's explicit preflight instruction prohibits every live provider/model call before deterministic implementation, so this review did not run that check. This is a recorded no-network limitation, not a waiver: after deterministic evidence, AC-06 still permits exactly one direct, non-secret `spec-ambiguity` proof for S22; it cannot be replaced by an S20 call or a proxy/fallback path.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The direct-only OpenRouter tool design is sound and remains bounded below the unchanged canonical gate. 3 pins + 1 flag:

1. **Direct policy boundary.** Make the exact tool-call policy construction-time and direct-OpenRouter-only. Add counter-server tests for zero-dispatch rejection on proxy OpenRouter, unprofiled OAI, and Ollama; direct exact-call rejection; and unchanged DeepSeek forced-tool behavior.
2. **Base-URL predicate.** Test relative, hostless, and non-HTTP(S) `SWORN_OPENROUTER_BASE_URL` values as pre-dispatch failures, and prove proxy OpenRouter and another provider ignore that override.
3. **Canonical semantic authority.** Keep the supplied canonical bytes and S04 requested/emitted equality authoritative; malformed or semantically invalid tool output must remain a local non-success with no repair or fallback.

Flags (not pins): (a) Captain did not run the normally prescribed live design-review LLM check because the Coach prohibited provider/model calls before deterministic implementation; this does not waive the single AC-06 direct S22 `spec-ambiguity` proof after deterministic evidence, and S20 remains blocked.

§2 direct-only route decision and its canonical-gate constraint acknowledged. §6 questions: none.

Address pins 1–3 inline during implementation, then proceed to in_progress.

## Captain verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The two boundary corrections are deterministic apply-inline work, the semantic gate aligns with accepted protocol memory, and the live LLM preflight is explicitly prohibited rather than waived.
-->
