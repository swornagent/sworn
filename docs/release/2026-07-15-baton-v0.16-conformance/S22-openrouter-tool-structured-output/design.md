# S22 Design TL;DR — repeatable diagnostics and head-bound certification

## 1. User-visible change

A release operator gets two explicit native `sworn llm-check --proof-receipt`
operations for the direct OpenRouter structured-output driver:

- `--driver-diagnostic` makes exactly one call with one explicitly supplied,
  capable direct `openrouter/` model and appends one sanitized structural
  diagnostic. A later diagnostic requires another explicit human command and
  may use the same or a different direct OpenRouter model.
- `--certify-driver` makes exactly one atomic certification call for one
  implementation head and explicit model, but only after a PASS diagnostic for
  that same head/model plus the committed regression, deterministic, proof,
  Captain, and Coach authorities required by AC-12.

Historical attempts 1–3 remain immutable. The new operations are neither
typed retries nor fallback: diagnostic ordinals form a separate namespace, and
certification generation 1 begins at proof attempt 4. A failed certification
cannot run again at the same head; another generation requires a relevant
driver/test commit and renewed same-head/model evidence.

The existing direct-only forced `emit_structured_output` transport, canonical
local validation, S04 requested/emitted identity authority, proxy default-deny
boundary, endpoint isolation, stable MCP failure text, and metadata-only output
policy remain unchanged.

## 2. Command and evidence state machine

### 2.1 Fail-closed mode selection

`cmd/sworn/llmcheck.go` will parse proof operation flags into one closed mode
before config loading, model construction, filesystem mutation, or provider
setup. `--driver-diagnostic` and `--certify-driver` each require
`--proof-receipt`, are mutually exclusive with each other and the exhausted
legacy `--configured-recovery` operation, and reject incompatible or missing
identity/model flags with zero dispatch and no record mutation.

Common preflight will require the exact S22 release, slice, check and immutable
start; a clean resolvable current implementation head; exact S21 identity,
authoritative status commit, immutable start, verified/PASS verdict time and
fresh-context evidence; strict unchanged attempts 1–2 under v1 and attempt 3
under v2; a safe in-slice evidence directory; and the current fresh Captain
`PROCEED` plus Coach acknowledgement fields. All checks precede reservation.

### 2.2 Diagnostic path

Diagnostic mode additionally requires a non-empty operator `--model` whose
prefix is direct `openrouter/`, direct routing is explicitly selected, and the
constructed verifier advertises `StructuredToolCall`. It never reads a model
default or chooses a fallback. One shared evidence lock serializes ordinal
allocation and atomic reservation/finalization. Records use
`receipts/diagnostic-<ordinal>.json` and the Planner-owned
`llm-check-driver-diagnostic-v1.schema.json` unchanged.

The reservation itself is a valid fail-closed diagnostic record:
`request_setup` / `unavailable` / `receipt_failure` / `UNPARSEABLE` /
`"unavailable"`. During the one call, model and gate layers may derive only
closed `stage` and `response_shape` enums in memory. The final record contains
only the schema allowlist; provider content is discarded and cannot enter an
error, renderer, receipt, journal, proof, or test failure.

### 2.3 Certification path

Certification mode requires the same explicit direct model rules and the
latest strict diagnostic PASS bound to the exact current head, model, check,
release, slice, start, and direct transport. It also requires a committed
synthetic regression fixture for the diagnosed shape, passing targeted tests,
`go test ./...`, `go vet ./...`, `make build`, a regenerated committed proof
bundle, and the fresh review/acknowledgement authority.

Records use `receipts/certification-<generation>.json` and the Planner-owned
`llm-check-proof-receipt-v3.schema.json` unchanged. Generation `n` maps to
proof attempt `3+n`. The shared lock reserves and finalizes exactly one strict
metadata-only record. Existing generation/head bindings are append-only. After
a failed generation, preflight accepts a later generation only when the prior
head is a first-parent ancestor of the current head and at least one intervening
non-merge commit changes a declared S22 driver or test touchpoint outside the
release evidence tree; documentation-only churn cannot create authority.

## 3. Key design choices

1. **Keep three record families distinct.** Existing `ProofReceipt` v1/v2
   decoding remains the immutable history reader. New typed diagnostic-v1 and
   certification-v3 records get separate strict decoders, validators, file
   names, ordinal rules, and renderers while reusing only private atomic
   persistence primitives. This prevents diagnostic evidence from consuming or
   rewriting proof attempt history.

2. **Carry only typed structural metadata across the model boundary.** Extend
   the existing structured-output error seam with closed, payload-free stage
   and shape values. Direct OpenRouter parsing identifies tool cardinality,
   type, name and argument shape; the gate identifies canonical parse, schema,
   identity and final-verdict stages. No raw string becomes classification or
   retry authority.

3. **Bind certification to engineering progress, not invocation count.** The
   current full commit ID is part of every new record. Same-head replay is
   rejected, and a later generation requires a real non-merge source/test
   change plus renewed diagnostics and gates. This permits diagnosis and repair
   without turning certification into blind retry.

4. **Preserve customer model authority and the direct/proxy split.** Both new
   operations require the operator to name one direct OpenRouter model. Sworn
   applies capability as a floor but never selects a default, reads configured
   verifier fallback, changes routing, or retries another model.

5. **Keep the existing typed retry classifier unchanged.** The new entry
   points are administrative authority. `proofReceiptRetryable`,
   `ClassifyProofReceiptError`, legacy `IsTransient`, error text, and historical
   outcomes never authorize another diagnostic or certification call.

