# Proof Bundle: `S21-openai-structured-envelope`

## Scope

Introduce a closed-world, exact-identity OpenAI structured-output envelope for
generic llm-check reports while retaining the unchanged canonical S04 semantic
gate and every non-OpenAI structured-schema path.

## Files changed

Generated from live scope with:

```sh
git diff --name-only ed0badf68673f0af84834458f07be0792555484f..HEAD
```

- `cmd/sworn/llmcheck_test.go`
- `docs/release/2026-07-15-baton-v0.15-conformance/S21-openai-structured-envelope/journal.md`
- `docs/release/2026-07-15-baton-v0.15-conformance/S21-openai-structured-envelope/proof.json`
- `docs/release/2026-07-15-baton-v0.15-conformance/S21-openai-structured-envelope/proof.md`
- `docs/release/2026-07-15-baton-v0.15-conformance/S21-openai-structured-envelope/status.json`
- `internal/gate/llmcheck_test.go`
- `internal/model/config.go`
- `internal/model/llmcheck_envelope.go`
- `internal/model/llmcheck_envelope_test.go`
- `internal/model/oai.go`
- `internal/model/oai_test.go`
- `internal/model/openai_responses.go`
- `internal/model/openai_responses_test.go`
- `internal/model/provider.go`
- `internal/model/structured_test.go`

The list contains no vendored canonical schema, vendored prompt, or
`internal/gate/llmcheck.go` source change.

## Test results

The following Go commands ran in an independent clean clone at implementation
commit `a58dbe498c52e60ad4fc3a6021e01b9c61589fd8`, with inherited worktree Git
environment variables cleared. All exited 0 without a credentialed provider
request or model dispatch.

- `go test ./internal/model -run 'Test(CompileOpenAILLMCheckEnvelopeExactIdentity|OpenAIEnvelopeProfileRejectsUnsupportedCanonicalSchemasBeforeHTTP|OAIChatStructuredUsesLLMCheckEnvelopeOnlyForOpenAICompletions|OpenAIResponsesChatStructuredUsesLLMCheckEnvelope|XAIChatStructuredRetainsRawSchemaProfile|ToolCallChatStructuredRetainsRawSchema|StructuredProviderProfileDefaultsClosed)' -count=1`
- `go test ./internal/gate ./cmd/sworn -run 'Test(OpenAIEnvelopeStillFailsFullCanonicalReportViolations|LLMCheckOpenAI)' -count=1`
- `go test ./...`
- `go vet ./...`
- `make build`

The slice coverage gate also passed:

```sh
bin/sworn lint coverage --slice S21-openai-structured-envelope \
  --release 2026-07-15-baton-v0.15-conformance \
  --base ed0badf68673f0af84834458f07be0792555484f
```

It reported six of six acceptance criteria covered. `git diff --check
ed0badf68673f0af84834458f07be0792555484f..HEAD` also exited 0.

The declared-local-fixture mock gate also passed:

```sh
bin/sworn lint mock --slice S21-openai-structured-envelope \
  --release 2026-07-15-baton-v0.15-conformance \
  --base ed0badf68673f0af84834458f07be0792555484f
```

It found no undeclared mock boundary. The deterministic proof-bundle
first-pass likewise exited 0 with `PASS` and `cost_usd: 0`, using an isolated
synthetic configuration and no agentic dispatch or provider request.

## Reachability artefact

`cmd/sworn/llmcheck_test.go:TestLLMCheckOpenAIResponsesStructuredEnvelopeBinaryReachability`
and
`cmd/sworn/llmcheck_test.go:TestLLMCheckOpenAICompletionsStructuredEnvelopeBinaryReachability`
build and invoke the public `sworn llm-check --type ac-satisfaction` command.
Each uses a deterministic local endpoint that inspects the appropriate outgoing
strict envelope, returns a real model-shaped `ac-satisfaction` PASS report, and
asserts that the built binary exits 0. The two tests exercise, respectively,
Responses `text.format` and chat/completions `response_format`.

## Delivered

- AC-01: a sealed `llm-check-report-v1-openai-envelope` selected only by the
  canonical report `$id` and pinned source digest through an explicit OpenAI
  profile and wire pair. Evidence: `internal/model/llmcheck_envelope.go` and
  `TestCompileOpenAILLMCheckEnvelopeExactIdentity`.
- AC-02: direct and proxy constructor routing for both supported OpenAI wires,
  including the deprecated Responses alias and local base-URL override.
  Evidence: `internal/model/provider.go`, `internal/model/config.go`, and both
  built-binary reachability tests.
- AC-03: local, deterministic, zero-HTTP rejection for unsupported generic
  identities and the dedicated ambiguity report. Evidence:
  `TestOpenAIEnvelopeProfileRejectsUnsupportedCanonicalSchemasBeforeHTTP` and
  `TestLLMCheckOpenAIUnsupportedCanonicalSchemaMakesZeroRequests`.
- AC-04 and AC-05: unchanged generic canonical validation and requested/emitted
  check equality still reject semantic violations rather than repairing them.
  Evidence: `internal/gate/llmcheck.go` is absent from the live diff and
  `TestOpenAIEnvelopeStillFailsFullCanonicalReportViolations` passes.
- AC-04: xAI native response format, forced tool calls, and unprofiled
  OAI-compatible clients retain the supplied schema. Evidence:
  `TestXAIChatStructuredRetainsRawSchemaProfile`,
  `TestToolCallChatStructuredRetainsRawSchema`, and
  `TestStructuredProviderProfileDefaultsClosed`.
- AC-06: transport evidence is deterministic and local only; no canonical
  schema, prompt, gate source, S20 lifecycle evidence, or real-home content
  changed. S20 remains blocked pending fresh S21 verification.
- Intentional local HTTP fixture boundaries are declared with `@mock-boundary`
  in the affected test files; `bin/sworn lint mock` passes without exempting
  production behavior.

## Not delivered

- Credentialed OpenAI acceptance smoke and the S20 readiness and maintainability
  reruns are deliberately deferred. Why: S21 proves only the deterministic
  envelope boundary, and a credentialed emitted report must remain S20 evidence.
  Tracking: `S20-v015-parity-portable-fixture` after fresh
  `S21-openai-structured-envelope` verifier PASS. Acknowledged by the Coach in
  S21 `status.json` at `2026-07-17T07:57:25+10:00`.

## Divergence from plan

- `internal/model/structured.go` was an anticipated touchpoint but did not need
  an edit. The new compiler selects the envelope before the existing
  `strictProjection`, deliberately retaining that generic projection unchanged
  for non-selected schemas and providers.
