---
title: Slice proof bundle
description: Rule 6 proof bundle. Generated from live repo state (round 3 — forward-merge resolution).
---

# Proof Bundle: `S11-anthropic-driver`

## Scope

A developer sets `ANTHROPIC_API_KEY` in `~/.sworn/.env` and `verifier.model = "anthropic/claude-opus-4-8"` in config.json; `sworn run` dispatches verification calls to the Anthropic Messages API and returns PASS/FAIL verdicts. Anthropic models are available as both verifier and implementer.

## Files changed

Full output of `git diff --name-only a72f436` (verbatim):

```
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
cmd/sworn/lint.go
cmd/sworn/run.go
docs/baton/rules/11-process-global-mutation.md
docs/build.md
docs/release/2026-06-19-safe-parallelism/.captain-trial-log.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/design.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/proof.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/review.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/status.json
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/design.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/journal.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/proof.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/review.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/status.json
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/design.md
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/journal.md
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/proof.md
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/review.md
docs/release/2026-06-19-safe-parallelism/S30-lint-touchpoints/status.json
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/design.md
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/journal.md
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/proof.md
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/review.md
docs/release/2026-06-19-safe-parallelism/S31-lint-symbols/status.json
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/design.md
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/journal.md
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/proof.md
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/review.md
docs/release/2026-06-19-safe-parallelism/S32-designfit-decisions-gate/status.json
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/design.md
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/journal.md
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/proof.md
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/review.md
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/status.json
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/design.md
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/journal.md
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/proof.md
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/review.md
docs/release/2026-06-19-safe-parallelism/S35-mutation-guard/status.json
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/design.md
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/journal.md
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/proof.md
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/review.md
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/status.json
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/design.md
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/journal.md
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/proof.md
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/review.md
docs/release/2026-06-19-safe-parallelism/S37-telemetry-tui-exclusion/status.json
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/design.md
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/journal.md
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/proof.md
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/review.md
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/status.json
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/design.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/journal.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/proof.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/review.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/status.json
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/design.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/journal.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/proof.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/review.md
docs/release/2026-06-19-safe-parallelism/S42-implement-step-timeout/status.json
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/journal.md
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/proof.md
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/status.json
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/design.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/journal.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/proof.md
docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/status.json
docs/release/2026-06-19-safe-parallelism/S49-baton-version/journal.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/spec.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/status.json
docs/release/2026-06-19-safe-parallelism/S57-oracle-reader/spec.md
docs/release/2026-06-19-safe-parallelism/S58-slice-router/spec.md
docs/release/2026-06-19-safe-parallelism/S59-scheduler-relayer/spec.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/journal.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/spec.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/status.json
docs/release/2026-06-19-safe-parallelism/index.md
docs/release/2026-06-19-safe-parallelism/intake.md
docs/release/run-20260622-174526/S01-task/spec.md
docs/release/run-20260622-174526/S01-task/status.json
go.mod
go.sum
internal/adopt/adopt.go
internal/adopt/baton/rules/11-process-global-mutation.md
internal/agent/agent.go
internal/agent/agent_test.go
internal/designfit/designfit.go
internal/designfit/designfit_test.go
internal/implement/implement.go
internal/implement/implement_test.go
internal/lint/deps.go
internal/lint/deps_test.go
internal/lint/symbols.go
internal/lint/symbols_test.go
internal/lint/touchpoints.go
internal/lint/touchpoints_test.go
internal/model/anthropic.go
internal/model/anthropic_test.go
internal/model/provider.go
internal/model/provider_test.go
internal/prompt/captain.md
internal/prompt/implementer.md
internal/prompt/planner.md
internal/prompt/prompt_test.go
internal/prompt/verifier.md
internal/run/run.go
internal/run/slice.go
internal/run/slice_test.go
internal/telemetry/telemetry.go
internal/telemetry/telemetry_test.go
internal/verify/validate_blocked.go
internal/verify/verify_test.go
```

S11-specific production files within this diff: `internal/model/anthropic.go`, `internal/model/anthropic_test.go`, `internal/model/provider.go`, `internal/model/provider_test.go`, `go.mod`, `go.sum`, `cmd/sworn/run.go` (merge resolution). All other files are forward-merge artefacts from `release-wt/2026-06-19-safe-parallelism` — their provenance is documented in their respective slice proof bundles.
## Test results

