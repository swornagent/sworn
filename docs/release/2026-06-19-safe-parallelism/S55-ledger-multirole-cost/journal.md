---
title: S55-ledger-multirole-cost journal
description: Implementation log for S55 — per-role model + USD cost capture in verdict ledger
---

# Journal: `S55-ledger-multirole-cost`

## Session log

### 2026-07-22 — session start / in_progress

- **State**: in_progress
- **Notes**:
  - S55 is the fourth slice in track T16-verdict-ledger (after S52/S53/S54, all verified).
  - Prior slices established: Record v:1 with Model/Attempt (S52), CLI ledger cmd (S53), history-backed routing (S54).
  - This slice evolves Record to v:2 with per-role dispatch costs.

### 2026-07-22 — implementation decisions

- **State**: → implemented
- **Decisions**:
  - **Dispatch type**: defined `state.Dispatch` in `internal/state/state.go` with Role/Model/CostUSD/Attempt. Ledger Record reuses `state.Dispatch` directly — no duplication.
  - **Implementer cost surfacing**: `agent.Run` already returns `(text, cost, messages, error)`. `implement.Run` was discarding cost. Changed `implement.Run` signature to `(costUSD float64, err error)` — chain is now: agent.Run → implement.Run → RunSlice.
  - **Captain cost surfacing**: Added `CostUSD float64` to `captain.ReviewResult`, computed from `resp.Usage.TotalTokens * 0.000002` (same nominal estimate as `agent.computeCost` — ~$2/1M tokens). Zero when Usage is nil (unpriced model).
  - **Orchestrator dispatch**: Not recorded. S47 triage is deterministic — no LLM hook exists yet. Per spec: "cost only when the hook fires." When the BLOCKED-resolvability hook lands, it will append an orchestrator dispatch.
  - **Dispatcher accumulation in RunSlice**: `var dispatches []state.Dispatch` declared before captain review. Appended: captain (after review), implementer (after successful implement), verifier (after verify). Written to `st.Verification.Dispatches` at PASS/BLOCKED/failed_verification transitions.
  - **v:2 back-compat**: `Dispatches` uses `omitempty` JSON tag. v:1 lines without the field unmarshal with nil Dispatches and V=1 — Go's `json.Unmarshal` handles missing fields natively. `Load` in `query.go` already skips unparseable lines, so a v:2 field on a v:1 reader would be silently ignored.
- **Trade-offs**:
  - Chose nominal token-cost estimate ($2/1M) for captain dispatch rather than plumbing the modelPricing table into the captain package. This keeps the package boundary clean; the captain package doesn't import model internals.
  - Chose not to add an orchestrator dispatch entry (cost 0) — the spec says "absent when deterministic path is taken" and adding a zero-cost entry would create ambiguity with "unpriced model".
- **Divergences**:
  - `internal/agent/agent.go`: listed in planned_files but no change needed (cost already returned by `agent.Run`).
  - `internal/captain/review.go`: NOT in planned_files but needed `CostUSD` field. Additive change; T13-owned file surfaced by dependency.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.