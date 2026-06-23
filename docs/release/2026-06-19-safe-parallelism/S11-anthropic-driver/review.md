# Captain review — S11-anthropic-driver
Date: 2026-06-23
Design commit: 6e54df935ed90e0592aac27c0dc26d3c07f33087

## Pins

1. [mechanical] §1 / spec-Risk-2 — Design §4 NOT-doing omits the OAI-import segregation required by spec Risk 2
   What I observed: Spec Risk 2 says "anthropic.go must import only the Anthropic SDK types, not any OAI types." Design §4 lists three NOT-doing items (no Chat(), no bedrock routing, no empty-key fallback) but never explicitly states that anthropic.go will not import internal/model/oai.go or OAI package types.
   What to ask the implementer: Add a §4 NOT-doing line: "anthropic.go will not import the OAI struct or any openai-compat types; Anthropic SDK types only." One line; confirm before writing the import block.

2. [memory-cited] §2.Decision-1 — SDK dep choice aligns with [[project_dep_policy]] and [[feedback_dep_justification_test]]; ack confirms
   What I observed: Decision 1 cites ADR-0007 (pre-ratifies `github.com/anthropics/anthropic-sdk-go` for S11) and justifies the dep as replacing auth header construction, JSON wire format, and error response parsing. This meets the component-replacement test: stateful, error-prone-to-hand-roll, from the provider's own official library with near-zero added surface.
   What to ask the implementer: Confirm the dep is pinned to a specific minor version in go.mod (per spec Risk 1 and ADR-0007). No design change needed; pin during `go get`.
   Citation: [[project_dep_policy]], [[feedback_dep_justification_test]]

3. [memory-cited] §2.Decision-3 — Error taxonomy routing claimed but extraction path for SDK errors not specified; [[project_provider_error_taxonomy]] requires this to work for S44
   What I observed: Decision 3 says "HTTP-level errors from the SDK are classified through the existing `ClassifyHTTP`/`NewProviderError` path." `NewProviderError(status int, provider, model string, body []byte)` takes raw bytes. The OAI driver calls this after doing its own HTTP round-trip. The anthropic-sdk-go returns typed error objects (e.g. `*anthropic.APIStatusError` with a `StatusCode` field), not raw HTTP buffers. The design does not specify how to bridge: type-assert the SDK error → extract StatusCode → call `NewProviderError(code, "anthropic", model, nil)`. If the implementer passes the SDK error opaquely, `ClassifyHTTP` never runs and all Anthropic errors land as `KindOther` — breaking S44's terminal/transient retry policy.
   What to ask the implementer: Before writing the error path: grep the anthropic-sdk-go source for its error type (likely `anthropic.APIStatusError`) and confirm it exposes a `.StatusCode` (or equivalent) field. Document the extraction in a one-line comment in the error path. Confirm `NewProviderError(statusCode, "anthropic", model, nil)` is the call site.
   Citation: [[project_provider_error_taxonomy]]

4. [mechanical] §5 / test-coverage — TestAnthropicVerify_APIError only asserts non-nil error; Kind not asserted, so taxonomy integration can ship silently wrong
   What I observed: The spec's required test `TestAnthropicVerify_APIError` says "mock returns 429; assert Verify returns non-nil error." Design §5 mirrors this without adding a Kind assertion. If the SDK error extraction path (Pin 3) is wired incorrectly, this test still passes while `IsTerminal`/`IsTransient` callers receive wrong Kind. A `KindRateLimit` assertion on the returned error would catch the failure at the model-layer boundary.
   What to ask the implementer: Add to `TestAnthropicVerify_APIError`: `var me *model.Error; if !errors.As(err, &me) || me.Kind != model.KindRateLimit { t.Fatalf(...) }`. This is the minimal assertion that confirms the taxonomy bridge is live.

## Summary

Pins: 4 total — 2 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: Pin 3 + Pin 4 together — if the SDK error extraction is wrong AND the test doesn't assert Kind, the taxonomy integration ships silently broken for all Anthropic errors. S44 retry policy depends on it.

## Smaller flags (not pins, worth one-line ack)

(a) `design_decisions` field absent from status.json — consistent with S10 and other T5 slices in this release; not a block but the designfit gate passes trivially empty. Pattern is recurring (noted in S10, S21, S23 trial-log entries).

(b) §5 end-to-end claim ("verified by the existing run-loop tests in `internal/run/`") is aspirational. No existing `internal/run/` test calls `model.NewClient` with an `anthropic/*` ID. The spec-required reachability artefact (live integration test) covers this adequately; the run-loop claim is surplus and shouldn't be cited as evidence.

(c) OAI unknown-model zero-cost behavior (cited in Decision 2 for unknown `claude-*` models): confirmed by reading `oai.go:264-266` — `if !ok { return 0 }`. Design claim is accurate.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is solid — 4 apply-inline pins before code. 2 mechanical, 2 memory-cited, 0 escalate.

1. **OAI-import segregation.** Add a §4 NOT-doing entry: "anthropic.go will not import internal/model/oai.go or any OAI struct types — Anthropic SDK types only." One line; confirm before writing the import block. (Spec Risk 2)

2. **Minor-version pin.** When running `go get github.com/anthropics/anthropic-sdk-go`, pin to a specific minor version (e.g. `@v0.x.y`) in go.mod. This is already spec-required (Risk 1) — flag confirms it's not overlooked. Decision 1 cites ADR-0007; confirmed.

3. **SDK error extraction path.** Before writing the error-handling code: grep the anthropic-sdk-go for its typed error type (likely `anthropic.APIStatusError`), confirm it exposes a `StatusCode` field, then use `NewProviderError(statusCode, "anthropic", model, nil)` as the call site. Add a one-line comment naming the type being unwrapped. This is how Decision 3's `ClassifyHTTP`/`NewProviderError` claim is implemented with a typed SDK (vs. the OAI driver's hand-rolled HTTP round-trip).

4. **Kind assertion in APIError test.** In `TestAnthropicVerify_APIError`, after asserting non-nil error, add: `var me *model.Error; if !errors.As(err, &me) || me.Kind != model.KindRateLimit { t.Fatalf("expected KindRateLimit, got %v", err) }`. This is the gate that confirms Pins 3 is wired correctly.

Flags (not pins): (a) `design_decisions` absent from status.json — pattern consistent with S10/S21/S23, not a block; (b) §5 "run-loop tests" end-to-end claim is aspirational — live integration test is the real reachability artefact; don't cite run-loop tests in proof.md.

§2 decisions 1 (SDK), 2 (pricing table), 4 (first text block), 5 (no Chat) ack. Decision 3 (error taxonomy) ack subject to Pin 3 resolution. §6 empty ack.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline corrections (import segregation note, version pin, SDK error extraction comment, Kind test assertion) — none require redesign or re-review; Verifier (Rule 7) backstops.
-->
