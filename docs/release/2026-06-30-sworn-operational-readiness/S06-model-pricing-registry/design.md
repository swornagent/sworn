# Design TL;DR — S06-model-pricing-registry

**Slice:** S06-model-pricing-registry
**Release:** 2026-06-30-sworn-operational-readiness
**Track:** T5-model-pricing-registry
**Stakes (Rule 9):** Type-2 throughout — static pricing-map data edits, locally
reversible, no schema/dependency/dispatch-logic change. No Type-1 or
architecturally-significant choice. The single ratified decision (intro vs
standard Sonnet 5 rate) was made by the planner in `spec.json`, not here.

## User outcome

When sworn dispatches to a current Anthropic model, the recorded per-dispatch
cost is correct: `claude-sonnet-5` is priced (not silently $0) and
`claude-opus-4-8` is billed at its real $5/$25 rather than the stale $15/$75
(an Opus 4.1 copy). This keeps the release's cost telemetry — the
operational-readiness signal this release exists to harden — from being
systematically wrong.

## Approach

Bring the **three duplicate Anthropic pricing maps into agreement**. The slice
is deliberately narrow: correct the static rate data + prove it with tests. It
does NOT consolidate the three maps into one canonical table — that is the
separately-tracked, Type-1 model-layer service refactor (out of scope, AC-07).

### The pricing surface (verified against live code)

| Map / function | File | Key form | Current entry |
|---|---|---|---|
| `Pricing` → `ComputeCost` | `internal/model/pricing.go` | `claude-opus-4-8` | `{15.00, 75.00}` (stale) |
| `anthropicPricing` → `computeAnthropicCost` | `internal/model/anthropic.go` | `claude-opus-4-8` | `{15.00, 75.00}` (stale) |
| `bedrockPricing` → `computeBedrockCost` | `internal/model/bedrock.go` | `anthropic.claude-opus-4-8` | `{15.00, 75.00}` (stale) |

`PriceForModel` (`client.go`, named in AC-01) is an **aggregator** that walks
OAI → `anthropicPricing` → Google → `bedrockPricing` in order. It needs **no
edit**: once `claude-sonnet-5` is in `anthropicPricing` and
`anthropic.claude-sonnet-5` is in `bedrockPricing`, `PriceForModel` resolves
both non-zero. `claude-sonnet-5` is absent from all three maps today, so
`ComputeCost`/`computeAnthropicCost`/`computeBedrockCost` currently fall through
to `return 0` for it.

### Planned changes (per file)

1. **`internal/model/pricing.go`** — in `Pricing`:
   - add `"claude-sonnet-5": {2.00, 10.00}` (AC-01, AC-02)
   - correct `"claude-opus-4-8"` `{15.00, 75.00}` → `{5.00, 25.00}` (AC-04)
   - AC-03 comment on the sonnet-5 entry (see "Comment content" below)

2. **`internal/model/anthropic.go`** — in `anthropicPricing`:
   - add `"claude-sonnet-5": {2.00, 10.00}`
   - correct `"claude-opus-4-8"` → `{5.00, 25.00}`
   - AC-03 comment

3. **`internal/model/bedrock.go`** — in `bedrockPricing`:
   - add `"anthropic.claude-sonnet-5": {2.00, 10.00}`
   - correct `"anthropic.claude-opus-4-8"` → `{5.00, 25.00}`
   - AC-03 comment

4. **`internal/model/pricing_test.go`** — add assertions (AC-05) driving the
   real exported `ComputeCost`:
   - `ComputeCost("claude-sonnet-5", 1M, 1M)` == `2.00 + 10.00` == `12.00`
   - `ComputeCost("claude-opus-4-8", 1M, 1M)` == `5.00 + 25.00` == `30.00`
     (explicitly NOT the old `90.00` from $15/$75)

5. **Audit (AC-06)** `internal/model/anthropic_test.go` and
   `internal/model/bedrock_test.go` for hardcoded price assertions on the two
   changed models. Live check already run: the only opus-4-8 reference in
   `anthropic_test.go` (line 132) constructs a client, it does **not** assert a
   price; existing price assertions cover sonnet-4-6 / haiku-4-5 only. Expect no
   edit needed, but the audit is a required, evidenced step — `go build ./...`
   and `go test ./internal/model/...` must stay green.

### Comment content (AC-03) — identical intent on all three entries

Each `claude-sonnet-5` entry carries a comment documenting BOTH rates and the
expiry with a flip instruction, e.g.:

```
// claude-sonnet-5: introductory $2/$10 per MTok through 2026-08-31 (ratified,
// Anthropic models-overview footnote 4). Standard rate $3/$15 applies AFTER
// 2026-08-31 — FLIP this entry to {3.00, 15.00} then. Tracked: <issue/punch-list ref>.
```

### AC-07 — durable tracking for the intro→standard flip

The 2026-08-31 price flip is a Rule 2 deferral: encoding the intro rate is
correct for the current billing period but under-counts 1.5x after expiry unless
flipped. Plan: file a **GitHub issue** (`gh issue create`) titled for the flip,
and cite its number in the AC-03 code comments. If `gh` is unavailable in the
worktree, fall back to a release punch-list entry under the release folder and
cite that path instead. Either way the deferral is durable and citable, never a
silent time-bomb.

## Files I intend to touch

- `internal/model/pricing.go`
- `internal/model/anthropic.go`
- `internal/model/bedrock.go`
- `internal/model/pricing_test.go`

(Exactly the slice `touchpoints`. `client.go` is read-only context, not touched.)

## Reachability artefact (Rule 1)

Backend-only slice, no UI affordance. The reachability proof is the test in
`pricing_test.go` driving the **real exported** `ComputeCost` (not a private
copy of the map) and asserting the corrected dollar figures — a `manual-smoke`
equivalent is `go test ./internal/model/ -run TestPricing` showing the sonnet-5
and opus-4-8 cases pass. This is the integration point that owns the cost
affordance for the dispatch-record path.

## Design-level risks / pins for the reviewer

1. **AC-01 names `PriceForModel`** alongside `ComputeCost`/`computeAnthropicCost`.
   `PriceForModel` is the real aggregator in `client.go` and is satisfied
   transitively (no edit). Flagging so the reviewer confirms "no client.go edit"
   is the intended reading and not a missed touchpoint.
2. **Intro-vs-standard rate** is already ratified in the spec ($2/$10 active
   now). Not re-deciding it here; AC-03 comment + AC-07 tracking are the guard
   against the future flip being forgotten.
3. **No map consolidation.** Three edits stay three edits. Consolidation is the
   deferred Type-1 refactor; doing it here would exceed slice scope and change
   the stakes classification.

## Traceability (AC → planned change)

| AC | Planned change |
|----|----------------|
| AC-01 | sonnet-5 added to Pricing + anthropicPricing + bedrockPricing (anthropic. key); PriceForModel resolves transitively |
| AC-02 | all three sonnet-5 entries = `{2.00, 10.00}` |
| AC-03 | both-rates + expiry + flip comment on each sonnet-5 entry |
| AC-04 | opus-4-8 `{15,75}`→`{5,25}` in all three maps |
| AC-05 | pricing_test.go asserts sonnet-5 $2/$10 and opus-4-8 $5/$25 via real ComputeCost |
| AC-06 | audit + keep anthropic_test.go / bedrock_test.go green |
| AC-07 | GitHub issue (or punch-list) for the 2026-08-31 flip, cited in the comment; no map consolidation |
