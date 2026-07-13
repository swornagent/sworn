---
title: 'Proof — cold-start self-bootstrap dress rehearsal'
description: 'The reachability artefact for the cold-start cluster: the real sworn binary runs a freshly-planned release end-to-end (to the model boundary) with zero manual scaffolding.'
date: 2026-06-30
---

# Proof bundle — cold-start self-bootstrap

## Scope
Prove that `sworn run --parallel` can cold-start a freshly-planned release from
the CLI with no Driver-1 (/implement-slice) scaffolding — the gap the 2026-06-28
eval called "the single biggest" (`docs/captures/2026-06-28-sworn-eval-findings.md`).

## Files changed (since release/v0.1.0 merge 3880009)
- `internal/run/parallel.go` — release-worktree-path default + branch auto-create
- `internal/run/slice.go` — start_commit self-bootstrap + dispatch quadrant stamp
- `internal/board/track.go` — YAML inline-comment strip
- `internal/scheduler/worker.go` + `cmd/sworn/run.go` — repo-local track worktrees
- tests: `cold_start_test.go`, `dispatch_quadrant_test.go`, `track_comment_test.go`,
  `worktree_path_test.go`

## Test results
- `go build ./...` clean; `go test ./...` green (full suite, timeout 300s).
- Per-finding unit tests green (start_commit bootstrap, YAML strip, repo-local path).

## Reachability artefact (the live smoke run)
A throwaway git repo with a cold board (placeholder `release_worktree_path`, one
`planned` track/slice, no branch, no `.sworn`, no `start_commit`) run with the real
binary. Verbatim engine output:

```
RunParallel: release_worktree_path unset — defaulting to .../smoke-repo-worktrees/release-smoke-test (cold-start)
RunParallel: materialising release worktree ...
RunParallel: branch release-wt/smoke-test absent — creating it from HEAD (cold-start bootstrap)
sworn run --parallel: loaded 1 tracks in 1 phases
[T1-smoke] materialising worktree at .../smoke-repo-worktrees/release-smoke-test-T1-smoke
[T1-smoke] router: S01-smoke → implement
[T1-smoke] running slice S01-smoke
[T1-smoke] slice S01-smoke failed: RunSlice: create implementer agent for "openai/gpt-4o-mini": model: SWORN_OPENAI_API_KEY not set
```

Every cold-start finding fired correctly: release-worktree-path default (1),
branch auto-create (1), YAML-comment strip → board parsed (2), `.sworn` migrate
(5+6), no SIGSEGV (9), repo-local track worktree in `smoke-repo-worktrees` not
`sworn-worktrees` (3). The run reached the implementer dispatch and stopped only
at the missing provider key — the expected config boundary, not an engine fault.

## Delivered
- Cold-start self-bootstrap, end-to-end, proven by live run (above).
- All cold-start findings 1,2,3,5,6,7,9 closed (5,6,9 were already on-branch).

## Not delivered (tracked, Rule 2)
- A *completed* slice through a real model — gated on a provider key in the
  environment (`SWORN_<PROVIDER>_API_KEY`). This is the fired-run prerequisite,
  not an engine gap. Acknowledged: the dogfood target is the consumer repo's queued
  release.
- T16 capture remainder (durable cross-run store, token enrichment for
  impl/captain, real cost) — tracked #26 / driver-contract S07.

## Divergence from plan
None. The release-worktree-path default was an in-scope completion of finding 1
surfaced by the dress rehearsal (the placeholder strips to empty, which the old
hard-error rejected).
