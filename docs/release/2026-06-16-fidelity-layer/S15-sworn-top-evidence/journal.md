---
title: Slice journal — S15-sworn-top-evidence
description: Implementation log for S15-sworn-top-evidence (read-only journey evidence surface). Append-only.
---
# Journal: S15-sworn-top-evidence
> Copy this file to `docs/release/<release-name>/<slice-id>/journal.md`. Append entries chronologically. Do not delete history. Decisions captured here must also land in commit message bodies per Rule 4 — this journal is a working surface, not a substitute for durable capture.

## Session log

### 2026-06-23 — implemented

- **State**: `planned → implemented` (single session)
- **Notes**:
  - Materialised track worktree for T4-evidence-surface.
  - Added `internal/journey/walkthrough.go` with `WalkStatus`, `Attestation`, and `AttestationArtefact` types — the attestation API surface sworn top reads. `LoadAttestations()` returns empty artefact when file doesn't exist (optional until S13).
  - Added `internal/journey/walkthrough_test.go` — 7 tests for load, parse, status lookup, path.
  - Implemented `cmd/sworn/top.go` with `cmdTop()` entry point and `renderEvidenceSurface()` that renders green-board / kill-list / empty-state.
  - Added `cmd/sworn/top_test.go` — 7 tests: empty-state, green-board, kill-list (un-walked), kill-list (failed), read-only assertion, mixed statuses, empty journeys.
  - Added `case "top"` to `cmd/sworn/main.go` switch.
  - Read-only guarantee enforced by `TestTop_ReadOnly` (filesystem snapshot before/after).
  - First-pass verification: 18/18 PASS.
  - **Divergence from planned_files**: added `internal/journey/walkthrough.go` and `walkthrough_test.go` — forward-extension of the journey package per spec's "existing public APIs" description. S13 will populate the attestation artefact; S15 reads it.

## Open questions
None.

## Deferrals surfaced
None.

## Verifier verdicts received

### 2026-06-18 — FAIL (round 1, fresh-context)

- **Actor**: verifier (fresh-context session)
- **Note**: Two violations.
  1. **Gate 2** — `start_commit` is set to the implementation commit itself (`a58733d`). Running `git diff --name-only a58733d..HEAD` (per protocol) returns only 4 doc files — none of the planned touchpoints (`cmd/sworn/top.go`, `cmd/sworn/main.go`) appear in the required diff range. proof.md "Files changed" used `git diff --name-only release-wt/2026-06-16-fidelity-layer` instead of `git diff --name-only <start_commit>`, masking this error. proof.md "Not delivered" incorrectly claims "None." Fix: set `start_commit` to `e3b0ec2` (the commit immediately before `a58733d`); update proof.md "Files changed" to use the corrected range.
  2. **Gate 3** — All 7 tests in `top_test.go` call `renderEvidenceSurface` directly, bypassing the CLI entry point `cmdTop`. The spec requires "Integration: `sworn top` against a fixture release with mixed journey statuses (Rule 1 via the command entry point)." The codebase convention (explicit comment in `lint_ac_test.go`: "drives the actual command entry point (cmdLintAC), not just the ears package") confirms that Rule 1 requires `cmdTop`. Fix: add at least one test calling `cmdTop([]string{"<release>", dir})` exercising the mixed-statuses path.
  - Gates 1, 4, 5, 6 all PASS: `case "top"` is wired in `main.go`; smoke step is described; no silent deferral markers; all 4 AC evidence references are real. Implementation is functionally correct — both violations are protocol/test-layer defects, not logic errors.
  - Slice state → `failed_verification`. Next: `/implement-slice S15-sworn-top-evidence 2026-06-16-fidelity-layer` in a fresh session to address the 2 numbered violations.
### 2026-06-23 — re-entry from failed_verification -> implemented

- **State**: `failed_verification -> implemented` (re-entry session)
- **Notes**:
  - Re-entered as implementer via /implement-slice to fix 2 verifier violations.
  - **Gate 2 fix**: Changed `start_commit` from `a58733d` to `e3b0ec2` (the materialise-commit before the first impl commit). `git diff --name-only e3b0ec2..HEAD` now correctly shows all 9 changed files including the 3 planned touchpoints.
  - **Gate 3 fix**: Added `TestTopCmd_MixedStatuses` — an integration test that calls `cmdTop([]string{"test-release", dir})` directly (the CLI entry point), following the same pattern as `lint_ac_test.go`. Verifies exit 1 for a mixed-status kill-list.
  - Updated proof.md with corrected diff range, new integration test results, and a "Verifier violations resolved" section.
  - All tests pass (8 total: 7 existing + 1 new cmdTop integration test).
  - First-pass verification: 18/18 PASS (re-run).
  - **No new divergence**: Both fixes are protocol/test-layer corrections to an otherwise functional implementation.
