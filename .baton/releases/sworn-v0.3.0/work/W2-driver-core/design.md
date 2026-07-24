# Approach

Bind this revision to admitted plan
`sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`,
approval
`sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`,
and the clean current T2 authority head
`d1f2bb62e011a0e14ccee4a73cd2f91115346fc6`. W2 changes only
`internal/driver`. It establishes one role-neutral process, submission, and
receipt boundary for W3 and W5; it does not advance Baton state or implement a
provider.

## Exact Baton process contract

1. Implement closed Go values and strict codecs for `baton.driver/v1`,
   `baton.driver-request/v1`, and `baton.driver-result/v1`. The wire request has
   exactly `schema_version`, `invocation_id`, `role`, `operation`, `model`,
   `workspace`, `inputs`, `fresh_context`, and `limits`; the result has exactly
   the RC2 required fields and optional `usage`. Unknown, duplicate,
   case-mismatched, missing, non-finite, out-of-range, invalid-UTF-8, or trailing
   values fail before use.
2. Load each canonical operation from the W0-admitted `internal/baton` package.
   The request operation ID, version, raw UTF-8/LF instructions, final newline,
   and SHA-256 must all equal those installed bytes. A self-consistent
   caller-supplied replacement is stale. The fixed role mapping is:
   `planner` to `baton-plan`, `implementer` to `baton-implement`, `captain` to
   `baton-design-review`, `verifier` to `baton-verify`, and `merge` to
   `baton-merge`.
3. Preserve Baton's limits: at most 1,048,576 request bytes, 262,144 operation
   instruction bytes, 256 ordered uniquely named and uniquely pathed inputs,
   timeout from 1 through 86,400,000 milliseconds, request
   `limits.output_bytes` from 1 through 1,048,576, and result `text` from 0
   through 1,048,576 bytes. An empty result string is valid; the positive
   minimum applies only to the request limit. IDs, semantic versions, digests,
   canonical repository-relative input paths, canonical absolute workspace
   paths, and JSON integers receive closed validators matching the RC2
   reference.
4. Keep the portable codec wider than Sworn production dispatch. It accepts all
   five role values and an explicit non-empty model or deliberate JSON `null`,
   as Baton requires. A direct conformance invocation exercises `merge` with
   `model:null`. No production resolver or dispatcher accepts `merge`.

## Explicit selection above one common driver

Define one `Driver`/`Invoker` path with no role methods. A closed
`RoleSelections` value has exactly four required entries: Planner,
Implementer, Captain, and Verifier. Each entry names one registered provider
configuration and one non-empty bounded model. Resolution must return that
single pair or an error; it never supplies a default, retries another provider,
rotates, falls back, or derives a model from the role. The returned result must
bind the expected driver ID/version, invocation ID, and configured model
exactly, including `observed_model`.

Provider configuration in W2 contains only public bounded values: a stable
provider key, the expected driver identity, a pinned executable identity,
explicit network policy, and an optional opaque credential-reference ID.
There is no free-form argv or environment map and no API-key, token, secret,
credential bytes, pricing, or endpoint fallback field. Secret resolution and
provider-specific injection belong to W5. W2 clears the inherited environment
and does not serialize the credential reference into the Baton request,
submission descriptor, result receipt, diagnostic, or error. The deterministic
fake is the sole configured driver supplied by W2.

## One bounded invocation boundary

`Invoker.Invoke(ctx, invocation)` performs exactly one attempt. It does not
schedule, retry, reconcile, interpret Baton state, or own lifecycle.

- The caller supplies a disposable workspace, exact ordered input bytes and
  digests, the selected provider/model, a submission permission, and the RC2
  limits. W2 never opens `.baton/releases`, a journal, resolver, canonical
  repository, release worktree, or engine log on the child's behalf.
- Every production invocation uses a new child and private temporary root. The
  only repository-bearing mount is the supplied disposable workspace. The only
  engine-authored data mount is a staged, read-only
  `<workspace>/.sworn-inputs/v1/` projection. Every request input path used by
  Sworn production dispatch must be beneath that prefix. Portable codec tests
  remain able to validate Baton's generic repository-relative fixture paths.
