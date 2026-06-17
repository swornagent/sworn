---
title: Slice journal template
description: Implementation log for one slice. Append-only. Visible to verifier as context, but verifier verdict is based on proof.md and repo state, not journal prose.
---

# Journal: `S02-ears-ac-format`

## Session log

### 2026-06-18 04:00 — session start

- **State**: `in_progress`
- **Notes**:
  - Track worktree at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core` (T1-fidelity-core, already materialised by S01).
  - Preceding slice S01-rtm-spine is `verified` — sequential gate clear.
  - No BLOCKED verdict — `verification.result` is `pending` on both track branch and release-wt.
  - `start_commit` set to `cd46236` (the start-implementation commit).

### 2026-06-18 04:15 — implementation decisions

- **Multi-line AC handling**: The real release's spec.md files use continuation indentation (checkbox line + indented continuation lines). The initial implementation only classified the checkbox line, which caused 20 false violations. Fixed by joining continuation lines (indented, non-checkbox, non-heading) into the AC text before classification. Added `TestValidate_MultiLineAC` to cover this.
- **IF-without-THEN classification**: An `IF` without `THEN` is an incomplete unwanted-behaviour pattern, not ubiquitous. The classifier now returns `PatternNone` for this case. Similarly, `THEN` without `IF` is a stray keyword and returns `PatternNone`.
- **Precondition matching scope**: Precondition keywords (WHEN/WHILE/WHERE/IF/THEN) are only meaningful before the SHALL clause. Keywords after SHALL are part of the action, not preconditions. The classifier extracts the precondition part (text before the SHALL clause) and only matches keywords there.
- **EARS vs Gherkin**: The spec records Gherkin as a considered-and-rejected alternative. This decision is not re-litigated; the Rule 08 doc extension records the rationale.

### 2026-06-18 04:30 — state transition to implemented

- **State**: `implemented`
- **Notes**:
  - All 4 acceptance checks demonstrably true (see proof.md Delivered section).
  - 20 unit tests + 6 integration tests pass. Full suite green. go vet clean. gofmt clean.
  - Live smoke test: `sworn lint ac 2026-06-16-fidelity-layer` exits 0 with 70 ACs classified; corrupted fixture exits 1 naming the slice + line.
  - No deferrals. No divergences from plan beyond the multi-line AC handling (additive, covered by test).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-06-18 — verifier verdict: FAIL

FAIL

Slice: `S02-ears-ac-format`

Violations:
1. Gate 2 — `cmd/sworn/ears_test.go` changed but not listed in planned touchpoints and not explained in proof.md "Divergence from plan"
   Evidence: `git diff --name-only cd462364..HEAD` includes `cmd/sworn/ears_test.go`; spec.md "Planned touchpoints" lists `cmd/sworn/ears.go` but not `cmd/sworn/ears_test.go`; proof.md "Divergence from plan" only mentions "Multi-line AC handling" and does not acknowledge `cmd/sworn/ears_test.go`.

Required to address:
1. Add to proof.md "Divergence from plan": "`cmd/sworn/ears_test.go` added as the integration test for `cmd/sworn/ears.go`; implied by the spec's Required tests section but not explicitly listed as a planned touchpoint."

All other gates (1, 3, 4, 5, 6) passed. The fix is a proof.md update only — no code changes needed.