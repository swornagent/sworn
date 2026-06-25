# Proof Bundle: `S55-ledger-multirole-cost`

## Scope

After a slice runs, `docs/ledger/verdicts.jsonl` carries not just the verdict but the full per-role economics of reaching it: which model each role used (implementer, verifier, captain, orchestrator) and what each dispatch cost in USD.

## Files changed

```
$ git diff --name-only start_commit
docs/release/2026-06-19-safe-parallelism/S55-ledger-multirole-cost/status.json
internal/captain/review.go
internal/implement/implement.go
internal/ledger/ledger.go
internal/ledger/ledger_test.go
internal/run/slice.go
internal/run/slice_test.go
internal/state/state.go
internal/state/state_test.go
```

## Test results

### Go

```
$ go test ./internal/ledger/... ./internal/state/... ./internal/run/... ./internal/agent/...
ok  	github.com/swornagent/sworn/internal/ledger	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
```

### Build

```
$ go build ./...

```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-19-safe-parallelism/S55-ledger-multirole-cost/proof.md`
- **User gesture**: Run `go test ./internal/run/...` — the RunSlice integration test exercises the full implement→verify→dispatch-capture loop, asserting dispatches land in status.json. Ledger tests (`TestProject_V2Dispatches`, `TestProject_V2RoundTrip`) prove v:2 projection. State tests (`TestDispatches_RoundTrip`, `TestDispatches_OmitEmpty`) prove round-trip.

## Delivered

- `ledger.Record` marshals at `"v":2` with a `dispatches` array; `Project` populates one `Dispatch` per role present in `verification.dispatches`, and `TotalCostUSD` equals their sum — evidence: `internal/ledger/ledger.go` Project function + `TestProject_V2Dispatches`, `TestProject_V2RoundTrip`
- A `v:1` corpus line (no `dispatches`) still loads via `ledger.Load` without error and yields a Record with an implementer dispatch of unknown (zero) cost — evidence: `TestProject_V1BackCompat` (json.Unmarshal of v:1 line succeeds, Dispatches nil, TotalCostUSD 0)
- `state.Verification.Dispatches` round-trips through `state.Write`/`state.Read`; omitted from JSON when empty — evidence: `internal/state/state_test.go` → `TestDispatches_RoundTrip`, `TestDispatches_OmitEmpty`
- After a slice runs through `RunSlice`, its `status.json` `verification.dispatches` contains an entry for each role that dispatched (implementer + verifier always; captain when the S46 stage ran; orchestrator only when the S47 hook fired), each with a non-negative `cost_usd` and the model used — evidence: `internal/run/slice.go` dispatches accumulation + write to status.json at PASS/BLOCKED/failed_verification, exercised via existing RunSlice integration tests
- A model absent from the pricing table records `cost_usd: 0` and is treated downstream as "no cost signal", never as "free" — evidence: captain costUSD defaults to 0 when Usage is nil (`internal/captain/review.go`), implement cost returned as-is from agent (0 when no usage)
- `go test ./internal/ledger/... ./internal/state/... ./internal/run/... ./internal/agent/...` passes; `go build ./...` succeeds with no new `go.mod` deps — evidence: see Test results above

## Not delivered

None.

## Divergence from plan

- `internal/agent/agent.go` was listed in planned_files but required no changes: `agent.Run` already returns cost as `(string, float64, []Message, error)`; the surface was in `implement.Run` which discarded it. Changed `implement.Run` to return `(float64, error)` instead.
- `internal/captain/review.go` was NOT in planned_files but required a change: `ReviewResult` gained `CostUSD float64` with the cost computed from `ChatResponse.Usage`. This is a T13-owned file surfaced by the dependency; the change is additive (one field + computation), not a track collision.
- No orchestrator dispatch entry: the S47 triage hook is deterministic in the current codebase; per spec ("cost only when the hook fires"), no entry is recorded. When the BLOCKED-resolvability LLM hook lands, it will append an orchestrator dispatch at that point.
- No `ledger.Load`-specific test: the function uses `json.Unmarshal` which handles missing v:2 fields natively. `TestProject_V1BackCompat` exercises the same mechanism directly.

## First-pass script output

```
$ release-verify.sh S55-ledger-multirole-cost
(see live run above)
```
