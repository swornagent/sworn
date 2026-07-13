# Captain review — S03-xai-driver
Date: 2026-07-12
Design commit: 7d47446c259b95d4fabb1d3b3037aa4c15d6ae26

## Pins

1. [mechanical] §3 (catalog.go) / §D3 — Wrong alphabetical placement for the catalog def.
   What I observed: The Files table says add `{"xai", …}` to `catalogProviderDefs`
   "(`:86-94`, alphabetical order → between mistral/ollama)". `xai` sorts LAST
   (after `openrouter`), not between `mistral`/`ollama`. `catalogProviderDefs`
   (catalog.go:85-94) carries the comment "fixed alphabetical iteration order —
   diff-stable output", and `TestCatalogProviderNames` (catalog_test.go:367-378)
   asserts the exact ordered slice `{"anthropic","google","groq","mistral",
   "ollama","openai","openrouter"}`. Inserting `xai` between mistral/ollama breaks
   both the invariant and the test.
   What to ask the implementer: append `xai` LAST in `catalogProviderDefs`, and
   extend the `TestCatalogProviderNames` `want` slice to 8 entries with `"xai"`
   last. Apply inline.

2. [mechanical] status.json — `design_decisions` array is absent; Rule 9 design-fit gate reads it.
   What I observed: The design.md declares D1–D4 (all Type-2, no Type-1) in prose,
   but `status.json` carries no `design_decisions` field. The Rule 9 design-fit
   gate (captain Step 2b) and the sibling convention (e.g.
   2026-06-27-conformance-foundation/S05 status.json: `design_decisions[]` with
   `id`/`classification`/`description`/`rationale`/`acknowledged`) require the
   decisions recorded in status.json, not only in design.md.
   What to ask the implementer: record D1–D4 in `status.json` `design_decisions`
   (all `classification: "Type-2"`, with description + rationale) at the
   `in_progress` transition so the design-fit gate passes. The Type-2
   classifications are sound — each mirrors an in-place sibling pattern inside the
   already-ratified driver-contract / ADR-0011 / pricing architecture; no Type-1
   choice is present.

3. [mechanical] §"Structured-output path" / AC-03 — httptest proves marshalling, not xAI acceptance.
   What I observed: R-01 is marked "RESOLVED at design" on the strength of the
   cited doc (`docs.x.ai/.../structured-outputs`). The AC-03 test is an httptest
   server returning valid JSON — it proves *our* request build + response parse
   against `StructuredResponseFormat`, NOT that xAI's live API honours strict
   `json_schema` `response_format`. `StructuredResponseFormat` is confirmed as
   exactly the mode `openai-completions` uses (provider.go:119; structured.go:29-38),
   so the code path is real — the open question is only the remote's behaviour.
   What to ask the implementer: at implementation, either run one live
   `ChatStructured` smoke against `api.x.ai` with `XAI_API_KEY` (record it in the
   proof bundle's reachability section), or explicitly note that strict-schema
   acceptance is doc-confirmed only and that D2's `StructuredToolCall` fallback is
   the containment if the live call rejects it — with the declared roles then
   reflecting what actually works (spec R-01 mitigation). Do not let the mock
   server stand in for provider acceptance.

4. [mechanical] §6 / Step 6 — cross-track shared-file surface on `internal/model/` with S02.
   What I observed: S02-model-response-structured (track T1-conformance, state
   `planned`) declares `internal/model/` as a touchpoint; S03 edits
   `internal/model/{provider,config,client,catalog}.go` (+ `oai.go`/`xai.go`).
   Both are `planned` in *different* tracks, so no active collision today, but if
   the tracks run in parallel this is a shared-package edit surface.
   What to ask the implementer: no action needed now (both planned, S03 confines
   its hunks to a new `case`/map/def per file). Flag for the merge step: whichever
   track lands in `release-wt` second re-runs `go test ./internal/model/...` and
   confines its diff, so the overlap is not read as a surprise at verify/merge.

5. [memory-cited] §D1/D2 — role honesty (implementer/verifier/captain) rests on riding the oai chat client.
   What I observed: The design declares impl/verify/captain for `xai/` by joining
   `chatPrefixes` (one shared `inprocess.NewOAIChat` instance →
   `model.ResolveLoopClient` → `NewClient` → `*OAI`). [[project_model_layer_service_refactor]]
   records "agentic implementer/verifier run on OpenAI-compatible only" — xAI IS
   OpenAI-compatible on that exact oai chat path, so inheriting the full role set
   is honest, not aspirational. ADR-0011 / [[project_keystone_structured_outputs]]
   is the structured-output contract the `StructuredResponseFormat` mode satisfies.
   What to ask the implementer: acknowledge the citation — the declared roles are
   honest precisely because `xai/` reuses the oai Chat client; if the D2 fallback
   is ever taken and any role stops working, the declared role set must be
   narrowed to match.

## Summary

Pins: 5 total — 4 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (would block a gate/test if unaddressed): Pin 1 (fails
`TestCatalogProviderNames`), Pin 2 (fails the Rule 9 design-fit gate). Both are
trivial apply-inline corrections the implementer's own test/gate run backstops —
no pin ships the slice silently broken.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) `cmd/sworn/capabilities.go` and `cmd/sworn/models.go` are spec touchpoints
  but need NO source edit — the prefix list (capabilities.go:43-57) and the
  `--provider` set (via `CatalogProviderNames()`) are derived dynamically from the
  registry/catalog, so `xai/` surfaces automatically once it joins `chatPrefixes`
  and `catalogProviderDefs`. Their touchpoint status is behavioural, not an edit
  site. `capabilities_test.go` uses substring `Contains` checks, so existing
  assertions won't break; just add `xai`-present coverage.
