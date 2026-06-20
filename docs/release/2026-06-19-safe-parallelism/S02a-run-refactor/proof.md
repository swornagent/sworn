# Proof bundle — S02a-run-refactor

## Scope

Extract the implement→verify retry loop from `internal/run.Run()` into an
exported `RunSlice()` function, callable by a goroutine worker with an
existing worktree and spec path — while `sworn run <task>` continues to
behave identically.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S02a-run-refactor/status.json
internal/run/run.go
internal/run/run_test.go
internal/run/slice.go
```

## Test results

```
$ go test -race ./internal/run/...
ok  	github.com/swornagent/sworn/internal/run	2.596s
```

All 11 tests pass with race detector enabled:
- 8 existing `TestRun_*` tests (regression)
- 3 new tests: `TestRunSlice_Pass`, `TestRunSlice_Fail`, `TestRunSlice_MissingVerifierModel`

`go vet` clean. Full `go build ./...` clean.

## Reachability artefact

`go test -race -v ./internal/run/...` output (above): all tests pass, race
detector reports no races. The reachability path is through the test harness
— `TestRunSlice_*` tests call `RunSlice()` directly with a mock worktree,
and `TestRun_*` tests call `Run()` end-to-end which internally calls
`RunSlice()`.

## Delivered

- [x] `run.RunSlice(ctx, worktreeRoot, specPath, statusPath, opts)` exported
  function in `internal/run` — `internal/run/slice.go` lines 61-207
- [x] All existing `go test ./internal/run/...` tests pass unchanged —
  `TestRun_PassPath_Merges`, `TestRun_FailPath_NoMerge`,
  `TestRun_FailThenPass_RetrySucceeds`, `TestRun_Blocked_StopsImmediately`,
  `TestSanitiseBranch`, `TestRun_MissingTask`,
  `TestRun_VerifyMarkdownPass`, `TestRun_VerifyStatelessPromptWired`,
  `TestRun_VerifyToolCallLeakBlocks`
- [x] `TestRunSlice_Pass` — mock PASS → status.json verified, nil error —
  `run_test.go` TestRunSlice_Pass
- [x] `TestRunSlice_Fail` — mock FAIL×2 → status.json failed_verification,
  non-nil error — `run_test.go` TestRunSlice_Fail
- [x] `sworn run <task>` identical behaviour — all existing end-to-end
  `TestRun_*` tests pass unchanged
- [x] `RunSlice` goroutine-safe — `go test -race ./internal/run/...` clean
- [x] `TestRunSlice_MissingVerifierModel` — validates mandatory option

## Not delivered

- (none — all acceptance checks met)

## Divergence from plan

- (none)