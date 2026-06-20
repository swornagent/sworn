# Journal: S01-process-ownership

## Replan — 2026-06-20 (planner)

**Role**: Planner
**Trigger**: Verifier returned BLOCKED (2026-06-26) on two grounds:

1. **Gate 1 (primary — spec defect):** spec named `sworn run --parallel` as entry point
   and reachability smoke step. That flag is S02b's exclusive scope. S01's implementation
   correctly uses `sworn run --task`. Spec was wrong; implementation was correct.

2. **Gate 6 (subsumed — proof defect):** `proof.md` falsely attributed supervisor
   integration to `cmd/sworn/run.go`. Actual: `internal/run/run.go`. Proof must be
   corrected by the implementer before re-entering verification.

**Planner actions taken:**
- `spec.md` "User outcome", "Entry point", and "Required tests / reachability artefact"
  amended: `sworn run --parallel` → `sworn run --task` throughout.
- `status.json` `verification.result` cleared from `"blocked"` back to `"pending"`.
- `status.json` `state` remains `"implemented"` — the existing implementation satisfies
  the corrected spec.

**Implementer must do before next verification attempt:**
- Correct `proof.md` "Delivered": move the supervisor-wiring description from
  `cmd/sworn/run.go` to `internal/run/run.go`, and move it to "Divergence from plan"
  since that file was not in the planned touchpoints.
