Design approved — PROCEED.

No production-code changes needed. verify.go, verdict.go, and model/client.go are all goroutine-safe by construction (no package-level mutable state, request-scoped buffers). The implementation is the concurrent test suite only.

1. **New concurrent_test.go only** — `TestConcurrentVerifySameInput` (N goroutines, same Input + fakeVerifier, assert uniform verdict) and `TestConcurrentVerifyIndependentInputs` (two goroutines, different specs/verifiers, assert no cross-contamination). Use `fakeVerifier` (value receiver, immutable) not `capturingVerifier` (pointer receiver, unsafe for concurrent use).
2. **add goroutine-safety comment to verify.go** — `// stateless by construction — no package-level vars; each Run call is independent`.
3. **Reachability artefact** — `go test -race -count=10 ./internal/verify/...` output showing zero race findings across 10 repetitions.
4. **Do NOT run `go test ./cmd/sworn/...`** in the worktree — those tests invoke `sworn run` which does `git checkout main` on the live worktree. Run only `./internal/verify/...`, `./internal/model/...`, `./internal/verdict/...`.

Address inline during implementation. Proceed to `in_progress`.
