# Design TL;DR — S21-openai-structured-envelope

Date: 2026-07-17T08:13:39+10:00
State: `design_review`
Contract owner: C-14, with C-02/S04 retained as the generic-report semantic
authority.

## Outcome and fixed boundary

S21 will let a generic `sworn llm-check` use strict structured output through
only the two explicit OpenAI transports:

- `openai/` uses Responses `text.format`;
- `openai-completions/` uses chat/completions `response_format`.

It will do so without changing the exact canonical
`llm-check-report-v1` source bytes, the vendored prompt or common user-payload
bytes, `internal/gate/llmcheck.go` semantic authority, local canonical
validation, or S04's requested/emitted `check` equality. The provider envelope
is an outbound constraint below that authority, never a replacement report
schema and never a source of synthetic fields.

The only source eligible for the envelope is the exact raw schema identity:

```
$id:    https://baton.sawy3r.net/schemas/llm-check-report-v1.json
SHA-256 ed38b77823af1b329c1dc7d8427b08849f15690d5afa9625e196505bdfa5b65b
```

S04 stays `verified`. S20 stays `blocked` and untouched: its credentialed
OpenAI smoke is later evidence, permitted only after a fresh S21 verifier PASS
and then S20's own preserved readiness and maintainability reruns.

## Existing seam and proposed authority shape

`runGenericLLMCheck` already supplies the embedded canonical bytes to
`model.ChatStructuredJSON`; after the provider returns an object, it performs
the unchanged `baton.ValidateSchema("llm-check-report-v1", ...)` validation and
the S04 requested/emitted identity check. Those calls remain in place and their
inputs remain byte-identical.

Today `strictProjection` is shared by native `OAI.ChatStructured` and
`OpenAIResponses.ChatStructured`. It seals object nodes but deliberately
preserves combinators, including the canonical report's top-level `allOf`/`if`/
`then`/`not` clauses. That is the configured OpenAI endpoint's pre-emission
failure. S21 adds a narrow branch before that projection; it does not teach the
generic projection to reinterpret or delete canonical semantics.

### 1. Closed-world compiler

`internal/model/llmcheck_envelope.go` will own one internal compiler/profile
selection operation. It will inspect only a decoded `$id` and the SHA-256 of
the original source bytes. It will not select from title, schema name, JSON-map
shape, endpoint URL, concrete Go type, or a merely similar digest.

For the one exact identity it will return static, deterministic bytes named
`llm-check-report-v1-openai-envelope`. The root will be a sealed object with
required `check`, `verdict`, and `findings`; `check` retains the canonical enum
and `verdict` retains `PASS|FAIL`. `findings.items` will be a sealed object with
required canonical vocabulary `id`, `severity`, `blocking`, `title`, and
`detail`, including the canonical severity enum and field primitives. The
envelope deliberately omits optional `evidence` and every other optional
canonical report field rather than inventing, nulling, or synthesizing them.
Neither its root nor any descendant may contain `allOf`, `if`, `then`, `else`,
or `not`.

The classifier has three deterministic local reject classes for an explicitly
profiled OpenAI response-format path, before either `postChat` or
`postResponses` can create an HTTP request:

1. the canonical generic-report `$id` with any digest other than the pin;
2. another recognised generic-report-family `$id`; and
3. the exact `spec-ambiguity-report-v1` `$id`, which remains C-02's dedicated
   map-report authority.

The errors will be stable, distinguish the dedicated ambiguity rejection from
the generic identity/digest rejections, and expose neither source bytes nor
credentials. A schema outside those report identities stays on its pre-S21
strict-projection path; it is not silently promoted into the generic envelope.
Malformed source continues to fail locally through the existing schema parse
path. There is no raw `Verify`, unconstrained text, retry, schema mutation,
map/array reconstruction, or synthesized report fallback.

### 2. Explicit provider and structured-mode profiles

`StructuredMode` alone is insufficient: it currently labels both
`openai-completions/` and `xai/` as native response-format clients. S21 will
add an internal, default-deny structured provider profile carried at client
construction and proxy resolution. The profile and the wire mode are separate
facts:

| Route | Wire mode | Envelope profile | Result for generic report |
|---|---|---|---|
| `openai/` Responses | `text.format` | explicit OpenAI Responses | exact source compiles to C-14 envelope |
| `openai-completions/` | `response_format` | explicit OpenAI completions | exact source compiles to C-14 envelope |
| `xai/` native strict output | `response_format` | non-OpenAI/default deny | supplied schema stays on existing path |
| forced tool-call (for example `deepseek/`) | tool parameters | non-OpenAI/default deny | supplied schema stays raw tool parameters |
| unprofiled OAI-compatible client | its existing mode | default deny | supplied schema stays on existing path |

`NewClient` will set the two OpenAI profiles directly. `proxyClient` and
`ResolveLoopClient` will set the same profile from the resolved provider
prefix, so proxy routing cannot erase the distinction. In particular, proxied
`openai-completions/` retains native response-format mode while proxied
`openai/` and the one-release `openai-responses/` alias remain Responses
clients. An xAI URL that happens to resemble OpenAI remains non-eligible.

`FromEnv` will apply the already documented provider-prefix base-URL override
to `OpenAIResponses` as well as `OAI`: `SWORN_OPENAI_BASE_URL` is the direct
`openai/` fake endpoint seam, while the retained
`SWORN_OPENAI_COMPLETIONS_BASE_URL` continues to target the legacy path. This
is needed for built-binary local fakes only; it does not add a credential or
live-provider path.

### 3. Wire emission and retained semantic gate

Both native response-format emitters will call the common selection operation:

- `OAI.ChatStructured` uses its fixed envelope only for the explicit OpenAI
  completions profile, then emits it as strict `response_format.json_schema`;
