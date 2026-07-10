# Design TL;DR — S08-honest-cost-telemetry

## User outcome (from spec.json)

Every dispatch record carries honest economics — real cost computed from the
CONFIRMED model's pricing or carried from the CLI's own report, an explicit
`cost_source`, true token split, duration, and confirmed model-id — so
verified-slices-per-dollar-per-model is finally a real number (sworn#70), and
a subscription dispatch is recorded as subscription-covered, never as fake $0
API spend.

## Grounding: the spec's rationale is stale on two points, live code is wider than touchpoints on a third

**1. `internal/verify/verify.go:246 computeAgenticCost` no longer exists.**
S06-loop-dispatch-rewire and S07-scheduler-failfast (both `verified`, this
branch) already rewired `RunAgentic` to dispatch through `driver.Driver` and
source `CostUSD`/`InputTokens`/`OutputTokens`/`DurationMS`/`ModelIDConfirmed`
straight off `driver.Result` (`withDispatchEconomics`,
`acceptStructuredVerdict`, verify.go:245-260). There is nothing to delete
here. What verify.go is missing is `CostSource` — `withDispatchEconomics`
copies four economics fields from `driver.Result` but not `CostSource`, and
neither `verdict.Result` (verdict.go:27-49) nor the two dispatch-record sites
that consume `lastVerdict.*` in `internal/run/slice.go` carry it either. AC-01
still stands (the *pricing* unification is real and undone); the *verify.go
deletion* clause is satisfied by inspection, not by an edit.

**2. `internal/driver/inprocess.go` is `internal/driver/inprocess/inprocess.go`** —
already recorded in S04's own file header comment (ADR-0012
`TestNoWireImports`: the `internal/driver` package itself may import neither
`internal/model` nor `internal/agent`; the in-process driver's own file
header names this explicitly as "S04 divergence, recorded in the slice
journal"). This design targets the landed path.

**3. There are (at least) four live pricing-lookup implementations, not the
two AC-01 names, and the touchpoints list omits the two that matter most for
AC-02's honesty claim:**

| # | Location | Map | Live caller(s) | Reachable from in-process `Dispatch`? |
|---|---|---|---|---|
| A | `internal/model/pricing.go` `Pricing`/`ComputeCost` | `Pricing` (merged Anthropic+OpenAI, 15 entries) | `anthropic.go` `Chat()` line 156 only | Yes — `Chat` is what `chatMeter` wraps |
| B | `internal/model/client.go` `PriceForModel`/`ComputeCostFromTokens` | aggregates `modelPricing`+`anthropicPricing`+`googlePricing`+`bedrockPricing` | **none** (dark code) | N/A today |
| C | `internal/model/oai.go` `computeCost` (unexported, package-local) | `modelPricing` | `oai.go` `Verify()` line 232, `openai_responses.go` line 215 | Yes — `Verify` is unrelated to `Dispatch`, but `OAI`/`OpenAIResponses` are the concrete types `chatMeter` wraps; their `Chat()` methods (not shown above) do **not** populate `ChatResponse.CostUSD` at all today — `chatMeter.observe` only reads `Usage`, never `resp.CostUSD` |
| D | `internal/model/anthropic.go` `computeAnthropicCost` (Verify) + inline `ComputeCost` call (Chat, line 156 = same as row A) | `anthropicPricing` | `anthropic.go` `Verify()` line 81, `Chat()` line 156 | `Chat()`: yes |

I diffed row A's `Pricing` map against rows C+D's `modelPricing`+`anthropicPricing`
by hand: the 15 keys and both price fields are byte-identical across the
split — `Pricing` is a hand-maintained merge of the other two maps, not an
independent source of truth. R-01's stated risk ("unification silently
changes recorded costs") does not materialise for the models already priced
in both places; it only bites if any of these maps drift out of sync before
S08 lands, which the design's regression test (`TestPricingUnified`) exists
to catch going forward.

**The touchpoints list (`pricing.go`, `pricing_test.go`, `client.go`,
`agent.go`, `agent_test.go`, `verify.go`, `driver/inprocess.go`,
`run/slice.go`, `state.go`, `state_test.go`) never names `oai.go` or
`anthropic.go`.** But row D's `Chat()` (line 156) is the literal function
`chatMeter.Chat` wraps for every in-process implementer/verifier/captain
dispatch — it is *the* path AC-02 is about. Leaving it calling `pricing.go`'s
`ComputeCost` while `inprocess.go`'s `economics()` (the function this slice
must rewrite) discards `resp.CostUSD` entirely and substitutes a flat
`nominalCostPerToken` estimate is precisely the bug sworn#70 describes.
Closing AC-02 honestly requires touching `oai.go` and `anthropic.go`, even
though spec.json doesn't list them. Both are additive, same-signature
redirects (see "Files to touch").

