---
title: 'S54-ledger-routing — history-backed implementer model routing from the verdict corpus'
description: 'Turns the verdict ledger into a routing signal: a recommendation engine over docs/ledger/verdicts.jsonl exposed as `sworn ledger recommend`, and a wire into S09''s ResolveImplementerModel so the resolved default is the model with the best measured pass-rate for the slice kind, falling back to current behaviour when the corpus is thin.'
---

# Slice: `S54-ledger-routing`

## User outcome

When a maintainer (or the loop) starts an implementer slice, the model picked is no longer
config-order-only — it is the model the ledger shows passes most reliably for that slice
kind, at the fewest attempts. A maintainer can also run `sworn ledger recommend harness` and
see the ranked recommendation with its evidence (sample size, pass-rate). The post's
"routing as salary banding" becomes a measured decision instead of a guess.

## Entry point

- `sworn ledger recommend <slice-kind>` — prints the ranked model recommendation with
  evidence (added to the `cmd/sworn/ledger.go` dispatch from S53).
- `config.ResolveImplementerModel(...)` (S09) consults `ledger.RecommendModel` for the
  slice kind and uses it as the resolved default when the corpus has enough signal;
  otherwise the existing flag → env → config → escalation-head precedence is unchanged.

## Background

S52 captures `model` + `attempt` per verdict; S53 aggregates pass-rate by (model,
slice_kind). This slice closes the loop: it consumes those aggregates to recommend a model
and wires that recommendation into the one place the harness chooses an implementer model —
S09's `ResolveImplementerModel` in `internal/config/config.go`. Because `config.go` is the
T3-owned hot file (also touched by T5, T6, T10), T16 depends on **T6** (the tail of the
T3 → T5 → T6 config.go chain) so this edit is serialised after every other config.go
writer, never parallel with one — matching the release's existing config.go convention.

## In scope

- New `internal/ledger/routing.go`:
  - `RecommendModel(records []Record, kind string) (Recommendation, bool)` — ranks models
    for the kind by pass-rate then attempts-to-pass, with a minimum-sample-size guard;
    returns `ok==false` (no confident recommendation) when evidence is below threshold.
  - `type Recommendation struct { Model string; PassRate float64; Sample int }`.
- `sworn ledger recommend <kind>` subcommand in `cmd/sworn/ledger.go` (extends S53's
  dispatch) printing the ranked recommendation + evidence; non-zero exit when no kind given.
- Wire into `internal/config/config.go`: `ResolveImplementerModel` consults
  `RecommendModel` (loading `docs/ledger/verdicts.jsonl` if present) and uses a confident
  recommendation as the resolved default. Precedence guard: an explicit flag or
  `$SWORN_IMPLEMENTER_MODEL` still wins; the ledger only improves the *default*, and an
  absent/thin corpus leaves S09's behaviour byte-for-byte unchanged.

## Out of scope

- Routing the **verifier** or other roles — only the implementer model is routed here. Why:
  implementer pass-rate is the signal the corpus measures; other roles need their own
  evidence axis. Tracking: future-release ledger follow-up. Ack: Brad, 2026-06-22.
- Cost-aware routing (cheapest model clearing a pass-rate bar) — **not deferred; sliced as
  S55 + S56** in this same track. The cost signal is local token-pricing, already returned by
  `model.Verifier.Verify` (`costUSD`) and computed by `internal/agent`/`oai.go` — it does NOT
  depend on the S06b commercial billing engine. S55 captures per-role cost into a `v:2`
  Record; S56 adds the `--optimize cost` objective. (Corrected 2026-06-23 — the original note
  conflated "billing engine" with "cost signal".)
- Auto-mutating `config.json` on disk — the recommendation influences the *resolved* model
  in memory, it does not rewrite the user's config file.

## Planned touchpoints

- `internal/ledger/routing.go` (new)
- `internal/ledger/routing_test.go` (new)
- `internal/config/config.go` (modify — `ResolveImplementerModel` consults the ledger;
  serialised behind the T3 → T5 → T6 config.go chain via the T6 dependency)
- `internal/config/config_test.go` (modify — recommendation-applied and thin-corpus
  fallback cases)
- `cmd/sworn/ledger.go` (modify — add `recommend` subcommand; same-track, after S53)

## Acceptance checks

- [ ] `RecommendModel` over a corpus where model A passes 9/10 harness slices and model B
  passes 3/10 returns A with its pass-rate and sample size, `ok==true`
- [ ] `RecommendModel` below the minimum-sample threshold returns `ok==false` (no confident
  pick) — the engine refuses to route on thin evidence
- [ ] `sworn ledger recommend harness` prints the ranked model with pass-rate + sample; with
  no kind argument it prints usage and exits non-zero
- [ ] `ResolveImplementerModel` returns the ledger-recommended model as the default when the
  corpus is confident AND no flag/env override is set
- [ ] With an explicit `--model` flag or `$SWORN_IMPLEMENTER_MODEL`, `ResolveImplementerModel`
  returns the override unchanged — the ledger never overrides an explicit choice
- [ ] With an absent or thin `docs/ledger/verdicts.jsonl`, `ResolveImplementerModel` returns
  exactly what S09 returned before this slice (regression-guarded by a fallback test)
- [ ] `go test ./internal/ledger/... ./internal/config/... ./cmd/sworn/...` passes;
  `go build ./...` succeeds with no new `go.mod` deps

## Required tests

- **Unit**: `internal/ledger/routing_test.go` — ranking, tie-break by attempts-to-pass,
  sample-size guard.
- **Integration**: `internal/config/config_test.go` — `ResolveImplementerModel` exercising
  recommendation-applied, flag-override-wins, and thin-corpus-fallback through the actual
  resolver (Rule 1: the routing is proven at the integration point that owns model choice,
  not only in the leaf engine).
- **Reachability artefact**: in `proof.md`, paste `sworn ledger recommend <kind>` output
  against the real corpus, plus the resolver tests passing.

## Risks

- The fallback path is the high-risk surface: a bug that lets a thin/empty corpus change the
  resolved model would silently alter harness behaviour for everyone. The byte-for-byte
  fallback regression test is the guard; treat its failure as a hard block.
- `config.go` conflict at `/merge-track` would mean the T6 serialisation was wrong. It is
  not a documented-shared file; a conflict here is a planner error per track-mode invariant 4.

## Deferrals allowed?

Yes, with Rule 2 compliance — verifier/other-role routing and cost-aware routing are
surfaced above with why / tracking / ack.
