---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S28-git-dir-guard`

## 2026-06-21 — planned (replan)

Added during `/replan-release` as the in-repo structural fix for sworn#6 (workers
writing to `main`). Root cause: `internal/git.Repo.run()` sets `cmd.Dir = r.Dir`,
which defaults to the ambient cwd when `Dir == ""`, so a `git checkout main` from a
zero-`Dir` Repo flips the calling track worktree. Observed on `T8-memory`/`S23`
(commit `ec97408` stranded on `main`); recovered manually in the same session.

Placed in a new track `T11-infra-safety` depending only on `T1-concurrency-core`
(merged) so it is immediately dispatchable and can land early — a safety fix should
not wait behind feature work. The harness defence-in-depth guard (coach-loop
post-dispatch worktree-branch assertion) already landed separately in
`~/.claude/bin/coach-loop`; this slice is the repo-side fix.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

*(None yet.)*