**Google/Bedrock (`google.go`/`bedrock.go`) have the same duplicate-lookup
shape (rows E/F, not tabled above) but are out of scope — filed as
sworn#89 (why: different SDK usage types, not a same-signature redirect;
tracking: sworn#89; acknowledgement: this design.md, pending Captain/Coach
sign-off at design review) per Rule 2.**

## CostSource vocabulary: what the spec asks for vs. what the live client set can actually produce

AC-02/AC-03/AC-04 name a five-value enum: `provider | pricing-table | cli |
subscription | unknown`. Grounding against the live provider clients:

- **`"provider"` is currently unreachable.** AC-02's rationale reads "when
  the provider itself reports a cost (Anthropic `resp.CostUSD`), that figure
  SHALL be carried with `CostSource="provider"`." But Anthropic's Messages
  API does not return a cost field — `anthropic.go`'s `Chat()` *computes*
  `ChatResponse.CostUSD` itself from the (soon-unified) pricing table (row D
  above); it is not provider-reported, it's the same pricing-table
  computation AC-02's other branch describes. No client wired into the
  in-process driver today receives a real billing figure over the wire. I am
  keeping `"provider"` in the vocabulary (future-facing — some provider may
  start returning billing data) but the in-process driver will never emit it
  with the current client set. Flagging this as a Rule 8 (requirements
  fidelity) discrepancy for the Captain to see, not silently narrowing AC-02
  to only its pricing-table branch.
- **`driver.Result.CostSource`'s own doc comment is already stale** —
  `driver.go` line ~139 reads `// (e.g. "provider-reported", "estimated")`,
  neither of which is in the five-value enum. Needs updating alongside the
  code (touchpoint addition below).
- **The subprocess drivers already half-implement this vocabulary under
  different names**, asserted by existing tests:
  - `claude.go`: `costSource()` returns `"provider-reported"` when the
    envelope carried `TotalCostUSD` or `Usage`, else `"unknown"`
    (`claude_test.go:55-56`, `:333-334` pin these two literal strings).
  - `codex.go`: `costSource()` returns `"provider-reported"` when `Usage`
    was present, else `"unknown"` (`codex_test.go:42-43`, `:323-324`).
  - `inprocess.go`: always `"estimated"` — the placeholder S04's own comment
    names as "S08 (honest cost telemetry) replaces it" (`inprocess_test.go:238-239`
    pins this, citing "Coach ack pin 5").

  None of these three strings (`provider-reported`, `unknown` used for two
  different meanings, `estimated`) are in the target enum. Reconciling them
  is in scope (AC-03) even though the touchpoints list only names
  `driver/inprocess.go`, not `claude.go`/`codex.go`/their test files.

**Design decision for claude.go (D1):** `claudeEnvelope.TotalCostUSD` is a
`*float64` — nil is already distinguished from a reported zero
(`reported()`'s doc comment says this explicitly, R-01 in spec.json). I
propose:
- `TotalCostUSD == nil` → `CostSource = "unknown"` (envelope carried no cost
  data at all — a genuine protocol gap, not a design choice).
- `TotalCostUSD != nil && *TotalCostUSD > 0` → `CostSource = "cli"` (the CLI
  reported a real, non-zero API cost — AC-03's `total_cost_usd` case
  verbatim).
- `TotalCostUSD != nil && *TotalCostUSD == 0` → `CostSource = "subscription"`
  (the CLI *explicitly* reported zero — this is the documented behaviour of
  `claude -p` when authenticated against a Claude subscription rather than
  an API key; an explicit zero is a different signal than "no field at
  all"). This is an inference from external CLI behaviour, not something
  provable from this repo alone — **flagging as a design pin for Captain
  review**, not asserting it as settled fact.

**Design decision for codex.go (D2):** the `codexEnvelope` never carries a
cost field in any documented shape (the file's own header comment: "no
`model` or `duration_ms` field at any level" — and no cost field either,
confirmed by re-reading the documented sample). Since codex CLI is
subscription-authenticated by default and the JSONL stream structurally
never reports cost, I propose: `Usage != nil` (a turn actually completed) →
`CostSource = "subscription"` (tokens were spent, cost is unreported *by
design*, matching AC-03's "subscription-covered CLI reports 0 by design"
language generalised to "never reports cost"); `Usage == nil` → `CostSource
= "unknown"` (the stream never produced a usable envelope — genuine protocol
drift). Same caveat as D1: codex.go's own header already notes its envelope
shape "is not verified against a live codex binary" (SIT-deferred) — this
classification inherits that same unverified-against-live-binary risk and is
flagged for Captain review, not asserted as settled.

**Design decision (D3): named CostSource constants, not scattered string
literals.** The vocab is about to be asserted by string literal in 3
production files and 6 test files. I will add
`driver.CostSourceProviderReported = "provider"`,
`CostSourcePricingTable = "pricing-table"`, `CostSourceCLI = "cli"`,
`CostSourceSubscription = "subscription"`, `CostSourceUnknown = "unknown"`
to `internal/driver/driver.go` (mirroring the existing `ErrKind*` constant
pattern in the same file) so every producer and every test references the
same symbol — a typo in a literal (`"pricing_table"` vs `"pricing-table"`)
currently fails silently (any string is schema-valid, `additionalProperties:
true`); constants make it a compile error instead.

## Approach per acceptance criterion

**AC-01 (unify pricing surfaces, delete flat-rate functions).**
- Delete `internal/model/pricing.go` (`Pricing` map + `ComputeCost` func)
  entirely — its one call site (`anthropic.go:156`) redirects to
  `ComputeCostFromTokens(a.Model, int64(inputTokens), int64(outputTokens))`.
- Redirect `oai.go`'s local `computeCost(model, usage)` (2 call sites:
  `oai.go:232`, `openai_responses.go:215`) to
  `ComputeCostFromTokens(model, int64(usage.PromptTokens),
  int64(usage.CompletionTokens))`; delete the local function once both call
  sites move.
- Redirect `anthropic.go`'s local `computeAnthropicCost` (1 call site,
  `Verify()` line 81) to `ComputeCostFromTokens`; delete the local function.
