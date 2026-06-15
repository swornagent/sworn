---
title: S05-state-and-git
description: Native slice state machine (status.json) + git branch/commit/diff operations.
---

# Slice: `S05-state-and-git`

## User outcome

Slice state (`status.json`) and git operations (branch, stage, commit, diff) are
driven natively in Go — the substrate the run-loop orchestrates.

## Entry point

Internal `state` + `git` packages, consumed by the run-loop (S07).

## In scope

- `state`: read/write `status.json`; enforce transitions
  `planned → in_progress → implemented → verified | failed_verification`.
- `git`: branch create/checkout, stage, commit, capture `start_commit`, compute a
  slice diff (`start_commit..HEAD`). Single backend (go-git or `git` exec).

## Out of scope

- Worktree/track orchestration and merge (S07).

## Planned touchpoints

- `internal/state/`, `internal/git/`

## Acceptance checks

- [ ] State transitions persist to `status.json` and reject illegal jumps
      (e.g. `planned → verified`).
- [ ] A branch + commit is created; `start_commit` is captured.
- [ ] The slice diff equals `start_commit..HEAD`.

## Required tests

- **Unit/Integration**: a temp git repo; assert commit creation, the diff range,
  and the state transitions (including a rejected illegal transition).

## Risks

- go-git vs `git` exec portability — pick one and document.
- Concurrent state writes — single-writer per slice.

## Deferrals allowed?

No.
