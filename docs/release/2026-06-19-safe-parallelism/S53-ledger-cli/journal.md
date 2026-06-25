---
title: 'Journal: S53-ledger-cli'
description: Implementation log for S53-ledger-cli — sworn ledger sync + report
---

# Journal: `S53-ledger-cli`

## Session log

### 2026-07-22 — session start

- **State**: planned → in_progress
- **Notes**:
  - Track worktree at `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T16-verdict-ledger`
  - Predecessor S52 is verified; dependency gate clear.

### 2026-07-22 — implementation complete

- **State**: in_progress → implemented
- **Notes**:
  - Registered `ledger` command via per-file `init()` calling `command.Register` — follows S51/T15 pattern, avoids editing `commands.go`.
  - `internal/ledger/query.go` adds `Load`, `PassRateByModelKind`, `AttemptsToPass`, `GateFailureHistogram`, and a `Report` renderer using `text/tabwriter`.
  - `cmd/sworn/ledger.go` implements `sync` (walk board → Project → Append) and `report` (Load → Render).
  - Added `CountLines` to `internal/ledger/ledger.go` (S52's file) for accurate idempotent-sync reporting. Non-breaking addition; does not change `Append` signature.
  - Sync reuses `findRepoRoot()` from `cmd/sworn/baton.go` — same package, no export needed.
  - Gate counting reads `- [ ]` lines from `spec.md`; non-standard AC styles under-count (acceptable per spec; noted in code).
  - 16 parse errors on sync from schema-evolved status.json files — `state.Read` can't parse older `open_deferrals` as objects or newer `design_decisions` as arrays. Handled gracefully (errors reported, processing continues).
  - 31 tests pass (23 in internal/ledger, 8 in cmd/sworn); build clean with no new deps.
  - Reachability artefact: `sworn ledger sync` → 87 records harvested; `sworn ledger report` → all three aggregates render.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-07-22 — verifier PASS

- **Verdict**: PASS
- **Verifier session**: fresh, artefact-only
- **Gates passed**: 1 (user-reachable outcome), 2 (touchpoints match), 3 (tests + integration point), 4 (reachability artefact), 5 (no silent deferrals), 6 (design conformance — no UI config), 7 (scope matches)
- **Next step**: /implement-slice S54-ledger-routing 2026-06-19-safe-parallelism