- Before start, W2 uses only filesystem and mount observations to reject an
  existing `.sworn-inputs` entry or mount conflict, a symlink or non-regular
  staged input, a duplicate or reordered input, a path escape, and any
  byte-count or digest mismatch. It stages at most 256 files, 1,048,576 bytes
  per file, and 8,388,608 bytes in aggregate in a private directory, then mounts
  that directory read-only into the disposable workspace view. W2 does not run
  Git, inspect tracked/untracked/ignored entries, establish product-only
  workspace provenance, or decide whether the reserved prefix is eligible for
  candidate capture. W1/W3 own those Git/product facts and reserved-prefix
  capture exclusion before and after invocation.
- On Linux, a small Bubblewrap launch profile creates the mount/PID namespace,
  an empty home and temporary directory, a fixed locale/timezone/PATH, no
  inherited environment, and no network for the fake. A read-only invocation
  mounts the complete disposable workspace read-only; a writable invocation
  mounts only that private workspace read-write while the input projection
  stays read-only. Unsupported hosts fail before child start; the pure contract
  and portable fake-codec cases remain host independent.
- Context cancellation or deadline closes the submission endpoint, signals the
  whole child process group, escalates to kill after a fixed short grace
  interval, waits for every descendant, and returns only an operational
  observation. A cancelled or timed-out process cannot manufacture a result or
  submission.
- Stdin is bounded by the RC2 request ceiling. Stdout is drained concurrently
  into an 8 MiB hard ceiling, sufficient for the bounded JSON result including
  escaping; overflow kills the process and is a protocol failure. Stderr is
  continuously drained but retains at most 1,024 bytes in memory. Raw stderr is
  discarded after deriving a fixed sanitized diagnostic code, byte count, and
  truncation flag. Exit zero requires exactly one strict bound result; non-zero
  requires empty stdout. Missing, multiple, malformed, extra, or mismatched
  output is operational failure.

The wire result text exists only long enough to enforce its byte limit and
validate the result. The production observation returned to W3 contains its
byte count and digest, not its bytes. Raw request, stdout, stderr, model text,
prompt, tool payload, and credential material are neither retained nor
returned. Strictly sealed Baton handoff bytes are intentional in-band
artifacts, not a transcript.

## One invocation-bound sealed submission

Use one Sworn side channel rather than adding fields to the exact Baton result.
Each child receives one full-duplex inherited endpoint and a fixed protocol
identifier. The common `SubmissionClient` exposes only `Describe` and
`Submit`; the fake and all future native or HTTP adapters must converge on the
same server and sealer.

Frames are a four-byte big-endian length followed by canonical compact UTF-8
JSON plus one LF. A frame is at most 4,194,304 bytes and a submission at most
2,097,152 bytes. Zero/oversized lengths, partial frames, invalid UTF-8,
noncanonical JSON, duplicate/unknown fields, trailing bytes, or a wrong
invocation close that endpoint without a seal.

The closed `sworn.submission/v1` value contains only:

- the `schema_version` and exact `invocation_id`;
- an ordered array of zero to two artifacts, each with closed kind, decoded
  byte count, SHA-256, and canonical padded base64 bytes; and
- either `decision:null` or one closed decision outcome with 1 to 262,144 bytes
  of digest-bound review evidence.

Plan bytes are limited to 1,048,576 bytes; design, work proof, work status, and
assembly status bytes are each limited to 262,144 bytes; decoded artifacts are
limited to 1,048,576 aggregate bytes. There are no refs, OIDs, paths, Git
commands, parents, commit messages, merge modes, arbitrary actions, or provider
data in this envelope.

An engine-created permission is bound to the request digest, invocation,
role/operation, selected driver/model, workspace access/freshness, exact input
array, and containment profile. It exposes only one of these structural rows:

