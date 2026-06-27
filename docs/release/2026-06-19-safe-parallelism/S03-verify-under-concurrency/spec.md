---
title: 'S03-verify-under-concurrency — verify gate goroutine-safe at N>1'
description: 'Goroutine-safety audit of the verify path; N-parallel verify calls produce independent, correct verdicts with no data races.'
---

# Slice: `S03-verify-under-concurrency`

## User outcome

`sworn run --parallel` running N≥2 concurrent verification sessions produces verdicts
identical to serial runs — no false PASSes, no false FAILs, no panics, no data races —
proving the fail-closed gate is safe under concurrency.

## Entry point

Called internally by `internal/scheduler/worker.go` (S02): each track worker calls
`verify.Run()` concurrently from its own goroutine.

## In scope

- Goroutine-safety audit of `internal/verify/verify.go`: identify and eliminate any
  package-level mutable variables, shared buffers, or unsynchronised state
- Goroutine-safety audit of `internal/verdict/verdict.go`: same
- Goroutine-safety audit of `internal/model/oai.go` and `internal/model/client.go`:
  confirm the model client is goroutine-safe or that each caller gets its own instance
- Fix any issues found — this is an audit-and-fix slice, not just a test slice
- `internal/verify/concurrent_test.go`: new test file proving safety under concurrency

## Out of scope

- The scheduler that invokes verify (S02)
- Model API rate limiting or quota management under concurrency
- The overclaim benchmark (S05) — that measures correctness, not goroutine safety
- Any changes to the verify prompt or verdict parsing logic

## Planned touchpoints

- `internal/verify/verify.go` (touch — fix goroutine-safety issues if found)
- `internal/verify/concurrent_test.go` (new)
- `internal/verdict/verdict.go` (touch if issues found)
- `internal/model/oai.go` (touch if issues found)
- `internal/model/client.go` (touch if issues found)

## Acceptance checks

- [ ] `go test -race ./internal/verify/...` passes with zero data race detector findings
  (both existing tests and the new concurrent tests)
- [ ] `go test -race ./internal/model/...` passes with zero data race findings
- [ ] `go test -race ./internal/verdict/...` passes with zero data race findings
- [ ] `TestConcurrentVerifySameInput`: 4 goroutines each call `verify.Run()` with
  identical `Input{SpecPath, DiffPath, ProofPath, Model, Verifier}` concurrently;
  all return the same `verdict.Result.Verdict`; no panics; race detector clean
- [ ] `TestConcurrentVerifyIndependentInputs`: 2 goroutines call `verify.Run()` with
  different specs and different mock verifiers concurrently; each result matches its
  own expected verdict (no cross-contamination between the two verification runs)
- [ ] If any package-level mutable state was found and fixed, a comment in the relevant
  file names the invariant being protected (e.g. `// stateless by construction — no
  package-level vars; each Run call is independent`)

## Required tests

- **Unit / concurrency**: `internal/verify/concurrent_test.go`
  — `TestConcurrentVerifySameInput` (see AC above)
  — `TestConcurrentVerifyIndependentInputs` (see AC above)
  Both tests must be run with `go test -race` to be meaningful; the race detector
  is the primary assertion mechanism here.
- **Reachability artefact**: `go test -race -count=10 ./internal/verify/...` output
  showing zero race conditions across 10 repetitions (captured in proof.md).
  A single non-deterministic race is sufficient to FAIL this slice.

## Risks

- The model client (`oai.go`) makes real HTTP calls. Tests must use the existing mock
  verifier injection (`verify.Input.Verifier`) to avoid live API calls in the race test.
  If the real HTTP client has goroutine-safety issues, they will only be caught in
  integration tests, not the unit race tests. Document this limitation in proof.md.
- `go test -race` adds ~2-5× runtime overhead. Keep test fixtures small (synthetic
  spec+diff, not large production data) so the race tests run in <10s.
## Deferrals allowed?

No. A data race in the verify path under concurrency means S02's parallel scheduler
produces undefined behaviour. This slice is a correctness gate for the whole release.
