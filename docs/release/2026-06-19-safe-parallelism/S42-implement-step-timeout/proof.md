# Proof Bundle: `S42-implement-step-timeout`

## Scope

Bound each implement attempt in `sworn run`'s escalation loop with a per-attempt context deadline, preventing a wedged/hung implementer from blocking the run indefinitely. On timeout, the implement step is cancelled and the loop escalates to the next model or fails closed to the human. This re-implementation removes the prior `internal/config/config.go` touchpoint that caused the BLOCKED verdict.

## Files changed

The verifier's diff base is `start_commit` (`62faf7d31f8ab9158d349f3a2859754aeece88c9`). A forward-merge from `release-wt/2026-06-19-safe-parallelism` at Step 0 pulled in the rest of the release's merged tracks; those files are NOT S42-owned and are documented in **Divergence from plan**. The S42-owned changes within this list are:

- `cmd/sworn/run.go`
- `internal/run/run.go`
- `internal/run/slice.go`
- `internal/run/slice_test.go`
- `docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json`
- `docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/proof.md`

### Verbatim `git diff --name-only 62faf7d31f8ab9158d349f3a2859754aeece88c9 HEAD`

```
cmd/sworn/account.go
cmd/sworn/account_test.go
cmd/sworn/commands_test.go
cmd/sworn/induction.go
cmd/sworn/induction_test.go
cmd/sworn/init.go
cmd/sworn/init_design_system_test.go
cmd/sworn/init_test.go
cmd/sworn/login.go
cmd/sworn/run.go
docs/adr/0008-canonical-baton.md
docs/api-contract.md
docs/considerations.md
docs/release/2026-06-19-safe-parallelism/.captain-trial-log.md
docs/release/2026-06-19-safe-parallelism/S06a-sworn-login-auth/design.md
docs/release/2026-06-19-safe-parallelism/S06a-sworn-login-auth/journal.md
docs/release/2026-06-19-safe-parallelism/S06a-sworn-login-auth/proof.md
docs/release/2026-06-19-safe-parallelism/S06a-sworn-login-auth/review.md
docs/release/2026-06-19-safe-parallelism/S06a-sworn-login-auth/status.json
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/design.md
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/journal.md
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/proof.md
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/spec.md
docs/release/2026-06-19-safe-parallelism/S06b-sworn-proxy-credits/status.json
docs/release/2026-06-19-safe-parallelism/S07-paging/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S07-paging/design.md
docs/release/2026-06-19-safe-parallelism/S07-paging/journal.md
docs/release/2026-06-19-safe-parallelism/S07-paging/proof.md
docs/release/2026-06-19-safe-parallelism/S07-paging/review.md
docs/release/2026-06-19-safe-parallelism/S07-paging/status.json
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/design.md
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/journal.md
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/proof.md
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/review.md
docs/release/2026-06-19-safe-parallelism/S09-per-role-model-config/status.json
docs/release/2026-06-19-safe-parallelism/S10-provider-foundation/spec.md
docs/release/2026-06-19-safe-parallelism/S10-provider-foundation/status.json
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/design.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/journal.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/proof.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/review.md
docs/release/2026-06-19-safe-parallelism/S18-consideration-catalog/status.json
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/design.md
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/journal.md
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/proof.md
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/review.md
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/status.json
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/design.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/journal.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/proof.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/review.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/status.json
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/journal.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/proof.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/spec.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json
docs/release/2026-06-19-safe-parallelism/S48-baton-vendor/journal.md
docs/release/2026-06-19-safe-parallelism/S48-baton-vendor/status.json
docs/release/2026-06-19-safe-parallelism/S57-oracle-reader/spec.md
docs/release/2026-06-19-safe-parallelism/index.md
docs/release/2026-06-19-safe-parallelism/intake.md
docs/templates/agents.md
docs/templates/considerations.md
docs/templates/decisions.md
internal/account/account.go
internal/account/account_test.go
internal/account/notify.go
internal/account/notify_test.go
internal/account/proxy.go
internal/account/proxy_test.go
internal/config/config.go
internal/config/config_test.go
internal/config/init.go
internal/model/config.go
internal/model/oai.go
internal/model/oai_test.go
internal/prompt/baton/README.md
internal/prompt/baton/VERSION.txt
internal/prompt/baton/brainstorm-patterns.md
internal/prompt/baton/rules.md
internal/prompt/baton/session-discipline.md
internal/prompt/implementer.md
internal/prompt/planner.md
internal/prompt/prompt.go
internal/prompt/prompt_test.go
internal/prompt/verifier.md
internal/run/parallel.go
internal/run/run.go
internal/run/run_test.go
internal/run/slice.go
internal/run/slice_test.go
internal/scheduler/worker.go
```

```sh
$ git diff --stat HEAD
cmd/sworn/run.go           | 41 ++++++++++++++++++++++++++++++++++++++++-
internal/run/run.go        |  8 +++++++-
internal/run/slice.go      | 16 ++++++++++++----
internal/run/slice_test.go | 11 +++++------
4 files changed, 64 insertions(+), 12 deletions(-)
```

