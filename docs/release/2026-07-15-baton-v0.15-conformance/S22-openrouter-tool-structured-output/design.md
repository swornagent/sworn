# Design TL;DR — S22-openrouter-tool-structured-output

Date: 2026-07-17T10:45:08+10:00
State: `design_review`
Contract boundary: C-02 and C-15. S04's canonical generic-report validation
and requested/emitted-check equality remain the sole semantic authority.

## Outcome and fixed boundary

S22 will let a release operator run a generic structured `sworn llm-check`
through the Coach-selected direct model `openrouter/z-ai/glm-5.2`. It adds one
narrow OpenRouter chat-completions transport: a single forced
`emit_structured_output` function whose nested `parameters` are the canonical
schema supplied to `model.ChatStructuredJSON`.

This is not an OpenAI strict-output route. It will not use `response_format`,
the S21 envelope compiler, `strictProjection`, a source-schema rewrite, raw
`Verify`, unconstrained text, repair, retry, or a synthetic report. The exact
canonical report remains the provider input; once the tool arguments return,
the existing generic gate continues to apply full
`llm-check-report-v1` validation and S04's requested/emitted `check` equality.

The authority boundary stays default-deny:

| Route | Structured result |
|---|---|
| Direct `openrouter/<model>` | Forced-tool route only, with the direct OpenRouter response guard. |
| Sworn-proxy `openrouter/<model>` | Structured output unsupported. |
| Ollama, unprofiled OAI-compatible clients, and unknown providers | Existing unsupported/default behavior. |
| S21 OpenAI Responses/completions routes | Unchanged envelope behavior. |

`parseModelID` already splits only at the first slash, so direct construction
will continue to send `z-ai/glm-5.2` as the model value. The Coach-selected
model is release-proof configuration only: no default model, pricing,
catalogue, hosted-service, or customer-key policy changes are planned.

## Existing seam and proposed route design

`gate.runGenericLLMCheck` already sends the embedded canonical schema through
`model.ChatStructuredJSON`. After model emission it performs the existing
canonical schema validation, finding/verdict consistency checks, and S04
identity check. `internal/gate/llmcheck.go` will not be changed.

`OAI.ChatStructured` already owns the OpenAI-compatible tool wire shape: one
nested function tool and a forced named `tool_choice`. Today its shared
tool-call path accepts the first call whenever any call exists. S22 will make
the direct OpenRouter construction carry an explicit, internal tool-response
policy, separate from `StructuredMode` and separate from endpoint URL or Go
concrete type. That policy prevents a broad behavior change to the existing
DeepSeek forced-tool route while giving direct OpenRouter the tighter contract.

### 1. Direct-only provider selection

The construction mapping will distinguish direct and proxy routes rather than
adding `openrouter` to a mapping shared by `NewClient` and `proxyClient`.

- Direct `NewClient("openrouter/...", ProviderConfig)` will construct `OAI`
  with `StructuredToolCall` and the direct-OpenRouter exact-tool policy.
- `proxyClient("openrouter", ...)` will leave the route unstructured. Because
  `FromEnv` resolves `ProxyRoute` before direct construction,
  `ResolveLoopClient` inherits the same split.
- The selection will be provider-prefix data established at construction, not
  inferred from a base URL, an OpenRouter-looking host, or a manual `OAI`.

This preserves direct OpenRouter's existing base URL
`https://openrouter.ai/api/v1`, its full model subpath, and ordinary unstructured
`Verify`/`Chat` behavior while advertising structured capability only for the
explicit direct route.

### 2. Forced canonical function and fail-closed extraction

For the selected direct route, `OAI.ChatStructured` will retain the existing
chat/completions tool encoding but make its invariants explicit:

1. send exactly one `tools` entry with nested function name
   `emit_structured_output`;
2. place the supplied canonical schema directly in that function's
   `parameters` field and force the same function through `tool_choice`;
3. omit `response_format` and every S21/envelope/projection path; and
4. after the one HTTP response, accept output only when there is exactly one
   tool call, its function name is `emit_structured_output`, and its arguments
   are one JSON object.

The extractor will reject zero, multiple, wrong-name, malformed, scalar, or
array arguments locally. It will make no second request, text fallback,
argument repair, or schema transformation. A valid object continues through
the existing structured-content normalisation and then unchanged into the
generic gate; provider tool syntax is not semantic validation.

### 3. Direct-only fake-endpoint configuration seam

`FromEnv` will recognize `SWORN_OPENROUTER_BASE_URL` only after proxy routing
has declined and direct OpenRouter construction succeeds. The override will be
validated as an absolute HTTP(S) URL with a host before it is assigned to the
direct `OAI` client. When unset, the direct client retains
`https://openrouter.ai/api/v1`.

An invalid override will fail setup before dispatch. A proxy-routed OpenRouter
model will neither consume the override nor change its proxy endpoint; another
provider will likewise retain its own endpoint. This creates a deterministic
`httptest` seam for the built binary without credentialed provider traffic.

