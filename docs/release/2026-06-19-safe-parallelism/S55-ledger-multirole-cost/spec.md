---
title: 'S55-ledger-multirole-cost — capture per-role model + USD cost for every dispatch'
description: 'Evolves the verdict Record to v:2 with a per-role dispatches[] ({role, model, cost_usd, attempt}) covering implementer, verifier, captain, and the orchestrator BLOCKED-resolvability hook. Captures each dispatch''s cost (already computed locally from token usage) at its in-binary site and aggregates it in RunSlice, so the corpus carries the full per-slice economics, not just the pass/fail.'
---

# Slice: `S55-ledger-multirole-cost`

## User outcome

After a slice runs, `docs/ledger/verdicts.jsonl` carries not just the verdict but the full
per-role economics of reaching it: which model each role used (implementer, verifier,
captain, orchestrator) and what each dispatch cost in USD. A maintainer can answer "what did
this slice actually cost to produce and check, broken down by role?" from the corpus — the
input every cost-aware decision in S56 needs.

## Entry point

`RunSlice` in `internal/run` — the per-role dispatch stages (implement via S45/S46, verify,
captain review via S46, orchestrator triage hook via S47) each surface their `costUSD` (already
returned by `model.Verifier.Verify` and computed by `internal/agent`), which RunSlice records
into the slice's `status.json`. `ledger.Project` reads it into the `v:2` Record.

## Background

The cost signal already exists and is **not** gated on the S06b commercial billing engine
(Stripe/subscriptions are post-R3; this is local token-pricing):

- `internal/model/client.go`: `Verify(...) (text string, costUSD float64, err error)` — every
  dispatch already returns its USD cost.
- `internal/model/oai.go`: parses `UsageBlock` (prompt/completion tokens) and `computeCost`
  from a `modelPricing` table; `internal/agent` has the same for the implementer loop.
- `internal/verdict`: `Result.CostUSD` already carries the verifier cost.

Every role is (or becomes, via T13) an in-binary dispatch: implementer = `internal/agent`;
verifier = `internal/verify`; **captain** = S46's review stage (its own `captain.model`);
**orchestrator** = S47's triage, deterministic-first with a single LLM hook for BLOCKED
resolvability (cost only when the hook fires). T16 already depends on T12 (agent/verify) and
T13 (captain/orchestrator), so all four sites exist when this slice runs. The cost is computed
today and simply discarded; this slice persists it.

## In scope

- Evolve `ledger.Record` to schema `v:2`:
  - Add `Dispatches []Dispatch`, `type Dispatch struct { Role, Model string; CostUSD float64; Attempt int }`.
  - Add `TotalCostUSD float64` (sum of `Dispatches`, convenience for reporting).
  - Retain `v:1` fields; the implementer dispatch remains derivable (back-compat: a `v:1`
    line with `Model`/`Attempt` still loads, its dispatch synthesised with unknown cost).
- Extend `state.Verification` with `Dispatches []state.Dispatch` (mirrors the Record shape;
  `omitempty`) so the per-role model + cost round-trips through `state.Write`/`state.Read`.
- Capture wiring in `RunSlice` (`internal/run`): record each role's `{role, model, cost_usd,
  attempt}` as its stage completes —
  - implementer: from `internal/agent`'s loop cost,
  - verifier: from `verdict.Result.CostUSD`,
  - captain: from the S46 review-stage dispatch cost,
  - orchestrator: from the S47 BLOCKED-resolvability hook cost (0 / absent when the
    deterministic path is taken).
- `ledger.Project` populates `Dispatches` + `TotalCostUSD` from `verification.dispatches`.

## Out of scope

- Cost-aware routing, the `--optimize` flag, and report cost columns — **S56** (consumes this).
- Per-role *quality* as a stored field — quality stays **derived** in S56's report/routing
  layer (e.g. captain-miss rate) from the captured per-role records; it is not hand-entered.
- True billed/proxy cost reconciliation against S06b credits — out of scope. Why: the local
  token-pricing cost is the routing signal; reconciling it against backend-metered credits is
  a separate accuracy concern. Tracking: S06b billing follow-up. Ack: Brad, 2026-06-23.
- Capturing planner cost — the planner is conversational/skill-driven, not an in-binary
  `sworn run` dispatch. Surfaced if/when planning moves in-binary.

## Planned touchpoints

- `internal/ledger/ledger.go` (modify — `v:2` Record, `Dispatch`, `TotalCostUSD`, projection)
- `internal/ledger/ledger_test.go` (modify — v:2 projection + v:1 back-compat load)
- `internal/state/state.go` (modify — `Dispatches` on `Verification` + `Dispatch` type)
- `internal/state/state_test.go` (modify — round-trip)
- `internal/run/slice.go` (modify — record per-role dispatch cost in RunSlice stages;
  serialised behind the T12→T13 chain that owns these stages)
- `internal/agent/agent.go` (modify — surface the implementer loop's total cost to the caller
  if not already returned; owned by T12 via S42/S43, covered by the dependency)

## Acceptance checks

- [ ] `ledger.Record` marshals at `"v":2` with a `dispatches` array; `Project` populates one
  `Dispatch` per role present in `verification.dispatches`, and `TotalCostUSD` equals their sum
- [ ] A `v:1` corpus line (no `dispatches`) still loads via `ledger.Load` without error and
  yields a Record with an implementer dispatch of unknown (zero) cost — back-compat holds
- [ ] `state.Verification.Dispatches` round-trips through `state.Write`/`state.Read`; omitted
  from JSON when empty
- [ ] After a slice runs through `RunSlice`, its `status.json` `verification.dispatches`
  contains an entry for each role that dispatched (implementer + verifier always; captain when
  the S46 stage ran; orchestrator only when the S47 hook ran), each with a non-negative
  `cost_usd` and the model used (asserted via the run package's RunSlice test — Rule 1: cost is
  proven through the loop that owns the dispatches, not a leaf)
- [ ] A model absent from the pricing table records `cost_usd: 0` and is treated downstream as
  "no cost signal", never as "free" (documented; consumed by S56)
- [ ] `go test ./internal/ledger/... ./internal/state/... ./internal/run/... ./internal/agent/...`
  passes; `go build ./...` succeeds with no new `go.mod` deps

## Required tests

- **Unit**: `internal/ledger/ledger_test.go` — v:2 marshal/round-trip, multi-role `Project`,
  `TotalCostUSD` sum, v:1 back-compat load.
- **Integration**: `internal/run` RunSlice test asserting per-role `verification.dispatches`
  are recorded across implement/verify/captain stages (Rule 1 reachability point).
- **Reachability artefact**: in `proof.md`, paste a real `verdicts.jsonl` `v:2` line with a
  multi-role `dispatches` array produced from an actual `sworn run`, plus `go test` output.

## Risks

- The capture sites live in `RunSlice` stages added by S45/S46/S47 (T13). If T13 lands the
  captain/orchestrator stages elsewhere (e.g. `internal/orchestrator/triage.go` for S47), record
  cost from there; the dependency on T13 exists precisely so these sites are settled first.
- `cost_usd: 0` ambiguity (unpriced model vs genuinely free) must be carried as a distinct
  "unknown" downstream, not folded into 0 — S56's routing guards on it.

## Deferrals allowed?

Yes, with Rule 2 compliance — proxy/billed-cost reconciliation and planner-cost capture are
surfaced above with why / tracking / ack.