- `OpenAIResponses.ChatStructured` uses the same fixed envelope only for the
  explicit OpenAI Responses profile, then emits it as strict `text.format`.

All other response-format requests retain `strictProjection`; all tool-call
requests retain the supplied canonical schema as the forced tool parameters.
The canonical source bytes passed into `ChatStructuredJSON` are never edited or
replaced. Once output returns, `normaliseStructuredContent` still only checks
for a non-empty JSON object, while the existing gate remains solely responsible
for canonical schema validation, verdict/finding consistency, and emitted-check
equality. Therefore a provider-accepted envelope can still produce a local
non-success for PASS-with-blocking, FAIL-without-blocking, missing `check`, or
a different `check`—with no repair of the model object.

## Planned surfaces and acceptance trace

| Surface | Planned responsibility | AC |
|---|---|---|
| `internal/model/llmcheck_envelope.go`, `internal/model/llmcheck_envelope_test.go` | Exact `$id`+digest classifier, fixed named envelope, recursive forbidden-keyword check, stable local reject classes. | AC-01, AC-03 |
| `internal/model/structured.go`, `internal/model/structured_test.go` | Keep generic projection intact; select the envelope only through explicit profile/mode and prove xAI/tool raw-schema retention. | AC-01, AC-04 |
| `internal/model/oai.go`, `internal/model/oai_test.go` | Carry profile through chat/completions response-format emission and test direct/proxy construction boundaries. | AC-01, AC-03, AC-04 |
| `internal/model/openai_responses.go`, `internal/model/openai_responses_test.go` | Carry profile through Responses `text.format` and test model-shaped response normalisation. | AC-01, AC-02 |
| `internal/model/provider.go`, `internal/model/config.go` | Assign profiles at direct construction and proxy resolution; make the documented Responses fake-endpoint override reachable. | AC-02, AC-04 |
| `internal/gate/llmcheck_test.go` | Prove provider-envelope acceptance does not weaken unchanged canonical validation or S04 identity enforcement. `internal/gate/llmcheck.go` is not modified. | AC-02, AC-05 |
| `cmd/sworn/llmcheck_test.go` | Build and run the real CLI through both local OpenAI wire formats, including error exits and zero-request rejection. | AC-02, AC-03, AC-05, AC-06 |

No vendored schema/prompt, `internal/gate/llmcheck.go`, S04, S20, real home,
or release/base branch path is a planned write.

## Test and reachability strategy

The implementation will start from the public affordance and use only
deterministic `httptest` endpoints and synthetic test keys:

1. `TestCompileOpenAILLMCheckEnvelopeExactIdentity` will table-test exact
   recognition, digest changes, generic-family lookalikes, and exact dedicated
   ambiguity identity. It will assert fixed envelope bytes/name, required and
   sealed objects, canonical check/finding vocabulary, and an absence walk for
   `allOf`, `if`, `then`, `else`, and `not`.
2. `TestOpenAIEnvelopeProfileRejectsUnsupportedCanonicalSchemasBeforeHTTP`
   will use a counting fake transport to prove each local reject has its stable
   error and exactly zero requests.
3. `TestOAIChatStructuredUsesLLMCheckEnvelopeOnlyForOpenAICompletions` and
   `TestOpenAIResponsesChatStructuredUsesLLMCheckEnvelope` will decode,
   respectively, a `response_format` and a `text.format` request. Each will
   assert strict `true`, the fixed name/envelope, unchanged message content,
   and a real model-shaped `check: "ac-satisfaction"` object.
4. `TestXAIChatStructuredRetainsRawSchemaProfile`,
   `TestToolCallChatStructuredRetainsRawSchema`, and
   `TestStructuredProviderProfileDefaultsClosed` will supply a schema that
   makes envelope substitution observable and prove xAI, forced-tool, and
   manually/unprofiled constructed clients retain the existing supplied-schema
   path.
5. `TestOpenAIEnvelopeStillFailsFullCanonicalReportViolations` will return
   PASS-with-blocking, FAIL-without-blocking, missing-check, and mismatched-
   check objects through the real generic gate. The unchanged canonical
   validator and S04 equality check must make each non-success without field
   repair.
6. `TestLLMCheckOpenAIResponsesStructuredEnvelopeBinaryReachability` and
   `TestLLMCheckOpenAICompletionsStructuredEnvelopeBinaryReachability` will
   build `sworn`, run `llm-check --type ac-satisfaction` against disposable
   release fixtures and local fake endpoints, inspect the actual outbound
   Responses/completions envelope, and observe exit 0 only for the canonical
   model-emitted PASS object. A companion built-CLI unsupported-schema case
   will prove deterministic non-zero exit and zero endpoint calls.

Required completion commands remain the slice-recorded focused model/gate/CLI
tests, `go test ./...`, `go vet ./...`, and `make build`. No live OpenAI call,
credentialed smoke, real-home access, or model spend occurs at this design
checkpoint or belongs in S21's deterministic transport evidence.

## Captain review focus

- Confirm the closed-world classifier's report-family boundary: only the exact
  generic source may compile; exact ambiguity and nearby generic identities
  fail locally, while unrelated structured schemas retain their existing path.
- Confirm provider identity is construction/proxy data rather than inferred
  from a base URL or concrete OAI type, preventing xAI/tool/unprofiled leakage.
- Confirm S04's post-emission semantic authority remains intact and S20's live
  smoke remains a later, separately evidenced lifecycle action.

## Deliberate non-delivery

- No product implementation, proof bundle, verification verdict, or S20
  unblock action at this checkpoint.
- No canonical schema or prompt mutation, generic semantic-gate change,
  fallback dispatch, synthetic report, or credentialed call.
- No modification of S04/S20 artefacts, vendored protocol bytes, real homes,
  `main`, or `release-wt`.
