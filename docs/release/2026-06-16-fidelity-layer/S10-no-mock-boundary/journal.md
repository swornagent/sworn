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

### `2026-06-25` — verifier verdict: FAIL (round 2, fresh-context)

FAIL: 1 violation

1. **Gate 2 — `start_commit` contains the implementation**: `status.json` records `start_commit: bfdede8de70d42dffecc26328e7d5df4f346e761`. That commit (labeled `docs(release/S10-no-mock-boundary): start re-implementation`) contains all the re-implementation's production code changes (`cmd/sworn/main.go`, `internal/run/run.go`, `internal/verify/verify.go`, `internal/verify/verify_test.go`). Running `git diff --name-only bfdede8..HEAD` yields only 3 docs files (journal.md, proof.md, status.json) — none of the planned touchpoints appear in the canonical verifier diff. The proof.md "Files changed" section diffs from `4d866d66af5b7fe33b1282eef458ea664dd30974` (the original implementation's `start_commit`), which disagrees with `status.json`'s field. The verifier cannot audit implementation scope via the `diff start_commit..HEAD` mechanism. Required fix: set `status.json` `start_commit` to `cec70a6e` (the round-1 FAIL verdict commit, immediately before the re-implementation began) and update proof.md "Files changed" to diff from that commit. Identical pattern to S07 round-1 FAIL, S05 round-4 FAIL, and S15 round-1 FAIL.

Gates 1, 3, 4, 5, 6 all PASS. All 12 S10 tests and all `internal/run/` tests pass fresh. `TestRun_DeclaredBoundaryMockAllowed` asserts rationale contains "Declared boundary mock" and mock type. Entry points correctly wired (`--deferral` flag, `open_deferrals` read in run.go). No silent deferrals in production files. All 4 ACs have verifiable evidence.

Next: `/implement-slice S10-no-mock-boundary 2026-06-16-fidelity-layer` to address 1 violation (update `start_commit` in `status.json` and proof.md "Files changed").

### `2026-06-25 13:00` — fix Gate 2 violation (start_commit + proof.md)

- **State**: `failed_verification → in_progress → implemented`
- **Violation fix**: Set `status.json` `start_commit` to `cec70a6e` (the round-1 FAIL verdict commit, immediately before re-implementation code). Updated proof.md "Files changed" to diff from `cec70a6e` instead of `4d866d66`, matching the canonical base the verifier expects.
- **Verification**:
  - All 12 S10 tests + full `internal/verify/` suite pass
  - All `internal/run/` tests pass
  - `go vet` clean on all affected packages
  - `release-verify.sh` — all checks pass (state transitioned from in_progress to implemented)
- **No deferrals** — this slice bans undeclared deferrals and carries none itself.
### `2026-06-19` — verifier verdict: FAIL

FAIL: 2 violations

1. **AC2 + Rule 1 — Declared-deferral path not wired at entry points**: `internal/run/run.go` (line 232) and `cmd/sworn/main.go` cmdVerify (line 111) both call `verify.Run()` without populating `OpenDeferrals`. Neither reads `open_deferrals` from `status.json` and passes it through. As a result, every boundary mock is treated as undeclared in any real invocation (`sworn verify` or `sworn run`). AC2 (declared boundary mock allowed) is only exercised in unit tests — not user-reachable via the integration entry point. Rule 1 violation.

2. **AC2 / Required Tests — "passes-with-note" not verified**: The spec's Required Tests say "declared mock (with the three components) passes-with-note." `TestRun_DeclaredBoundaryMockAllowed` only asserts `got.Verdict == verdict.Pass` — it does not verify the declared mock is surfaced in the output as a known deferral. AC2 states "THE SYSTEM SHALL allow it and surface it in the run output as a known deferral," a SHALL with no test coverage.