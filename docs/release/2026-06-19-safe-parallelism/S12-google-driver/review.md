# Captain review — S12-google-driver
Date: 2026-07-09
Design commit: 39773e81f067a0a9b8e6a3cfccbfd569bf177a2d

## Pins

1. **[mechanical] §3 — `internal/model/config.go` missing from file plan; production dispatch path unreachable.**
   What I observed: The design §3 lists `internal/model/provider.go` as the routing touchpoint (adding `google/*` and `vertex/*` cases to `NewClient`). But the production path is `sworn run` → `model.FromEnv()` (in `internal/model/config.go`) → `swornProviderConfig()` → `NewClient()`. `FromEnv()` builds a `ProviderConfig` via `swornProviderConfig()` (config.go:118-133), which reads `SWORN_GOOGLE_API_KEY` — not `GOOGLE_API_KEY`. The design's §2.2 says `GoogleCloudProject` and `GoogleCloudLocation` are "read from env vars and stored on `ProviderConfig`, consistent with how every other provider field works" — but `swornProviderConfig()` in config.go is the actual production builder and it is not in the file plan. `ProviderConfigFromEnv()` in provider.go (the one the design references) is only called from tests. Without touching config.go, `sworn run --verifier-model google/gemini-2.0-flash` will fail at `FromEnv()` line 73-75 with `SWORN_GOOGLE_API_KEY not set` even when `GOOGLE_API_KEY` is set, because `FromEnv()` gates on `SWORN_<PREFIX>_API_KEY` before it ever reaches `NewClient()`.
   What to ask the implementer: Add `internal/model/config.go` to planned_files. In `swornProviderConfig()`, add `GoogleCloudProject: os.Getenv("GOOGLE_CLOUD_PROJECT")` and `GoogleCloudLocation: os.Getenv("GOOGLE_CLOUD_LOCATION")`. Also resolve the `SWORN_GOOGLE_API_KEY` gate: either (a) add a `google`/`vertex` bypass in `FromEnv()` that skips the `SWORN_*_API_KEY` check when `GOOGLE_API_KEY` is set (for `google/*`) or when the provider is `vertex` (ADC, no key), or (b) document that users must set `SWORN_GOOGLE_API_KEY` as the canonical env var and `GOOGLE_API_KEY` is only for `ProviderConfigFromEnv()` (test-only). The spec says `GOOGLE_API_KEY` in `~/.sworn/.env`; the production path requires `SWORN_GOOGLE_API_KEY`. This is a gap that will cause the slice to ship broken.

2. **[mechanical] §3 — `vertex/*` provider blocked by `FromEnv()` API-key gate (no key for ADC).**
   What I observed: `FromEnv()` (config.go:73-75) requires `SWORN_<PREFIX>_API_KEY` to be set before dispatching to `NewClient()`. For `vertex/*`, the prefix is `VERTEX`, so it checks `SWORN_VERTEX_API_KEY`. But Vertex AI uses Application Default Credentials (ADC) — there is no API key. The spec says "uses Application Default Credentials (no explicit key)". `FromEnv()` will return `model: SWORN_VERTEX_API_KEY not set` before `NewClient()` is ever called. The design does not address this.
   What to ask the implementer: Add a bypass in `FromEnv()` for providers that don't use API keys (vertex uses ADC). The cleanest approach: skip the `SWORN_*_API_KEY` gate for `vertex` provider, or add a general mechanism for keyless providers. Confirm the approach matches how Ollama (also keyless, uses `ollama` placeholder) is handled — Ollama currently goes through `FromEnv()` which would also fail the `SWORN_OLLAMA_API_KEY` check, so there may be a pre-existing pattern or gap to follow.

3. **[mechanical] §2.3 — Error mapping via "extract HTTP status code from SDK error" is unverified for genai SDK.**
   What I observed: Design §2.3 says "the genai SDK returns typed API errors. We'll extract the HTTP status code from the SDK error and route through `NewProviderError`, identical to the Anthropic pattern." The Anthropic driver (anthropic.go:91-107) extracts the status code by string-parsing the error's `.Error()` output (`strings.Index(s, '": ')`) — a brittle heuristic specific to anthropic-sdk-go's error formatting. The genai SDK is a different package with a different error type. The design says "identical to the Anthropic pattern" but the genai SDK may not format errors the same way. The spec Risk #1 explicitly calls this out: "Check the SDK docs for `genai.ClientConfig.Backend` enumeration at implementation time."
   What to ask the implementer: Before implementing, verify how `google.golang.org/genai` surfaces HTTP errors — check the SDK's error type and whether it exposes a status code directly (not via string parsing). If the genai SDK provides a typed error with a `.Code` or `.Status` field, use that directly instead of string-parsing. Do not assume the Anthropic string-parse pattern transfers.

