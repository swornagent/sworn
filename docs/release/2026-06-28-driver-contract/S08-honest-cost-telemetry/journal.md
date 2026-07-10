# Journal — S08-honest-cost-telemetry

## 2026-07-11 — Implementer session: design_review → implemented

**Entry state.** Coach acknowledgement already committed (`captain-proceed.md`
@4753eb3, verdict PROCEED) on `track/2026-06-28-driver-contract/T4-resolution-loop`.
Verified the ack exists and cites the exact commit before starting (Rule 7
handoff discipline — the implementer does not re-litigate a design review).

**Design_decisions recorded before code (Rule 9 gate).** `status.json` had no
`design_decisions` field despite review.md pins 1–2 requiring Type-1 Coach
records. Added D1/D2 (both Type-1, Coach-ratified fail-closed) and D3 (Type-2,
confirmed-as-designed) before transitioning to `in_progress`, citing
captain-proceed.md@4753eb3 as `decision_ref`.

- **D1/D2 fail-closed ruling.** The task brief's Coach ruling was stricter
  than design.md's own proposal: design.md proposed implementing a
  `TotalCostUSD==0 -> "subscription"` inference for claude.go (D1 in the
  design) and a `Usage!=nil -> "subscription"` inference for codex.go (D2 in
  the design), flagging both as "informed guesses... not verified against a
  live binary" and asking the Captain/Coach to ratify. The Coach ratified the
  OPPOSITE of the design's proposal: ship `CostSourceUnknown` universally for
  both — no positively identified, testable marker exists in the currently
  observed CLI output for either binary, so neither inference is implemented.
  This is the binding instruction for this session (task brief pins 1/2) and
  is what got built.

**AC-02 "provider" branch.** Confirmed already amended on this branch
(`aaa2861` replan commit, forward-merged via `053e624`) before this session
started — spec.json's AC-02 text already reads "reserved... no live dispatch
path claims CostSource=provider in this slice." No spec.json edit was needed
this session; implementation follows the amended text as-is.

**Implementation order.**

1. `internal/driver/driver.go` — added `CostSource*` named constants (D3),
   fixed the stale `Result.CostSource` doc comment.
2. AC-01: deleted `internal/model/pricing.go` (the `Pricing` map/`ComputeCost`
   func); redirected `anthropic.go` (`Verify`+`Chat`), `oai.go` (`Verify`),
   `openai_responses.go` (`Verify`) to `ComputeCostFromTokens`; deleted the
   now-dead `computeAnthropicCost` and `oai.go`'s local `computeCost`.
   Rewrote `pricing_test.go` against the surviving API and added
   `TestPricingUnified` (enumerates every model key across all four provider
   maps, asserts `PriceForModel` resolves each to the exact source-map rate —
   the R-01 "one path, no drift" regression guard).
3. `internal/agent/agent.go` — deleted the flat-rate `computeCost` and
   dropped the `float64` cost return from `Run`'s signature (pre-authorised
   per the task brief; blast radius independently re-confirmed at exactly the
   two call sites `inprocess.go:183` and `inprocess_verify.go:43`, both
   already discarding the value). Updated all 7 call sites in
   `agent_test.go`.
4. AC-02: rewrote `inprocess.go`'s `economics()` to compute `CostUSD` from
   the CONFIRMED response model-id (`meter.modelID`) and the true
   accumulated token split via `model.PriceForModel`/`ComputeCostFromTokens`;
   `CostSource=pricing-table` on a hit, `CostSource=unknown` + a stderr
   warning naming the model on a miss (AC-04). Deleted
   `nominalCostPerToken`. The `"provider"` branch is deliberately NOT
   implemented — no wired client returns a real provider-reported figure
   today; an unreachable branch would be untestable dead code.
5. AC-03: rewrote `claude.go`'s `costSource()` to the D1 ruling — only a
   strictly positive `TotalCostUSD` earns `CostSourceCLI`; nil OR an explicit
   zero both classify `CostSourceUnknown`. Rewrote `codex.go`'s
   `costSource()` to the D2 ruling — always `CostSourceUnknown` (codex never
   carries a cost field in any documented shape, and `Usage!=nil` is not a
   positively identified subscription marker). Added a new
   `fakeClaudeZeroCost` fixture + `TestClaudeEnvelopeExplicitZeroCostIsUnknown`
   to prove D1's ambiguous-zero case explicitly (the pre-existing minimal-
   envelope test only covered the nil case).
6. AC-05: `driver.Result.CostSource` already existed and is populated by
   steps 4/5 above — no driver-side struct change needed here. Added
   `CostSource` to `verdict.Result` and `state.Dispatch` (additive,
   `omitempty`); threaded it through
   `verify.go`'s `withDispatchEconomics` and `acceptStructuredVerdict`, and
   through all 5 `state.Dispatch{}` construction sites in `slice.go`
   (captain, implementer, verifier carry it from their driver.Result /
   verdict.Result source; the two synthetic zero-cost sites — proof-absent,
   first-pass — correctly leave it empty, since no driver call was made).
   Added `TestRunSlice_CostSourceThreadedToStatusJSON` (Rule 1 reachability
   artefact — drives the real `RunSlice` entry point, not a leaf
   `state.Dispatch` struct literal) proving two distinct `CostSource` values
   reach the written `status.json` and that the file still validates against
   `slice-status-v1`.
7. Updated the pre-existing `"estimated"`/`"provider-reported"` fixtures in
   `captain/review_test.go` and `verify/verify_agentic_test.go` to the new
   named-constant vocabulary (not required for those tests to pass, but the
   old strings are no longer valid vocabulary members).

**Full-suite proof.** `go test -count=1 -timeout 300s ./...` — all 45
packages green, zero regressions in any package this slice did not touch
(the project's known newline-eating-edit hazard: always confirmed via the
full suite, not the named-package subset alone).

**Rule 2 deferrals recorded in proof.json `not_delivered`:** D1's
subscription inference (claude.go), D2's subscription inference (codex.go),
sworn#89 (Google/Bedrock pricing duplication — already filed and
acknowledged at design review, carried forward here).

**State transition:** `design_review` → `in_progress` (this session, commit
f8ca6b5) → `implemented` (this session, closing commit below).