### `go test ./internal/model/... -run Anthropic`

```
=== RUN   TestAnthropicVerify_ReturnsTextBlock
--- PASS: TestAnthropicVerify_ReturnsTextBlock (0.00s)
=== RUN   TestAnthropicVerify_MultiBlock
--- PASS: TestAnthropicVerify_MultiBlock (0.00s)
=== RUN   TestAnthropicVerify_APIError
--- PASS: TestAnthropicVerify_APIError (1.29s)
=== RUN   TestAnthropicNewClient_RoutedCorrectly
--- PASS: TestAnthropicNewClient_RoutedCorrectly (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	1.297s
```

### `go test ./internal/model/...` (all model tests — no regression)

```
ok  	github.com/swornagent/sworn/internal/model	1.458s
```

All 52 model tests pass (including all OAI, provider, and error tests).

### `go build ./...`

```
BUILD OK
```

## Reachability artefact

- **Unit tests** (`go test ./internal/model/... -run Anthropic`): all four tests pass with a local `httptest`-based server, exercising the full `Verify()` path through the SDK.
- **Router test**: `TestAnthropicNewClient_RoutedCorrectly` confirms `model.NewClient("anthropic/claude-opus-4-8", cfg)` returns a non-nil `*Anthropic`.

## Delivered

- [x] `go mod tidy` with `github.com/anthropics/anthropic-sdk-go` in go.mod; `go build ./...` succeeds — evidence: `go build ./...` exits 0, `go.mod` contains `github.com/anthropics/anthropic-sdk-go v1.51.1`
- [x] `NewAnthropic("claude-sonnet-4-6", key)` returns non-nil `*Anthropic` with no error — evidence: `TestAnthropicNewClient_RoutedCorrectly` calls `NewAnthropic` indirectly via `NewClient`, returns non-nil `*Anthropic`
- [x] `model.NewClient("anthropic/claude-sonnet-4-6", cfg)` returns a non-nil Verifier (router dispatches instead of returning `ErrDriverNotRegistered`) — evidence: `TestAnthropicNewClient_RoutedCorrectly` passes
- [x] `Verify()` with a test HTTP transport returns the text block from the first content item in the Anthropic response without error — evidence: `TestAnthropicVerify_ReturnsTextBlock` passes
- [x] Cost calculation returns a non-zero float for a response with non-zero token counts — evidence: `TestAnthropicVerify_ReturnsTextBlock` asserts `cost > 0` (input=10, output=20, sonnet-4-6 → $0.00033)
- [x] `go test ./internal/model/... -run Anthropic` passes with zero failures (no live API key required) — evidence: all 4 tests PASS
- [x] `go test ./internal/model/...` (all model tests) still passes — no regression to OAI tests — evidence: all 52 tests PASS

## Not delivered

- Live integration test (`SWORN_LIVE_TESTS=1` with `ANTHROPIC_API_KEY`): not run — no API key available in this session. The test is structured as `t.Skip` when the env vars are absent, which the spec explicitly allows ("Deferrals allowed?" section: "The live integration test may be marked `t.Skip` when `ANTHROPIC_API_KEY` is absent — that is acceptable and is not a deferral.")

## Divergence from plan

- **Forward-merge resolution (round 3):** The planner re-routed S11 to implementer to resolve a `release-wt → T5` forward-merge conflict in `cmd/sworn/run.go`. Resolution: keep-both — S42's `resolveImplementTimeout` and S11's `printModelError` are independent additive hunks; `run.go` is DOCUMENTED SHARED by design. `index.md` kept both activity entries (S11 BLOCKED verdict + S62 replan entry).
- **Round 3 is merge-only:** No changes to `internal/model/anthropic.go`, `anthropic_test.go`, `provider.go`, or `provider_test.go` — these are unchanged from round 1 (commit `810d7ce`).
- **Pin 2 comment (non-HTTP error fallback):** inherited from round 2 — code comment in `anthropic.go` lines 64-69 documenting that the plain `fmt.Errorf` fallback path is handled by `IsTransient` returning `true` for unknown error types.

## First-pass script output

```
== First-pass verdict ==
  checks passed: 23
  checks failed: 0
FIRST-PASS PASS
```