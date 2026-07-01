# Proof — S07-pause-resume-committed

## Scope
Resume seeds the frontier from committed git-visible state using the same terminal-set as the router (verified/shipped/deferred), so implemented-but-unverified slices are re-verified rather than abandoned.

## Files changed
- `cmd/sworn/run.go` — added `--resume` flag + usage gate
- `internal/router/router.go` — exported `IsTerminal`; replaced 2 inline terminal-set switch blocks
- `internal/router/router_test.go` — `TestIsTerminal` (9 cases)
- `internal/run/parallel.go` — hoisted `ora` to function scope; wired `Oracle` on `WorkerOptions` at both construction sites
- `internal/scheduler/worker.go` — added `Oracle` field to `WorkerOptions`; rewrote `findFirstNonTerminal` to read committed state; fixed fused-line bug at line 232
- `internal/scheduler/worker_test.go` — 6 new tests (AC1-AC5)

## Test results
```
=== RUN   TestFindFirstNonTerminalCommitted
--- PASS: TestFindFirstNonTerminalCommitted (0.00s)
=== RUN   TestFindFirstNonTerminalAllTerminalMergesTrack
--- PASS: TestFindFirstNonTerminalAllTerminalMergesTrack (0.00s)
=== RUN   TestFindFirstNonTerminalNilOracle
--- PASS: TestFindFirstNonTerminalNilOracle (0.00s)
=== RUN   TestFindFirstNonTerminalEmptySlices
--- PASS: TestFindFirstNonTerminalEmptySlices (0.00s)
=== RUN   TestFindFirstNonTerminalOracleErrorSeedsAtUnreadable
--- PASS: TestFindFirstNonTerminalOracleErrorSeedsAtUnreadable (0.00s)
=== RUN   TestFindFirstNonTerminalIsTerminalImport
--- PASS: TestFindFirstNonTerminalIsTerminalImport (0.00s)
=== RUN   TestIsTerminal
--- PASS: TestIsTerminal (0.00s)
PASS
```

## Reachability artefact
`go test ./internal/scheduler/... ./internal/router/... -run 'TestFindFirstNonTerminal|TestIsTerminal' -v` exits 0 (7 tests, all PASS). Integration-point reachability: `TestFindFirstNonTerminalCommitted` verifies committed-state oracle read via `findFirstNonTerminal`; the production path wires `Oracle` via `RunParallel` (`internal/run/parallel.go`) hoisting `ora` to function scope and setting `WorkerOptions.Oracle` at both construction sites.

## Delivered
- AC1 — committed seed: `findFirstNonTerminal` reads committed state via `oracle.ReadSliceStatus`
- AC2 — implemented is non-terminal: `router.IsTerminal("implemented")` → false; seed at implemented slice
- AC3 — track-ref-unreadable fallback: on oracle error, seed AT the unreadable slice (seed-don't-skip)
- AC4 — all-terminal track merges: fused-line fix; `findFirstNonTerminal` returns "" when all terminal
- AC5 — single terminal-set: `router.IsTerminal` consumed by both router and scheduler
- AC6 — `--resume` observable contract: flag + usage gate in `cmd/sworn/run.go`

## Not delivered
None.

## Divergence
Pin 5 (Captain review): design.md originally planned to skip on oracle read error; changed to seed AT the unreadable slice per the seed-don't-skip thesis (consistent with DD-1). All 5 Captain pins addressed inline.

## First-pass verification gate
`sworn verify` requires `SWORN_ANTHROPIC_API_KEY` which is not configured in this environment. Deterministic gates (`go build`, `go vet`, `go test`, `gofmt`) all pass.