| Invocation responsibility | Exact ordered artifacts | Permitted decision |
| --- | --- | --- |
| Planner proposal | `plan` | none |
| Implementer design/revision | `design`, `work_status` | none |
| Implementer implementation/repair | `work_proof`, `work_status` | none |
| Captain review | `work_status` | `proceed`, `revise`, or `escalate` |
| Work Verifier | `work_status` | `pass`, `fail`, or `blocked` |
| Assembly Verifier | `assembly_status` | `pass`, `fail`, or `blocked` |

W2 checks canonical shape, binding, byte limits, exact artifact order, and this
permission only. W1/W3 remain responsible for deriving the permission from
authoritative status, parsing the submitted Baton bytes, admitting protected
evidence, and applying `recordTransition`. A seal is therefore a captured
handoff, never a plan approval, Captain decision, Verifier verdict, status
transition, proof of candidate quality, or Git effect by itself.

The first valid `Submit` atomically seals the in-memory endpoint as accepted or
rejected. An exact byte replay returns the exact same canonical seal bytes and
digest; a different replay returns `submission_conflict` without replacement.
There is no cross-invocation lookup or mutable global submission state. Durable
publication and crash recovery of a seal belong to W3's journal.

The invoker releases an accepted sealed handoff to its caller only when the
same child exits zero, emits one valid bound `completed` result, stays within
all bounds, and satisfies the workspace post-check. Any transport status other
than `completed`, process/protocol failure, absent/rejected seal, cancellation,
timeout, or read-only mutation returns no eligible handoff and cannot create a
Baton decision or verdict.

## Normalized usage without estimation

Keep the RC2 wire `usage` object exact: when present it has only non-negative
safe-integer `input_tokens` and `output_tokens`; when absent it is unavailable.
Normalize it into a Sworn receipt with nullable `input_tokens` and
`output_tokens`. Presence with value zero produces non-null zero pointers;
absence produces nulls.

The same receipt has nullable `cost_micro_units`, `currency`, and `source`.
Those three fields are all present or all null. Cost is a non-negative integer,
currency is a closed uppercase three-letter code, and source is a bounded
provider-report identifier. The normalizer accepts cost only as a typed trusted
provider observation; it never derives cost from tokens, model names, a price
table, configuration, or result text. Baton RC2 has no cost field, so W2 does
not extend its result envelope and the fake records cost as unavailable. W5
may populate the typed observation only when a provider actually reports cost;
otherwise it must remain null.

## Deterministic fake

Implement one `baton.fake` driver with version `1.0.0` and the exact `info` and
`run` commands. It validates the common request and supports all five role
values through the same code. Its explicit per-process profile is one of
`completed`, `transport_error`, `timeout`, `cancelled`, or `runner_error`.
Every profile exits zero with one valid bound result; `completed` echoes the
explicit model, reports duration zero and observed token zeros, while the other
profiles omit usage. Text is fixed by profile/role and truncated safely to the
request limit. A digest-bound conformance script may select the distinct
contract-valid completed variant with `text:""`; this does not add a sixth
transport profile.

For sealed-handoff tests and W3 fixtures, an optional canonical fake script is
itself an ordered digest-bound file in the read-only input overlay. It selects
one submission, blocking point, attempted write, or malformed behavior for
that invocation only. It cannot select a provider, model, authority action, or
path outside the disposable workspace. No package global, clock, random value,
home state, network, or inherited environment affects fake output. Repeating
the same request, profile, script, and invocation identity produces identical
wire result and seal bytes.

# Surfaces

- Replace `internal/driver/doc.go` with the package contract and add focused
  production files for strict JSON/wire values, canonical operation binding,
  role selection, usage normalization, invocation/process control, input
  projection, submission framing/sealing, and the deterministic fake.
- Add Linux process/projection code and a non-Linux fail-closed stub under
  `internal/driver`; do not add another production package.
- Add `internal/driver/testdata` only for exact RC2 request/result goldens,
  malformed process helpers, fake scripts, and isolation canaries. A tiny
  test-built fake executable delegates to the production fake implementation;
  it is not a production provider adapter or a `cmd/sworn` command.