- `client.go`'s `PriceForModel`/`ComputeCostFromTokens` becomes the sole
  surviving registry — no functional change to `client.go` itself, it goes
  from zero call sites to five.
- Delete `internal/agent/agent.go:182 computeCost`. Its only caller
  accumulates `totalCost` inside `Run`, and `Run`'s `float64` return value is
  already discarded by both of its only two callers
  (`inprocess.go:183`, `inprocess_verify.go:43`, both `text, _, _/transcript,
  err := agent.Run(...)`). Proposal: drop the `float64` return from `Run`'s
  signature entirely (`(string, []Message, error)`) rather than leave a
  dead accumulator — a signature change, not a silent no-op deletion,
  because a future caller reading `Run`'s doc comment ("Returns the final
  text response, the total cost, ...") would otherwise be misled into
  thinking the returned cost is real. Both call sites and `agent_test.go`
  update accordingly (mechanical — touchpoints already name both files).
- `verify.go`: no `computeAgenticCost` to delete (see grounding above); add
  `CostSource` to the four-field copy in `withDispatchEconomics`.
- New `TestPricingUnified` (pricing_test.go, rewritten — existing tests
  reference the deleted `Pricing`/`ComputeCost` symbols directly and must
  move to `PriceForModel`/`ComputeCostFromTokens`): enumerates every model
  key across `modelPricing`+`anthropicPricing`+`googlePricing`+`bedrockPricing`
  and asserts `PriceForModel` resolves each to the exact same
  `(InputPricePer1M, OutputPricePer1M)` pair the source map holds — a
  structural "one path, no drift" regression guard, plus the original
  `TestPricing_Sonnet4_6`/`TestPricing_Haiku4_5`/
  `TestPricing_UnknownModelReturnsZero` cases ported to the surviving API.

**AC-02 (in-process driver honest cost).** Rewrite `inprocess.go`'s
`economics()`:

```go
func (d *InProcess) economics(res driver.Result, in driver.DispatchInput, meter *chatMeter, start time.Time) driver.Result {
	res.InputTokens = meter.inputTokens
	res.OutputTokens = meter.outputTokens
	res.ModelID = meter.modelID(in.ModelID)
	if p, ok := model.PriceForModel(res.ModelID); ok {
		res.CostUSD = model.ComputeCostFromTokens(res.ModelID, meter.inputTokens, meter.outputTokens)
		res.CostSource = driver.CostSourcePricingTable
		_ = p // price already folded into ComputeCostFromTokens
	} else {
		res.CostUSD = 0
		res.CostSource = driver.CostSourceUnknown
		fmt.Fprintf(os.Stderr, "inprocess: no pricing entry for model %q — cost recorded as 0 (CostSource=unknown)\n", res.ModelID)
	}
	res.DurationMS = time.Since(start).Milliseconds()
	return res
}
```

