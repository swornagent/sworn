# Proof Bundle: `S42-implement-step-timeout`

## Scope

Bound each implement attempt in `sworn run`'s escalation loop with a per-attempt context deadline, preventing a wedged/hung implementer from blocking the run indefinitely. On timeout, the implement step is cancelled and the loop escalates to the next model or fails closed to the human.

## Files changed

```
$ git status --porcelain
 M cmd/sworn/run.go
 M docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json
 M internal/config/config.go
 M internal/run/run.go
 M internal/run/slice.go
 ?? internal/run/slice_test.go
```

## Test results

### Go

```
$ go test -race -count=1 ./internal/run/...
ok      github.com/swornagent/sworn/internal/run      3.500s
```

### Test detail

```
$ go test -race -run 'TestImplementTimeout' ./internal/run/... -v
=== RUN   TestImplementTimeoutEscalates
sworn run: attempt 1/2 — implementing with blocking
sworn run: implement attempt 1 timed out after 500ms — escalating
sworn run: escalating implementer model for retry
sworn run: attempt 2/2 — implementing with working
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutEscalates (0.56s)
=== RUN   TestImplementTimeoutExhaustsToHuman
sworn run: attempt 1/2 — implementing with blocking1
sworn run: implement attempt 1 timed out after 100ms — escalating
sworn run: escalating implementer model for retry
sworn run: attempt 2/2 — implementing with blocking2
sworn run: implement attempt 2 timed out after 100ms — escalating
--- PASS: TestImplementTimeoutExhaustsToHuman (0.23s)
=== RUN   TestImplementTimeoutHappyPath
sworn run: attempt 1/1 — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutHappyPath (0.05s)
=== RUN   TestImplementTimeoutZeroUsesDefault
sworn run: attempt 1/1 — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutZeroUsesDefault (0.06s)
=== RUN   TestImplementTimeoutNegativeNoTimeout
sworn run: attempt 1/1 — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutNegativeNoTimeout (0.06s)
PASS
```

## Reachability artefact

- **Type**: unit test output + stderr demonstration
- **Path**: `internal/run/slice_test.go`
- **Evidence**: `TestImplementTimeoutEscalates` exercises `RunSlice` end-to-end with a blocking fake agent on slot 0, a 500ms timeout, and a working agent on slot 1. The stderr output shows `implement attempt 1 timed out after 500ms — escalating` followed by slot 2 running and verification passing. `TestImplementTimeoutExhaustsToHuman` confirms fail-closed behavior when all models time out.

## Delivered

- Per-attempt context deadline wrapping in `RunSlice` (`internal/run/slice.go`) — evidence: `context.WithTimeout` wrapping `implement.Run` call
- Timeout detection via `errors.Is(err, context.DeadlineExceeded)` — evidence: distinct stderr message `"implement attempt N timed out after <d> — escalating"`
- `--implement-timeout` CLI flag — evidence: `cmd/sworn/run.go`, `flag.Duration("implement-timeout", 0, ...)`
- `SWORN_IMPLEMENT_TIMEOUT` env var — evidence: `config.ResolveImplementTimeout` reads `os.Getenv("SWORN_IMPLEMENT_TIMEOUT")`
- Config file tier (`implementer.timeout`) — evidence: `Config.Implementer.Timeout` field in `internal/config/config.go`, resolved by `config.ResolveImplementTimeout`
- `DefaultImplementTimeout` constant (15m) — evidence: `internal/config/config.go` line `const DefaultImplementTimeout = 15 * time.Minute`
- Precedence: flag > env > config > default — evidence: `config.ResolveImplementTimeout` implements this chain
- Zero means use default, negative means no timeout — evidence: `slice.go` timeout resolution and tests `TestImplementTimeoutZeroUsesDefault`, `TestImplementTimeoutNegativeNoTimeout`
- S44 forward-compatibility: `context.DeadlineExceeded` is a sworn-internal signal, not a `model.Error{Kind}` — S44's Kind-based routing leaves it on the existing escalate path

## Not delivered

- default `http.Client.Timeout` on `internal/model/oai.go` — deferred; per-attempt ctx deadline already bounds the HTTP call, and oai.go is a future S39/T5 touchpoint (Rule 2, Coach ack 2026-06-21). **Acknowledged**: Coach, 2026-06-21.
- killing agent-spawned OS subprocesses — deferred; supervisor stale-PID reaping covers cross-session orphans (Rule 2, Coach ack 2026-06-21). **Acknowledged**: Coach, 2026-06-21.
- per-step timeouts for the verify step — out of scope per spec.

## Divergence from plan

None. All implementation follows the design TL;DR with Coach's 3 approved pins applied inline:
1. Config tier added (`internal/config/config.go`) per Pin 1
2. `design_decisions` populated in `status.json` per Pin 2
3. `context.DeadlineExceeded` documented as sworn-internal signal for S44 per Pin 3

## First-pass script output

```
$ scripts/release-verify.sh S42-implement-step-timeout
FIRST-PASS PASS
22 checks passed, 0 checks failed
```