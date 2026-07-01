# Captain review — S06-model-pricing-registry
Date: 2026-07-01
Design commit: 7a3fb74e9aa838a0939d5a23fa43d2430841884f

## Pins

1. [mechanical] §2b (Rule 9 design-fit gate) — `status.json` has no `design_decisions` field.
   What I observed: `design.md` states "Stakes (Rule 9): Type-2 throughout" with a one-line
   rationale in prose, but `status.json` (checked live: `slice_id, release, track, state,
   covers_needs, spec_path, start_commit, effort_complexity, verification` only) carries no
   `design_decisions` array. Confirmed this is a release-wide gap, not S06-specific — S02, S03,
   S04, S05 also lack the field; only the already-`verified` S01 has it populated (schema
   reference: `S01-d6-record-reconciliation/status.json`, 5-entry array with
   `choice/stake_class/options/human_decision/rationale`). Because the gap is repo-wide it does
   not indicate a defect specific to this design, but the Rule 9 gate is meant to be
   machine-checkable and currently is not for this slice.
   What to ask the implementer: add a single-entry `design_decisions` array to `status.json`
   recording the Type-2 classification already justified in `design.md`'s "Stakes" line —
   `{"choice": "...", "stake_class": "Type-2", "rationale": "..."}` (no `options`/`human_decision`
   needed for Type-2 per the S01 schema). Apply inline; this doesn't change the design.

2. [memory-cited] §2 (Approach) Design-level risk #3 — "No map consolidation."
   What I observed: the design explicitly declines to consolidate the three duplicate Anthropic
   pricing maps ("Consolidation is the deferred Type-1 refactor; doing it here would exceed slice
   scope and change the stakes classification"). This matches [[project_model_layer_service_refactor]]
   verbatim: that memory independently identifies `internal/model` as a sparse provider×capability
   matrix needing a Type-1 wire-vs-usage service-layer refactor, and explicitly places it as a
   "foundation-track item of the combined post-R3 release" — i.e. a separate release, not this one.
   What to ask the implementer: none — acknowledging the citation is sufficient. This is the
   correct scope boundary per the standing memory.
   Citation: [[project_model_layer_service_refactor]]

Pins: 2 total — 1 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none — neither pin would ship the slice broken if unaddressed.

## Summary

Design is unusually well-audited for a "chore" slice: the implementer independently verified
live map contents, the `PriceForModel` walk order, and existing test-assertion coverage before
writing `design.md`, and every factual claim I re-checked against the current worktree held up
(see Smaller flags). Two pins, both apply-inline, neither reopens the design.

## Smaller flags (not pins, worth one-line acknowledgement)

- Confirmed independently (`client.go:72-90`) that `PriceForModel` checks
  `modelPricing`(oai.go) → `anthropicPricing` → `googlePricing` → `bedrockPricing`, exactly the
  order the design claims, and that neither `modelPricing` nor `googlePricing` contains any
  Anthropic key — so the design's "three duplicate maps, nothing else to touch" inventory is
  complete, not an unverified inference. Design-level risk #1 (implementer's own flag: "confirm
  no client.go edit is the intended reading") is resolved: correct, no edit needed.
- Confirmed live map contents match the design's "current entry" table exactly — all three maps
  currently hold `claude-opus-4-8: {15.00, 75.00}` and no `claude-sonnet-5` key anywhere.
- Confirmed AC-06's audit claim independently: grepped `anthropic_test.go` and `bedrock_test.go`
  — the only `claude-opus-4-8` reference (`anthropic_test.go:132`) constructs a client and asserts
  no price; all hardcoded price assertions in both files (and in `pricing_test.go`) cover
  `claude-sonnet-4-6`/`claude-haiku-4-5` only. The design's "live check already run" claim holds;
  AC-06's audit should be a no-op confirmation, not new edits.
- Verified `ComputeCost`'s actual division (`tokens/1_000_000 * pricePer1M`) — the design's test
  assertions ($12.00 for sonnet-5, $30.00 for opus-4-8 vs. the old $90.00) are arithmetically
  correct against the real function, not a guessed API.
- `gh` is installed and authenticated in this environment (`sawy3r`, github.com) — AC-07's primary
  path (file a GitHub issue for the 2026-08-31 price flip) will work without falling back to a
  punch-list entry.
- Sequencing reminder (not a pin): the AC-03 example comment in `design.md` carries a literal
  `<issue/punch-list ref>` placeholder. AC-07 already sequences "file the issue, then cite its
  number" correctly in prose — just flagging so the implementer doesn't commit the literal
  placeholder text by writing the comment before running `gh issue create`.
- `spec.json` has no `risks` key. Confirmed release-wide (S01–S06 all lack it) — not an S06-specific
  spec-completeness defect, so no pin under the Step-1B risk-drift check.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR strong design, unusually well pre-verified. 2 pins, 6 flags:

1. **Populate `status.json.design_decisions`.** Add a one-entry array recording the Type-2
   classification already stated in `design.md`'s "Stakes" line (`choice`/`stake_class:
   "Type-2"`/`rationale` — no `options`/`human_decision` needed for Type-2). Mirrors the schema
   already in `S01-d6-record-reconciliation/status.json`.
2. **No-consolidation scope boundary confirmed against memory.** Citing
   [[project_model_layer_service_refactor]] — the three-map consolidation is correctly deferred
   to the separate post-R3 foundation-track refactor, not this slice.

Flags (not pins): (a) `PriceForModel` transitive resolution confirmed, no `client.go` edit needed;
(b) current map contents match the design's table exactly; (c) AC-06's "no hardcoded opus-4-8
price assertions exist" claim confirmed by grep; (d) AC-05's dollar-figure test assertions are
arithmetically correct against the real `ComputeCost`; (e) `gh` is available/authenticated, so
AC-07's GitHub-issue path works without the punch-list fallback; (f) don't let the AC-03 example
comment's `<issue/punch-list ref>` placeholder land literally — file the issue first, then cite
the real number.

§2 decisions: sonnet-5 rate add, opus-4-8 correction, and no-consolidation ([[project_model_layer_service_refactor]]-cited) all acknowledged. No §6/design-level-risk items remain open — all three were resolved during this review (see Smaller flags).

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline (a status.json field to populate, a memory citation to acknowledge); every implementer-flagged design risk was independently confirmed correct against live code during this review. No product/architectural judgement call remains.
-->
