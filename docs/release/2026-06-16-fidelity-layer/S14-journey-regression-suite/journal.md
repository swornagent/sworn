---
title: 'S14-journey-regression-suite journal'
description: 'Implementation log for S14-journey-regression-suite.'
---

# Journal: `S14-journey-regression-suite`

## Session log

### 2026-06-19 — Planner decision: Option A ratified (exit 1 on gap-at-start)

- **State**: `implemented (BLOCKED) → failed_verification`
- **Trigger**: Verifier issued BLOCKED (2nd consecutive) routing to `/replan-release`. Both BLOCKED verdicts named AC1's "exit non-zero" requirement as unmet — implementation exits 0 when gaps are filled during the same run (CodifyWalkedJourneys always sets HasRegression=true, making the exit-1 branch dead code).
- **Decision**: Human ratified **Option A** — AC1 is correct as written. `sworn journeys --regen` SHALL exit non-zero if any coverage gaps existed at run start, even if those gaps were filled during the same run. Exit 0 only when no gaps existed at start.
- **Spec.md**: No change needed — AC1 as written is the ratified intent.
- **Required implementer fixes**:
  1. Capture pre-codification gap count (call `RegressionCoverageGaps()` before `CodifyWalkedJourneys()` runs). If any gaps existed, exit 1 after codification completes — even if gaps are now 0.
  2. Update `TestJourneysRegenCmd_CoverageGapFilled` in `cmd/sworn/journeys_regen_test.go` to assert exit 1 (gap existed at run start → signals scaffolds were generated and must be committed).
  3. Fix self-contradiction in `internal/adopt/baton/rules/10-customer-journey-validation.md` Coverage check — "exits non-zero if gaps remain after codification" must read "exits non-zero if gaps existed at run start."
  4. Update proof.md Divergence section to document the pre/post gap-count pattern.
- **Cleared**: `verification.result` reset to `pending` so verifier session starts fresh after re-implementation.

## Open questions

None.

## Deferrals surfaced

- `Scaffold-not-complete-oracle`: sworn emits a structured starting test per journey + a coverage check, not a complete oracle. **Why** — a complete journey oracle is project-specific E2E work. **Tracking** — project E2E backlog per consuming project. **Acknowledged** — 2026-06-16 (from spec).
