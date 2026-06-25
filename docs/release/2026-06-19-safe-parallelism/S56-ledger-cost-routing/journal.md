# Journal: `S56-ledger-cost-routing`

## Session log

### 2026-06-26T06:07 UTC — start implementation

- **State**: `in_progress`
- **Notes**:
  - Read spec, existing routing.go (S54), query.go, config.go, ledger.go
  - Identified all callers of `RecommendModel` (config_test.go, ledger_test.go, cmd/sworn/ledger.go, cmd/sworn/run.go)
  - Identified all callers of `ResolveImplementerModel` (config_test.go, cmd/sworn/run.go)
  - Design decisions:
    - `RecommendModel` signature: `(records, role, kind, obj Objective, floor float64)`. `role` param added per spec but currently used only for forward-compatibility — cost aggregation uses `TotalCostUSD` which is correct for implementer-only routing.
    - `modelStats` extracted to package level so helper functions (`qualityLess`, `costLess`, `balancedLess`, `pickCost`, `pickBalanced`) can use it.
    - `Recommendation` extended with `MeanCostUSD float64` and `Objective Objective`.
    - `Objective` uses `iota` enum with `String()` and `ParseObjective()` methods.
    - Default floor 0.8, configurable via `--floor` flag.
    - `OptimizeCost`: among models with pass-rate ≥ floor, sample ≥ MinSampleSize, and non-zero cost, pick lowest mean cost. Unpriced excluded. Fallback to quality mode when no model qualifies.
    - `OptimizeBalanced`: pass-rate per dollar, excluding unpriced. Fallback to quality mode.
    - `OptimizeQuality`: S54 behaviour preserved byte-for-byte — `qualityLess` function mirrors original sort.
    - `ResolveImplementerModel`: added `optimizeMode` and `passRateFloor` params. Precedence: param → `SWORN_OPTIMIZE_MODE` → config field. Config struct gains `OptimizeMode` and `PassRateFloor` fields.
    - `cmd/sworn/ledger.go`: `cmdLedgerRecommend` gains `--optimize` and `--floor` flags, `--role` becomes positional arg. Prints mean cost when available. Shows all ranked candidates for transparency.
    - `Report.Render`: added COST/EA column and per-role quality section (MISS_RATE, OVERTURN_RATE).
    - `CaptainMissRate`: share of slices with captain dispatch where verdict is FAIL/BLOCKED.
    - `VerifierOverturnRate`: share of multi-verdict slices where first and last verdict differ.

### 2026-06-26T06:30 UTC — implementation complete

- **State**: `implemented`
- **Notes**:
  - All 8 acceptance checks delivered with unit/integration test evidence.
  - 22 new/updated tests across routing, query, and config packages.
  - `go build ./...` passes with zero new dependencies.
  - S54 behaviour regression-tested: all existing quality-mode tests pass unchanged.
  - Divergences noted: `cmd/sworn/run.go` updated for signature change (not in planned_files); role-based cost filtering deferred per spec's out-of-scope boundary.

## Open questions

None.

## Deferrals surfaced

- **Non-implementer role routing** — **Why**: out of scope for S56 (implementer-only routing in this slice). **Tracking**: future-release ledger follow-up. **Acknowledged**: Brad, 2026-06-23 (in spec.md).
- **Proxy/billed-cost reconciliation against S06b credits** — **Why**: out of scope. **Tracking**: deferred as in S55. **Acknowledged**: Brad, 2026-06-23.
- **Role-specific cost filtering from Dispatches** — **Why**: `TotalCostUSD` used as proxy; per-role Dispatch filtering deferred until non-implementer roles are routed. **Tracking**: same future-release follow-up as above. **Acknowledged**: Brad (implicit in spec's out-of-scope).