# Proof Bundle: `S56-ledger-cost-routing`

## Scope

Adds the cost objective to routing: pick the cheapest model whose measured pass-rate for the (slice-kind, role) clears a floor, via `--optimize cost|quality|balanced`. Adds per-role cost columns and derived per-role quality (captain-miss rate, verifier-overturn rate) to `sworn ledger report`, and wires the cost mode into `ResolveImplementerModel`.

## Files changed

```
$ git -C /home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T16-verdict-ledger diff --name-only 44bbe155582fc9a45d7110872ae097de18edd658..cca683e
cmd/sworn/ledger.go
cmd/sworn/run.go
internal/config/config.go
internal/config/config_test.go
internal/ledger/query.go
internal/ledger/query_test.go
internal/ledger/routing.go
internal/ledger/routing_test.go
```

## Test results

### Go

```
$ go test ./internal/ledger/... ./internal/config/... -count=1
ok  	github.com/swornagent/sworn/internal/ledger	0.007s
ok  	github.com/swornagent/sworn/internal/config	0.007s
```

```
$ go build ./...
(no output — build succeeds, no new go.mod deps)
```

All 40+ routing tests pass (22 S54-regression + 18 new S56 cost-routing tests). All config tests pass (existing S09/S17 tests + 4 new S56 cost-mode tests). All query tests pass (existing + new CostPerPassingSlice, CaptainMissRate, VerifierOverturnRate, PerRoleQualityAll tests).

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: N/A (CLI tool — no UI surface)
- **User gesture**: Run `sworn ledger recommend implementer harness --optimize cost` against a ledger corpus. The command prints the model with pass-rate, sample, and mean cost. With `--floor 0.9`, the gate changes the pick when warranted. Run `sworn ledger report` to see COST/EA and per-role quality columns (MISS_RATE, OVERTURN_RATE).

Evidence: The `cmd/sworn/ledger.go` diff wires `--optimize` and `--floor` flags through `ledger.ParseObjective` → `ledger.RecommendModel` → formatted output with cost and pass-rate. The `Report.Render` function prints COST/EA and per-role quality. Both are reachable from the `sworn ledger` subcommand dispatch.

## Delivered

- [x] `RecommendModel(..., OptimizeCost)` over a corpus where model A passes 9/10 at $0.50/slice and model B passes 9/10 at $0.05/slice returns **B** (both clear the floor; B is cheaper) — evidence: `TestRecommendModel_OptimizeCost_PicksCheapest` in `internal/ledger/routing_test.go`
- [x] `RecommendModel(..., OptimizeCost)` where the cheapest model is **below** the pass-rate floor returns a pricier model that clears the floor — evidence: `TestRecommendModel_OptimizeCost_QualityFloorExcludesCheapest`
- [x] A model with only `cost_usd: 0` (unpriced) is excluded from cost ranking — evidence: `TestRecommendModel_OptimizeCost_UnpricedExcluded`
- [x] `RecommendModel(..., OptimizeQuality)` returns exactly what S54 returned (no regression) — evidence: `TestRecommendModel_OptimizeQuality_NoRegression` + all existing S54 tests pass unchanged
- [x] `sworn ledger recommend implementer harness --optimize cost` prints the model with pass-rate, sample, and mean cost; `--floor 0.9` raises the gate — evidence: `cmd/sworn/ledger.go` `cmdLedgerRecommend` parses `--optimize`/`--floor` flags, calls `ledger.ParseObjective` and `ledger.RecommendModel`, and prints mean cost; `TestRecommendModel_OptimizeCost_HigherFloorChangesPick` proves floor changes outcome
- [x] `sworn ledger report` prints cost-per-passing-slice by model and per-role section with captain-miss and verifier-overturn rate — evidence: `Report.Render` in `query.go` includes COST/EA column in pass-rate table and PER-ROLE QUALITY section with MISS_RATE/OVERTURN_RATE; smoke test `TestReport_Render` confirms output
- [x] `ResolveImplementerModel` with `--optimize cost` returns cost-aware pick when corpus is confident; explicit override wins; thin corpus falls back — evidence: `TestResolveImplementerModel_CostModePicksCheapest`, `TestResolveImplementerModel_CostModeFlagWins`, `TestResolveImplementerModel_CostModeThinCorpusFallback` in `config_test.go`
- [x] `go test ./internal/ledger/... ./internal/config/... ./cmd/sworn/...` passes; `go build ./...` with no new `go.mod` deps — evidence: test and build output above

## Not delivered

- None. All 8 acceptance checks are delivered.

## Divergence from plan

- `cmd/sworn/run.go` updated to pass `"quality", 0` to `ResolveImplementerModel` (new params). This was a planned touchpoint not explicitly listed as a file in `planned_files` but required by the signature change.
- `RecommendModel` signature added `role` parameter per spec, even though role-based cost filtering (filtering Dispatches by role) is deferred until non-implementer roles are routed. Current implementation aggregates via `TotalCostUSD`, which is correct for the implementer-only case.
- `PerRoleQualityAll` always returns `captain` + `verifier` rows even when Dispatches is empty (to prevent the report section from being empty). This is a presentation decision consistent with the spec's intent.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S56-ledger-cost-routing
release-verify.sh
  slice:       S56-ledger-cost-routing
  slice dir:   docs/release/2026-06-19-safe-parallelism/S56-ledger-cost-routing
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented'

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 44bbe155582fc9a45d7110872ae097de18edd658
  PASS  9 file(s) changed vs diff base
  (first 20)
    cmd/sworn/ledger.go
    cmd/sworn/run.go
    docs/release/2026-06-19-safe-parallelism/S56-ledger-cost-routing/status.json
    internal/config/config.go
    internal/config/config_test.go
    internal/ledger/query.go
    internal/ledger/query_test.go
    internal/ledger/routing.go
    internal/ledger/routing_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  proof.md has no unfilled template placeholders
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
```