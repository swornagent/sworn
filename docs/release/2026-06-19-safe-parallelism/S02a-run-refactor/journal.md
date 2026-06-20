# Journal — S02a-run-refactor

## 2026-06-20 — implementation

- **Actor**: implementer (Claude)
- **Note**: Extracted RunSlice() from Run()'s inner loop. Created
  `internal/run/slice.go` with `RunSliceOptions` struct and `RunSlice()`
  function. Refactored `Run()` to call `RunSlice()` after setup, keeping
  merge-on-PASS in `Run()`. The failure-exhausted path commits the
  `failed_verification` state transition to keep the working tree clean
  (enables callers to `git checkout` afterward).

- **Key decisions**:
  1. `RunSlice` takes `worktreeRoot` separately from `specPath`/`statusPath`
     so paths can be relative or absolute — `implement.Run` and
     `verify.Run` already handle both.
  2. `RunSlice` reads `startCommit` from `status.json` (not from a
     parameter) — matches the design doc decision.
  3. `RunSlice` commits its own `failed_verification` state transition
     (with `repo.Stage` + `repo.Commit`) to avoid leaving a dirty working
     tree. On PASS, the state transition is committed by `Run()` as part
     of the merge preparation.
  4. Added `IsBlocked()` / `IsFailed()` sentinel helpers so callers can
     distinguish BLOCKED (no-retry) from FAIL-exhausted (escalation needed).

- **Test results**: 11/11 pass; `go vet` clean; `go build ./...` clean; race
  detector clean. No regressions in existing `TestRun_*` tests.

- **Touchpoints clean**: Only `internal/run/run.go` (refactor),
  `internal/run/slice.go` (new), `internal/run/run_test.go` (add tests) —
  no cross-track touchpoint collisions.