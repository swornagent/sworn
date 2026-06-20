# Journal: S03-verify-under-concurrency

## State transitions

### 2026-06-27 — design_review → in_progress → implemented

**Transition**: `design_review` → `in_progress` (commit 7e0ddec) → `implemented` (this commit).

**Coach approval**: `approved-ack.md` present — PROCEED with implementation. Key directives:
- New `concurrent_test.go` only, using `fakeVerifier` (immutable), not `capturingVerifier`
- Add goroutine-safety comment to `verify.go`
- Reachability artefact: `go test -race -count=10 ./internal/verify/...`
- Do NOT run `go test ./cmd/sworn/...` in the worktree

**Goroutine-safety audit results (all packages confirmed safe by construction):**

1. **internal/verify/verify.go**: `systemPrompt`, `knownBoundaryPatterns`, `mockMarkerPatterns` are package-level vars initialised at program start and read-only thereafter. All `Run()` logic uses only local state. Safe.

2. **internal/verdict/verdict.go**: Pure type definitions (`Result` struct, `Verdict` string type). Value receiver methods only (`ExitCode()`). No package-level mutable state. Safe.

3. **internal/model/client.go**: `Verifier` interface, `Unconfigured` struct with no state. `ErrNotConfigured` is a const error. Safe.

4. **internal/model/oai.go**: `modelPricing` is a read-only map after init (literal assignment, no writes). `OAI.Verify()` and `OAI.Chat()` use request-scoped buffers (`bytes.Buffer`), local variable bindings. `*http.Client` is documented by Go as safe for concurrent use. Safe. `ToolDef.MarshalJSON()` is value receiver. Safe.

**Production code changes**: None beyond the goroutine-safety documentation comment added to `verify.go`'s package doc.

**Concurrent test design decisions:**
- `TestConcurrentVerifySameInput`: 4 goroutines, same `Input` + same `fakeVerifier` → uniform PASS. Uses `sync.WaitGroup` for coordination. Tests that no internal state is corrupted by concurrent `Run()` calls.
- `TestConcurrentVerifyIndependentInputs`: 2 goroutines, different specs/diffs/verifiers → each returns its own expected verdict (PASS for verifier 1, FAIL for verifier 2). Tests no cross-contamination between independent verification sessions.
- Both use `fakeVerifier` (value receiver, immutable) to avoid test-level data races.
- Both run under `go test -race` which is the primary assertion mechanism.

**Reachability artefact**: `go test -race -count=10 ./internal/verify/...` — zero race findings across 10 repetitions (1.129s cumulative).

**Open deferrals**: None.

**Skeptic panel**: Skipped — runtime does not support subagent dispatch (single-threaded API call mode).

## Verifier verdicts received

### 2026-06-21 — verifier verdict: PASS (round 1)

**Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant).
**Verified against**: `ed4919d12cecc3f34c5e16e6b0d14c7cfb3e62e7`

All six gates passed:

- **Gate 1 (User-reachable outcome)**: `verify.Run()` is called from `internal/run/slice.go:183` in `RunSlice()` — the function the concurrent scheduler dispatches. Production code, not a test fixture. (The spec named `internal/scheduler/worker.go` as the entry point; S02a refactored it to `internal/run/slice.go` — the actual integration surface is correct.)
- **Gate 2 (Touchpoints)**: `internal/verify/verify.go` and `internal/verify/concurrent_test.go` changed as planned. `verdict.go`, `oai.go`, `client.go` unchanged — correct, spec says "touch if issues found"; audit found all three goroutine-safe by construction. No unrelated changes.
- **Gate 3 (Tests exercise integration point)**: `TestConcurrentVerifySameInput` and `TestConcurrentVerifyIndependentInputs` both call `verify.Run()` directly. Re-run with `-count=1 -race`: all 25 tests PASS, zero race findings. `go test -race -count=10`: PASS (1.231s, zero races).
- **Gate 4 (Reachability artefact)**: Verifier ran `go test -race -count=10 ./internal/verify/...` independently — PASS, zero races. Artefact names the user gesture (concurrent `verify.Run()` calls from N goroutines) and matches spec outcome (race-free verdicts, no cross-contamination).
- **Gate 5 (No silent deferrals)**: Grep of `concurrent_test.go` and `verify.go` — zero TODO/FIXME/deferred/placeholder/XXX/HACK hits.
- **Gate 6 (Claimed scope)**: All 6 ACs verified against live repo state. Package doc goroutine-safety comment present in `verify.go:7-10`.

**Next**: S03 is the last slice in T1-concurrency-core. Track is complete. Run `/merge-track T1-concurrency-core` in a fresh session.