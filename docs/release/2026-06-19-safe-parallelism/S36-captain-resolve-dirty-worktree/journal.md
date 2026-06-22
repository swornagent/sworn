---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S36-captain-resolve-dirty-worktree`

## 2026-06-21 — planned (replan)

Added per Coach direction: dirty track worktrees are only ever caused by workers, so
the Coach has no context to resolve them — a fresh-context Captain call should
auto-resolve (commit by default, discard only if clearly wrong) and record the diff +
resolution, never paging. Captures the recurring T3/S06a + T8/S24 dirty-tree friction.

## Open questions

- Exact clean-worktree gate entry point in the sworn-native loop (may need a follow-up
  if `sworn run` has no captain-orchestration surface yet).

## Deferrals surfaced

None.

## Verifier verdicts received

*(None yet.)*

## 2026-07-04 — implemented

State transition: `design_review` → `in_progress` → `implemented`.

**Coach directives (from approved-ack.md):**
- Pin 1: Detector contract filters to tracked changes + touchpoint-scoped untracked files, aligning with spec Risk 2. No fire on stray `sworn` binary.
- Pin 2: `design_decisions` added to `status.json` (4 Type-2 decisions, matching S35 schema).
- Pin 3: `internal/prompt/prompt_test.go` added to `planned_files`.

**Implementation:**
- Added `resolve-dirty-worktree` function to `internal/prompt/captain.md` (lines 362–444): detector contract, decision rule (commit-by-default, discard only if clearly wrong), procedure, journal record format, session-end discipline.
- Added `TestCaptain_ResolveDirtyWorktree` to `internal/prompt/prompt_test.go` — verifies embedded prompt contains function name, commit-by-default rule, and discard guard.
- All 12 prompt tests pass; `go build ./...` passes.

**Design decisions recorded** (Type-2):
1. Function format matches existing Captain function structure.
2. Conservative commit-by-default with narrow discard criteria. Memory-cited: [[project_coach_loop_worktree_hygiene]].
3. Detector contract with filtered detection (spec Risk 2 alignment).
4. Prescriptive structured journal record format.

**Deferrals:** None — all spec ACs satisfied. The clean-worktree gate wiring in the sworn-native loop is a tracked follow-up (spec places it out of scope: "the sworn-native successor (sworn run) owns this going forward").

**Skeptic panel:** Skipped — runtime does not support subagent dispatch.