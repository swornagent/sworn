---
title: Slice journal — S10-no-mock-boundary
description: Implementation log. Append-only. Verifier reads this as context; verdict is based on proof.md and repo state.
---

# Journal: `S10-no-mock-boundary`

## Session log

### `2026-06-23 12:00` — session start / state transition to in_progress

- **State**: `planned → in_progress`
- **Notes**:
  - Slice S10-no-mock-boundary assigned to track T2-delivery-cutover
  - Track worktree exists at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T2-delivery-cutover` (was checked out to `main` — fixed to track branch)
  - Prior slice S06-definition-of-ready is `verified` — sequential gate clear
  - status.json updated: state → in_progress, start_commit → 4d866d66
  - Implementation plan:
    1. Add `CheckBoundaryMocks` function + types to `internal/verify/verify.go`
    2. Wire boundary-mock check as first-pass gate in `verify.Run()`
    3. Add `OpenDeferrals` field to `Input` struct
    4. Write 12 tests in `internal/verify/verify_test.go`
    5. Add stop-don't-mock hard constraint to `internal/prompt/implementer.md`
    6. Add no-mock-boundary section to `internal/adopt/baton/rules/10-customer-journey-validation.md`

### `2026-06-23 12:30` — implementation complete

- **State**: `in_progress → implemented`
- **Notes**:
  - **Design decision**: Boundary-mock detection uses heuristic scanning — a line must match both a mock-marker pattern AND a validated-boundary pattern to be flagged. This is deliberately conservative: false negatives (missed mocks) are mitigated by breadth of patterns, and ambiguous cases are surfaced to the declared-deferral path where the human adjudicates.
  - **Boundary patterns**: DB (`sql.DB`, `*sql.Tx`, `database/sql`, `DB`), auth (`auth`, `Auth`, `Authenticate`, `Authorize`), entitlement (`entitle`, `premium`, `subscription`)
  - **Mock markers**: `mock`, `fake`, `stub`, `testdouble`, `newMock`, `newTest` (and case variants)
  - **Declared-mock registry**: `isDeclared()` checks open_deferrals for case-insensitive matches on boundary name + mock/fake/stub keyword
  - **All 12 tests pass** (12 S10 tests + 11 existing = 23 total, all green)
  - **proof.md** generated from live repo state with test output, git diff, vet results
  - **No deferrals** — this slice bans undeclared deferrals and carries none itself

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None (not yet submitted for verification).