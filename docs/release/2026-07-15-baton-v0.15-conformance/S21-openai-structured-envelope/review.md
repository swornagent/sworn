# Captain review — S21-openai-structured-envelope

Date: 2026-07-17T08:24:54+10:00
Design commit: 46c79220397cce2a00a0ef402eadfc58a41e93a7

## Pins

1. [mechanical] §1.Closed-world compiler — make the generic-report-family rejection grammar explicit and anchored before any HTTP.
   What I observed: §1 rejects “another recognised generic-report-family `$id`”, but does not state the bounded `$id` predicate. The live canonical inputs distinguish `llm-check-report-v1` from the dedicated `spec-ambiguity-report-v1`; a title, substring, map-shape, or endpoint heuristic would violate the closed-world boundary.
   What to ask the implementer: Use an explicitly anchored Baton schema-ID family rule (not a substring) for unsupported generic reports, with exact canonical `$id` plus source-byte SHA-256 as the sole compile condition. Add zero-request cases for a digest-altered canonical ID, a future/nearby generic family ID, and the exact ambiguity ID, plus an unrelated schema that retains its existing strict-projection path. Assert stable local error classes contain no supplied schema canary, request body, or credential.

2. [mechanical] §2.Explicit provider and structured-mode profiles — carry one prefix-to-wire-mode-and-profile mapping through both direct and proxy construction.
   What I observed: direct `NewClient` currently assigns native response-format mode to `openai-completions/` and `xai/`, while current `proxyClient` constructs a bare `OAI` for all non-Responses providers. A profile-only edit could therefore lose a retained structured mode or select the envelope by concrete client type. The design states that mode and profile are separate but its test list does not explicitly prove proxy preservation.
   What to ask the implementer: Make direct and proxy resolution use one default-deny mapping: `openai/` and its supported `openai-responses/` alias use the Responses OpenAI profile; `openai-completions/` uses the completions OpenAI profile; xAI remains native response-format with no envelope; forced-tool paths remain tool parameters with no envelope; unknown/unprofiled clients remain no-envelope. Test actual wire payloads from factory/proxy construction, including endpoint resemblance, rather than setting a profile field by hand.

3. [mechanical] §5.Reachability and no-HTTP proof — make built-binary fake tests hermetic and assert the local-rejection exit contract.
   What I observed: `FromEnv` evaluates proxy routing before direct provider routing, and `cmdLLMCheck` maps a pre-dispatch structured error to exit 2. The design calls for fake endpoints and a non-zero unsupported case, but does not yet require isolation from inherited proxy credentials or an exact exit. That would weaken both the zero-request claim and the no-credential-leak boundary.
   What to ask the implementer: Build child environments from a scrubbed set, not an inherited `os.Environ()` append: set `SWORN_DIRECT=1`, temporary HOME/XDG credential paths, only a synthetic key, and the relevant fake base URL. Assert exit 0 for both retained OpenAI paths and exit 2 with zero handler hits for each pre-HTTP rejection. Drive the dedicated `spec-ambiguity` CLI fixture through an OpenAI profile as a zero-HTTP rejection too, proving it neither posts a generic envelope nor flattens/reconstructs its maps.

4. [memory-cited] §2.Closed-world compiler — preserve the dedicated ambiguity contract rather than treating it as a generic findings array.
   What I observed: the design rejects the exact `spec-ambiguity-report-v1` identity before HTTP and explicitly prohibits map-to-array reconstruction. This matches the project record that spec-ambiguity has its own contract while the other checks use `llm-check-report-v1`.
   What to ask the implementer: Retain the dedicated-map route and include its rejection case in the no-HTTP tests; do not add a generic adapter or renderer fallback.
   Citation: [[Baton spec-ambiguity protocol redesign and Sworn follow-up handoff]]

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2, 3

## Summary

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]. Critical pins: 1, 2, 3.

## Smaller flags (not pins, worth one-line acknowledgement)

S04 is live-verified and S20 remains blocked; the design correctly leaves both slices and all canonical prompt/schema bytes untouched. Existing S04/S19/S20 test-path overlap is not an active in-progress or implemented collision, so it needs no new sequencing edge.

## Suggested acknowledgement reply

TL;DR the closed-world transport design is sound enough to implement once its selector and construction proof are made explicit. 4 pins + 1 flag:

1. **Anchor the family guard.** Compile only exact canonical ID-plus-digest. Define an anchored generic-report-family rejection rule; test canonical-digest mismatch, a nearby/future generic ID, exact ambiguity, and unrelated raw-schema retention with zero HTTP and redacted stable errors.
2. **Unify profile and wire-mode construction.** Use one direct/proxy prefix mapping: Responses (`openai/` and its deprecated alias) and completions are the only envelope profiles; xAI stays raw native response-format; forced-tool stays raw parameters; unknown stays default-deny. Inspect real factory/proxy wire payloads.
3. **Make binary evidence hermetic.** Use scrubbed child environments with `SWORN_DIRECT=1`, temporary credential homes, synthetic keys, and fake base URLs. Assert exact exit 0 for both OpenAI happy paths, exact exit 2 plus zero endpoint hits for every local reject, including the real spec-ambiguity CLI route.
4. **Keep ambiguity distinct.** Preserve its map contract and reject its exact schema before HTTP; do not flatten, reconstruct, or fall back to a generic report.

Flags (not pins): S04 remains semantic authority and S20 remains blocked pending a fresh S21 verifier PASS and its separate credentialed smoke.

§2 decision preserving the dedicated ambiguity contract is acknowledged against project memory. §6 questions: none.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The bounded transport adapter is sound; all pins are deterministic, apply-inline proof and construction safeguards.
-->
