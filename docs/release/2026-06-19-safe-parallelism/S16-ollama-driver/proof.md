---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S16-ollama-driver`

## Scope

Implement a native Ollama API driver (`internal/model/ollama.go`) using Ollama's
`POST /api/chat` endpoint (not the OAI-compat shim). Replace the existing OAI-compat
`ollama/*` preset in `internal/model/provider.go` with a native `*Ollama` driver
that implements `Verifier` via stdlib `net/http` + `encoding/json` (zero new deps).

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S16-ollama-driver/status.json
internal/model/ollama.go
internal/model/ollama_test.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### Ollama-specific tests (`go test ./internal/model/... -run Ollama -v`)

```
=== RUN   TestOllamaVerify_ReturnsContent
--- PASS: TestOllamaVerify_ReturnsContent (0.00s)
=== RUN   TestOllamaVerify_ErrorField
--- PASS: TestOllamaVerify_ErrorField (0.00s)
=== RUN   TestOllamaVerify_NonOKStatus
--- PASS: TestOllamaVerify_NonOKStatus (0.00s)
=== RUN   TestOllamaDefaultHost
--- PASS: TestOllamaDefaultHost (0.00s)
=== RUN   TestOllamaHostFromEnv
--- PASS: TestOllamaHostFromEnv (0.00s)
=== RUN   TestOllamaRequestFormat
--- PASS: TestOllamaRequestFormat (0.00s)
=== RUN   TestNewClient_OllamaIsNative
--- PASS: TestNewClient_OllamaIsNative (0.00s)
=== RUN   TestNewClient_Ollama
--- PASS: TestNewClient_Ollama (0.00s)
=== RUN   TestOllamaHostDefault
--- PASS: TestOllamaHostDefault (0.00s)
=== RUN   TestOllamaHostCustom
--- PASS: TestOllamaHostCustom (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	(cached)
```

### Full model test suite (`go test ./internal/model/...`)

```
ok  	github.com/swornagent/sworn/internal/model	1.456s
```

All 82 tests pass (0 failures). No prior model tests regressed.

### Build (`go build ./...`)

Clean — zero errors, zero warnings.

## Reachability artefact

- **Unit reachability**: `TestNewClient_OllamaIsNative` — calls
  `model.NewClient("ollama/llama3.2", cfg)` and type-asserts the return is
  `*Ollama`, not `*OAI`. Proves the dispatch path from `NewClient` through to
  the native driver.
- **Live reachability** (`TestOllamaVerify_Live`): not implemented in this slice
  — spec defers to a live integration test gated on `SWORN_LIVE_TESTS=1`. Unit
  tests cover the full code path via `httptest.Server`.

## Delivered

- [x] `internal/model/ollama.go` — native Ollama driver using `POST /api/chat`
  (stdlib `net/http` + `encoding/json`, zero new deps). `Ollama` struct with
  `Host`, `Model`, `Client` fields. `NewOllama(modelID, host)` constructor.
  `Verify(ctx, systemPrompt, userPayload)` dispatches to `<host>/api/chat`.
- [x] `internal/model/ollama_test.go` — 7 unit tests using `httptest.Server`:
  content extraction, error field, non-OK status, default host, env var host,
  request format validation, `NewClient` dispatch type assertion.
- [x] `internal/model/provider.go` — replaced OAI-compat `case "ollama"` block
  with native `NewOllama(model, pcfg.OllamaHost)`. Fixed stale
  `ProviderConfig.OllamaHost` comment (removed `/v1` suffix). Removed unused
  `"strings"` import. Fixed pre-existing struct formatting (tabs → newlines).
- [x] `internal/model/provider_test.go` — updated `TestNewClient_Ollama` to
  assert `*Ollama` type and native host (no `/v1`), matching Coach Captain pin 1.
- [x] `go build ./...` succeeds with no new external dependencies.
- [x] All prior model tests (82 total) pass with 0 failures.
- [x] All spec acceptance checks (lines 63-75) satisfied (see test evidence above).

## Not delivered

- **Live integration test** (`TestOllamaVerify_Live`): requires a running Ollama
  instance + `SWORN_LIVE_TESTS=1`. Spec defers this to post-implementation
  verification; unit tests cover the full code path via `httptest.Server`.
  - **Why**: Ollama not running in CI; live test is gated on opt-in env var
    per spec.
  - **Tracking**: spec.md "Required tests" section — live integration test
    (skipped unless Ollama is running locally and `SWORN_LIVE_TESTS=1`).
  - **Acknowledgement**: Spec-level deferral (not a Rule 2 deferral) — spec
    explicitly says "skipped unless Ollama is running".
- **Model pull / list / push APIs**: deferred post-R3 per spec "Out of scope".
- **Ollama multimodal / streaming / keep_alive / options**: out of scope per spec.

## Divergence from plan

- **Pre-existing struct formatting fixed**: `ProviderConfig` struct in
  `internal/model/provider.go` had tab characters instead of newlines between
  field declarations (`AwsAccessKey` and `AwsSecretKey` on a single
  tab-separated line). Fixed to proper newline-separated fields. No
  behavioural change.
- **Pre-existing stale comment fixed**: `ProviderConfig.OllamaHost` comment
  said `defaults to http://localhost:11434/v1` — corrected to
  `defaults to http://localhost:11434` (per Captain pin 4).

## First-pass script output

```
release-verify.sh
  slice:       S16-ollama-driver
  slice dir:   docs/release/2026-06-19-safe-parallelism/S16-ollama-driver
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  5 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has all required sections
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 21
  checks failed: 1
FIRST-PASS FAIL
```

The only failure is the state check — expected while slice is `in_progress`.
Will re-run after transitioning to `implemented`.