---
title: 'S07 — Pause/resume reads committed status.json'
description: 'Fix findFirstNonTerminal to read committed (git-visible) status.json from the track branch rather than the working-tree copy, so sworn run --resume correctly identifies the first non-terminal slice after a crash or pause.'
---

# Slice: `S07-pause-resume-committed`

## User outcome

When a `sworn run` session is interrupted and then resumed with `sworn run --resume --release <name>`, the loop correctly identifies the first non-terminal slice by reading committed status.json files (via `git show <track-branch>:path/to/status.json`) rather than the working-tree copy, so a dirty or partially-written working-tree does not cause the resume to re-run the wrong slice.

## Entry point

`sworn run --resume --release <name>` → `internal/scheduler/worker.go` `findFirstNonTerminal` function (confirmed location from audit + source grep).

## In scope

- `internal/scheduler/worker.go`: update `findFirstNonTerminal` to use `board.Oracle.ReadSliceState(sliceID)` (committed read) instead of `state.Read(workingTreePath)`
- `cmd/sworn/run.go`: add `--resume` flag if not already present; on resume, the scheduler calls the updated `findFirstNonTerminal`
- Test: `internal/scheduler/worker_test.go` — add resume scenario where the working-tree status.json shows `in_progress` but the committed status.json shows `planned` → assert findFirstNonTerminal returns the `planned` slice (not the `in_progress` one)

## Out of scope

- Changing what the oracle reads from (index.md vs board.json — that is S14)
- The PAGE event path (S03)
- Any changes to triage or model dispatch

## Planned touchpoints

- `internal/scheduler/worker.go` (update findFirstNonTerminal to use oracle)
- `cmd/sworn/run.go` (add or confirm --resume flag)

## Acceptance checks

- [ ] WHEN `sworn run --resume` is called and a slice's working-tree status.json shows `in_progress` but the committed git-visible status.json shows `planned`, THE SYSTEM SHALL use the `planned` state (committed wins over working-tree)
- [ ] WHEN `sworn run --resume` is called and a slice's committed status.json shows `implemented`, THE SYSTEM SHALL skip past it (non-terminal states `verified`, `implemented`, `shipped` are terminal for findFirstNonTerminal purposes — start from the first `planned` or `in_progress` committed slice)
- [ ] WHEN the track branch cannot be read (worktree not yet created), THE SYSTEM SHALL fall back to the release-wt slice state (not working-tree)
- [ ] Test: a dirty working-tree state (status.json showing `in_progress`) does not confuse `findFirstNonTerminal` when committed state is `planned`

## Required tests

- **Unit**: `internal/scheduler/worker_test.go` add `TestFindFirstNonTerminalCommitted` — mock oracle returning different state than working-tree; assert oracle state wins
- **Reachability artefact**: `go test ./internal/scheduler/... -v -run TestFindFirstNonTerminalCommitted` exits 0

## Risks

- The oracle read requires a git `show` command; if run in a detached HEAD or a bare repo clone, this may fail — the fallback to release-wt (per AC3) covers the common crash-recovery case

## Deferrals allowed?

No.