4. **[mechanical] §2.1 — SDK version "latest stable" is not pinned; ADR-0007 requires justification.**
   What I observed: Design §2.1 says "will use the latest stable release; the version will be pinned in `go.mod` after `go get`." ADR-0007 (committed as `docs/adr/0007-dep-policy-minimal-justified.md`) pre-ratifies `google.golang.org/genai` for S12, so the ADR requirement is satisfied. However, the design does not mention the ADR or cite it. The spec's acceptance check #1 is "`go build ./...` succeeds with `google.golang.org/genai` in go.mod" — the ADR entry is the governance gate.
   What to ask the implementer: Confirm the ADR-0007 entry for `google.golang.org/genai` is sufficient (it is — pre-ratified). No new ADR file is needed. Just ensure the commit that adds the dep includes a `Co-Authored-By:` trailer per ADR-0007's requirement.

5. **[mechanical] §2.4 — Gemini pricing table not in file plan; spec says "add to existing pricing table pattern".**
   What I observed: Design §2.4 says "prices sourced from Google's public pricing page at implementation time (Gemini 2.0 Flash, 2.5 Flash, 2.5 Pro)." The spec says "Cost: uses `UsageMetadata.PromptTokenCount` + `CandidatesTokenCount` with known Gemini pricing (add to existing pricing table pattern)." The Anthropic driver has `anthropicPricing` as a package-level var in `anthropic.go`. The design's §3 file plan lists `internal/model/google.go` for "cost computation" — so the pricing table will live there. This is consistent. No pin needed, but confirm the pricing map uses `UsageMetadata.PromptTokenCount` and `UsageMetadata.CandidatesTokenCount` (not `OutputTokens` or similar — the genai SDK field names need verification at implementation time per spec Risk #1).
   What to ask the implementer: Verify the genai SDK's `UsageMetadata` field names at implementation time. The spec says `PromptTokenCount` + `CandidatesTokenCount`; confirm these are the actual field names in the SDK (not `InputTokenCount`/`OutputTokenCount` or similar).

6. **[mechanical] §4 — `parseModelID` for `vertex/*` claim is correct but should be verified.**
   What I observed: Design §4 says "the `parseModelID` function splits on first `/`, so `vertex/gemini-2.0-flash` already parses correctly. No changes needed." I verified: `parseModelID` (config.go:141-152) splits on first `/`, so `vertex/gemini-2.0-flash` → provider=`vertex`, model=`gemini-2.0-flash`. This is correct. No pin — just confirming the claim holds.
   What to ask the implementer: No action needed; the claim is verified.

7. **[escalate] §2.2 — `ProviderConfig` struct changes are additive but `FromEnv()` key-gate design is a product decision.**
   What I observed: The design adds `GoogleCloudProject` and `GoogleCloudLocation` to `ProviderConfig`. This is additive and safe. But the broader question of how `FromEnv()` handles keyless/ADC providers (vertex) and the `GOOGLE_API_KEY` vs `SWORN_GOOGLE_API_KEY` naming is a product-level decision: does sworn standardise on `SWORN_*` env vars (current convention) or does it accept canonical provider env vars (`GOOGLE_API_KEY`, `GOOGLE_CLOUD_PROJECT`)? The spec says `GOOGLE_API_KEY` in `~/.sworn/.env`; the production path requires `SWORN_GOOGLE_API_KEY`. This is a coherence gap between the spec's user-facing contract and the implemented dispatch path.
   What to ask the implementer: This is a Coach decision: should `FromEnv()` accept canonical provider env vars (`GOOGLE_API_KEY`) as a fallback when `SWORN_GOOGLE_API_KEY` is unset, or should the spec be amended to say `SWORN_GOOGLE_API_KEY`? The current `envOrAlias` pattern (provider.go:52-57) already does this for OpenAI (`OPENAI_API_KEY` with `SWORN_OPENAI_API_KEY` fallback). The same pattern could apply here. But the spec says `GOOGLE_API_KEY`, so either the code or the spec needs to align.

