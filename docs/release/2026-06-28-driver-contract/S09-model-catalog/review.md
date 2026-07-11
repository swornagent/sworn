# Captain review — S09-model-catalog
Date: 2026-07-11
Design commit: 2b5ce2e1f2996c6cf78ca74cb051e8fcf445a0dd

## Pins

1. [mechanical] §Grounding.HTTP-client-convention — anthropic.go does not use stdlib net/http; it uses the anthropic-sdk-go SDK.
   What I observed: design.md states "every provider driver in `internal/model` (`oai.go`, `ollama.go`, `anthropic.go`) uses stdlib `net/http` + `encoding/json` directly ... `google.go` is the one exception." Live code: `internal/model/anthropic.go` imports `github.com/anthropics/anthropic-sdk-go` and `.../option`, and has no `net/http` import at all — it is a second SDK-based exception, ADR-0007-justified, exactly like `google.go`, not a `net/http` implementation.
   What to ask the implementer: correct the citation in design.md before/while implementing — it does not change the chosen approach (catalog.go's own Anthropic `models/list` client still uses raw `net/http`, consistent with AGENTS.md's stdlib-for-model-client rule), but the false precedent could mislead someone checking for existing HTTP scaffolding to reuse in `anthropic.go` (there is none — the SDK owns dispatch there).

