# Design TL;DR — S03-verify-under-concurrency

## §1. User-visible change

No user-visible change. Under `sworn run --parallel` with N≥2 concurrent verify workers, each independent verification call produces a correct, race-free verdict identical to a serial run. The fail-closed gate (non-PASS blocks merge) remains safe even when N concurrent verify calls execute simultaneously on different track slices from the scheduler's worker pool.

## §2. Design decisions not in spec (max 5)

1. **verify.go is already goroutine-safe by construction.** A thorough audit reveals zero package-level mutable variables that are written to during `Run()`: `systemPrompt` is initialised once at program start (via `prompt.VerifyStateless()`), `knownBoundaryPatterns` / `mockMarkerPatterns` are read-only slices, and all non-test code uses only local state. No code changes to production verify.go are needed — the slice's value is the concurrent test proof, not a fix. **Rationale:** the audit must confirm this; the test suite proves it.

2. **verdict.go is goroutine-safe by construction.** `Result` is a plain struct. `ExitCode()` is a value receiver. No package-level state. **Rationale:** same as above — confirmed by audit, proven by tests.

3. **OAI model client is goroutine-safe by construction.** Each `Verify()` call builds its own `bytes.Buffer`, `http.Request`, and locally-scoped variables. `http.DefaultClient` (used when `c.Client` is nil) is documented by Go as safe for concurrent use. The `modelPricing` map is read-only after initialisation — concurrent map reads are safe in Go. **Rationale:** the model client's design (request-scoped state, immutable configuration) naturally avoids races.

4. **The `capturingVerifier` test helper is NOT safe for concurrent use.** It modifies `capturedPrompt` via pointer receiver. The concurrent tests must use `fakeVerifier` (value receiver, immutable) to avoid test-level data races. **Rationale:** obvious but worth documenting so no one introduces a shared `capturingVerifier` into a concurrent test and gets a race detector failure that looks like a production bug.

5. **Concurrent test design: same-input and independent-inputs, both with `-race`.** `TestConcurrentVerifySameInput` uses N goroutines with identical `Input` + same `fakeVerifier`, asserting all return the same verdict. `TestConcurrentVerifyIndependentInputs` uses two goroutines with different specs/verifiers, asserting each result matches its own expected verdict (no cross-contamination). Both run under `go test -race` which is the primary assertion mechanism — if the code had a race, the detector catches it.

## §3. Files I'll touch grouped by purpose

- **New concurrent test** — `internal/verify/concurrent_test.go` (new): `TestConcurrentVerifySameInput`, `TestConcurrentVerifyIndependentInputs`
- **Documentation comments** — `internal/verify/verify.go`: add `// stateless by construction — no package-level vars; each Run call is independent` comment per AC6
- **No changes** — `internal/verdict/verdict.go`, `internal/model/oai.go`, `internal/model/client.go`: minor audit confirms goroutine-safety, add comment confirming invariant if needed

## §4. Things I'm NOT doing

- Not modifying the production verify logic — the audit confirms it's already safe
- Not touching the scheduler (S02) or worker pool
- Not adding rate limiting or quota management — explicitly out of scope
- Not changing the verdict parser or verify prompt
- Not adding integration tests with real HTTP — risk is documented, unit-level `fakeVerifier` isolation is sufficient
- Not running the benchmark (S05) — that's a separate slice

## §5. Reachability plan

1. **Reachability artefact:** output of `go test -race -count=10 ./internal/verify/...` showing zero race conditions across 10 repetitions (captured in proof.md). This proves the verify path is deterministic and race-free under sustained concurrent pressure.
2. **Test commands:** `go test -race ./internal/verify/...`, `go test -race ./internal/model/...`, `go test -race ./internal/verdict/...` — all must pass with zero race findings.
3. **Verification entry point:** `verify.Run()` is the integration surface — called by the scheduler worker (S02). The concurrent tests exercise `Run()` directly, which is how the scheduler will call it.

## §6. Open questions for the Coach

None. The audit is unambiguous, the test design is straightforward, and the spec acceptance checks are complete. The only decision (which concurrency patterns to use — `sync.WaitGroup` + `sync.Mutex` for result collection) is a mechanical one well within implementer discretion.

---

## Design reviewer notes (from design gate)

**Audit results:**

| File | Goroutine-safe? | Notes |
|---|---|---|
| `internal/verify/verify.go` | ✅ | `systemPrompt`, `knownBoundaryPatterns`, `mockMarkerPatterns` all init-once, read-only. |
| `internal/verdict/verdict.go` | ✅ | Types + constants + value-receiver method on `Result`. No mutable state. |
| `internal/model/oai.go` | ✅ | `modelPricing` map init-once, read-only. `http.DefaultClient` goroutine-safe per Go docs. |
| `internal/model/client.go` | ✅ | Interface + sentinel error only. No state. |
| `internal/prompt/prompt.go` | ✅ | All vars set in `init()` from embed.FS; read-only thereafter. |

**Design review verdict: PROCEED.**
