# Proof bundle — fix prodmerge-gitfile-noop (2026-07-02)

## Scope

Make `ProductionMergeTrack`'s Rule 11 target assertion fail closed: accept a
linked worktree's `.git` FILE (gitdir pointer) as a valid git target so the
production track auto-merge actually engages, and return an error — not nil —
when the target is not a git worktree at all.

## Files changed

`git diff --name-only 632d4f3`:

```
cmd/sworn/run_test.go
internal/run/parallel.go
internal/run/parallel_test.go
```

## Test results

RED first (guard inversion reproduced before the fix):

```
$ go test -timeout 120s -run 'TestProductionMergeTrack' ./internal/run/ -v
=== RUN   TestProductionMergeTrack_LinkedWorktree
    parallel_test.go:943: track commit e8a101d940a04b8ac04cf073b2fc19bae013f29b not an ancestor of release HEAD: merge silently skipped
--- FAIL: TestProductionMergeTrack_LinkedWorktree (0.04s)
=== RUN   TestProductionMergeTrack_NonGitTargetErrors
    parallel_test.go:952: expected error for non-git merge target, got nil
--- FAIL: TestProductionMergeTrack_NonGitTargetErrors (0.00s)
FAIL	github.com/swornagent/sworn/internal/run	0.059s
```

GREEN after the fix:

```
$ go test -timeout 120s -run 'TestProductionMergeTrack' ./internal/run/ -v
--- PASS: TestProductionMergeTrack_LinkedWorktree (0.05s)
--- PASS: TestProductionMergeTrack_NonGitTargetErrors (0.00s)
ok  	github.com/swornagent/sworn/internal/run	0.064s
```

Touched-package suites (slice-relevant, not full repo):

```
$ go test -timeout 300s ./cmd/... ./internal/run/... ./internal/scheduler/...
ok  	github.com/swornagent/sworn/cmd/sworn	44.251s
ok  	github.com/swornagent/sworn/internal/run	4.734s
ok  	github.com/swornagent/sworn/internal/scheduler	0.164s
```

`go vet ./internal/run/... ./cmd/sworn/` clean. `gofmt -l` residue in those
packages is pre-existing drift (identical list with this change stashed) and
was left untouched.

## Reachability artefact

Two levels:

1. `TestProductionMergeTrack_LinkedWorktree` builds a real repo, creates the
   release worktree via `git worktree add -b release-wt/r1` (exactly how
   `RunParallel` bootstraps it, so `.git` is a FILE), commits on a track
   branch, calls the production `ProductionMergeTrack`, and asserts via
   `git merge-base --is-ancestor` that the track commit landed in the release
   worktree HEAD. Before the fix this failed with "merge silently skipped".
2. `TestCmdRun_Parallel` (cmd/sworn) drives the full `cmdRun --parallel`
   entry path — the integration point that wires `MergeTrackFn:
   run.ProductionMergeTrack` — over a git fixture; it now exercises the real
   merge path (previously the guard no-op'd it). Ran `-count=5`, all pass.

## Delivered

- Fail-closed target assertion in `internal/run/parallel.go`
  (`ProductionMergeTrack`): `os.Stat(.git)` accepts file-or-dir; missing
  `.git` returns an error instead of nil
  (evidence: `TestProductionMergeTrack_NonGitTargetErrors`).
- Production auto-merge engages on linked worktrees (evidence:
  `TestProductionMergeTrack_LinkedWorktree`).
- `TestCmdRun_Parallel` fixture upgraded from a bare temp dir (which depended
  on the fail-open guard) to a real git repo with the track branches present
  (evidence: `cmd/sworn/run_test.go`, test passes 5/5 runs).

## Not delivered

- No change to `finishTrack`'s handling of a MergeTrackFn error (it already
  fails the track — `TestDependentTrack_MergeTrackFnErrorFails`); nothing
  further needed there for this fix.
- The stale S05-gate-bypass rationale comment in
  `internal/scheduler/worker.go:495-515` is now true again (the merge really
  happens), so no edit was made; flagged here for the verifier rather than
  widening the diff into a scheduler file owned by in-flight release scope.

## Divergence from plan

- The finding suggested only guard + test; the fix additionally had to update
  `cmd/sworn/run_test.go`'s `TestCmdRun_Parallel` fixture, which had encoded
  the fail-open behaviour (non-git release path expected exit 0). Making the
  fixture a real git repo preserves the fail-closed guard instead of
  weakening it for tests.
- `internal/run/parallel.go` is a planned touchpoint of in-flight release
  2026-07-01-render-drift-reconciliation T5/S06 (oracle migration — different
  functions). Diff kept surgical to the guard and its tests.
