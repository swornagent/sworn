# S22-openrouter-tool-structured-output proof bundle

Fail-closed handoff. Deterministic evidence and the no-model-dispatch proof
preflight passed, but the sole Coach-authorized AC-06 provider attempt did not
produce a valid sanitized receipt. This is not a completion claim.

## Scope

Add only the direct OpenRouter forced-tool transport and fail-closed malformed
tool-call guards required for the Coach-selected S22 proof. Preserve the
canonical Baton gate and keep proxy OpenRouter, Ollama, and unprofiled routes
unsupported.

## Files changed

Generated from `git diff --name-only
a09b0e46df465862d00469d4aef2a997442b3d5b`:

- `cmd/sworn/llmcheck_test.go`
- `docs/release/2026-07-15-baton-v0.16-conformance/S22-openrouter-tool-structured-output/{journal.md,proof.json,proof.md,spec.json,status.json}`
- `docs/release/2026-07-15-baton-v0.16-conformance/{index.md,intake.md}`
- `internal/gate/llmcheck_test.go`
- `internal/model/{config.go,llmcheck_envelope.go,llmcheck_envelope_test.go,oai.go,oai_test.go,provider.go,provider_test.go,structured_test.go}`

## Test results

- PASS — focused model transport and guard tests, including
  `TestOpenRouterStructuredRejectsInvalidToolCall` (AC-07/AC-08), exit 0.
- PASS — focused gate and built-command tests, including
  `TestLLMCheckOpenRouterToolStructured`, exit 0.
- PASS — `go test ./... -count=1`, exit 0, in an isolated no-credential,
  no-model environment.
- PASS — `go vet ./...`, exit 0, in the same isolated environment.
- PASS — `make build`, exit 0, in the same isolated environment.
- PASS — `go test -cover ./internal/model -count=1`, exit 0; 81.4% of
  statements covered.
- PASS — current binary proof gate with `--spec`, `--diff`, and `--proof`,
  exit 0. It used a synthetic direct OpenRouter construction with an unroutable
  local endpoint; the deterministic first-pass path reported cost 0 and did not
  dispatch a model.

`GOFLAGS=-buildvcs=false` was required for Go commands in this worktree because
the host-level VCS-status probe cannot resolve repository metadata; it does not
change source, test selection, or provider routing.

## Reachability artefact

`cmd/sworn/llmcheck_test.go:TestLLMCheckOpenRouterToolStructuredBinaryReachability`
builds and drives `sworn llm-check` through direct OpenRouter configuration to a
local fake chat-completions endpoint with a synthetic key. It verifies the exact
forced-tool wire, canonical report acceptance, and exit 0 without a provider
call.

## Sanitized AC-06 receipt

- Check identity: `spec-ambiguity`
- Model ID: `openrouter/z-ai/glm-5.2`
- Immutable start commit: `a09b0e46df465862d00469d4aef2a997442b3d5b`
- Process exit code: `unavailable`
- Result: `UNPARSEABLE`

The raw temporary file was destroyed. The exactly-one budget is consumed: S22
is blocked for the Planner, with no retry or fallback, and S20 remains
untouched.

## Delivered

- AC-01: direct-only selection and default-deny proxy/Ollama/unprofiled routes
  — `TestNewClient_OpenRouterDirectUsesStructuredToolCall`,
  `TestProxyOpenRouterRemainsStructuredUnsupported`, and
  `TestOllamaRemainsVerifyOnly`.
- AC-02: unchanged canonical schema is the forced
  `emit_structured_output` function parameters —
  `TestOpenRouterChatStructuredUsesCanonicalForcedTool`.
- AC-03: built-command direct transport reaches a local fake and accepts the
  canonical `ac-satisfaction` report —
  `TestLLMCheckOpenRouterToolStructuredBinaryReachability`.
- AC-04: malformed calls and canonical report violations fail locally with no
  repair or second request — `TestOpenRouterStructuredRejectsInvalidToolCall`,
  `TestOpenRouterToolPathStillFailsFullCanonicalReportViolations`, and
  `TestLLMCheckOpenRouterToolStructuredBinaryRejectsInvalidResponse`.
- AC-05: direct-only validated endpoint override —
  `TestFromEnvOpenRouterDirectBaseURLOverride` and
  `TestFromEnvOpenRouterProxyIgnoresDirectBaseURLOverride`.
- AC-07: literal wire `function.arguments: null` remains distinguishable from
  an empty string and is rejected locally after exactly one request —
  `FunctionCall.UnmarshalJSON` and
  `TestOpenRouterStructuredRejectsInvalidToolCall`.
- AC-08: a direct tool-call type other than `function` is rejected locally
  after exactly one request — `structuredToolCallArguments` and
  `TestOpenRouterStructuredRejectsInvalidToolCall`.

## Not delivered

- AC-06 did not produce a valid sanitized receipt. The sanctioned receipt
  fields are recorded above; the raw temporary file was destroyed. The sole
  attempt is consumed, so there is no retry, fallback, `implemented` state, or
  S20 activity. Tracking: `S22-openrouter-tool-structured-output AC-06`;
  acknowledged by the Coach.
- The role-prompt `sworn coverage` command does not exist in the current binary
  (`unknown command`). Why: adding it is outside this S22 recovery. Tracking:
  `sworn#122`; acknowledged by the Coach. The AC-to-test matrix and local Go
  coverage measurement are retained instead.

## Divergence from plan

None.
