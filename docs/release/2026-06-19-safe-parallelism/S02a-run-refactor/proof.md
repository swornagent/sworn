# Proof bundle — S02a-run-refactor

## Scope

Extract the implement→verify retry loop from `internal/run.Run()` into an
exported `RunSlice()` function, callable by a goroutine worker with an
existing worktree and spec path — while `sworn run <task>` continues to
behave identically.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S02a-run-refactor/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/run/run.go
internal/run/run_test.go
internal/run/slice.go
docs/release/2026-06-19-safe-parallelism/S02a-run-refactor/proof.md
docs/release/2026-06-19-safe-parallelism/S02a-run-refactor/journal.md
```

## Test results

```
$ go test -race -v ./internal/run/...
=== RUN   TestRun_PassPath_Merges
--- PASS: TestRun_PassPath_Merges (0.14s)
=== RUN   TestRun_FailPath_NoMerge
--- PASS: TestRun_FailPath_NoMerge (0.21s)
=== RUN   TestRun_FailThenPass_RetrySucceeds
--- PASS: TestRun_FailThenPass_RetrySucceeds (0.23s)
=== RUN   TestRun_Blocked_StopsImmediately
--- PASS: TestRun_Blocked_StopsImmediately (0.16s)
=== RUN   TestSanitiseBranch
--- PASS: TestSanitiseBranch (0.00s)
=== RUN   TestRun_MissingTask
--- PASS: TestRun_MissingTask (0.00s)
=== RUN   TestRun_VerifyMarkdownPass
--- PASS: TestRun_VerifyMarkdownPass (0.18s)
=== RUN   TestRun_VerifyStatelessPromptWired
--- PASS: TestRun_VerifyStatelessPromptWired (0.16s)
=== RUN   TestRun_VerifyToolCallLeakBlocks
--- PASS: TestRun_VerifyToolCallLeakBlocks (0.13s)
=== RUN   TestRunSlice
--- PASS: TestRunSlice (0.05s)
=== RUN   TestRunSliceFail
--- PASS: TestRunSliceFail (0.07s)
=== RUN   TestRunSlice_MissingVerifierModel
--- PASS: TestRunSlice_MissingVerifierModel (0.04s)
PASS
ok  	github.com/swornagent/sworn/internal/run	2.399s
```

All 11 tests pass with race detector enabled:
- 8 existing `TestRun_*` tests pass unchanged (regression — `Run()` still works end-to-end)
- 2 new tests matching spec AC names: `TestRunSlice` (mock→PASS path) and `TestRunSliceFail` (mock→FAIL path)
- 1 additional test: `TestRunSlice_MissingVerifierModel` (validates mandatory option — not in spec ACs but provides defensive coverage)

`go vet ./internal/run/...` clean.

## Reachability artefact

`go test -race -v ./internal/run/...` output (above): all tests pass, race
detector reports no races. The reachability path is through the test harness
— `TestRunSlice` and `TestRunSliceFail` call `RunSlice()` directly with a mock
worktree, and `TestRun_*` tests call `Run()` end-to-end which internally calls
`RunSlice()`.

## Delivered

- [x] `run.RunSlice(ctx, worktreeRoot, specPath, statusPath, opts)` exported
  function in `internal/run` — `internal/run/slice.go` lines 61-207
- [x] All existing `go test ./internal/run/...` tests pass unchanged —
  8 pre-existing `TestRun_*` tests
- [x] `TestRunSlice` — mock PASS → status.json verified, nil error
- [x] `TestRunSliceFail` — mock FAIL×2 → status.json failed_verification, non-nil error
- [x] `sworn run <task>` identical behaviour — all existing end-to-end
  `TestRun_*` tests pass unchanged
- [x] `RunSlice` goroutine-safe — `go test -race ./internal/run/...` clean
- [x] `TestRunSlice_MissingVerifierModel` — validates mandatory option

## Not delivered

- (none — all acceptance checks met)

## Divergence from plan

- **Test names renamed to match spec AC names.** The initial implementation
  named the PASS-path test `TestRunSlice_Pass` and the FAIL-path test
  `TestRunSlice_Fail`. The spec's acceptance checks require `TestRunSlice` and
  `TestRunSliceFail` respectively (see AC-3 and AC-4). Renamed during the fix
  pass (after verifier FAIL verdict) to match the spec exactly. `TestRunSlice_MissingVerifierModel`
  was kept as-is (it is an additional validation not in spec ACs but provides
  defensive coverage — retained as a bonus).
- **start_commit field** was omitted entirely in the initial implementation.
  Set to `0aaa4b1` (the design TL;DR commit, immediately before the feat
  commit `3cefd09`) during the fix pass, giving the verifier a valid diff base.