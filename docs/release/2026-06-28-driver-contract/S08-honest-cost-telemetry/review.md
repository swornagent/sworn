# Captain review — S08-honest-cost-telemetry
Date: 2026-07-10
Design commit: 884b5a8ca09d8f28c505c7043489229a85322d5a (design.md content unchanged since 76c44c1; HEAD only advanced via the S14-blocked-terminal replan commit, which sits serially after S08 and does not touch this slice's files)

## Pins

1. [escalate] Risks/open-items #1, §D1/D2 — status.json carries no `design_decisions` at all; D1/D2's unverified CLI-behaviour inference needs Coach-recorded Type-1 ratification before code.
   What I observed: design.md's own "Risks / open items for Captain review" #1 explicitly flags D1 (`claude.go`: `TotalCostUSD == 0` → `CostSource="subscription"`) and D2 (`codex.go`: `Usage != nil` → `CostSource="subscription"`) as "informed guesses about external CLI behaviour, not verified against a live binary," and asks for "explicit ratification, not silently landing as fact." But `status.json` has **no `design_decisions` field at all** — not even a draft entry with `stake_class`. This is the identical gap [[project_driver_contract_recut]] recorded as a valid Captain pin on this exact track's sibling S04 (2026-07-09: "status.json has NO design_decisions (Rule 9 gate can't pass); D1/D5 look Type-1"), and the resolution pattern there (S04, S02, S07) was always the same: classify Type-1, record options + human_decision in status.json via a `captain-proceed.md` pin, then proceed. D1/D2 carry real financial-honesty stakes: a wrong classification would mislabel real API spend as subscription-covered — under-reporting true cost, which is precisely the dishonesty sworn#70 (this slice's own rationale) exists to close. This is architecturally significant because the CostSource vocabulary this slice establishes binds every future driver (mirroring the ErrKindAuth precedent already binding on this track).
   What to ask the implementer: Populate `status.json`'s `design_decisions` with D1 and D2, `stake_class: "Type-1"`, both proposed options (the chosen inference vs. leaving both `"unknown"` until live-binary verification), and get Brad's `human_decision` recorded (ratify, reject, or accept-with-caveat) before `in_progress` — same mechanism used for S02 pin 2, S04 pins 1/2/6, and S07 pin 1.

2. [escalate] Step 1 Part A (§AC-02 vs design's Approach section) — the `CostSource="provider"` branch AC-02 requires is narrower than what the design implements, and the design asks the Coach to resolve which way.
   What I observed: spec.json AC-02 reads: "...when the provider itself reports a cost (Anthropic resp.CostUSD), that figure SHALL be carried with CostSource="provider" instead of recomputed." The design's own "CostSource vocabulary" section and "Risks / open items" #2 confirm this branch is **not implemented** because it's unreachable with the wired client set today (verified independently: `anthropic.go`'s `Chat()` at line 156 computes `CostUSD` from the pricing table, it never receives a provider-reported billing figure over the wire). The design explicitly proposes two resolutions — amend AC-02 in spec.json to mark `"provider"` reserved-but-unreachable, or accept the narrowing at design review — and defers the pick to the Captain/Coach rather than choosing unilaterally. This is the same amend-vs-accept pattern S07's D4 used for AC-01 (ratified live in that review session).
   What to ask the implementer: Coach picks one of the two options design.md proposes (amend AC-02's text via a mechanical spec.json correction, or accept the narrowing as a recorded Type-1 `design_decisions` entry) before code. Either is fine; only the "neither, ship silently" path is not.

3. [mechanical] Rule 2 deferral check — sworn#89 (Google/Bedrock pricing-lookup duplication, out of scope).
   What I observed: design.md defers Google/Bedrock's identical duplicate-lookup shape to sworn#89. Independently verified via `gh api repos/swornagent/sworn/issues/89`: filed, open, and its body states why (different SDK usage types — `genai.GenerateContentResponseUsageMetadata`/`types.TokenUsage`, not `model.UsageBlock` — needs a type-conversion shim, not a same-signature redirect; also not reachable from the in-process `Dispatch` path today), tracking (the issue itself), and acknowledgement (design.md, pending this review). All three Rule 2 elements present.
   What to ask the implementer: none — confirm and acknowledge; no further action needed on this pin.

4. [mechanical] §"Files to touch" — `agent.Run`'s public signature change (drop the `float64` cost return) blast-radius claim.
   What I observed: design.md flags this itself as "worth the Captain's eyes" since it's a public-API signature edit, not a same-signature internal rewrite. Independently grepped the entire worktree (not just the two files design.md names) for every call site: `grep -rn "agent\.Run(" --include="*.go"` returns exactly two hits — `internal/driver/inprocess/inprocess.go:183` (`text, _, _, err :=`) and `internal/driver/inprocess/inprocess_verify.go:43` (`text, _, transcript, err :=`) — matching design.md's claim exactly, both already named in touchpoints, both already discard the cost return today.
   What to ask the implementer: none — blast radius confirmed contained to the two already-listed files; proceed as designed.

5. [memory-cited] D3 — named `driver.CostSource*` constants mirroring the existing `ErrKind*` pattern.
   What I observed: design decision D3 proposes `CostSourceProviderReported`/`CostSourcePricingTable`/`CostSourceCLI`/`CostSourceSubscription`/`CostSourceUnknown` constants in `internal/driver/driver.go`, explicitly "mirroring the existing `ErrKind*` constant pattern in the same file." This aligns with [[project_provider_error_taxonomy]]'s established convention (`model.Error{Kind}` typed vocabulary) and the binding recorded in [[project_driver_contract_recut]] (2026-07-03): "Any future driver... MUST reuse [the named constant], not invent its own label." Named constants replacing scattered string literals is exactly this project's precedent for cross-cutting driver vocabularies.
   Citation: [[project_provider_error_taxonomy]], [[project_driver_contract_recut]]

6. [memory-cited] AC-05 — `state.Dispatch` `cost_source` enrichment.
   What I observed: AC-05 (adding `CostSource` to `state.Dispatch`, threaded through `verify.go`/`verdict.go`/`slice.go`'s five dispatch sites) is exactly Day-1 design item 1 of [[project_telemetry_eval_foundation]]: "Enrich `state.Dispatch`: + `duration_ms`, `input_tokens`, `output_tokens`, `provider`, `outcome`... fix model attribution; wire `modelPricing` for real cost." `DurationMS`/`InputTokens`/`OutputTokens`/`ModelIDConfirmed` are already present on `Dispatch` (confirmed at `internal/state/state.go:83-97`) — `cost_source` is the one field that memory's Day-1 design called for and this slice was scoped to close. Confirms the design is on-thesis for the eval/routing-moat data model this memory traces to sworn#70, not a one-off addition.
   Citation: [[project_telemetry_eval_foundation]]

## Summary

Pins: 6 total — 2 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins (if any): none would ship the slice *broken* if unaddressed in the mechanical sense (all code citations verified accurate — this design is unusually well-grounded), but pins 1 and 2 gate `in_progress`: coding against an un-ratified financial-honesty inference (D1/D2) or an un-reconciled AC-02 narrowing risks the exact class of dishonest-cost defect sworn#70 exists to close, discovered only after code is written.

## Smaller flags (not pins, worth one-line acknowledgement)

- `internal/agent/agent.go:182`'s existing comment on the doomed `computeCost` says "the model package's pricing table is authoritative (**S10**)" — a stale slice reference (should read S08, not S10-conformance-sit, which doesn't touch this file). Moot once the function is deleted per AC-01's approach; flagging only in case a partial edit leaves the comment behind.
- Verified R-01's mitigation (TestPricingUnified, hand-diffed rate parity) and R-02's mitigation (additive `omitempty` field, `additionalProperties:true`) both hold exactly as spec.json's Risks section requires — clean match, no pin needed.
- The `claude-sonnet-5` introductory-pricing tracked note (sworn#41, flip-date 2026-08-31) exists in both `pricing.go` (being deleted) and `anthropic.go`'s `anthropicPricing` map (survives unification) — confirmed byte-identical comment text in both, so AC-01's unification does not silently drop that tracked footnote.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is unusually well-grounded — every code citation checked (verify.go's deleted `computeAgenticCost`, the four pricing-lookup implementations, `agent.Run`'s two call sites, `claude.go`/`codex.go`'s literal `costSource()` strings, all five `state.Dispatch{}` sites in slice.go) held exactly against live code. 6 pins + 3 flags, none requiring a design rewrite:

1. **Record D1/D2 as Type-1 in status.json.** `status.json` has no `design_decisions` field yet. Add D1 (claude.go `TotalCostUSD==0`→`"subscription"`) and D2 (codex.go `Usage!=nil`→`"subscription"`) with `stake_class: "Type-1"`, both options, and Brad's `human_decision` before writing code — same mechanism as S02/S04/S07's captain-proceed.md pins.
2. **Resolve the AC-02 "provider" branch narrowing.** Either amend AC-02's spec.json text to mark `CostSource="provider"` reserved-but-unreachable with the current client set, or record accepting the narrowing as a Type-1 design_decision. Coach picks; either is fine.

Flags (not pins): (a) `agent.go:182`'s stale "S10" comment reference is moot once the function is deleted — confirm it doesn't survive a partial edit; (b) R-01/R-02 mitigations confirmed satisfied as designed; (c) the sworn#41 claude-sonnet-5 flip-date tracking note survives the pricing.go deletion (present in `anthropic.go`'s surviving map) — no action needed.

§2 decisions D3 (named CostSource* constants) and AC-05 (state.Dispatch enrichment) [memory-cited] — [[project_provider_error_taxonomy]] and [[project_telemetry_eval_foundation]] acknowledged, both align cleanly. sworn#89 (Google/Bedrock deferral) [mechanical] confirmed filed correctly — acknowledged, no action needed. `agent.Run` signature-change blast radius [mechanical] confirmed contained to the two already-listed call sites — acknowledged, no action needed.

Pins 1–2 need a live Coach ratification (they're judgment calls on financial-honesty inference and an AC-narrowing acceptance, not facts the code settles) before `in_progress`; pins 3–4 and the memory citations are pre-acknowledged above and need no further action. Once pins 1–2 are ratified, address them inline (record the design_decisions entries + the AC-02 resolution) during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pins 1-2 are genuine Coach-authority judgment calls (unverified-against-live-binary financial-honesty inference for D1/D2; AC-02 scope-narrowing acceptance) with no single determinable answer — both explicitly self-flagged by the design for Captain/Coach ratification, matching the exact captain-proceed.md pattern already used three times on this track (S02, S04, S07).
-->