## Planned surfaces and acceptance trace

| Surface | Planned responsibility | AC |
|---|---|---|
| `internal/model/llmcheck_envelope.go`, `internal/model/llmcheck_envelope_test.go` | Extend construction-time structured-route metadata with an explicit direct-OpenRouter tool policy while preserving S21's envelope eligibility and default-deny profiles. | AC-01, AC-02 |
| `internal/model/provider.go`, `internal/model/provider_test.go` | Construct direct OpenRouter with `StructuredToolCall`, preserve `z-ai/glm-5.2`, and prove proxy/unknown/Ollama capability boundaries remain closed. | AC-01 |
| `internal/model/oai.go`, `internal/model/structured_test.go` | Emit the nested canonical forced tool and enforce exact direct-OpenRouter tool-call cardinality, name, and object arguments without fallback or retry. | AC-02, AC-04 |
| `internal/model/config.go`, `internal/model/oai_test.go` | Apply and validate `SWORN_OPENROUTER_BASE_URL` only for direct OpenRouter after proxy precedence; prove unset, invalid, proxy, and other-provider cases. | AC-01, AC-05 |
| `internal/gate/llmcheck_test.go` | Prove a valid tool object still passes only through unchanged canonical validation and that invalid report semantics or emitted-check mismatch remain non-success. | AC-03, AC-04 |
| `cmd/sworn/llmcheck_test.go` | Build and run the public `llm-check` command through the direct OpenRouter fake endpoint, inspect its exact tool wire shape, and prove reject exits/no repair. | AC-03, AC-04, AC-05 |

No change is planned to a vendored schema or prompt, S04, S21 implementation,
`internal/gate/llmcheck.go`, defaults, real credential homes, or S20 artefacts.

## Test and reachability strategy

Implementation will begin at the public affordance and use only local fakes,
synthetic keys, and a scrubbed child environment:

1. Factory/profile tests will assert that direct `openrouter/z-ai/glm-5.2`
   preserves the full subpath, advertises only `StructuredToolCall`, and that
   proxy OpenRouter, Ollama, and unprofiled OAI clients do not advertise it.
2. Structured transport tests will decode the outgoing request and assert one
   nested `emit_structured_output` function, forced `tool_choice`, canonical
   parameters (not the S21 envelope), and absence of `response_format`. A
   counting fake will exercise zero/wrong/multiple calls and non-object
   arguments, asserting one request total and a local error.
3. Gate tests will feed a tool-shaped canonical object through the real generic
   gate. `PASS` with a blocking finding, `FAIL` without a blocking finding,
   missing `check`, invalid canonical content, and a check other than
   `ac-satisfaction` must remain non-success without field synthesis.
4. `TestLLMCheckOpenRouterToolStructuredBinaryReachability` will build `sworn`
   and invoke `llm-check --type ac-satisfaction --model
   openrouter/z-ai/glm-5.2` against an `httptest` chat-completions endpoint.
   Its child environment will not inherit `os.Environ()`; it will set a
   temporary HOME/XDG configuration, `SWORN_DIRECT=1`, a synthetic
   `OPENROUTER_API_KEY`, `SWORN_OPENROUTER_BASE_URL`, and dead external proxy
   values. The valid canonical response must print PASS and exit 0.
5. Companion built-command cases will prove malformed tool responses return a
   non-success exit, do not make a second request, and cannot escape to a proxy
   or provider endpoint. Focused model/gate/CLI tests, `go test ./...`,
   `go vet ./...`, and `make build` remain the required deterministic evidence.

Only after that deterministic evidence and an immutable S22 implementation
start commit exist will the implementer run the single direct credentialed
`spec-ambiguity` command required by AC-06. Its proof will retain only check
identity, model ID, exit/result, and any non-secret planning finding. It is not
run at this design checkpoint. A finding returns to Planner rework; S20 remains
blocked until a fresh S22 verifier PASS, and its later smoke is independently
owned by S20.

## Captain review focus

- Confirm the direct/proxy split is construction-time authority, so a proxy
  OpenRouter route cannot accidentally inherit direct structured capability.
- Confirm the direct-only exact-tool policy does not weaken canonical schema
  validation or silently alter another provider's forced-tool semantics.
- Confirm the fake base-URL override is direct-only, validates before dispatch,
  and cannot redirect a proxy or another provider.
- Confirm the credentialed ambiguity check and all S20 activity remain later,
  separately evidenced lifecycle actions.

## Deliberate non-delivery

- No production implementation, tests, build, proof bundle, verification
  verdict, provider request, or credentialed model call at this checkpoint.
- No schema/prompt mutation, OpenAI-envelope reuse, raw-text fallback, retry,
  default-model change, proxy enablement, S20/S21 change, or real-home access.
- No write to `main` or `release-wt`; this checkpoint belongs only on the T1
  track branch.
