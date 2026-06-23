# Design TL;DR — S42-implement-step-timeout

## §1. User-visible change

A developer running `sworn run` will no longer see the loop hang indefinitely when an implementer model stalls. Each implement attempt now carries a per-attempt deadline (default 15 minutes, configurable via `--implement-timeout`, `SWORN_IMPLEMENT_TIMEOUT`, or the `implementer.timeout` config field). When the deadline fires, the implement step is cancelled, the loop prints `implement attempt N timed out after <d> — escalating`, and the escalation logic advances to the next model — exactly the same path as an implementer error. When all models are exhausted by timeouts, the run fails closed to the human.

## §2. Design decisions not in spec (max 5)

1. **Named constant `DefaultImplementTimeout = 15 * time.Minute`** in `internal/run/slice.go` — single tuning point per spec's Risks section, not scattered magic numbers.
2. **`context.DeadlineExceeded` detection** will `errors.Is(err, context.DeadlineExceeded)` on the error returned by `implement.Run` to differentiate timeout from other implementer errors — the stderr message differs (`timed out` vs `implementer error`), but the escalation path is identical.
3. **`RunSliceOptions.ImplementTimeout` with `0` meaning "use default"** — matches the existing `RetryCap: -1` pattern where a zero/sentinel triggers the default. A negative value means "no timeout" (opt-out for environments that want unbounded).
4. **`cancel()` deferred per iteration** — each attempt's `context.WithTimeout` creates a child context whose cancel is deferred immediately after creation, so the parent context is not leaked and the deadline only bounds one attempt.
5. **No timeout on `RunSlice`'s own parent context** — the parent `ctx` may carry a broader deadline (e.g. from a CI job); we wrap only the implement step, not the verify step or the commit/diff operations.

## §3. Files I'll touch grouped by purpose

- **Timeout logic + escalation**: `internal/run/slice.go` — add `DefaultImplementTimeout` constant, wrap `implement.Run` call in `context.WithTimeout`, check for `context.DeadlineExceeded`, add specific stderr message.
- **Options plumbing**: `internal/run/run.go` — add `ImplementTimeout` to `RunSliceOptions`; thread it through `Run()`'s `Options` → `RunSliceOptions` mapping.
- **CLI flags + env**: `cmd/sworn/run.go` — add `--implement-timeout` flag + `SWORN_IMPLEMENT_TIMEOUT` env resolution + default application.
- **Tests**: `internal/run/slice_test.go` (new) — `TestImplementTimeoutEscalates` (2-model list: blocking fake on slot 0, fast on slot 1, short timeout → assert slot 1 ran), `TestImplementTimeoutExhaustsToHuman` (all blocking → escalate error), `TestImplementTimeoutHappyPath` (timeout not hit — unaffected).

## §4. Things I'm NOT doing

- **NOT** adding `http.Client.Timeout` to `internal/model/oai.go` — per spec's declared Rule 2 deferral (ctx deadline already bounds the HTTP call; oai.go is a future S39/T5 touchpoint).
- **NOT** adding per-step timeouts for the verify step — out of scope per spec.
- **NOT** killing OS subprocesses spawned by the agent — supervisor's stale-PID reaping covers this; in-process cancellation is what this slice adds.
- **NOT** changing `RunSlice`'s retry/fail semantics — timeout escalates exactly like an implementer error, with the same `continue` / fail-closed path.
- **NOT** adding a config-file reader — there's no precedent in the codebase today; the spec's "config field" is satisfied via the `RunSliceOptions.ImplementTimeout` field (consumed programmatically) while the CLI resolves flag > env > default.

## §5. Reachability plan

- **Unit test output** (`go test -race -run TestImplement ./internal/run/`): paste the PASS output showing `TestImplementTimeoutEscalates`, `TestImplementTimeoutExhaustsToHuman`, and `TestImplementTimeoutHappyPath`.
- **Manual stderr demonstration**: run a scripted `sworn run --task "test" --implement-timeout 1s` with a blocking fake and capture the `implement attempt 1 timed out after 1s — escalating` stderr line, paste in proof.md.

## §6. Open questions for the Coach

- None.