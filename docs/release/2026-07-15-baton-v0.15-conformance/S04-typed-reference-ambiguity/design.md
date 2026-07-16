# Design TL;DR — S04-typed-reference-ambiguity

Date: 2026-07-17T05:42:04+10:00
State: `design_review`
Contract owner: C-02 typed-reference resolution and the generic requested/emitted check-identity boundary.

## Outcome and boundary

S04 will make `spec-ambiguity` a dedicated, stateless planning check over only
the reviewed `spec.references` inputs. It will keep its report distinct from
the generic `llm-check-report-v1` shape, fail before any model call on unsafe
reference-resolution failures, and preserve the exact safe-UNRESOLVED payload
contract for the ambiguity model.

In the same gate authority, every non-ambiguity generic check will bind the
model-emitted `check` identity to the requested check. A missing, unknown, or
mismatched identity is a blocking contract failure even when the model says
`PASS`. The retired generic `maintainability-review` spelling will stop before
model/configuration/diff dispatch and direct callers to `sworn maintainability
review`; S13 remains the owner of that dedicated command and lifecycle.

S20's schema-valid canned responses are not changed. Its blocked
`ac-satisfaction` evidence is the motivating consumer of this correction, not
permission to alter S20 or synthesize a workaround. S20 may resume only after
a fresh S04 verifier PASS and its own later readiness/evidence rerun.

## Proposed authority shape

### 1. Typed references in `internal/spec`

`internal/spec/spec.go` will retain the exact typed `references` records from
`spec-v1`; `internal/spec/references.go` will own the one resolver used by the
ambiguity check. It will return either:

- a deterministic collection of resolved repo-relative artifacts and safe
  unresolved entries; or
- one classified pre-dispatch failure, with no model interaction.

The resolver will follow C-02's order exactly:

1. duplicate-key-safe parse and `spec-v1` validation of the reviewed
   `spec.json`;
2. physical workspace-root discovery from the repository containing that
   reviewed spec;
3. physical reviewed-spec confinement plus exact canonical
   `docs/release/<release>/<slice>/spec.json` source-path agreement, followed
   by matching `release` and `slice_id` validation;
4. slash-only lexical-path validation; and
5. physical target confinement beneath the physical workspace root.

Those five unsafe classes stop before dispatch in the prescribed precedence:
`reviewed-spec-schema-invalid`, `workspace-root-unavailable`,
`reviewed-spec-source-path-mismatch`, `reference-path-invalid`, then
`reference-path-escape`.

Only after those checks will the resolver examine reference content. `file`
references are raw regular UTF-8 workspace files; `contract` resolves only
`docs/release/<release>/contracts.json`; and `slice` resolves only the sibling
`docs/release/<release>/<slice>/spec.json`. Generated contract and slice files
receive duplicate-safe JSON parsing, their embedded schema validation, loaded
release comparison, then exact contract-count or slice-ID comparison. Content
failures become the exact safe `UNRESOLVED <key>: <reason>` vocabulary in its
specified per-reference order. They are never silently skipped, recursively
discovered, network-fetched, or reflected as file bytes in diagnostics.

The renderer will emit each unique resolved physical target once as
`--- ARTIFACT <repo-relative-path> ---` plus verbatim UTF-8 bytes, bytewise
sorted by path. It will then emit unresolved entries bytewise by their stable
`contract:`, `slice:`, or `file:` key. This yields O(reference count plus
referenced bytes) work and gives the model no unlisted artifact.

### 2. Dedicated ambiguity report path in `internal/gate`

`spec-ambiguity` will no longer enter the generic `RunLLMCheck` parser. A
single gate-level dispatcher will select a dedicated ambiguity authority that:

- obtains the vendored `spec-ambiguity` system prompt verbatim;
- appends the C-02 renderer's `--- REFERENCED ARTIFACTS ---` block to the
  common user payload only after pre-dispatch resolution succeeds;
- validates the raw model object only against
  `spec-ambiguity-report-v1`, never against `llm-check-report-v1`;
- rejects duplicate raw JSON keys and any fingerprint occurring in both
  `blocking_findings` and `advisory_findings`; and
- derives `FAIL` exactly from a non-empty `blocking_findings` map rather than
  trusting a contradictory stated verdict.

The dedicated result retains fingerprint-keyed maps through its JSON and
plain-text renderers; it is not flattened into generic findings. Schema errors,
cross-schema output, duplicate keys, overlapping fingerprints, malformed
output, and a contradictory verdict remain non-success and preserve the
existing fail-closed exit behavior.

### 3. Generic identity binding and retired dispatch

The generic branch remains the authority for the five
`llm-check-report-v1` check identities. Its raw response representation will
retain the emitted `check` field through schema validation and compare it
exactly with the requested identity before a PASS can be reported. The generic
schema already rejects missing/unknown identities; the new requested/emitted
comparison closes the valid-but-wrong-label case. Any failure produces the
existing blocking contract-result shape and therefore a non-zero exit, without
weakening severity/blocking or verdict derivation.