- Add a reusable test-only adapter conformance harness in the same package so
  W5 adapters can run the identical case IDs without editing the W2 contract.
- Consume canonical operations through W0's admitted `internal/baton` API.
  Do not edit `internal/baton`, `cmd/sworn`, `internal/runtime`,
  `internal/journal`, `internal/gitx`, `go.mod`, `go.sum`, CI, or Baton records
  in the product candidate.
- Do not add Codex, Claude, OpenAI-compatible, DeepSeek, Gemini, Bedrock, HTTP,
  SDK, tool-loop, pricing, managed-inference, journal, cockpit, evaluation, or
  OTel code.

# Consequential decisions and risks

- **Baton wire bytes remain exact.** The sealed submission and optional typed
  cost observation are separate Sworn values; neither adds a field to
  `baton.driver-request/v1` or `baton.driver-result/v1`. Golden drift tests
  compare the Go codec and fake with the admitted RC2 contract and fixtures.
- **Role selection is above, not inside, the driver.** Four explicit
  production selections prevent defaults and make Merge undispatchable. Direct
  five-role fake coverage proves portability without creating model-owned
  composition.
- **A seal is not authority.** W2 can prevent envelope and permission
  confusion, but only W1/W3 can prove that artifact/status bytes are the exact
  next Baton state. The returned type and tests never expose an API that
  applies a transition or converts transport completion into an outcome.
- **Isolation is Linux-first.** Real mount/read-only/cancellation evidence
  depends on the probed Bubblewrap boundary. Missing or inadequate support
  fails before dispatch; it is not silently replaced by permissions or prompt
  instructions.
- **The workspace is caller-supplied but mount-minimal.** W3 must provide a
  disposable private workspace. W2 proves what it mounts and rejects reserved
  filesystem/mount conflicts only; it does not query Git, establish
  product-only provenance, exclude a path from candidate capture, create
  canonical Git worktrees, or import candidates. W1/W3 own those facts.
- **Provider secrets stay out of generic configuration.** W2 deliberately
  supplies only opaque references and a clean launch environment. W5 must add
  typed provider-specific resolution and independently prove its credential
  transport; no generic environment/argv escape hatch is reserved here.
- **Reported cost may remain unavailable.** RC2 stdout cannot carry cost. W2
  records null rather than inventing a price. W5 may use the typed trusted
  observation only where a real provider reports a cost and source.
- **Output is transient.** The invoker must briefly buffer bounded stdout to
  validate JSON, but callers receive only normalized facts, counts/digests,
  sanitized diagnostics, and an eligible sealed handoff. Secret-sentinel tests
  guard accidental retention in errors and receipts.
- **The fake is an oracle, not an adapter claim.** Passing it proves the common
  boundary and deterministic failure profiles, not any production provider,
  live credential, model quality, runtime recovery, or autonomous-engine case.

# Evidence plan

## A-W2-contract

- Run the shared adapter conformance harness against the built fake. Require
  exact three-field `info`; all five role/operation pairs through one
  executable; explicit models for the four model-facing cases; `merge` with
  deliberate `null`; exact operation bytes/digests; ordered inputs; all five
  transport statuses; and no outcome/verdict/action fields.
- Validate `RoleSelections` with four distinct provider/model pairs, absent
  entry, empty model, unknown provider, mismatched observed model, attempted
  fallback/default fields, and production `merge`. The first five resolve
  exactly or fail as specified, and Merge fails before process creation.
- Launch the fake through the real Linux boundary. Inspect its mount/env view
  and require only the disposable workspace and digest-bound
  `.sworn-inputs/v1` data among repository/engine material. Conflicting
  `.sworn-inputs`, record-root, journal, resolver, canonical-worktree, engine
  log, inherited-environment, and sibling-invocation canaries remain absent.
  Filesystem file/directory/symlink and mount conflicts at `.sworn-inputs` fail
  before start, while an instrumented boundary proves W2 invokes no Git command
  and performs no tracked/ignored-entry query. W1/W3 tests separately own
  product provenance and candidate-capture exclusion.