**Note:** `internal/config/config.go` is intentionally **not** in the slice's `git diff --stat HEAD`. The config-file tier is deferred per the spec's Out of scope section.

## Test results

### Go

```sh
$ go test -race -count=1 ./internal/run/...
ok      github.com/swornagent/sworn/internal/run      3.644s
```

```sh
$ go vet ./...
(no output)
```

```sh
$ go build ./...
(no output)
```

### Test detail

```sh
$ go test -race -run 'TestImplementTimeout' ./internal/run/... -v
=== RUN   TestImplementTimeoutEscalates
sworn run: attempt 1/2 — implementing with blocking
sworn run: implement attempt 1 timed out after 500ms — escalating
sworn run: escalating implementer model for retry
sworn run: attempt 2/2 — implementing with working
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutEscalates (0.57s)
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
--- PASS: TestImplementTimeoutHappyPath (0.06s)
=== RUN   TestImplementTimeoutZeroUsesDefault
sworn run: attempt 1/1 — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutZeroUsesDefault (0.05s)
=== RUN   TestImplementTimeoutNegativeNoTimeout
sworn run: attempt 1/1 — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
--- PASS: TestImplementTimeoutNegativeNoTimeout (0.05s)
PASS
ok      github.com/swornagent/sworn/internal/run      1.988s
```

## Reachability artefact

- **Type**: unit test output + stderr demonstration
- **Path**: `internal/run/slice_test.go`
- **Evidence**: `TestImplementTimeoutEscalates` exercises `RunSlice` end-to-end with a blocking fake agent on slot 0, a 500ms timeout, and a working agent on slot 1. The stderr output shows `implement attempt 1 timed out after 500ms — escalating` followed by slot 2 running and verification passing. `TestImplementTimeoutExhaustsToHuman` confirms fail-closed behaviour when all models time out.

## Delivered

- Per-attempt context deadline wrapping in `RunSlice` (`internal/run/slice.go`) — evidence: `context.WithTimeout` wrapping `implement.Run` call.
- `DefaultImplementTimeout` named constant in `internal/run/slice.go` (15m) — evidence: `const DefaultImplementTimeout = 15 * time.Minute`.
- Timeout detection via `errors.Is(err, context.DeadlineExceeded)` — evidence: distinct stderr message `"implement attempt N timed out after <d> — escalating"`.
- `--implement-timeout` CLI flag — evidence: `cmd/sworn/run.go`, `flag.Duration("implement-timeout", 0, ...)`.
- `SWORN_IMPLEMENT_TIMEOUT` env var — evidence: `resolveImplementTimeout` reads `os.Getenv("SWORN_IMPLEMENT_TIMEOUT")`.
- Precedence: flag > env > default — evidence: `resolveImplementTimeout` in `cmd/sworn/run.go`.
- Zero means use default, negative means no timeout — evidence: `resolveImplementTimeout` and tests `TestImplementTimeoutZeroUsesDefault`, `TestImplementTimeoutNegativeNoTimeout`.
- `ImplementTimeout` threaded through `internal/run/run.go` `Options` → `RunSliceOptions`.
- `internal/config/config.go` is **not** in the slice's diff — evidence: `git diff --stat HEAD` lists only `cmd/sworn/run.go`, `internal/run/run.go`, `internal/run/slice.go`, `internal/run/slice_test.go`.

## Not delivered

- Config-file `implementer.timeout` tier — **deferred** (Rule 2). Why: adding it requires touching `internal/config/config.go`, which is owned by T3-merged and a planned touchpoint of T6/T16, causing the cross-track collision that produced the first BLOCKED verdict. Tracking: a later slice may add an `implementer.timeout` config field once `config.go` ownership is settled. Ack: planner replan 2026-06-23.
- Default `http.Client.Timeout` on `internal/model/oai.go` — **deferred** (Rule 2). Why: per-attempt context deadline already bounds the HTTP call, and `oai.go` is a future S39/T5 touchpoint. Tracking: revisit with S39 if a non-context hang path appears. Ack: Coach 2026-06-21.
- Killing agent-spawned OS subprocesses — **deferred** (Rule 2). Why: supervisor stale-PID reaping covers cross-session orphans. Tracking: supervisor reaping is the cross-session mechanism; in-process cancellation is this slice's scope. Ack: Coach 2026-06-21.
- Per-step timeouts for the verify step — out of scope per spec.

## Divergence from plan

The forward-merge from `release-wt/2026-06-19-safe-parallelism` at Step 0 pulled in 91 additional files belonging to other tracks/slices that had merged into `release-wt` since S42's `start_commit` (`62faf7d31f8ab9158d349f3a2859754aeece88c9`). Those files appear in `git diff --name-only start_commit HEAD` but are NOT S42-owned; they are present because the track worktree must stay current with the release integration branch. The S42-owned changes remain only:

- `cmd/sworn/run.go`
- `internal/run/run.go`
- `internal/run/slice.go`
- `internal/run/slice_test.go`
- `docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json`
- `docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/proof.md`

The original S42 implementation diverged by adding `internal/config/config.go` changes; this re-implementation removes them and restores the spec-mandated precedence (flag > env > default) with `DefaultImplementTimeout` located in `internal/run/slice.go`.
