# Design TL;DR — S07-pause-resume-committed

## Approach

Three surgical changes in two packages, plus a flag documentation in `cmd/sworn/run.go`.
No new files; scope is tightly bounded to the three planned touchpoints.

## Key design choices + rationale

### 1. Export `IsTerminal` in `internal/router/router.go`

Add a two-line exported function:

```go
func IsTerminal(state string) bool {
    switch state {
    case "verified", "shipped", "deferred":
        return true
    }
    return false
}
```

Replace the two inline switch statements at router.go:307 and router.go:393 with
calls to `IsTerminal`. The function is pure, takes `string` not `state.State`
(to avoid a dependency cycle — the scheduler can call it without importing
`internal/state`), and the terminal-set is literally the existing definition
promoted to an exported helper. No semantic change to the router.

### 2. Rewrite `findFirstNonTerminal` in `internal/scheduler/worker.go`

New signature:

```go
func findFirstNonTerminal(ctx context.Context, oracle OracleReader, release, trackID string, slices []string) string
```

- `oracle` is the `router.OracleReader` — already available from `WorkerOptions.Router`
  (the `SliceRouter` interface does not embed `OracleReader`; we will thread the
  oracle separately via a new `OracleReader` field on `WorkerOptions`).
- Iterates `slices` in order and calls `oracle.ReadSliceStatus(ctx, release, sid)`.
- If the call succeeds and `IsTerminal(string(ss.State))` is true → skip to next.
- If the call succeeds and state is non-terminal → return that slice ID.
- If the call errors (e.g. track ref doesn't exist yet): fall back per AC3 —
  skip the slice (treat as readable-when-track-exists, not terminal, not error).
  Specifically: a `ReadSliceStatus` error on a track that hasn't been pushed yet
  is not a hard error; we log it and continue to the next slice. The release-wt
  fallback is already inside the oracle (track → release-wt → working-tree chain),
  so the implementer's role is to confirm this path returns empty/nil rather than
  propagating an error.

Returns `""` only when every slice is terminal.

**Threading the oracle to `runTrackRouter`:** `WorkerOptions` already has `Router
SliceRouter`. The `SliceRouter` interface (`internal/scheduler/model.go`) does not
embed `OracleReader`. Rather than widening `SliceRouter`, add an explicit
`Oracle RouterOracle` field (where `RouterOracle` is `router.OracleReader`, type-aliased
to avoid a circular import — or just use the interface directly since scheduler
already imports router types). The `runTrackRouter` call to `findFirstNonTerminal`
passes `opts.Oracle`. This is a Type-2 change (DD-3) — reversible, local.

### 3. Fix the worker.go:232 fused-line bug

The current line 232 reads:

```go
// All slices already in a terminal state.		return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)
```

where the `return` is part of the comment text. Replace with:

```go
// All slices already in a terminal state.
return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)
```

This is dead code today (`findFirstNonTerminal` never returns `""`); change 2 makes
`""` reachable, so this fix becomes load-bearing.

### 4. `--resume` flag in `cmd/sworn/run.go` (AC6)

Add `resume := fs.Bool("resume", false, "resume an in-flight parallel release (re-seed each track from committed state)")` to the flag set. Add a usage gate:

```go
if *resume && !*parallel {
    fmt.Fprintln(os.Stderr, "sworn run: --resume requires --parallel")
    return 64
}
```

The flag is **observational** — it doesn't change the code path because with this
slice's changes, `findFirstNonTerminal` always reads committed state on every
`--parallel` run (resume or fresh). The flag exists to make the contract explicit
(AC6: "SHALL have a stated observable effect") and to provide a user-facing entry
point. The help text will state: "On every --parallel run, each track seeds from
committed state; --resume is an explicit alias that makes this contract visible."

### Files touched

| File | Change |
|------|--------|
| `internal/router/router.go` | Add `IsTerminal`; replace 2 inline switch blocks |
| `internal/scheduler/worker.go` | Rewrite `findFirstNonTerminal`; fix line 232; add `Oracle` to `WorkerOptions`; thread to `runTrackRouter` |
| `cmd/sworn/run.go` | Add `--resume` flag + usage gate |
| `internal/scheduler/worker_test.go` | `TestFindFirstNonTerminalCommitted`, `TestFindFirstNonTerminalAllTerminalMergesTrack` |
| `internal/router/router_test.go` | `TestIsTerminal` table |

## Design-level risks / pins

- **Oracle threading to `WorkerOptions`:** `WorkerOptions` is constructed in
  `internal/scheduler/run_parallel.go`. Must ensure the oracle is wired at
  construction time. The oracle is built via `board.NewReleaseOracle` (or
  similar) — we need to confirm the constructor path and ensure it's not nil
  in test paths (tests use `fakeRouter` which may not implement `OracleReader`;
  the `Oracle` field defaults to nil and `findFirstNonTerminal` is only called
  when the field is non-nil — legacy fallback preserves old behaviour).

- **Import cycle risk:** `internal/scheduler` already imports `internal/router`
  (`SliceRouter` is `router.Router`). Adding `router.IsTerminal` and
  `router.OracleReader` to the scheduler does not create a new cycle.

- **Non-existent track ref (AC3):** The oracle's `ReadSliceStatus` has a
  track → release-wt → working-tree fallback chain. For a freshly-created track
  (first run, no track branch yet), the track ref read may error. The
  implementation must handle this gracefully — NOT error out, NOT read the
  working-tree. The oracle already has this fallback; we need to confirm the
  error path returns empty/falls through rather than propagating. Testing this
  with a mock oracle that returns an error on a nonexistent ref confirms AC3.

## AC traceability

| AC | Implemented by |
|----|---------------|
| AC1 — committed seed | `findFirstNonTerminal` calls `oracle.ReadSliceStatus` |
| AC2 — implemented is non-terminal | `IsTerminal("implemented")` → false → seeded |
| AC3 — track-ref-unreadable fallback | Error handler in `findFirstNonTerminal` skips (not errors) |
| AC4 — all-terminal track merges | Fixed line 232 `return finishTrack(...)` |
| AC5 — single terminal-set | `IsTerminal` exported from router, consumed by scheduler |
| AC6 — `--resume` observable contract | Flag + usage gate in `cmd/sworn/run.go` |