- (b) R-3 golden-list churn is correctly pre-flagged: `registry_test.go` needs
  `xai` added to the `want["oai-inprocess"]` set (:52-58), the unknown-prefix
  error list (:105-110), and the `wantChat` CSV (:300). Expected, not scope creep.
- (c) Pricing (D3): key `PriceForModel` on the bare model id (`grok-4.5`), matching
  the existing per-provider maps — `NewClient` passes the post-prefix `model`, so
  the bare-key convention is correct.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong, well-grounded design — every code citation checks out against live
code, the reuse-the-oai-chat-driver approach is the honest sibling pattern, and
R-01/R-02 are handled correctly. 5 pins + 3 flags, all apply-inline; proceed.

1. **Catalog placement.** In `catalogProviderDefs`, append `xai` LAST (it sorts
   after `openrouter`, not "between mistral/ollama" as the design says), and
   extend `TestCatalogProviderNames`'s `want` to 8 entries with `"xai"` last —
   otherwise the fixed-alphabetical invariant + test break.
2. **Record design decisions.** Write D1–D4 into `status.json` `design_decisions`
   (all `Type-2`, id/classification/description/rationale) at the `in_progress`
   transition so the Rule 9 design-fit gate passes. The all-Type-2 read is sound.
3. **Structured-output proof.** The AC-03 httptest proves our marshalling, not
   xAI's acceptance of strict `json_schema` `response_format`. Add one live
   `ChatStructured` smoke against `api.x.ai` (record in proof reachability), or
   explicitly note strict-schema is doc-confirmed only with D2's
   `StructuredToolCall` as the contained fallback and roles narrowed if it's taken.
4. **Shared-package sequencing.** S02 (T1-conformance, planned) also touches
   `internal/model/`. No action now; at merge, second-lander re-runs
   `go test ./internal/model/...` and confines its hunk so the overlap isn't a
   surprise.
5. **Role-honesty citation.** Acknowledge [[project_model_layer_service_refactor]]:
   the impl/verify/captain roles are honest because `xai/` rides the oai Chat
   client (OpenAI-compatible). If the D2 fallback is ever taken and a role stops
   working, narrow the declared role set to match.

Flags (not pins): (a) `capabilities.go`/`models.go` need no source edit — both
surfaces are dynamic from the registry/catalog; substring tests won't break;
(b) R-3 golden-list churn (`registry_test.go` want-set, error list, `wantChat`
CSV) is expected, not scope creep; (c) key `PriceForModel` on the bare `grok-4.5`
id, matching the per-provider-map convention.

§2 decisions D1/D2 ([memory-cited]), D3/D4 (clean) acknowledged. §6 open items
addressed (structured path resolved in-design; availability no-dispatch).

Address pins 1–5 inline during implementation, then proceed to in_progress.

## Non-gating findings

None requiring a GitHub issue — every pin is in-scope for this slice and
apply-inline. No out-of-scope defect surfaced during review.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Design is sound and citation-accurate; all 5 pins are apply-inline mechanical/memory-cited corrections (catalog ordering, design_decisions record, structured-output live proof, shared-file sequencing, role citation) that don't change the approach — Verifier (Rule 7) backstops.
-->