2. [memory-cited] §Design-gap/D1 — D1's ad hoc 7-provider credential-presence check bypassing `internal/driver/registry.Drivers()` repeats the accretion pattern [[project_model_layer_service_refactor]] documents as a live gap.
   What I observed: [[project_model_layer_service_refactor]] finds `internal/model` is "a sparse (provider × capability) matrix, not a service layer" because "each capability filled the cell it needed; nothing forced filling the row" — and flags this as an open Type-1 item (sworn#15 is only the registration half). D1 adds a third, independently-scoped "is provider X configured" check (7 providers, ad hoc against `model.ProviderConfig`) alongside `internal/driver/registry.Drivers()` (4 registered driver identities) and whatever provider-config checks already exist elsewhere in the codebase — the exact shape the memory calls out.
   What to ask the implementer: confirm D1 is a deliberately narrow, locally-scoped exception (design's own rationale: reversible, contained to this one command, registry extension is out of scope per spec) rather than a quiet widening of the accretion the memory already tracks as a gap. No design change required — this is a citation/awareness confirmation, not a rework ask.
   Citation: [[project_model_layer_service_refactor]]

3. [escalate] §Pricing-is-not-part-of-this-slice's-contract — optional pricing-column follow-on is named but has no tracking issue filed.
   What I observed: design.md's own text: "Left out of `catalog.go` entirely; call out in `journal.md` as a Rule 2 deferral (why: no AC requires it, adding it un-asked risks a second, unreviewed capability-shaped surface; tracking: none filed — flag to the Coach at design review whether a follow-on issue is wanted; acknowledgement: pending this design review)." Rule 2 requires why + tracking + acknowledgement all three; "tracking" is explicitly absent, and the design defers the decision to this review.
   What to ask the implementer: the Coach decides — (a) file a tracking issue now for a future OpenRouter-pricing-column addition to `sworn models` output, or (b) explicitly decline tracking (speculative, not committed scope, no issue needed). This is a product-priority call with no code-determinable answer; either answer is non-blocking for S09's implementation (pricing stays out of `catalog.go` either way).

4. [mechanical] §Files-to-touch — spec.json touchpoints lists `cmd/sworn/main.go`; design.md correctly declines to touch it.
   What I observed: `spec.json` `touchpoints` includes `cmd/sworn/main.go`, but design.md states the new command self-registers via `init()` + `command.Register` and explicitly does not touch `main.go`, citing `main.go`'s own header comment. Verified live: `cmd/sworn/main.go:10` reads "Adding a new CLI command never edits this file," and `cmd/sworn/capabilities.go` already follows the `init()` + `command.Register` pattern the design proposes to reuse.
   What to ask the implementer: no action needed — spec.json's touchpoint list is descriptive, not a binding must-touch list, and the design's divergence is independently verified correct. Acknowledge and proceed.

5. [mechanical] §Test-plan — no explicit built-binary reachability artefact named, unlike S05's `cli-run: SWORN_DIRECT=1 sworn capabilities` precedent.
   What I observed: design.md's Test plan is entirely table-driven/fixture-driven (`TestCatalogAnnotations`, `TestListCatalog_*`, `TestModelsCommand*`) through the real `cmdModels` entry point — this satisfies Rule 1's reachability gate (it renders through the integration point, not a leaf), but the design names no explicit reachability-artefact line the way S05's status.json did (`reachability_artifacts: ["cli-run: ...", ...]`).
   What to ask the implementer: confirm the proof bundle (Rule 6) will name `TestModelsCommand`'s fixture-driven end-to-end run through `cmdModels` explicitly as the reachability artefact — live-provider network calls make a real binary smoke run impractical/non-deterministic here, so the fixture-driven end-to-end test is the correct substitute, but it should be named as such rather than left implicit in "tests pass."

Pins: 5 total — 3 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): none — no pin would cause the slice to ship broken if unaddressed; pin 3 is a scope-tracking judgement call, pins 1/4/5 are citation/documentation corrections, pin 2 is an awareness confirmation.

## Summary
Pins: 5 total — 3 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- Fail-closed capability-honesty (AC-02) is well specified: `ToolSupport` is a closed tri-state (`yes`/`no`/`unknown`), Google is unconditionally `unknown` (D4, matches spec.json's own rationale verbatim), and no annotation path ever reads `CatalogModel.ID` — the name-heuristic ban (R-02) is structurally enforced by the table, not just documented.
- Catalog staleness is a non-issue for this slice: caching/auto-refresh is explicitly out of scope, every invocation queries live, so there is no cached state to go stale within a run. Cross-run staleness (a model appearing/disappearing between `sworn models` and later resolution) is inherent to any live listing and correctly left unaddressed.
- All D1–D4 design decisions are self-classified Type-2 with two-option trade-offs and stated rationale — Rule 9's format is satisfied on its face; pin 2 above is the one place a memory suggests the Coach may want visibility beyond the implementer's own narrow-stake framing.
- Registry/prefix citations (4 compiled-in driver entries, `google`/`vertex`/`bedrock`/`azure`/`oci`/`ollama` deliberately unregistered, `Info{Available,Detail}` shape, `ProviderConfig`'s 7 key/host fields, `PriceForModel` signature, `capabilities.go`'s `sort.Slice` pattern) all verified accurate against live code — only the anthropic.go citation (pin 1) was wrong.
- No cross-release ancestry surprises: `git log release/v0.1.0..HEAD -- <every file design.md names>` shows only the S05/S06 commits the design's own "Grounding" section already accounts for; `catalog.go`/`catalog_test.go`/`models.go`/`models_test.go` are genuinely new (zero prior commits).
- No touchpoint collision with any sibling slice's `actual_files` (S01–S08, S13, S14 verified; S10–S12 planned, no overlapping files declared).

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR strong design, well-grounded against live code (~20 citations checked, one wrong). 5 pins, 0 blocking flags:

1. **Fix the anthropic.go citation.** design.md's HTTP-client-convention paragraph says `anthropic.go` uses stdlib `net/http` — it doesn't (it uses `anthropic-sdk-go`, ADR-0007-justified, same shape as `google.go`'s exception). Correct the sentence; the actual design choice (catalog.go's Anthropic list client uses raw `net/http`) is unaffected and correct as-is.
2. **D1 memory awareness.** [[project_model_layer_service_refactor]] documents the exact "each provider need fills its own ad hoc availability cell" pattern D1 repeats (a third availability check alongside `registry.Drivers()`'s 4 entries). Confirmed narrow/reversible per your own rationale — proceed, just noted for the Coach's visibility, no rework needed.
3. **Pricing tracking — Coach decision needed.** File a tracking issue for a future OpenRouter pricing-column addition, or explicitly decline (no issue needed, still out of scope either way for this slice). Non-blocking for S09 code either way.
4. **main.go touchpoint divergence confirmed correct.** spec.json lists `cmd/sworn/main.go` as a touchpoint; your design correctly declines to touch it (verified against `main.go`'s own header comment + `capabilities.go`'s existing `init()`/`command.Register` precedent). No action needed.
5. **Name the reachability artefact explicitly.** In the proof bundle, name `TestModelsCommand`'s fixture-driven end-to-end run through `cmdModels` as the Rule 1 reachability artefact (live-provider calls make a real binary smoke run impractical here — the fixture-driven E2E test is the correct substitute, just say so explicitly rather than leaving it implicit).

Flags (not pins): (a) AC-02 fail-closed handling is structurally sound (`ToolSupport` tri-state, Google always-unknown matches spec rationale, no ID-based heuristics reachable); (b) catalog staleness is a non-issue given caching/auto-refresh is out of scope; (c) all other live-code citations (registry driver count, `ProviderConfig` fields, `PriceForModel` signature, `capabilities.go` render pattern) checked out exactly.

§2 decisions D1 (memory-cited, pin 2), D2/D3/D4 (clean, no memory conflicts) acknowledged. No §6 questions in this design — none to acknowledge.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 3 (pricing-tracking issue: file vs decline) is a product-priority judgement call with no code-determinable answer — Coach authority required before the acknowledgement is final, even though every pin is otherwise non-blocking for implementation.
-->
