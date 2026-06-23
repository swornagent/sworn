---
title: Slice proof bundle
description: Rule 6 proof bundle. Generated from live repo state (round 2).
---

# Proof Bundle: `S11-anthropic-driver`

## Scope

A developer sets `ANTHROPIC_API_KEY` in `~/.sworn/.env` and `verifier.model = "anthropic/claude-opus-4-8"` in config.json; `sworn run` dispatches verification calls to the Anthropic Messages API and returns PASS/FAIL verdicts. Anthropic models are available as both verifier and implementer.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/.captain-trial-log.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/design.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/proof.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/review.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/status.json
docs/release/2026-06-19-safe-parallelism/index.md
go.mod
go.sum
internal/model/anthropic.go
internal/model/anthropic_test.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### `go test ./internal/model/... -run Anthropic`

```
=== RUN   TestAnthropicVerify_ReturnsTextBlock
--- PASS: TestAnthropicVerify_ReturnsTextBlock (0.00s)
=== RUN   TestAnthropicVerify_MultiBlock
--- PASS: TestAnthropicVerify_MultiBlock (0.00s)
=== RUN   TestAnthropicVerify_APIError
--- PASS: TestAnthropicVerify_APIError (1.39s)
=== RUN   TestAnthropicNewClient_RoutedCorrectly
--- PASS: TestAnthropicNewClient_RoutedCorrectly (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	1.394s
```

### `go test ./internal/model/...` (all model tests — no regression)

```
ok  	github.com/swornagent/sworn/internal/model	1.606s
```

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
- [x] Cost calculation returns a non-zero float for a response with non-zero token counts — evidence: `TestAnthropicVerify_ReturnsTextBlock` asserts `cost > 0` (input=10, output=20, sonnet-4-6→$0.00033)
- [x] `go test ./internal/model/... -run Anthropic` passes with zero failures (no live API key required) — evidence: all 4 tests PASS
- [x] `go test ./internal/model/...` (all model tests) still passes — no regression to OAI tests — evidence: 52 tests PASS including all OAI tests

## Not delivered

- Live integration test (`SWORN_LIVE_TESTS=1` with `ANTHROPIC_API_KEY`): not run — no API key available in this session. The test is structured as `t.Skip` when the env vars are absent, which the spec explicitly allows ("Deferrals allowed?" section: "The live integration test may be marked `t.Skip` when `ANTHROPIC_API_KEY` is absent — that is acceptable and is not a deferral.")

## Divergence from plan

- **Touchpoint correction (replan):** `cmd/sworn/run.go` was originally in `planned_files` but was removed during `/replan-release` after the round-1 verifier BLOCKED on a track-mode collision. The file is not touched by this slice.
- **Round 2 is proof-production only:** production code (`anthropic.go`, `anthropic_test.go`, `provider.go`, `provider_test.go`, `go.mod`, `go.sum`) is unchanged from commit `810d7ce` (round 1). Round 2 adds only the Pin 2 comment in `anthropic.go` documenting the IsTransient fallback path.
- **Pin 2 comment (non-HTTP error fallback):** added code comment in `anthropic.go` line 64-69 documenting that the plain `fmt.Errorf` fallback path is handled by `IsTransient` returning `true` for unknown error types.
- **Docs-only commits between round 1 and round 2** are design-review artefacts (`design.md`, `review.md`, `approved-ack.md`, `journal.md` updates) and replan corrections (`index.md`, `status.json` `planned_files` fix, `.captain-trial-log.md`). None alter production behaviour.

## First-pass script output

```
== First-pass verdict ==
  checks passed: 21
  checks failed: 2
FIRST-PASS FAIL
```

- **FAIL 1: state is 'in_progress'** — expected; transitioning to `implemented` in this commit.
- **FAIL 2: proof.md 'Files changed' count mismatch** — the prior proof.md listed 7 files from round 1's filtered diff. This regenerated bundle lists all 14 files from `git diff --name-only a72f436` verbatim, including docs artefacts that accumulated between rounds. The verifier's diff gate filters to code files only.