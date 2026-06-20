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