## 4. Intended file changes

Primary semantic edits stay within the ratified S22 touchpoints:

- `cmd/sworn/llmcheck.go` — closed mode parsing; common identity/review/S21/
  history/head preflight; exact direct model construction; diagnostic and
  certification eligibility; stable sanitized exits.
- `cmd/sworn/llmcheck_test.go` — built-binary reachability, conflicting flags,
  explicit/repeated model selection, zero-dispatch authority failures,
  same-head replay, changed-head generation, one-call bounds, and leak canaries.
- `internal/gate/llmcheck_receipt.go` — strict diagnostic/certification record
  types, append-only preflight, shared atomic lock/reservation/finalization,
  schema-aligned decoding and sanitized rendering while leaving v1/v2 history
  immutable.
- `internal/gate/llmcheck_receipt_test.go` — schema cross-assertions, ordinal/
  generation allocation, history preservation, double-fault durability,
  binding/replay rejection, and record allowlist tests.
- `internal/gate/llmcheck.go`, `internal/gate/llmcheck_test.go`, and
  `internal/gate/llmcheck_live_test.go` — sanitized diagnostic runner outcomes
  and canonical-stage classification without changing ordinary check results.
- `internal/model/errors.go`, `internal/model/oai.go`, and their declared tests
  — closed structural stage/shape metadata for the direct exact-tool route;
  typed provider classes and public error strings remain source-free.
- `internal/model/llmcheck_envelope.go`, provider/config files, MCP files, and
  their declared tests are preservation surfaces: touch them only if a named
  AC regression requires an in-scope correction. Proxy OpenRouter and every
  unprofiled route remain unsupported.

The Planner-owned diagnostic-v1 and proof-receipt-v3 schema files are consumed
unchanged and cross-validated by tests. No dependency or new package is added.
Any required path outside `spec.json.touchpoints` stops for replanning.

## 5. Acceptance trace and reachability

| AC | Planned change and evidence |
|---|---|
| AC-01 | Preserve direct `openrouter/` `StructuredToolCall`; retain proxy, Ollama and unprofiled default-deny construction tests. |
| AC-02 | Preserve the exact forced function name, unchanged canonical parameters, and absence of `response_format`/S21 envelope in wire tests. |
| AC-03 | Retain built `sworn llm-check` fake-endpoint PASS/exit-0 reachability with a synthetic key. |
| AC-04 | Retain cardinality/name/canonical/requested-check failures as one-call non-success with no repair or fallback. |
| AC-05 | Retain absolute HTTP(S) direct override validation and proxy/other-provider endpoint isolation before dispatch. |
| AC-06 | Reuse atomic reservation/finalization only beneath strict per-family records; prove mismatched bindings, write faults and double faults fail closed before or after the sole call as specified. |
| AC-07 | Preserve JSON `null` arguments rejection after exactly one request. |
| AC-08 | Preserve non-`function` tool-call rejection after exactly one request. |
| AC-09 | Validate attempts 1–3 without rewriting them; prove neither new entry point consults retry classification or makes an automatic second call. |
| AC-10 | Map only closed error/stage/shape enums into strict records; prove terminal classes, explicit later diagnostics, and changed-head-only later certification. |
| AC-11 | Retain exact CLI malformed-output and MCP provider-error strings, empty/raw-free output boundaries, and all leak canaries. |
| AC-12 | Built-command certification tests exercise complete same-head diagnostic, regression, proof, review and S21 authority; every stale/missing fact dispatches zero calls, while one eligible call writes the next v3 generation. |
| AC-13 | Built-command diagnostics prove explicit direct model authority, repeated human invocations, append-only ordinals, one call each, and zero mutation on invalid authority. |
| AC-14 | Table-driven structural-shape fixtures and CLI/gate/MCP leak canaries prove that only enums survive and every synthetic regression contains no live payload. |

The Rule-1 reachability artefact is the built `sworn llm-check` binary driven
through local deterministic OpenRouter-shaped endpoints. No provider call is
part of this design checkpoint. Any later live diagnostic or certification is
policy-governed evidence and never substitutes for deterministic reachability.

## 6. Review pins and risks

- **Pin: head definition.** Captain should confirm that the clean current full
  commit ID used by both record families is the intended `implementation_head`
  before maintainability pins its later semantic-review head.
- **Pin: proof freshness.** Captain should inspect the proposed committed-proof
  check so stale but syntactically passing `proof.json` cannot authorize a call.
- **Pin: relevant changed head.** Captain should confirm the exact declared
  source/test allowlist and first-parent non-merge rule used after a failed
  certification; release-record-only commits must not qualify.
- **Pin: atomic family separation.** Shared persistence code must not create a
  generic record type that permits cross-family fields, filenames or ordinal
  reuse.
- **Risk: diagnostic metadata becomes a covert payload channel.** All stage and
  shape values are enums; tests use secret-like canaries across files, stdout,
  stderr and MCP.
- **Risk: direct construction silently falls back to configured/proxy routing.**
  Resolve the exact explicit model once, require the direct route and capability,
  and assert the request model at the fake endpoint.

## 7. Open questions

None for the Implementer. The Type-1 diagnostic/certification split is already
Coach-ratified. A fresh Captain must review this design and the four pins above,
then the Coach must acknowledge `PROCEED` before any source edit, diagnostic, or
certification invocation.
