---
title: 'S56-ledger-cost-routing — quality-gated, cost-optimized model routing from the corpus'
description: 'Adds the cost objective to routing: pick the cheapest model whose measured pass-rate for the (slice-kind, role) clears a floor, via --optimize cost|quality|balanced (default quality, so S54 is unchanged). Adds per-role cost columns and derived per-role quality (captain-miss rate, verifier-overturn rate) to sworn ledger report, and wires the cost mode into ResolveImplementerModel.'
---

# Slice: `S56-ledger-cost-routing`

## User outcome

A maintainer runs `sworn ledger recommend implementer harness --optimize cost` and gets the
**cheapest** model whose measured pass-rate for that slice-kind clears the quality floor — not
the absolute-best and not the blindly-cheapest. With `--optimize cost` configured, `sworn run`
defaults the implementer to that model. `sworn ledger report` now shows cost-per-passing-slice
by model and a per-role quality/cost breakdown (what the captain costs and how often its
passed designs later failed; what the verifier costs and how often its verdicts were
overturned). "Spend tokens where the task is genuinely hard" becomes a measured default.

## Entry point

- `sworn ledger recommend <role> <slice-kind> [--optimize cost|quality|balanced]` — ranked
  recommendation with evidence (pass-rate, sample, mean cost), in `cmd/sworn/ledger.go`.
- `sworn ledger report` — extended with cost-per-pass and per-role quality/cost columns.
- `config.ResolveImplementerModel(...)` honours the `--optimize` mode (flag → env → config),
  consulting the cost-aware recommendation when mode is `cost`/`balanced`.

## Background

S55 captures per-role `{model, cost_usd}` per dispatch into the `v:2` corpus. S54 already
ranks by pass-rate (quality). This slice adds the cost objective the maintainer chose:
quality is a **hard gate**, cost is the **optimizer**. It also turns the per-role captured
data into derived quality signals in the report, so captain and orchestrator economics are
legible alongside implementer/verifier — no hand-entered quality fields.

## In scope

- Extend `internal/ledger/routing.go`:
  - `type Objective int` (`OptimizeQuality`, `OptimizeCost`, `OptimizeBalanced`).
  - `RecommendModel(records, role, kind string, obj Objective) (Recommendation, bool)`:
    - `OptimizeQuality` — S54 behaviour (best pass-rate), unchanged.
    - `OptimizeCost` — among models whose pass-rate ≥ floor (default 0.8, configurable) **and**
      whose sample ≥ the minimum, pick the lowest mean `cost_usd`; tie-break by pass-rate.
      Models with only `cost_usd: 0` (unpriced) are excluded from cost ranking (no signal),
      never treated as free.
    - `OptimizeBalanced` — pass-rate per dollar among models clearing the sample guard.
  - `Recommendation` gains `MeanCostUSD float64` and `Objective`.
- Extend `internal/ledger/query.go` with derived per-role quality:
  - `CaptainMissRate` — share of slices the captain passed (no escalate pin) that later got a
    FAIL/BLOCKED verdict.
  - `VerifierOverturnRate` — share of verifier verdicts later overturned (INCONCLUSIVE re-run
    flipping, or a PASS later reopened).
  - cost-per-passing-slice by (model, kind, role).
- `sworn ledger report` prints the cost + derived-quality columns; `recommend` gains
  `--optimize` and a `--floor` override.
- Wire into `internal/config/config.go`: `ResolveImplementerModel` resolves the `--optimize`
  mode and, for `cost`/`balanced`, uses the cost-aware recommendation as the default. Guards
  intact: explicit `--model`/`$SWORN_IMPLEMENTER_MODEL` still win; an absent/thin corpus or a
  no-confident-pick returns exactly the pre-S54 default (byte-for-byte fallback).

## Out of scope

- Capturing the cost (that is S55) — consumed here.
- Auto-routing the captain/verifier/orchestrator models from history — this slice **reports**
  their economics and routes the **implementer**. Why: implementer is the highest-volume,
  highest-leverage dispatch; routing other roles is a follow-up once their report data is
  trusted. Tracking: future-release ledger follow-up. Ack: Brad, 2026-06-23.
- Proxy/billed-cost reconciliation against S06b credits — deferred (Rule 2), as in S55.

## Planned touchpoints

- `internal/ledger/routing.go` (modify — `Objective`, cost/balanced ranking)
- `internal/ledger/routing_test.go` (modify — cost-gated ranking, unpriced-exclusion, floor)
- `internal/ledger/query.go` (modify — captain-miss / verifier-overturn / cost-per-pass)
- `internal/ledger/query_test.go` (modify)
- `internal/config/config.go` (modify — `--optimize` mode in `ResolveImplementerModel`;
  serialised behind the T3→T5→T6 config.go chain via T6, as S54)
- `internal/config/config_test.go` (modify — cost-mode applied, flag-override-wins, fallback)
- `cmd/sworn/ledger.go` (modify — `--optimize`/`--floor` flags; report cost columns)

## Acceptance checks

- [ ] `RecommendModel(..., OptimizeCost)` over a corpus where model A passes 9/10 at $0.50/slice
  and model B passes 9/10 at $0.05/slice returns **B** (both clear the floor; B is cheaper)
- [ ] `RecommendModel(..., OptimizeCost)` where the cheapest model is **below** the pass-rate
  floor returns a pricier model that clears the floor — quality is a hard gate, never traded away
- [ ] A model with only `cost_usd: 0` (unpriced) is excluded from cost ranking, not selected
  as "free"
- [ ] `RecommendModel(..., OptimizeQuality)` returns exactly what S54 returned (no regression)
- [ ] `sworn ledger recommend implementer harness --optimize cost` prints the model with
  pass-rate, sample, and mean cost; `--floor 0.9` raises the gate and changes the pick when warranted
- [ ] `sworn ledger report` prints cost-per-passing-slice by model and a per-role section
  including captain-miss rate and verifier-overturn rate
- [ ] `ResolveImplementerModel` with `--optimize cost` returns the cost-aware pick when the
  corpus is confident; with an explicit `--model`/env override, returns the override unchanged;
  with a thin/absent corpus, returns the pre-S54 default (byte-for-byte fallback test)
- [ ] `go test ./internal/ledger/... ./internal/config/... ./cmd/sworn/...` passes;
  `go build ./...` with no new `go.mod` deps

## Required tests

- **Unit**: `internal/ledger/routing_test.go` — cost/balanced ranking, floor gate,
  unpriced-exclusion, quality-mode regression; `query_test.go` — captain-miss / overturn /
  cost-per-pass over a fixed corpus.
- **Integration**: `internal/config/config_test.go` — `ResolveImplementerModel` across
  cost-mode-applied, override-wins, thin-corpus-fallback (Rule 1: routing proven at the
  resolver that owns model choice).
- **Reachability artefact**: in `proof.md`, paste `sworn ledger recommend ... --optimize cost`
  and `sworn ledger report` output against the real corpus.

## Risks

- The byte-for-byte fallback is the highest-risk surface (as in S54): a bug letting a thin
  corpus or `cost`-mode change the resolved model when it should not silently alters harness
  behaviour. The fallback regression test is a hard block.
- Floor + unpriced-exclusion interaction: if every priced model is below the floor, cost mode
  must fall back to quality mode (best available), not return nothing — assert this.
- `config.go` conflict at `/merge-track` means the T6 serialisation was wrong (invariant 4).

## Deferrals allowed?

Yes, with Rule 2 compliance — non-implementer role routing and proxy-cost reconciliation are
surfaced above with why / tracking / ack.