`maintainability-review` remains a recognized spelling only so it can receive
the dedicated guidance. Gate, CLI, and MCP entry points will reject it before
prompt loading, model setup, git diff, model call, record write, or other
mutation. This keeps generic dispatch at zero and leaves the future dedicated
S13 command as the only maintainability route.

CLI and MCP will call the same gate dispatcher/result renderers. The CLI owns
flag parsing, diff acquisition for dispatchable checks, stable text/JSON output,
and exit mapping; MCP owns transport parameter parsing and serializes the same
result. Neither adapter will independently resolve typed references, parse a
report, or infer a verdict.

## Exact prompt/schema boundary — Captain decision required

The v0.15.1-vendored generic prompt files currently show only `verdict` and
`findings`, whereas the exact vendored generic schema requires `check`.
Changing those vendored prompt bytes would violate S20's C-01 byte-parity
boundary; omitting the identity requirement would violate S04 AC-04.

**Single review pin:** choose one authorized route before code is written:

1. allow S04 to add a clearly separated, non-vendored runtime output-contract
   instruction inside the existing gate/CLI authority that requires one
   `llm-check-report-v1` object with `check` exactly equal to the requested
   generic check; or
2. decline that local overlay and return the release to planning for an explicit
   planned-file/scope amendment that names the authoritative source of the
   prompt correction.

Until this is decided, S04 does not modify vendored prompt bytes, does not
accept a report without the emitted identity, and does not route S20 around the
failure.

## Planned surfaces and acceptance trace

| Surface | Planned responsibility | AC |
|---|---|---|
| `internal/spec/references.go`, `internal/spec/references_test.go`, `internal/spec/spec.go`, `internal/spec/spec_test.go` | Typed-reference carrier, duplicate-safe resolver, exact hard/safe failure ordering, deterministic artifact rendering. | AC-01, AC-02 |
| `internal/gate/spec_ambiguity.go`, `internal/gate/spec_ambiguity_test.go` | Dedicated prompt payload, schema/parser, map-preserving result/rendering, no-model-call and cross-schema matrices. | AC-01, AC-02, AC-03 |
| `internal/gate/llmcheck.go`, `internal/gate/llmcheck_test.go`, `internal/gate/llmcheck_blocking_test.go` | Shared dispatcher, generic requested/emitted identity comparison, contract failure preservation, retired gate rejection. | AC-03, AC-04, AC-05 |
| `cmd/sworn/llmcheck.go`, `cmd/sworn/llmcheck_test.go` | Thin CLI dispatch, built-binary typed-reference reachability, exit-equivalence and retired guidance. | AC-01, AC-02, AC-04, AC-05 |
| `internal/mcp/lint.go`, `internal/mcp/lint_test.go` | Thin MCP dispatch and retired generic-maintainability no-dispatch reachability. | AC-05 |

## Test and proof strategy

- Start at the owning public surface:
  `TestSpecAmbiguityTypedReferencesBinaryReachability` will build `sworn`, run
  it in a disposable physical Git workspace against a local deterministic
  model endpoint, and assert a canonical file/sibling-slice/contract payload,
  dedicated PASS schema, and exit 0. It will prove unreferenced canaries are
  not supplied.
- `TestReferenceResolutionFailureMatrixBeforeDispatch` will mutation-test the
  five unsafe classes in their fixed precedence, prove zero verifier calls, and
  exercise the safe missing/non-regular/unreadable/UTF-8/JSON/schema/release/
  contract-count/slice-ID cases as exact ordered `UNRESOLVED` output.
- `TestAmbiguityCheckRendersSafeUnresolvedReferenceAndSkipsUnsafe` and
  `TestDedicatedAmbiguityReportContractFailureMatrix` will cover map order,
  deduplication, duplicate raw keys, generic-schema impostors, overlapping
  fingerprints, and inconsistent verdicts.
- `TestGenericReportCanonicalCheckIdentity` and
  `TestCheckIdentityMismatchFailsClosed` will table-test each generic identity
  plus missing, unknown, and wrong-but-schema-valid `check` values; every
  claimed PASS must produce a non-zero/violating result on mismatch.
- CLI and MCP tests will prove retired `maintainability-review` returns the
  dedicated-command guidance before configuration/model/diff work, invokes a
  counting fake zero times, and leaves the fixture tree unchanged. The MCP test
  exercises the registered `sworn.llm_check` handler, not a copied policy
  helper.

Implementation completion will run the slice's focused package suite, full
`go test ./...`, `go vet ./...`, and `make build`; proof evidence will pair each
result's PASS/FAIL with its observed public exit code. No implementation or
proof bundle is produced at this design checkpoint.

## Deliberate non-delivery

- No maintainability lifecycle, report ledger, semantic scope, commit, push,
  migration, or adjudication behavior; those remain S07–S13.
- No typed-reference discovery from prose or recursive artifacts.
- No vendored Baton prompt/schema byte change pending the Captain decision.
- No S20 code, proof, state, or unblock action. Fresh S04 verification is the
  only prerequisite release transition that can unblock S20.