## Summary

Pins: 7 total — 6 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins: 1, 2 (production dispatch path unreachable without config.go changes; vertex blocked by API-key gate)

## Smaller flags (not pins, worth one-line ack)

- The design has no `design_decisions` array in `status.json`. This is the 6th+ recurrence of this pattern in the trial log. If `sworn designfit` runs against this slice, it will trivially pass (empty array = no Type-1 checks). Not a pin because the design's §2 decisions are all Type-2 (implementation details following an established pattern), but the status.json should still carry the array for consistency.
- The spec's acceptance check #5 ("`Verify()` with a mock transport returns the first text part of the first candidate") — the genai SDK may not expose an injectable `http.RoundTripper` the same way the OAI client does. The implementer should verify the SDK's test surface (e.g., `option.WithHTTPClient` or similar) before assuming a mock transport is straightforward.
- S13 (bedrock-driver, planned) also touches `internal/model/provider.go` and `go.mod`/`go.sum`. Since S13 is `planned` and S12 is `in_progress`, there's no active collision, but the second-lander will need to forward-merge.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no meta-prose, no "here is
     the suggested reply" framing inside the section, just the pasteable ack text itself. -->

TL;DR design follows the Anthropic pattern well but has a critical gap: the production dispatch path (`FromEnv()` in `internal/model/config.go`) is not in the file plan, and the `SWORN_*_API_KEY` gate will block both `google/*` and `vertex/*` before `NewClient()` is ever reached. 7 pins + 3 flags:

1. **Add `internal/model/config.go` to planned_files.** The production path is `FromEnv()` → `swornProviderConfig()` → `NewClient()`. `swornProviderConfig()` reads `SWORN_GOOGLE_API_KEY` (not `GOOGLE_API_KEY`). Add `GoogleCloudProject`/`GoogleCloudLocation` to `swornProviderConfig()`. Resolve the `GOOGLE_API_KEY` vs `SWORN_GOOGLE_API_KEY` naming (see pin 7 / the escalate).
2. **Bypass the `SWORN_*_API_KEY` gate for `vertex/*`.** Vertex uses ADC (no API key). `FromEnv()` line 73-75 will reject `vertex/*` with `SWORN_VERTEX_API_KEY not set` before `NewClient()` is called. Add a keyless-provider bypass for `vertex`.
3. **Verify genai SDK error type before reusing Anthropic string-parse.** The Anthropic driver string-parses `err.Error()` for HTTP status codes. The genai SDK may expose a typed error with a direct status field. Check the SDK; use the direct field if available.
4. **ADR-0007 pre-ratifies `google.golang.org/genai` — no new ADR needed.** Ensure the commit that adds the dep includes a `Co-Authored-By:` trailer per ADR-0007.
5. **Verify genai `UsageMetadata` field names at implementation time.** Spec says `PromptTokenCount` + `CandidatesTokenCount`; confirm these match the SDK.
6. **`parseModelID` for `vertex/*` is verified correct — no action needed.**
7. **Coach decision: `GOOGLE_API_KEY` vs `SWORN_GOOGLE_API_KEY`.** The spec says `GOOGLE_API_KEY`; the production path requires `SWORN_GOOGLE_API_KEY`. Either add a `GOOGLE_API_KEY` fallback in `FromEnv()` (like the `envOrAlias` pattern for OpenAI) or amend the spec to say `SWORN_GOOGLE_API_KEY`.

Flags (not pins): (a) `design_decisions` array absent from `status.json` (6th+ recurrence); (b) verify genai SDK exposes an injectable HTTP transport for mock tests; (c) S13 will also touch `provider.go` + `go.mod` — second-lander forward-merges.

§2 decisions 1-5 ack (all Type-2, pattern-following). §6 questions: none (design has no open questions).

Address pins 1-3 and 5 inline during implementation. Pin 7 (Coach decision on env var naming) — if the Coach chooses the `envOrAlias` fallback pattern, apply it inline; if the spec is amended, that routes to `/replan-release`. Pin 4 is confirmation-only. Then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: IMPLEMENTER_FIX
CONSTITUTIONAL: no
REASON: Pin 1-2 materially change the file plan (config.go must be touched) and the FromEnv() key-gate design must be re-checked before code is safe — the production dispatch path is currently unreachable for both google/* and vertex/*.
-->