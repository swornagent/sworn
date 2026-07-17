# Captain review — S04-typed-reference-ambiguity

Date: 2026-07-17
Design commit: 913de419fbfe314d2c1c539b82cedea2579236db

## Pins

1. [mechanical] Exact prompt/schema boundary — use the existing out-of-band schema-constrained output channel, not a text overlay.
   What I observed: the exact v0.15.1 ac-satisfaction and design-review prompt bodies request only verdict and findings, while llm-check-report-v1 requires check. The live current-head design-review invocation failed closed on that exact missing field. The tagged LLM-check README requires the prompt body and common user payload verbatim; existing model.ChatStructuredJSON accepts those unchanged two messages with a separately supplied schema.
   What to ask the implementer: keep prompt.LLMCheck and the common user payload byte-for-byte intact; use the unmodified embedded llm-check-report-v1 bytes as a separate schema-constrained output envelope for every dispatchable generic check, with no raw-Verify fallback. Retain the raw emitted check through duplicate-safe parsing and schema validation, compare it exactly with the requested check, and fail closed on missing, unknown, duplicate, or mismatched identity. Never synthesize check from the request. Unsupported structured output is a non-zero configuration failure. This is faithful, safe, and wholly within S04's existing gate/CLI/test surfaces; no replan is required.

2. [mechanical] Generic identity binding needs public CLI and MCP reachability, not only a gate table test.
   What I observed: the planned built-binary reachability artefact covers only spec-ambiguity, while the planned generic identity tests are gate-level and the MCP test named by the design covers only retired maintainability. The current CLI and registered MCP handler are separate adapter paths.
   What to ask the implementer: add deterministic public-path coverage that sends a schema-valid wrong check through sworn llm-check and observes non-zero, and through the registered sworn.llm_check handler and observes non-success. Cover missing and unknown identity as well as a wrong valid identity, and assert the raw response was not relabelled by either adapter.

3. [mechanical] Retired maintainability must short-circuit before all adapter work, with proof stronger than a zero-call fake.
   What I observed: current cmd/sworn/llmcheck.go and internal/mcp/lint.go accept maintainability-review as valid, then resolve release/model/base/diff before calling the gate. The design states the new route stops before model, configuration, or diff work, but its test plan does not yet prove that early ordering at both public adapters.
   What to ask the implementer: retain the recognized spelling solely for guidance, but guard it immediately after CLI flag/type validation and MCP parameter/type validation, as well as at gate entry. Test valid slice/release inputs with unavailable model and unusable diff/base conditions: CLI returns the dedicated maintainability command guidance with exit 64, MCP returns the same non-success guidance, the counting verifier remains at zero, and the fixture tree is unchanged.

4. [memory-cited] Typed-reference and dedicated-report authority remains the sole ambiguity contract.
   What I observed: the recorded Type-1 decision adopts typed spec.references and spec-ambiguity-report-v1 without legacy discovery. The project memory records that those were deliberately introduced as the only normative discovery surface, with deterministic confined resolution and a report separate from the five generic checks.
   What to ask the implementer: acknowledge that the resolver never scans prose, touchpoints, acceptance text, test references, or recursively discovered artefacts, and that generic report parsing is never used for spec-ambiguity.
   Citation: [[Baton spec-ambiguity protocol accepted]]

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: 1, 2, 3

## Summary

The typed-reference ordering, physical confinement, dedicated ambiguity report, and S20 handoff all conform to C-02. The explicit runtime-boundary pin is mechanically resolvable: an out-of-band schema constraint preserves exact vendor prompt bytes while requiring a model-emitted identity; a textual prompt append or synthesized identity would not.

## Smaller flags (not pins, worth one-line acknowledgement)

- S20's current-head fixture change already supplies canonical generic check fields. Treat it as the baseline, add mismatch cases under S04, and do not alter S20 state, proof, or unblock evidence.
- S03 is still planned and shares internal/spec/spec.go only as a future carrier touchpoint; no in-progress or implemented sibling collision exists today.
- The S20 unblock remains hard-gated: only a fresh S04 verifier PASS, followed by S20's own readiness and maintainability rerun, may clear it.
- The live current-head design-review LLM check exited 1 because the exact prompt elicited a report missing check. That is direct confirmation of Pin 1, not permission to weaken the schema.

## Suggested acknowledgement reply

TL;DR The C-02 resolver, dedicated ambiguity report, and S20 boundary are sound. 4 pins + 4 flags:

1. **Use a separate output contract.** Keep exact vendored prompt and user-payload bytes unchanged. Use the existing schema-constrained output channel with the exact generic schema; retain and compare the model-emitted check, with no synthesis or unconstrained fallback.
2. **Prove generic identity at adapters.** Drive wrong, missing, and unknown check identities through the public CLI and registered MCP handler, and prove each returns non-success without relabelling raw output.
3. **Short-circuit retired maintainability.** Reject the generic spelling immediately in CLI, MCP, and gate before release/model/diff work; prove guidance, exit 64 or MCP non-success, zero calls, and no fixture mutation despite unusable configuration or diff inputs.
4. **Keep ambiguity authority typed.** Do not add prose, touchpoint, test-reference, recursive, or generic-report fallback discovery.

Flags (not pins): (a) retain S20's canonical fixture fields as baseline; (b) no active S03 collision exists; (c) S20 remains blocked until fresh S04 verification plus its own rerun; (d) the live missing-check failure confirms the output-boundary test case.

The recorded Type-1 typed-reference decision and its memory citation are acknowledged. The single runtime-boundary question is resolved as the separate schema-constrained envelope described above.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The runtime output contract can use existing schema-constrained authority without prompt-byte drift; every remaining pin is bounded and mechanically verifiable in S04's planned files.
-->
