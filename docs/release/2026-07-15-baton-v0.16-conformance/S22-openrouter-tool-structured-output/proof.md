# S22-openrouter-tool-structured-output proof bundle

Fail-closed handoff. The implementation and every deterministic precondition
passed, but the one authorised native attempt-2 proof returned a terminal
sanitized non-result. This is not an `implemented` or verification claim.

## Scope

Complete the direct OpenRouter forced-tool path with a native, bounded,
metadata-only proof receipt. Fail closed across malformed output, retry-policy,
receipt-persistence, upstream-evidence, and MCP-public-error boundaries while
keeping `openrouter/z-ai/glm-5.2` proof-only.

## Files changed

`git diff --name-only a09b0e46df465862d00469d4aef2a997442b3d5b`
reports 176 paths because the immutable S22 start predates serialized track and
release integration. The S22 implementation/proof scope within that live diff is:

- `cmd/sworn/llmcheck.go`
- `cmd/sworn/llmcheck_test.go`
- `internal/gate/llmcheck.go`
- `internal/gate/llmcheck_live_test.go`
- `internal/gate/llmcheck_receipt.go`
- `internal/gate/llmcheck_receipt_test.go`
- `internal/gate/llmcheck_test.go`
- `internal/mcp/lint.go`
- `internal/mcp/lint_test.go`
- `internal/model/config.go`
- `internal/model/errors.go`
- `internal/model/errors_test.go`
- `internal/model/llmcheck_envelope.go`
- `internal/model/llmcheck_envelope_test.go`
- `internal/model/oai.go`
- `internal/model/oai_test.go`
- `internal/model/provider.go`
- `internal/model/provider_test.go`
- `internal/model/structured_test.go`
- `docs/release/2026-07-15-baton-v0.16-conformance/S22-openrouter-tool-structured-output/`

## Test results

- PASS — targeted S22 gate/model/MCP/CLI suite, exit 0.
- PASS — `GOFLAGS=-buildvcs=false go test ./...`, exit 0.
- PASS — `GOFLAGS=-buildvcs=false go vet ./...`, exit 0.
- PASS — `GOFLAGS=-buildvcs=false make build`, exit 0.
- PASS — built-command `TestLLMCheckOpenRouterToolStructuredBinaryReachability`,
  exit 0.
- PASS — built-command `TestLLMCheckProofReceiptBinaryReachability`, exit 0.
- PASS — current binary deterministic proof-bundle gate with the immutable-start
  diff on stdin, exit 0, verdict PASS, cost USD 0, and an unroutable synthetic
  endpoint proving no provider dispatch.
- FAIL CLOSED — the sole native attempt-2 direct `spec-ambiguity` proof, exit 2,
  class `opaque`, result `UNPARSEABLE`; the strict receipt contains no raw data.
- EXPECTED UNAVAILABLE — `sworn coverage`; the current binary returns unknown
  command, tracked by `sworn#122` and previously acknowledged by the Coach.

`GOFLAGS=-buildvcs=false` suppresses only host VCS stamping and does not change
source, test selection, model routing, or provider behavior.

## Reachability artefact

`TestLLMCheckOpenRouterToolStructuredBinaryReachability` builds and drives
`sworn llm-check` through direct OpenRouter configuration to a deterministic
local endpoint, asserts the exact forced-tool request, accepts only the local
canonical report, prints PASS, and exits 0.

`TestLLMCheckProofReceiptBinaryReachability` builds and drives the native S22
proof-receipt entry point, proves exact v0.16/S21 binding and two-attempt policy,
and verifies that only the strict metadata receipt is rendered.

## Delivered

- AC-01–AC-05: direct-only OpenRouter capability, exact forced-tool transport,
  built-command reachability, malformed/canonical fail-closed behavior, and
  direct-base isolation remain covered by their named spec tests.
- AC-06: private atomic reservation/finalization now uses a durable trust guard;
  a post-rename plus failed-restoration double fault cannot leave a trusted
  model verdict. Binding mismatches and preflight write faults dispatch zero.
- AC-07–AC-08: JSON null arguments and non-function tool-call types fail locally
  after exactly one synthetic request with no repair or fallback.
- AC-09–AC-10: the narrow typed classifier alone controls retry eligibility;
  final, terminal, opaque, contract, and binding failures never retry, and a
  second recorded attempt never permits a third dispatch.
- AC-11: receipts and CLI output remain metadata-only; the registered MCP tool
  returns exactly `llm_check: provider request failed` without provider-derived
  text. Leak-canary reachability tests pass.
- AC-12 deterministic preconditions: exact current S21 verified/PASS identity,
  immutable start, historical authoritative status ref, non-empty verdict time,
  and fresh-context bit are mechanically bound; Captain review `798e114c` is
  acknowledged; targeted/full tests, vet, build, reachability, and this
  regenerated proof bundle's deterministic PASS are complete.
- Captain pin 4: the receipt cohesion audit is recorded in `journal.md`; keeping
  the private seams together avoids an intermediate unguarded-verdict contract.
- Captain pins 5–6: typed classifier scope is unchanged and GLM remains
  proof-only, with no default, catalogue, proxy, or routing-policy expansion.

## Not delivered

- AC-12 did not obtain a valid PASS receipt. Sanitized evidence: release
  `2026-07-15-baton-v0.16-conformance`, slice
  `S22-openrouter-tool-structured-output`, check `spec-ambiguity`, model
  `openrouter/z-ai/glm-5.2`, immutable start
  `a09b0e46df465862d00469d4aef2a997442b3d5b`, attempt 2, class `opaque`,
  result `UNPARSEABLE`, exit 2. Why: the provider outcome did not pass the local
  canonical gate. Tracking: S22 AC-12. Acknowledgement: the Coach authorised no
  fallback or third dispatch; Planner re-scope is required.

## Sanitized attempt-2 receipt

The durable receipt is `receipts/attempt-2.json`. It contains only the ten
allowlisted metadata fields. The two-attempt budget is exhausted and S20
remains untouched.

## Divergence from plan

The acknowledged design anticipated PASS as the only route to completion. The
terminal attempt-2 opaque outcome therefore stops the lifecycle at `blocked`.
The earlier v0.15 identity remains historical provenance only.
