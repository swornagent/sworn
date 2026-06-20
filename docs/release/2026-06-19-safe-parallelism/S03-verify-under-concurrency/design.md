# S03-verify-under-concurrency — Design TL;DR

## Summary

Audit + prove: the verify gate is already goroutine-safe. All four planned
touchpoints (verify, verdict, model/oai, model/client) — plus the indirect
dependency `internal/prompt` — have zero package-level mutable state that
survives init. No fixes needed; this slice is pure concurrency-proof work:
two race-detector-backed tests + invariant comment + reachability artefact.

## Audit results

| File | Goroutine-safe? | Notes |
|---|---|---|
| `internal/verify/verify.go` | ✅ | `systemPrompt`, `knownBoundaryPatterns`, `mockMarkerPatterns` all init-once, read-only. Every function is pure — local vars only, no shared state. `CheckBoundaryMocks` is a pure scan over string inputs. |
| `internal/verdict/verdict.go` | ✅ | Types + constants + value-receiver method on `Result`. No mutable state whatsoever. |
| `internal/model/oai.go` | ✅ | `modelPricing` map init-once, read-only (safe for concurrent reads in Go). `OAI` struct fields read-only during Verify/Chat. `http.DefaultClient` fallback is explicitly goroutine-safe per Go docs. Each caller owns its `*OAI`. |
| `internal/model/client.go` | ✅ | Interface + sentinel error only. No state. |
| `internal/prompt/prompt.go` (indirect dep) | ✅ | All vars set in `init()` from embed.FS; read-only thereafter. Confirmed as part of the dependency chain through `verify.systemPrompt`. |

**Conclusion: zero goroutine-safety issues found. The codebase is goroutine-safe by construction.** This means the "fix" half of the audit-and-fix slice requires zero production code changes — the work is entirely the two concurrent tests, the invariant comment, and the reachability artefact.

## Implementation plan (estimated 1 session)

### Step 1: Add invariant comment to `verify.go`

Per spec AC-6: even when no fix was needed, document the invariant so future
maintainers know the contract.

```go
// Package verify is goroutine-safe by construction — no package-level mutable
// state after init.  systemPrompt, knownBoundaryPatterns, and mockMarkerPatterns
// are read-only after package init.  Every exported function operates on its
// own locals or copies of its arguments; no shared buffers, no unsynchronised
// maps, no sync primitives required.
```

### Step 2: Write `internal/verify/concurrent_test.go`

Two tests, both annotated `// Concurrent test — must be run with -race`:

**`TestConcurrentVerifySameInput`:**
- Construct one `verify.Input` with a deterministic fake verifier
- Launch 4 goroutines via `sync.WaitGroup`, each calling `verify.Run(ctx, in)`
- Collect all results; assert all 4 have the same `Verdict` (PASS)
- Assert no panics in any goroutine
- Primary assertion: `go test -race` detects zero data races

**`TestConcurrentVerifyIndependentInputs`:**
- Two different specs, two different fake verifier replies (one PASS, one FAIL)
- Launch 2 goroutines concurrently, each with its own `Input`
- Assert goroutine 1 gets PASS, goroutine 2 gets FAIL
- Confirms no cross-contamination between concurrent verification runs

### Step 3: Reachability artefact

Run and capture:
```sh
go test -race -count=10 ./internal/verify/...
```

Document the output (all tests pass, race detector reports zero findings) in
proof.md. A single race finding across 10 repetitions is sufficient to FAIL
the slice.

### Step 4: Run acceptance checks from spec

```sh
go test -race ./internal/verify/...    # AC 1
go test -race ./internal/model/...     # AC 2
go test -race ./internal/verdict/...   # AC 3
```

(`internal/verdict` has no test files currently — this command is a no-op pass.)

## Test design notes

### Mock verifier reuse

The existing `fakeVerifier` (value type, value receiver) in `verify_test.go` is
safe for concurrent use: each goroutine's `Input` carries a copy of the interface
value, and `fakeVerifier` has zero pointer fields and zero mutable state. No new
mock infrastructure needed.

### Race detection is the primary assertion

These are concurrency-safety tests, not correctness tests. The `-race` flag is
the real gate. The verdict-equality assertions are secondary — they confirm the
API behaves deterministically, but the race detector is what proves goroutine
safety.

### No HTTP in concurrent tests

Per spec Risk: the fake verifier avoids live HTTP. The real `*OAI` client is
excluded from the race tests by design (it depends on `http.DefaultClient`,
which is itself goroutine-safe, but verifying that requires integration tests
outside this slice's scope).

## Files this slice creates/modifies

| File | Action | Approximate size |
|---|---|---|
| `internal/verify/verify.go` | Touch — add invariant comment (~5 lines) | Trivial |
| `internal/verify/concurrent_test.go` | New — two test functions (~60-80 lines) | Small |
| `docs/release/.../S03-verify-under-concurrency/proof.md` | New — proof bundle | Captured |

**No changes to `internal/verdict/`, `internal/model/oai.go`, or `internal/model/client.go`** — the audit found no issues, so no production code changes are needed there (the spec says "touch if issues found").

## Dependency note

`TestConcurrentVerifySameInput` and `TestConcurrentVerifyIndependentInputs` add
new tests to the verify package. These tests use only `fakeVerifier` (already
defined in `verify_test.go`) and `context.Background()` — no new imports, no new
test infrastructure. They do not depend on S02a or S02b being complete; the
verify package is self-contained and its goroutine safety is independent of the
scheduler that will call it.

## Risks reviewed

- **HTTP client race risk (spec Risk):** Mitigated — `http.Client` is documented
  goroutine-safe. The concurrent tests use fake verifiers, not live HTTP, which
  is the correct boundary for unit-level race detection. The limitation is
  explicitly noted in the spec; document it in proof.md.
- **Race detector runtime overhead (spec Risk):** Acceptable — synthetic specs
  and diffs (one-line each) keep test fixtures under 100 bytes. 4 goroutines ×
  small inputs = sub-second runtime even with `-race`, well under the 10s target.

## Verdict

**PROCEED.** The design is sound: the code is already goroutine-safe, the two
proposed concurrent tests exercise the right paths, the race detector is the
correct gate, and no live API calls are needed. The slice is a one-session
test-and-prove unit with zero production-code risk.