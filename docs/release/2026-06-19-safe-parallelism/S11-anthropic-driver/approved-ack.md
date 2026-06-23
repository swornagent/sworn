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
