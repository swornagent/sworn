# Proof Bundle: `S44-feedback-driven-retry`

## Scope

When `sworn run` retries a slice after a verifier FAIL, the next implement attempt is told exactly why the previous attempt failed (the verifier's rationale) and is instructed to address it. A resolvable failure gets resolved on the next pass, rather than the loop discarding the diagnosis and re-implementing from scratch. Provider-error retry policy (ACs 5-6) is deferred on S10-provider-foundation.

## Files changed

```sh
$ git diff --name-only 359fe6b..HEAD
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/proof.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/status.json
internal/implement/implement.go
internal/implement/implement_test.go
internal/run/slice.go
internal/run/slice_test.go
```

```sh
$ git diff --stat 359fe6b..HEAD
.../S44-feedback-driven-retry/proof.md   | 119 +++++++++++
.../S44-feedback-driven-retry/status.json |  19 +-
internal/implement/implement.go          |  34 ++-
internal/implement/implement_test.go     | 103 ++++++--
internal/run/slice.go                    |  11 +-
internal/run/slice_test.go               | 233 ++++++++++++++++++-
6 files changed, 470 insertions(+), 36 deletions(-)
```

## Test results

### Go

```sh
$ go test -race -count=1 ./internal/run/... ./internal/implement/...
ok      github.com/swornagent/sworn/internal/run        3.723s
ok      github.com/swornagent/sworn/internal/implement  1.296s
```

### Test detail

```sh
$ go test -race -run 'TestRunInjectsPriorFeedback|TestRetryPassesVerifierRationale|TestAttempt0EmptyFeedback|TestRetryFeedbackResolvesToPass' ./internal/run/... ./internal/implement/... -v
=== RUN   TestRunInjectsPriorFeedback
--- PASS: TestRunInjectsPriorFeedback (0.07s)
=== RUN   TestRetryPassesVerifierRationale
sworn run: attempt 1/2 — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: gate 1 — no feedback block in implementer prompt
sworn run: verification failed — retrying with escalated implementer model
sworn run: attempt 2/2 — implementing with model-b
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestRetryPassesVerifierRationale (0.07s)
=== RUN   TestAttempt0EmptyFeedback
sworn run: attempt 1/1 — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestAttempt0EmptyFeedback (0.05s)
=== RUN   TestRetryFeedbackResolvesToPass
sworn run: attempt 1/2 — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: verification failed — retrying with escalated implementer model
sworn run: attempt 2/2 — implementing with model-b
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestRetryFeedbackResolvesToPass (0.06s)
PASS
```

```sh
$ go vet ./...
(no output)

$ go build ./...
(no output)
```

## Reachability artefact

- **Type**: unit test output + stderr demonstration
- **Path**: `internal/run/slice_test.go`
- **Evidence**: `TestRetryPassesVerifierRationale` exercises the full retry path: implementer[0] is verified and gets a FAIL with a specific rationale → that rationale is captured before the verification reset → `implement.Run` on attempt 2 receives it as `priorFeedback` → the recording agent on slot 2 confirms the feedback block is present in its prompt. `TestRetryFeedbackResolvesToPass` confirms the end-to-end FAIL→PASS scenario: attempt 0 FAILs, attempt 1 (with feedback) PASSes, and the slice transitions to `verified`.
- **Stderr demonstration**: `sworn run: attempt 1/2 — implementing with model-a` → `sworn run: verdict FAIL` → `sworn run: verification failed — retrying with escalated implementer model` → `sworn run: attempt 2/2 — implementing with model-b` → feedback block present → `sworn run: verdict PASS`

## Delivered

- `implement.Run` accepts `priorFeedback string` parameter — evidence: `internal/implement/implement.go:37`.
- When `priorFeedback` is non-empty, a delimited feedback block is injected ahead of the spec — evidence: `internal/implement/implement.go:94-105` (`"Previous attempt failed verification — address these specifically:\n\n%s\n\n---\n\nImplement the following spec..."`).
- `RunSlice` captures `lastVerdict.Rationale` before the verification reset — evidence: `internal/run/slice.go:155` (`priorFeedback = lastVerdict.Rationale`).
- Attempt 0 passes empty feedback — evidence: `internal/run/slice.go:169` (`priorFeedback = ""`).
- Retry path passes feedback to `implement.Run` — evidence: `internal/run/slice.go:186,188` (`implement.Run(implCtx, worktreeRoot, specPath, priorFeedback, implAgent)`).
- `TestRunInjectsPriorFeedback` — evidence: `internal/implement/implement_test.go:690`.
- `TestRetryPassesVerifierRationale` — evidence: `internal/run/slice_test.go:403`.
- `TestAttempt0EmptyFeedback` — evidence: `internal/run/slice_test.go:452`.
- `TestRetryFeedbackResolvesToPass` — evidence: `internal/run/slice_test.go:485`.

## Not delivered

- **AC5: terminal dispatch errors (`model.Error` Kind=Credits/Auth)** — deferred (Rule 2). Why: depends on S10-provider-foundation's `model.Error{Kind}`, `IsTerminal`, `IsTransient` helpers, which are not yet merged into `release-wt/2026-06-19-safe-parallelism`. Tracking: S10 in T5-providers (state `implemented`, awaiting verification/merge); re-enter S44 once S10 lands. Ack: Coach approved-ack.md 2026-06-23.
- **AC6: transient dispatch errors (Kind=RateLimit)** — deferred (Rule 2). Why: same as AC5; the error taxonomy does not exist on this track branch. Tracking: same as AC5. Ack: same as AC5.

## Divergence from plan

None from the approved design TL;DR. ACs 5-6 are deferred as planned. All four in-scope acceptance checks (AC1-4) are delivered. The implementation follows the design exactly: `implement.Run` adds a `priorFeedback string` parameter, `RunSlice` captures rationale before reset, and a delimited feedback block is injected when non-empty.