Delete `nominalCostPerToken`. This is the single choke point `dispatchLoop`,
`dispatchCaptain`, and `dispatchVerifier` all already call — one fix, three
roles fixed (confirmed by re-reading `inprocess.go`/`inprocess_verify.go`:
all three call `d.economics(...)` as their last step). The `"provider"`
branch is not implemented (see vocabulary section above — no wired client
produces a real provider-reported figure today); implementing a branch that
can never be exercised would be untestable dead code, which is worse than
documenting the gap.

**AC-03 (subprocess CLI cost + subscription).** `claude.go`/`codex.go`
`costSource()` methods rewritten per D1/D2 above, using the new
`driver.CostSource*` constants. `claude_test.go`/`codex_test.go`'s existing
`"provider-reported"`/`"unknown"` assertions update to the new vocabulary
(literal string changes, not new test scenarios) plus new cases for the
`TotalCostUSD == 0` (claude) and `Usage != nil` (codex) branches.

**AC-04 (fail-closed unknown).** Covered by AC-02's `economics()` `else`
branch (in-process) and the `TotalCostUSD == nil` / `Usage == nil` branches
(subprocess, AC-03). No pricing DATA is added for models absent from the
unified registry (spec's explicit out-of-scope) — an unrecognised model
returns `(ModelPricing{}, false)` from `PriceForModel` today and continues
to after unification, so AC-04's "no pricing entry" case is exercised by the
exact same models that hit it before this slice (nothing new needed there).

**AC-05 (state.Dispatch persists cost_source).**
- `internal/state/state.go`: add `CostSource string
  \`json:"cost_source,omitempty"\`` to `Dispatch` (additive, schema-safe per
  spec's own R-02 mitigation — `slice-status-v1` is
  `additionalProperties: true`).
- `internal/verdict/verdict.go`: add the same field to `Result` (needed so
  `withDispatchEconomics`/`acceptStructuredVerdict` in verify.go have
  somewhere to put `res.CostSource` before it reaches slice.go).
- `internal/run/slice.go`: thread `CostSource` through all five
  `state.Dispatch{...}` construction sites:
  - captain (line ~415): `CostSource: reviewResult.Dispatch.CostSource`
  - implementer (line ~561): `CostSource: implRes.CostSource`
  - verifier-blocked-proof-absent (line ~600): synthetic zero-cost record,
    no driver call was made — `CostSource` left empty (`omitempty`, honest:
    there is no source because nothing dispatched).
  - first-pass (line ~682): same — deterministic $0 gate, no dispatch,
    `CostSource` left empty.
  - verifier-final (line ~754): `CostSource: lastVerdict.CostSource`
- New test in `internal/run/slice_test.go` (Rule 1 reachability — through
  `RunSlice`, not a leaf `state.Dispatch` struct-literal unit test):
  drives a full slice run against a fake driver returning distinct
  `CostSource` values per role and asserts the written `status.json`'s
  `verification.dispatches[].cost_source` carries each one through, and that
  the written file still validates against `slice-status-v1`
  (`baton.ValidateSchema`).

## Files to touch (supersedes spec.json's touchpoints — see grounding above)

| File | Change |
|---|---|
| `internal/model/pricing.go` | **Delete** (`Pricing` map + `ComputeCost`) |
| `internal/model/pricing_test.go` | Rewrite against `PriceForModel`/`ComputeCostFromTokens`; add `TestPricingUnified` |
| `internal/model/client.go` | No functional change — becomes the sole surviving registry |
| `internal/model/oai.go` | Delete local `computeCost`; redirect its 2 call sites to `ComputeCostFromTokens` |
| `internal/model/openai_responses.go` | Redirect its 1 `computeCost` call site (shares `oai.go`'s deleted func) |
| `internal/model/anthropic.go` | Delete local `computeAnthropicCost`; redirect `Verify()` + `Chat()` to `ComputeCostFromTokens` |
| `internal/agent/agent.go` | Delete `computeCost`; drop `float64` cost return from `Run`'s signature |
| `internal/agent/agent_test.go` | Update for `Run`'s new signature |
| `internal/verify/verify.go` | Add `CostSource` to `withDispatchEconomics`'s field copy (no `computeAgenticCost` to delete — already gone) |
| `internal/verdict/verdict.go` | Add `CostSource string` to `Result` |
| `internal/driver/driver.go` | Add `CostSource*` named constants (D3); fix `Result.CostSource` doc comment's stale examples |
| `internal/driver/inprocess/inprocess.go` | Rewrite `economics()` per AC-02; delete `nominalCostPerToken` |
| `internal/driver/inprocess/inprocess_test.go` | Update the `CostSource != "estimated"` assertion (Coach ack pin 5, now superseded) to `"pricing-table"`/`"unknown"` per case |
| `internal/driver/claude.go` | Rewrite `costSource()` per D1 (3-way: unknown / cli / subscription) |
| `internal/driver/claude_test.go` | Update `"provider-reported"` assertions to `"cli"`; add a `TotalCostUSD == 0` → `"subscription"` case |
| `internal/driver/codex.go` | Rewrite `costSource()` per D2 (2-way: unknown / subscription) |
| `internal/driver/codex_test.go` | Update `"provider-reported"` assertions to `"subscription"` |
| `internal/driver/subprocess_test.go` | Update the doc-comment reference to the renamed vocabulary (no assertion change expected — confirm at implementation) |
| `internal/state/state.go` | Add `CostSource string \`json:"cost_source,omitempty"\`` to `Dispatch` |
| `internal/state/state_test.go` | Cover round-trip of the new field |
| `internal/run/slice.go` | Thread `CostSource` through all 5 `state.Dispatch{...}` sites |
| `internal/run/slice_test.go` | New end-to-end `RunSlice` test proving `cost_source` reaches the written `status.json` (Rule 1 reachability artefact) |
| `internal/captain/review_test.go` | Extend (not break) — assert the fixture's `CostSource` reaches the captain dispatch record once `internal/run/slice_test.go`'s coverage exists; confirm at implementation whether this is redundant with the new slice_test.go case |
| `internal/verify/verify_agentic_test.go` | Extend similarly for the verifier leg |

## Risks / open items for Captain review

1. **D1/D2 (subscription-vs-unknown inference for claude/codex) are informed
   guesses about external CLI behaviour, not verified against a live
   binary** — same unverified-against-live-binary posture S03's own design
   already accepted for codex's envelope shape (R-01, SIT-deferred). If
   wrong, a real API-billed codex/claude dispatch could be mislabelled
   `"subscription"` (under-reporting real spend) or vice versa. Flagging for
   explicit ratification, not silently landing as fact.
2. **AC-02's `"provider"` CostSource branch is unimplemented because it is
   currently unreachable** with the wired client set (see vocabulary
   section). This is a narrower reading of AC-02 than its literal text.
   Recommend the same treatment S07's design got: either amend AC-02 in
   status.json to note `"provider"` is reserved-but-unreachable pending a
   client that returns real billing data, or explicitly accept the
   narrowing at design review. Flagging — not resolving unilaterally.
3. **Google/Bedrock pricing duplication deferred to sworn#89** (Rule 2: why
   = different SDK usage types + not on the in-process Dispatch path;
   tracking = sworn#89; acknowledgement = this design.md, pending Captain/
   Coach sign-off).
4. **`agent.Run`'s signature change** (drop the `float64` cost return) is a
   small blast-radius decision beyond spec.json's literal touchpoints
   (`agent.go`, `agent_test.go` — already listed, so no *new* file, but the
   *nature* of the change, a public signature edit vs. a same-signature
   internal rewrite, is worth the Captain's eyes).

## Acceptance-criteria traceability

- **AC-01** — `pricing.go` deletion + `oai.go`/`anthropic.go` redirects +
  `agent.go` deletion + `TestPricingUnified` (`internal/model/pricing_test.go`).
- **AC-02** — `inprocess.go` `economics()` rewrite + `inprocess_test.go`.
- **AC-03** — `claude.go`/`codex.go` `costSource()` rewrites (D1/D2) +
  their test files.
- **AC-04** — the `else`/nil branches of AC-02/AC-03's implementations
  (no separate code path — same fail-closed default the codebase already
  has for unknown models).
- **AC-05** — `state.go` `CostSource` field + `verdict.go` `CostSource`
  field + `slice.go` five-site threading + new `internal/run/slice_test.go`
  case (Rule 1 reachability artefact — proves it through `RunSlice`, not a
  leaf struct test) + `go test ./internal/model/... ./internal/agent/...
  ./internal/state/... ./internal/run/...` (spec's named command) plus
  `./internal/driver/... ./internal/verify/... ./internal/verdict/...
  ./internal/captain/...` (the additional touched packages this design
  found beyond spec.json's list).

## Out of scope (per spec.json, confirmed unchanged by this design)

Adding pricing data for models absent from every table (fail-closed
`"unknown"` instead — unchanged posture). Cross-run durable telemetry
storage (FT-7, `internal/db`). `board-v1`/`proof-v1` schema changes (this
slice only touches `slice-status-v1`, which is `additionalProperties: true`).
Google/Bedrock pricing-lookup unification (sworn#89, see Risk 3).
