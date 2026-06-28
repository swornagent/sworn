# Design TL;DR — S07-pause-resume-committed

## Approach

Modify `findFirstNonTerminal` to accept a committed-state reader so `sworn run --resume` correctly identifies the first non-terminal slice after a crash/pause. The oracle (`OracleReaderAdapter.ReadSliceStatus`) already reads from git refs (track branch → release-wt → HEAD priority chain) — the fix is to wire it into `findFirstNonTerminal` and add the `--resume` flag.

## Key design choices

### 1. Function signature change: closure over concrete type

Change `findFirstNonTerminal` from `func([]string) string` to accept a reader function:

```go
func findFirstNonTerminal(
    ctx context.Context,
    slices []string,
    reader func(ctx context.Context, sliceID string) (board.SliceState, error),
) (string, error)
```

**Rationale**: A closure parameter keeps the function testable (tests pass a mock) and avoids coupling to a concrete oracle type. The reader returns `board.SliceState` — the `.State` field is what we inspect.

### 2. Terminal state set

Define an `isTerminal` helper:

```go
func isTerminal(state string) bool {
    switch state {
    case "verified", "implemented", "shipped":
        return true
    }
    return false
}
```

States `planned`, `in_progress`, `failed_verification` are non-terminal — `findFirstNonTerminal` returns the first slice in one of these. Matches AC2 exactly.

### 3. Wiring: `OracleReader` on WorkerOptions + `--resume` flag

Add a `SliceStateReader` interface (or reuse `OracleReaderAdapter`'s existing shape) to `WorkerOptions`. When non-nil, `runTrackRouter` constructs the closure from it. When nil (legacy/tests), fall back to returning the first slice (existing behaviour).

On read error: conservatively treat the slice as non-terminal (don't skip potentially unprocessed slices). This covers AC3 implicitly — the oracle's priority chain already falls back to release-wt when the track branch is unreadable.

`--resume` flag is added to `cmd/sworn/run.go`. In `--parallel` mode it ensures the oracle reader is wired; `--resume` without `--parallel` is a usage error (exit 64).

### 4. Caller in parallel.go

`run.RunParallel` already constructs an `OracleReaderAdapter` (line 183 of parallel.go). It just needs to pass it through to `WorkerOptions.OracleReader` so `runTrackRouter` can use it.

## Files intended to touch

| File | Change |
|------|--------|
| `internal/scheduler/worker.go` | Update `findFirstNonTerminal` signature, add `isTerminal`, update call site in `runTrackRouter`, add `SliceStateReader` interface + `OracleReader` field to `WorkerOptions` |
| `internal/scheduler/worker_test.go` | Add `TestFindFirstNonTerminalCommitted` — mock reader returns `planned` while "working-tree" shows `in_progress`; assert committed state wins |
| `cmd/sworn/run.go` | Add `--resume` flag to `cmdRun`'s FlagSet; validate `--resume` requires `--parallel` |
| `internal/run/parallel.go` | Wire `OracleReaderAdapter` into `WorkerOptions.OracleReader` |

## Design-level risks / pins for reviewer

- **git dependency**: The oracle reads via `git show`. AC3 covers the fallback to release-wt when track branch is absent, and the conservative-on-error approach in `findFirstNonTerminal` treats unreadable slices as non-terminal (won't skip past them).
- **No change to the oracle**: The oracle's priority chain (track branch → release-wt → HEAD) already exists and is the correct committed-read path. We're wiring it in, not rewriting it.
- **Backward-compatible**: When `OracleReader` is nil, behaviour is identical to current (return first slice). All existing callers (including tests) continue to work.

## AC traceability

- **AC1** (committed wins over working-tree): `OracleReaderAdapter.ReadSliceStatus` reads from git refs (priority: track branch → release-wt → HEAD). It never consults the working-tree uncommitted copy for the first two priorities.
- **AC2** (skip terminal states): `isTerminal` checks for `verified`, `implemented`, `shipped`; `findFirstNonTerminal` returns first non-terminal.
- **AC3** (fallback to release-wt when track branch unavailable): Already handled by `Oracle.ReadSliceStatus` priority chain — Priority 2 is release-wt. Our conservative-on-error approach in `findFirstNonTerminal` also handles this.
- **AC4** (dirty working-tree test): `TestFindFirstNonTerminalCommitted` mocks a reader returning `planned` while the "working-tree" would show `in_progress`. Asserts reader state wins.