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
