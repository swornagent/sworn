---
title: 'S02a-run-refactor — extract RunSlice() for goroutine-callable execution'
description: 'Refactor internal/run so the implement→verify inner loop is exported as RunSlice(), callable by a goroutine worker with an existing worktree and spec path. The turnkey Run() path is unchanged.'
---

# Slice: `S02a-run-refactor`

## User outcome

A track worker (S02b) can call `run.RunSlice(ctx, worktreeRoot, specPath, opts)` to
implement and verify one slice in an existing worktree without creating branches or
release directories — while `sworn run <task>` continues to behave identically to today.

## Entry point

Internal API change: `internal/run` exports a new `RunSlice()` function. The existing
`Run()` entry point is unchanged from the user's perspective.

## In scope

- Extract the implement→verify loop from `Run()` into `RunSlice(ctx, worktreeRoot, specPath, statusPath, opts RunSliceOptions) error`
- `RunSliceOptions`: ImplementerModel, VerifierModel, EscalationModels, RetryCap,
  NewAgent, NewVerifier (subset of current Options, minus task/branch/base concerns)
- `Run()` is refactored to call `RunSlice()` internally after its setup phase
  (create release dir, branch, commit initial slice files, record start_commit)
- `RunSlice()` assumes: the worktree exists, the spec.md is at specPath, the
  status.json is at statusPath, the branch is already checked out
- `RunSlice()` owns: the implement→verify retry loop, verdict handling, state
  transitions (in_progress → implemented → verified | failed_verification)
- `RunSlice()` does NOT: create branches, commit the merge, manage git-level setup —
  those remain in `Run()` for the turnkey path, and will be handled by the scheduler
  worker in S02b for the parallel path

## Out of scope

- The scheduler that calls RunSlice() (S02b)
- The `--parallel` flag (S02b)
- Board reading / track discovery (S02b)
- Worktree materialisation (S02b)

## Planned touchpoints

- `internal/run/run.go` (refactor — extract inner loop, keep Run() signature stable)
- `internal/run/slice.go` (new — RunSlice() and RunSliceOptions live here)
- `internal/run/run_test.go` (update — existing tests still pass; add TestRunSlice)

## Acceptance checks

- [ ] `run.RunSlice(ctx, worktreeRoot, specPath, statusPath, opts)` exists as an
  exported function in `internal/run`
- [ ] All existing `go test ./internal/run/...` tests pass unchanged after the refactor
- [ ] `TestRunSlice`: given a fixture spec.md + status.json in a temp dir, calling
  `RunSlice` with a mock implementer and mock verifier (returning PASS) transitions
  the status.json to `verified` and returns nil
- [ ] `TestRunSliceFail`: mock verifier returns FAIL on all attempts; RunSlice returns
  a non-nil error; status.json ends in `failed_verification`
- [ ] `sworn run <task>` (end-to-end via existing tests) produces identical behaviour
  to before the refactor — no regression
- [ ] `RunSlice` is goroutine-safe: `go test -race ./internal/run/...` passes

## Required tests

- **Unit**: `internal/run/run_test.go` — update existing tests to pass; add
  `TestRunSlice` and `TestRunSliceFail` as above
- **Reachability artefact**: `go test -race ./internal/run/...` output showing all
  tests pass. Document in proof.md.

## Risks

- The refactor may expose hidden shared state in the current `run.go` that was safe
  for single-threaded use but will fail the race detector when `RunSlice` is called
  concurrently. The race detector requirement (AC-6) catches this immediately.

## Deferrals allowed?

No. S02b cannot be implemented until RunSlice() exists.