- Exercise every submission permission row with exact accepted bytes, then
  swap role, invocation, artifact order/kind, decision, operation, driver/model,
  input digest, and endpoint. Each swap rejects without an eligible handoff.
  Confirm submitted plan/design/proof/status bytes are not treated as valid
  Baton content by W2 and that no transition API exists in the package.

## A-W2-isolation-usage

- For Verifier, mount a clean workspace read-only, attempt file, Git metadata,
  reserved-input, and sibling writes, cancel a blocking descendant, and compare
  the complete before/after manifest. Require no mutation, no surviving
  process, no result/seal after cancellation, and only an operational
  observation.
- Run completed, transport-error, timeout, cancelled, runner-error, child
  crash, a valid completed result with `text:""`, non-zero-with-stdout,
  zero-without-result, malformed JSON, duplicate object, extra stdout,
  oversized stdout/stderr/text, partial submission frame, and rejected
  submission cases. The empty-text case must parse and bind successfully; an
  empty stdout stream remains a missing-result failure. Only one clean
  `completed` result plus one accepted bound seal can release a handoff; none
  creates a Baton verdict in W2.
- Test usage `{0,0}`, positive safe integers, absent usage, negative/float/
  overflow values, partial cost triples, zero reported cost, bad currency,
  forbidden estimated source, and unavailable cost. Canonical receipt bytes
  must preserve observed zero and null separately.

## Adversarial and replay matrix

| Concern | Required observation |
| --- | --- |
| Envelope drift | Unknown/missing/duplicate fields, role-operation mismatch, self-consistent replacement instructions, reordered/duplicate inputs, path escape, extra result fields, and changed RC2 golden bytes fail closed. |
| Cross-talk | Parallel invocations cannot exchange result IDs, submission endpoints, permissions, scripts, staged inputs, models, or seals; no descriptor leaks to a sibling child. |
| Secret leakage | Raw-key config fields are rejected; a canary in the parent environment, hostile stdout/stderr, malformed output, and fake input is absent from returned errors, diagnostics, receipts, and serialized observations. |
| Malformed output | Empty stdout, multiple objects, invalid-UTF-8, trailing, partial, oversized, wrong-driver/model/invocation, and non-zero-with-output cases return no result or eligible seal; a single valid result whose `text` is empty succeeds. |
| Cancellation and bounds | Deadline, explicit cancellation, output overflow, control-frame overflow, and blocked descendants terminate the whole process tree within the test bound and leave a read-only workspace unchanged. |
| Deterministic replay | Strict encode/decode is byte-stable; identical submission replay returns the original seal bytes; conflicting replay cannot replace it; repeated fake request/profile/script produces identical result, usage, and seal bytes. |

Run the approved check from a clean product candidate:

`GOFLAGS=-buildvcs=false go test -race ./internal/driver/...`

Also run focused non-race repetitions for cancellation/cross-talk tests, Go
vet for `internal/driver`, product-only formatting, `git diff --check`, and a
changed-path check proving the product candidate touches only
`internal/driver`. Raw outputs remain local evidence; the proof references
their command, exit status, and digest rather than retaining model transcripts.

# Revisions

Initial plan-bound W2 design produced by
`codex:/root/w2_driver_implementer/sworn-v0.3.0/T2-driver/W2-driver-core/design/1`.
The approved plan and admitted Baton RC2 driver contract are authority. The
Coach, driver/evaluation, runtime-reset, and v0.2 captures informed failure
tests only; no legacy package, provider implementation, lifecycle, journal,
cockpit, or telemetry design is carried forward.

Revision 2 produced by
`codex:/root/w2_driver_implementer/sworn-v0.3.0/T2-driver/W2-driver-core/design/2`
after Captain `REVISE`. It corrects RC2 result `text` to allow 0 through
1,048,576 bytes while retaining the positive request output limit and an
explicit empty-text conformance case. It also contracts W2's
`.sworn-inputs` responsibility to filesystem/mount conflict checks and assigns
Git provenance plus reserved-prefix candidate-capture exclusion to W1/W3.
