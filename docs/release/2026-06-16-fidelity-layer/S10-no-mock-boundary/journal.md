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

### `2026-06-25 12:00` — re-implementation session (from failed_verification)

- **State**: `failed_verification → in_progress → implemented`
- **Violation 1 fix — Wire OpenDeferrals at entry points**:
  - `cmd/sworn/main.go`: Added `--deferral` repeatable flag to `sworn verify` subcommand;
    values passed through as `verify.Input.OpenDeferrals`.
  - `internal/run/run.go`: Before calling `verify.Run()`, reads `open_deferrals` from
    slice's `status.json` and passes them through to `verify.Input.OpenDeferrals`.
  - `internal/bench/runner.go` not changed — benchmarks use synthetic tasks without
    status.json context; not a production entry point.
- **Violation 2 fix — Surface declared mocks in output**:
  - `internal/verify/verify.go`: After model verification, if `CheckBoundaryMocks` found
    declared mocks, prepend the declared mock info to the result's `Rationale`.
  - `internal/verify/verify_test.go`: Updated `TestRun_DeclaredBoundaryMockAllowed` to
    assert `Rationale` contains "Declared boundary mock" and the mock type detail.
- **Verification**: 
  - All 12 S10 tests + full `internal/verify/` suite pass
  - All `internal/run/` tests pass
  - `go vet` clean on all affected packages
  - `go test ./internal/...` — all green
- **No deferrals** — this slice bans undeclared deferrals and carries none itself.

## Verifier verdicts received
### `2026-06-19` — verifier verdict: FAIL

FAIL: 2 violations

1. **AC2 + Rule 1 — Declared-deferral path not wired at entry points**: `internal/run/run.go` (line 232) and `cmd/sworn/main.go` cmdVerify (line 111) both call `verify.Run()` without populating `OpenDeferrals`. Neither reads `open_deferrals` from `status.json` and passes it through. As a result, every boundary mock is treated as undeclared in any real invocation (`sworn verify` or `sworn run`). AC2 (declared boundary mock allowed) is only exercised in unit tests — not user-reachable via the integration entry point. Rule 1 violation.

2. **AC2 / Required Tests — "passes-with-note" not verified**: The spec's Required Tests say "declared mock (with the three components) passes-with-note." `TestRun_DeclaredBoundaryMockAllowed` only asserts `got.Verdict == verdict.Pass` — it does not verify the declared mock is surfaced in the output as a known deferral. AC2 states "THE SYSTEM SHALL allow it and surface it in the run output as a known deferral," a SHALL with no test coverage.