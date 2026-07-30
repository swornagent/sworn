# Sworn S0/S1 runtime reset design

Date: 2026-07-24
Status: revised after Captain REVISE; pending approved plan and plan-bound Captain
Scope: S0 reset/conformance seam and S1 single-track walking skeleton
Design base: admitted asset commit
`a15a04c3b5993d6002265e0a9412fbe9dad33bc0`, tree
`23a289dc0e5242cfa2c76a99c8c66861823bf43a`
Baton authority: digest-pinned published annotated tag `v1.0.0-rc.2`

## Outcome

Replace the production kernel rather than refactoring it. Keep the single
SQLite dependency and port only failure-preventing invariants introduced by
focused tests. S0/S1 has six production packages, one command service, one
SQLite journal, one scheduler loop, one contained-process boundary, and no
mutable copy of Baton's lifecycle.

`internal/baton` is a self-contained Go implementation of Baton's complete
deterministic record, transition, product-identity, Git-composition, and
seven-action contract. Baton RC2 does not publish a scheduler-facing
action-selection or result-validation API. Sworn therefore computes eligible
work from exact committed records and refs and invokes the Go action facade; it
does not pretend to adapt nonexistent exported methods.

The released RC2 JavaScript reference is a development oracle only. CI uses it
to generate or verify checked-in golden conformance vectors. The production
binary does not execute Node, ship a JavaScript bridge, or depend on Node being
installed.

S1 proves one approved one-track delivery through the deterministic external
fake: materialise the exact owner ref, dispatch roles through a common
submission proxy, admit exact handoffs and verdicts, publish a passed track,
compose it, verify the assembly in a fresh invocation, and atomically integrate
the exact release result and terminal record. Each external edge is journalled
and reconciled before retry.

This capture is not a plan-bound Baton design. No approved Baton plan or
baseline `status.json` exists on this line. Its implementation gate remains
closed until Planner produces a final exact plan, protected external approval
binds that plan, one canonical `installApprovedPlan` mutation creates only the
release ref plus baseline plan/status records while atomically verifying every
track ref absent, `materializeTrack` creates only dependency-ready T0, the
assigned Implementer writes the plan-bound design, and a distinct Captain
returns `PROCEED` over that exact plan/design pair. Every other track ref
remains absent until eligible. The later implementation candidate and proof
still require an independent Verifier.

## Binding authority and baseline

The protocol contract is pinned to these published identities:

| Identity | Exact value |
| --- | --- |
| annotated tag object | `b80f3e27f0e0a71a4883bcc282e4843e085f0e04` |
| peeled commit | `890238ef063bb53cf51fb3359f1ff527f14846c6` |
| peeled tree | `97513f3e6f798f3ad04d5b510a49496a605a8ea4` |
| release archive | `sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63` |
| generated support package | `sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436` |
| published-tag ruleset | `19678047`, active update/delete protection for `refs/tags/v*`, no bypass actors |

The protected annotated tag makes the admitted Git object graph stable under
the current no-bypass ruleset. RC2 was published before GitHub release-asset
immutability was enabled, so Sworn treats it as a digest-pinned published RC2:
downloadable assets are publication evidence bound by the exact digests above,
not GitHub-immutable objects. Future Baton releases must enable full release
immutability before publication in addition to retaining exact tag, tree,
archive, and support-package digests.

Production embeds exactly these 14 path-preserving tagged blobs in bytewise
path order. The closed inventory is 50,387 bytes:

| Path | Bytes | Released SHA-256 |
| --- | ---: | --- |
| `VERSION` | 11 | `sha256:0c654c00f94741d78169de333d4d9e866be0667b41f9c54cafe3c6b700b15a43` |
| `baton/PROTOCOL.md` | 15,739 | `sha256:8e6eb570b2eeb27d84b64fb182d71f7591995ba6a1318500769a9db9144eca5a` |
| `conformance/engine-adapter.md` | 2,503 | `sha256:dbb3d5c3d22b79a3da4e98fb96f4db1eaa16d2bda04567f4d181bda001705450` |
| `conformance/manifest.json` | 5,830 | `sha256:3bf2535cc1e92ac132576dd0c646062b9d33a0ba33201823f1d92409a6387a92` |
| `operations/baton-design-review.md` | 1,838 | `sha256:ead3a7d0e22a794ca5430fdbaca5c29f3ae5d5f6fad7c102d1f2bd878f28e356` |
| `operations/baton-implement.md` | 2,247 | `sha256:2444bead5b1a32188003ce515ac8862bd04d373b740bd89646a86ac5341c2f88` |
| `operations/baton-merge.md` | 2,312 | `sha256:94b8fb6026c903569cd375cafd11d27868759072dde256265556c710387ae62c` |
| `operations/baton-plan.md` | 2,154 | `sha256:e5c3ace4177cb10c9b0d3b5e569aa7cbe43bfdb3b7f4a17071a925a5ba3b77d3` |
| `operations/baton-verify.md` | 2,160 | `sha256:a6f0e9b9bf95cb59e5030b7f95f72d8d3545b52ef771c7d20e7be44a20e45bed` |
| `reference/driver/contract.md` | 3,038 | `sha256:660a1ce7b44cdd150d902fddc80043814b5d6dc4fc28c29a7daed9973abe60bf` |
| `schemas/work-status-v1.json` | 9,542 | `sha256:70219641e954afefa35fe20cf702eeabac3ce7c9290d09d5ce29082bf4a497c1` |
| `templates/design.md` | 583 | `sha256:10e4a2097bffab99464454f9389b5c72f8e3cb12680943ae945401e7b0ebc146` |
| `templates/plan.md` | 1,686 | `sha256:7caac5f8fc8baccacb2787902c1f86d97a92728db0a42b63a4674444886a276c` |
| `templates/proof.md` | 744 | `sha256:0bc58a34505859792ac734ff50a23420ad9f24e0227aee19c4e71d84ef9fd225` |

Startup recomputes the compiled inventory before admitting a run. A missing
publication, lightweight tag, changed byte, incomplete or extra asset, unknown
schema/operation version, or digest mismatch fails closed. The released
JavaScript record modules and portable fixtures remain exact development/CI
oracles read from the pinned release archive; they are not production embeds.

The three templates are immutable operation inputs, not mutable defaults.
`internal/baton` reads them from the verified compiled inventory and the runtime
places the applicable template first in the driver's ordered inputs:

| Responsibility | First immutable input |
| --- | --- |
| Planner plan or pristine rebound | `templates/plan.md` |
| Implementer design or revision | `templates/design.md` |
| Implementer candidate proof | `templates/proof.md` |
| Merge assembly-proof renderer | `templates/proof.md`, consumed engine-side |

Captain and Verifier receive no authoring template. Merge reads the verified
embedded proof template directly and is not dispatched. Real process drivers
receive template bytes through the read-only projection defined below; the
`baton.driver-request/v1` entry still carries only its stable name, projected
repository-relative path, and exact raw-byte digest.

The revised greenfield scope is
`docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md` with raw-byte digest
`sha256:64066240d713e8b89cee8a9adfd20a1f6a19b1029617b7769ba5465e5f234093`.
That digest is recorded here, not inside the scope capture itself, so the
binding is exact without being self-referential. The v0.2 kernel and archived
v0.3 construction are archaeology, not sources. Useful invariants to restate
in new tests are:

- atomic command/effect insertion and idempotency conflict;
- immutable results and stop-before-retry for uncertain effects;
- exact candidate objects, expected revisions, and ref compare-and-set;
- private read-only verification and contained child processes;
- no cleanup until a writable process tree is proven quiescent;
- old/foreign SQLite databases never migrate on open; and
- paths, symlinks, replacement objects, hooks, config, credentials, and
  canonical Git metadata fail closed at process and worktree boundaries.

<!-- sworn-history-only:begin -->
The earlier pre-publication capture at Sworn commit
`3ca3af92a44553d52a1e5202f26dd5502c012fb4` described Baton candidate commit
`893f6fe8b6a52cebc8e7ccecc745ed5d138f3184`, tree
`8770f15e6f6919dc92458f071205eb7552800d3a`, an 11-asset/47,374-byte scope,
and a publication hold; related design prose called this an `immutable-RC2`
pin. That historical decision and wording are superseded in full by the
published 14-asset/50,387-byte admission at Sworn commit
`a15a04c3b5993d6002265e0a9412fbe9dad33bc0`; none of the former candidate
identities or counts is active authority.
<!-- sworn-history-only:end -->

R0 adds a documentation guard that scans active Markdown and release metadata.
Outside exact `sworn-history-only` marker pairs and dedicated negative test
fixtures it rejects every former OID in full or 12-character prefix, former
inventory count/size, former publication-state wording, and former RC2
immutability wording enumerated in the preceding history block. Its positive
fixture requires the current published commit/tree, 14 paths, 50,387 bytes,
and digest-pinned-publication wording. This makes later documentation drift a
pre-merge failure while still allowing explicit history to explain the
correction.

## Product cut

### S0: reset and Baton seam

S0:

1. admits the published RC2 package by exact release evidence;
2. replaces the old production tree and starts a new incompatible database
   identity and schema;
3. embeds the released protocol, schema, operations, fixture manifest, and
   other assets required by the Go compatibility implementation;
4. exposes Sworn and exact Baton package identity/digests;
5. runs the portable-kit fixtures against the Go implementation and built
   fake-driver harness; and
6. exposes the autonomous-engine adapter without claiming unexecuted cases.

### S1: one-track walking skeleton

S1 accepts bounded intent through Planner when no plan exists, pauses on its
strict proposal until protected external approval is resolved, and installs
that exact plan through one canonical mutation and resulting receipt. Exact
replay or reconciliation may return the same canonical `changed:false`
reconciliation receipt without duplicating a mutation, commit, ref, status,
effect, or receipt identity. S1 may instead admit an already externally
approved strict plan through the same approval and action gates. It supports
one run and one track, but uses the exact release and track authority refs
required by Baton. It performs:

```text
capture bounded intent and dispatch configured Planner
  -> seal and validate one non-authoritative plan proposal
  -> await protected external approval over its exact raw digest
  -> journal one canonical install effect; create only release ref plus plan/status baseline
  -> recapture its receipt, plan/status records, release head, and absent track refs
  -> call materializeTrack for the one dependency-ready track as a separate action
  -> compute the next eligible responsibility or mechanical action
  -> atomically record intent, effect, and scope claim in SQLite
  -> execute one contained process or exact Git action
  -> seal and bind its immutable observation
  -> resolve trusted evidence and validate prospective records/transitions
  -> reread every bound ref and record before the next action
  -> compose the frozen passed track and transfer authority
  -> prepare assembly proof/status and dispatch a fresh read-only Verifier
  -> atomically integrate the passed assembly and terminal release status
```

Planner, Implementer, Captain, and Verifier author permitted bytes or decisions
through the submission proxy. Merge is deterministic engine-owned action
execution; no model chooses refs, parents, tree, merge mode, target, or commit
message.

Real model drivers, parallel/dependency scheduling, operator controls, HTTP
providers, telemetry, cockpit, and DBOS remain outside S0/S1. The shared proxy
contract is defined now so S2 native CLIs and S4 HTTP tool loops cannot create
new lifecycle seams. S3 supplies parallel/dependency execution and its evidence.

## Six production packages

```text
cmd/sworn         CLI, signals, process lifetime, version, real-binary harness
internal/baton    embedded RC2 assets, strict records, validation, seven actions
internal/runtime  command service, scheduler, recovery, effect dispatch
internal/journal  SQLite commands, claims, effects, receipts, events, outbox
internal/gitx     repository binding, private worktrees, exact Git primitives
internal/driver   driver contract, submission proxy, containment, external fake
```

There is no shared model/types/policy/executor utility package. Owner types stay
with their package and consumers declare narrow interfaces only at test seams.

## Complete Go Baton contract

### Compatibility ownership

`internal/baton` owns a literal Go compatibility implementation of the
released contract, not a reduced adapter:

- strict JSON parsing, closed record shapes, limits, canonical paths and refs;
- admitted plan metadata, approval bindings, status identity and semantics;
- work and assembly transition validation, including unchanged `NO_VERDICT`;
- owner-aware record selection and materialisation evidence;
- handoff byte/digest checks and trusted-evidence admission;
- candidate first-parent history, product-tree identity, path scope, and
  record-only transition validation;
- exact fast-forward or deterministic two-parent composition verification;
- atomic expected-head ref transactions and retry reconciliation; and
- all seven deterministic actions below.

The Go names may be idiomatic, but behavior, accepted values, exact bytes,
commit construction, parent ordering, failure conditions, and receipts map
one-for-one to RC2. Unknown fields, states, outcomes, roles, action inputs,
refs, evidence, Git modes, or reconciliation observations fail closed.

The compatibility map is checked in beside the implementation:

| Baton action | Go facade responsibility |
| --- | --- |
| `installApprovedPlan` | create only release ref with exact plan/baseline statuses while verifying every track ref absent |
| `reboundPristinePlan` | replace only a pristine unmaterialised plan |
| `recordTransition` | validate and record ordinary work/assembly transition |
| `materializeTrack` | create one dependency-ready owner marker and atomically move release/create that track |
| `composeTrack` | compose frozen track and transfer all work authority |
| `prepareAssembly` | write Merge-produced proof and initial assembly status |
| `integrateRelease` | integrate passed assembly and terminal status atomically |

The public production surface is one immutable package value with those seven
typed methods plus read-only parsing, snapshot, eligibility, and validation
queries. The queries are pure over supplied bytes, Git object facts, and an
opaque evidence admission. They cannot mutate a ref or status. The action
methods accept logical identities and exact authored bytes only; they derive
all refs, paths, messages, trees, parents, and expected values from the
admitted plan and captured snapshot.

`internal/gitx` supplies narrowly typed object reads, product-tree calculation,
prospective record-only commit construction, deterministic merge-tree and
commit-tree construction, and atomic ref transactions. It cannot independently
advance Baton lifecycle. `internal/runtime` decides when an eligible facade
method becomes a journalled effect, but cannot weaken facade validation.
SQLite stores only runtime claims, attempts, effect identities, and receipts.

### Golden compatibility

Development tooling may run the exact RC2 reference modules to produce
fixtures containing inputs, canonical output bytes, digests, prospective
commit identities under fixed Git identity/time, receipts, and typed errors.
Checked-in vectors cover every transition, seven actions, owner selection,
record limits, adversarial Git facts, fast-forward, two-parent composition,
retry, and stale/third-value refusal.

Go tests consume the vectors without starting Node. A separate CI job with the
released package regenerates them and requires a byte-for-byte clean diff.
Reference JavaScript, generated vector tooling, and portable fixture runners
are non-production assets and are measured separately.

## Strict driver and submission boundaries

### Exact `baton.driver/v1`

Every process driver implements `info` and `run` exactly as RC2 specifies.
`baton.driver-request/v1` contains:

- stable `invocation_id` and exact Baton role;
- canonical operation `id`, `version`, digest, and raw instructions;
- explicit non-empty model or deliberate `null`;
- absolute workspace path and `read_only` or `read_write`;
- ordered uniquely named inputs with canonical repository-relative path and
  raw-byte digest;
- explicit `fresh_context`; and
- positive timeout and output limits.

The driver validates the whole operation tuple against the canonical embedded
operation, not against caller-replacement text.

`baton.driver-result/v1` binds invocation, driver identity/version, observed
model, duration, optional usage, bounded response text, and exactly one
transport status. `completed` text is transport-only. Sworn never parses it as
a design, proof, status, decision, verdict, evidence, or Merge instruction.

### Read-only invocation inputs

The reserved process-visible input root is exactly `.sworn-inputs/v1/`.
It is never a repository path or candidate artifact. Before process dispatch,
the engine creates a private `0700` staging directory outside every repository,
worktree, object database, and journal; writes the applicable embedded template
and other engine-owned input bytes under stable names; verifies their lengths
and SHA-256 digests; fsyncs each file and the directory; and opens the staged
files by descriptor. The containment builder presents the private worktree
through an ephemeral per-invocation overlay mount, creates the reserved
mountpoint only in that overlay, and projects the staging directory read-only at
`<workspace>/.sworn-inputs/v1`. A read-only role sees the completed overlay
remounted read-only; a writable role has a private upper layer and Git directory
whose exact committed objects are quarantined after quiescence. The lower
candidate and canonical repository receive no directory, placeholder, index
entry, or object. Failure of the startup overlay/read-only-projection probe
refuses dispatch.

The entire top-level `.sworn-inputs` prefix is reserved. The engine checks the
captured Git tree and the checked-out filesystem with no-follow descriptor
walks before `exec.Cmd.Start`. Any tracked, untracked, ignored, symlink,
submodule, case-fold-equivalent, mount, or replacement entry at that prefix is
`INPUT_PATH_COLLISION`; dispatch is refused and the candidate remains
byte-for-byte unchanged. Candidate import also rejects that prefix. A repository
may freely contain its own `templates/plan.md`, `templates/design.md`, or
`templates/proof.md`: those paths neither shadow nor receive the projected
files.

These are the only synthetic paths and stable input names:

| Input name | Process-visible path | Source |
| --- | --- | --- |
| `template-plan` | `.sworn-inputs/v1/templates/plan.md` | embedded `templates/plan.md` |
| `template-design` | `.sworn-inputs/v1/templates/design.md` | embedded `templates/design.md` |
| `template-proof` | `.sworn-inputs/v1/templates/proof.md` | embedded `templates/proof.md` |
| `planning-intent` | `.sworn-inputs/v1/planning-intent.json` | canonical compact UTF-8 JSON+LF bounded intent |
| `prior-authority` | `.sworn-inputs/v1/prior-authority.json` | canonical plan-ordered ref/status path-and-digest snapshot |
| `captain-evidence` | `.sworn-inputs/v1/captain-evidence.md` | exact sealed evidence for the current Captain decision |
| `verifier-evidence` | `.sworn-inputs/v1/verifier-evidence.md` | exact sealed evidence for the current Verifier result |

The remaining stable names and canonical Baton paths are:

| Input name | Canonical path |
| --- | --- |
| `plan` or `prior-plan` | `.baton/releases/<release>/plan.md` |
| `work-status` | `.baton/releases/<release>/work/<work-id>/status.json` |
| `design` | `.baton/releases/<release>/work/<work-id>/design.md` |
| `work-proof` | `.baton/releases/<release>/work/<work-id>/proof.md` |
| `assembly-status` | `.baton/releases/<release>/assembly/status.json` |
| `assembly-proof` | `.baton/releases/<release>/assembly/proof.md` |

The complete `request.inputs` order is:

| Invocation | Exact ordered names |
| --- | --- |
| Planner, new | `template-plan`, `planning-intent` |
| Planner, pristine rebound | `template-plan`, `planning-intent`, `prior-plan`, `prior-authority` |
| Implementer, initial design | `template-design`, `plan`, `work-status` |
| Implementer, design revision | `template-design`, `plan`, `work-status`, `design`, `captain-evidence` |
| Implementer, initial proof | `template-proof`, `plan`, `work-status`, `design`, `captain-evidence` |
| Implementer, proof after work `FAIL` | the initial-proof order, then `work-proof`, `verifier-evidence` |
| Captain | `plan`, `work-status`, `design` |
| Work Verifier | `plan`, `work-status`, `design`, `work-proof` |
| Assembly Verifier | `plan`, `assembly-status`, `assembly-proof` |

Optional suffixes are permitted only in the named state. Before dispatch the
engine independently opens every input beneath the correct backing root,
checks exact regular-file shape, bytes, digest, name, path, uniqueness, and
order, then checks the final request again. Missing or substituted projected
bytes, a digest mismatch, an added or reordered input, or a changed Baton file
fails before the process exists. After quiescence it rechecks the staged
digests and proves the backing repository still has no reserved-prefix entry
before accepting any candidate.

The process-boundary tests launch the built fake through the real executor and
namespace. The fake opens each request path and returns the observed bytes and
digest; the test requires byte identity with all three embedded templates.
Separate cases keep conflicting repository `templates/**` files unchanged,
refuse a reserved-prefix collision, and prove substitution, missing input,
reordering, and request/file digest mismatch all fail before fake-process
dispatch.

### `sworn.submission/v1` action proxy

Sworn owns one strict role-output seam for the fake, native CLI, and HTTP
tool-loop drivers. All transports use the same Go decoder, canonical encoder,
server-held capability, and one-shot sealer.

#### Schema

The exact closed top-level shape and field order are:

```json
{"schema_version":"sworn.submission/v1","invocation_id":"invocation","artifacts":[],"decision":null,"action":null}
```

It is compact UTF-8 JSON followed by one LF, at most 2,097,152 bytes. Closed Go
structs fix field order; arrays preserve order. `encoding/json` runs with HTML
escaping disabled. The decoder rejects BOM, whitespace outside strings,
duplicate/unknown keys, invalid UTF-8, floats, trailing bytes, and values whose
canonical re-encoding differs. HTTP tool arguments decode to the same structs
and canonical bytes, so transport syntax cannot change the digest.

`invocation_id` is 1-200 ASCII characters matching
`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`. `artifacts` contains zero to two unique
values in permission-matrix order:

```json
{"kind":"design","byte_count":123,"sha256":"sha256:<64 lowercase hex>","bytes_base64":"..."}
```

`kind` is one of `plan | design | work_proof | work_status | assembly_status`;
`byte_count` is an integer from 1 through the kind limit; and `sha256` matches
`^sha256:[0-9a-f]{64}$`. Base64 is padded canonical RFC 4648 and must re-encode
identically. Count and digest bind decoded bytes; aggregate decoded artifacts
are at most 1,048,576 bytes. `plan` is limited to 1,048,576 bytes. Every other
kind is limited to 262,144 bytes. Capability maps the kind to the one admitted
Baton path. Engine-created assembly proof uses the same 262,144-byte handoff
limit but is never an external submission.

`decision` is `null` or:

```json
{"outcome":"proceed","evidence":{"byte_count":123,"sha256":"sha256:<64 lowercase hex>","bytes_base64":"..."}}
```

Evidence uses the same encoding, is 1-262,144 bytes, and is operational review
evidence only. The matrix closes its outcome. `action` is `null` or exactly
`{"name":"recordTransition"}`; it has no caller arguments.

No envelope, descriptor, decision, or action field accepts a ref, OID, path,
Git command, commit message, parent order, merge mode, or arbitrary effect.
Strict plan/status artifact bytes contain their Baton-required refs and OIDs;
the Baton parser validates those as content rather than control arguments.

#### ABI and capability

For every fake or native process, Sworn creates one
`socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0)`. The child endpoint is the
first `exec.Cmd.ExtraFiles` entry and therefore exactly descriptor 3 after
`exec`; the parent endpoint and every unrelated descriptor remain close-on-exec.
The clean child environment adds only `SWORN_SUBMISSION_FD=3` and
`SWORN_SUBMISSION_CONTROL=sworn.submission-control/v1`. The endpoint has no
path, token, reconnectable credential, or second client.

Both directions use the same stream framing: a four-byte big-endian unsigned
payload length followed by exactly that many bytes of canonical compact UTF-8
JSON plus one final LF. The LF is included in the length. The control payload
maximum is 4,194,304 bytes; zero and larger lengths are invalid. The nested
`sworn.submission/v1` value remains limited to 2,097,152 bytes. A server reads
one complete request, writes its one complete response, then reads the next;
there is no pipelining, multiplexing, or concurrent request handling.
`io.ReadFull` semantics make fragmented prefixes/payloads and multiple
coalesced frames equivalent to ordinary reads.

An EOF after zero bytes at a frame boundary is orderly half-close. EOF in a
prefix or payload, zero/oversized length, invalid UTF-8, missing final LF, BOM,
duplicate or unknown field, noncanonical JSON, or trailing payload bytes closes
the endpoint without a response and creates no seal for that frame. A canonical
control request that contains a schema-valid but disallowed submission receives
a sealed rejection instead. No parse error is model output or a Baton result.

`internal/driver.SubmissionClient` has exactly:

```text
Describe(context) -> Descriptor
Submit(context, sworn.submission/v1) -> Seal
```

`Describe` returns this closed shape:

```json
{"schema_version":"sworn.submission-descriptor/v1","invocation_id":"invocation","state":"open","role":"planner","baton_state":null,"artifacts":[{"kind":"plan","max_bytes":1048576}],"decisions":[],"action":null,"limits":{"frame_bytes":4194304,"submission_bytes":2097152,"artifact_total_bytes":1048576,"evidence_bytes":262144}}
```

`role` is `planner | implementer | captain | verifier`. `baton_state` is `null`
or has the exact field order
`{"kind":"work","stage":"design","status":"ready","next_role":"implementer"}`;
each value uses the corresponding closed Baton enum and `kind` may be `work` or
`assembly`. The descriptor exposes only the selected permission row; it
contains no capability, ref, OID, candidate, or mutable path. The driver maps
it to one runner-visible `sworn_submit` tool. The fake uses the same framed FD3
client as a real process; native adapters translate their local tool transport;
HTTP loops call the same Go service without the socket.

The two request payloads have exact field order:

```json
{"schema_version":"sworn.submission-control/v1","operation":"describe","invocation_id":"invocation"}
{"schema_version":"sworn.submission-control/v1","operation":"submit","invocation_id":"invocation","submission":{"schema_version":"sworn.submission/v1","invocation_id":"invocation","artifacts":[],"decision":null,"action":null}}
```

`describe` may repeat only while open. The first canonical `submit` is evaluated
once and seals the endpoint whether accepted or rejected. The durable response
has this closed shape:

```json
{"schema_version":"sworn.submission-seal/v1","invocation_id":"invocation","state":"sealed","accepted":true,"submission_sha256":"sha256:<64 lowercase hex>","error_code":null}
```

A sealed rejection has `accepted:false`, a null submission digest, and one
lowercase error code of at most 64 characters. After sealing, the connection
accepts only another canonical `submit`: byte-identical submission replay
returns the exact stored response bytes; any different submission returns the
same shape with `submission_conflict` and cannot replace the record. A
post-seal `describe` or other operation closes the endpoint. The server remains
available only for those replay responses until the child half-closes, exits,
or reaches its deadline, then closes its endpoint. A seal proves durable
capture, never a Baton action.

The live endpoint holds a non-serializable capability. Its binding is canonical
`sworn.submission-binding/v1` with these exact fields and order:

1. `schema_version`, `run_id`, `effect_id`, `attempt`,
   `effect_request_sha256`, and `invocation_id`;
2. `repository`: canonical repository identity, real root, and fixed Baton
   record root;
3. `baton`: annotated tag object, peeled commit/tree, release archive digest,
   and support-package digest;
4. `operation`: ID, version, and digest; then `driver`: ID, version, and
   configured model;
5. `workspace`: private-repository identity, starting commit/tree, access,
   `fresh_context`, and containment-profile digest;
6. `inputs`: the exact ordered `{name,path,digest}` array;
7. `authority`: plan digest, approval reference/digest, selected status
   path/digest, and captured `{ref,oid}` heads in bytewise ref order, with
   explicit nulls in the pre-Baton Planner phase;
8. `candidate`: immutable commit, tree, and product-tree digest for read-only
   roles, or null for a writable role until post-quiescence capture;
9. `evidence`: protected dispatch-evidence and record-root-inertness decision
   digests, each explicitly null when not applicable; and
10. `permission`: exact Baton state or pre-Baton phase, permitted artifact
    kinds in order, permitted decision outcomes in order, and permitted action.

`binding_sha256` is over that compact UTF-8 JSON+LF. The capability and its
constructors are absent from environment, prompt, workspace, driver request,
descriptor, output, and journal. The journal stores the immutable effect
request and expected binding bytes/digest for comparison, not a capability.
Writable final candidate facts are observed only after the complete process
tree is quiescent; no caller may submit them.

The first canonical submit produces one private
`sworn.sealed-submission/v1` record under the effect/attempt directory outside
the repository and workspace. Its exact fields are `schema_version`, the full
`binding`, `binding_sha256`, `control_byte_count`, `control_sha256`,
`submitted_byte_count`, `submitted_sha256`, `accepted`, `error_code`,
`artifacts`, `decision`, `action`, `response_byte_count`, and
`response_sha256`, in that order. On acceptance, `artifacts`, `decision`, and
`action` contain only the strictly decoded canonical Baton handoff/evidence
values needed for recovery; on rejection they are empty/null and only counts,
digests, and the error survive. Each count/digest covers its canonical JSON+LF
payload, excluding the derived four-byte prefix. The raw control frame and
provider/tool transport encoding are never written. While the original
connection remains open, the server keeps the first canonical submission bytes
in memory solely for byte-exact replay comparison. The canonical response is
reproduced from the record and must match `response_sha256`.

The server writes a same-directory temporary record with mode `0400`, fsyncs
it, publishes it with no-replace atomic rename, fsyncs the parent directory,
and only then sends the response frame. It never overwrites or edits a
published seal.

On restart, a complete final seal plus a quiescent old process is performed; an
exact unpublished temporary plus no process is not performed and is removed
before a new invocation; a partial, foreign, conflicting, or live-process
observation is inconsistent or uncertain. For a performed seal, recovery does
not redispatch. It reconstructs a fresh non-serializable capability from the
immutable journal request and freshly recaptured repository, refs, records,
starting or immutable candidate as applicable, inputs, protected evidence, and
inertness facts. It requires the new canonical binding bytes and digest to
equal the sealed binding. For an accepted seal it reconstructs the canonical
submission from the normalized handoff fields and validates it under that fresh
capability; a sealed rejection binds only its exact rejection receipt and can
never cause an action. Any writable final candidate is captured and checked
separately after quiescence. Only then may `internal/baton` mint a fresh
single-snapshot admission. Changed facts leave the sealed record and every
Baton ref/record unchanged. Neither capability nor Baton admission is
serialized or recreated from submission, response, runner text, or any other
agent-controlled bytes.

#### Permission matrix

| Origin | State or eligibility | Ordered handoff | Decision | Baton action/result |
| --- | --- | --- | --- | --- |
| Planner driver | pre-Baton new/replacement proposal | `plan` | `null` | none; store non-authoritative proposal |
| engine | exact protected approval over new proposal | sealed plan | none | `installApprovedPlan` |
| engine | exact approval over pristine unmaterialised replacement | sealed plan | none | `reboundPristinePlan` |
| engine | first eligible work on unmaterialised track | none | none | `materializeTrack` |
| Implementer | work `design / ready / implementer` | `design`, `work_status` | `null` | `recordTransition` / `DESIGN_WRITTEN` |
| Captain | work `design / ready / captain` | `work_status` | `proceed | revise | escalate` | matching `recordTransition` |
| Implementer | work `implement / ready / implementer` | `work_proof`, `work_status` | `null` | `recordTransition` / `IMPLEMENTED` |
| Verifier | work `verify / ready / verifier` | `work_status` | `pass | fail | blocked` | matching `recordTransition` |
| Verifier | assembly `verify / ready / verifier` | `assembly_status` | `pass | fail | blocked` | matching `recordTransition` |
| Verifier | either ready verification | none | `no_verdict` | operational receipt; status unchanged |
| engine Merge | all track work `merge / ready / merge` | none | none | `composeTrack` |
| engine Merge | every planned track transferred | assembly proof from tagged template | none | `prepareAssembly` |
| engine Merge | unchanged assembly `PASS` | none | none | `integrateRelease` |

Every other combination fails closed. Captain/Verifier decisions require
evidence. Planner/Implementer cannot decide; Captain/Verifier cannot submit
design/proof. Each engine row is a separate journalled effect with fresh
facade admission. Sworn never dispatches Merge to a model: it has model `null`
and exact records/actions only. Portable `baton.driver/v1` role `merge` does
not create a Sworn inference seam.

#### Planner, admission, and recovery

From bounded intent, Sworn journals the configured Planner driver/model,
dispatches `baton-plan` with tagged template and immutable repository/release
bindings, validates one strict plan artifact, and stores its raw bytes/digest as
a non-authoritative proposal. Runtime becomes
`awaiting_external_approval`, exposes those exact bytes/digest, and creates no
Baton file, status, object, ref, or action. A protected resolver outside Planner
must bind that digest and approval reference before the engine freshly captures
authority and records one canonical installation mutation and resulting
receipt. On exact replay or reconciliation, repeated action calls may return
the same canonical `changed:false` receipt but cannot duplicate the mutation,
commit, ref, status, effect, or receipt identity.
Missing/rejected/changed/stale approval pauses. Pristine rebound uses the same
edge; post-materialisation replan creates new identities. S1 exercises this
edge when intent is supplied, and S3 reuses it for replacement work.

For every process result and restart:

1. prove the complete old process tree quiescent;
2. classify the fsynced seal as `performed` (valid matching seal),
   `not performed` (no seal/process/residue), `uncertain` (live/unidentified
   process), or `inconsistent` (partial/foreign/conflicting/mismatched);
3. never retry `uncertain` or `inconsistent`; bind the exact performed seal to
   the `driver.invoke` receipt without rerunning, or record not-performed before
   issuing a new invocation identity;
4. quarantine writable output or prove read-only output unchanged, recapture
   every bound fact, reconstruct the capability, require the sealed binding,
   and revalidate the stored submission;
5. for Planner, restore `awaiting_external_approval` from the proposal receipt
   and re-resolve approval without redispatch;
6. otherwise mint a fresh Baton admission, prospectively validate the
   transition and Git transaction, then execute the one permitted action;
7. reconcile that Baton action separately against deterministic objects and
   exact old/new refs; never replay it merely because submission replayed; and
8. on absent/invalid submission or stale facts leave Baton bytes unchanged:
   Verifier is operational `NO_VERDICT`, Captain has no decision, and
   Planner/Implementer fail operationally.

Runner text is never a fallback artifact or verdict. No transport, submission,
approval, persistence, or environment failure becomes `fail`, `blocked`,
`pass`, or `merged`.

## Trusted evidence and opaque admission

The evidence resolver and behavioral-inertness resolver are separate
engine-owned policy authorities. Neither is mounted or described to an agent.
Given an exact status and execution profile, the evidence resolver reads
protected approval and Verifier-dispatch evidence by canonical reference,
verifies exact bytes, digest, provenance, candidate read-only access,
clean/fresh context, invocation separation, and profile-specific isolation,
then returns immutable bytes plus provenance to `internal/baton`.

Product-tree exclusion uses a distinct opaque admission. For every immutable
commit whose product identity excludes `.baton/releases`, the trusted host
policy resolver receives exactly:

```text
kind: baton.record-root-inertness/v1
repository: canonical repository identity and real path
record_root: .baton/releases
commit: exact immutable commit OID
```

It returns the same exact binding plus only `inert` or `consumed`. The policy
authority must establish that build, test, package, deploy, hook, and runtime
behavior at that commit do not consume the record root; an agent assertion is
not evidence. An unavailable, throwing, asynchronous, malformed, extra-field,
or mismatched decision fails closed before action. `consumed` is a
`RECORD_ROOT_CONSUMED` refusal: Sworn cannot exclude the records or admit the
candidate.

The facade mints an opaque, unforgeable in-process admission bound to:

- exact status bytes/digest and action;
- plan, approval, authority ref/head, target and component heads;
- execution profile (`guided` or `autonomous`);
- exact resolved evidence bytes/digests and provenance;
- candidate, candidate tree, product tree, workspace mode, and freshness; and
- the exact repository/root/commit-bound `inert` policy decision.

Admissions are single-snapshot values, never accepted from JSON, the journal,
the board, a candidate file, or a driver. They are not visible to agents and
cannot be copied to a changed status or profile. Recovery resolves fresh
evidence and a fresh inertness decision for every required commit after
recapturing refs. A decision may be cached only inside one facade admission;
stored evidence or policy digests are diagnostic and never recreate either
admission.

## SQLite runtime truth

SQLite remains behind `database/sql` and `modernc.org/sqlite`, with one
serialized connection, rollback journal, `synchronous=FULL`,
`foreign_keys=ON`, `trusted_schema=OFF`, DQS disabled, bounded busy timeout,
new application ID, and schema version 1. Read-only open never creates or
migrates. Old or foreign stores fail untouched.

Seven strict tables are sufficient:

| Table | Truth owned |
| --- | --- |
| `runs` | immutable repo/package/plan bindings; revision and dispatch gate |
| `commands` | idempotent closed command bytes and expected revision |
| `effects` | external intent, attempt, scope, immutable request, state |
| `claims` | finite owner/generation/token-bound scope leases |
| `receipts` | immutable command/effect observations |
| `events` | append-only runtime/operator history |
| `outbox` | optional bounded lossy projection only |

Every command transition, effects, receipt, event, and configured outbox row
commits in one `BEGIN IMMEDIATE` transaction. Reusing a command ID with the same
bytes returns its receipt; different bytes fail without mutation. Expired or
ownerless executing effects become uncertain, never pending. No effect retries
until reconciliation proves performed, not performed, or inconsistent.

The journal never stores a mutable Baton stage/status/outcome projection.
Committed refs and records remain lifecycle truth. Receipt/event text cannot
manufacture verdict or integration authority. Outbox loss or Linux containment
availability is not an architectural blocker: a disabled sink is valid, and
unsupported containment fails dispatch without weakening records.

The S1 effect registry covers:

```text
private-repository.ensure   driver.invoke          candidate.import
approval.resolve            baton-action.apply     verifier-workspace.ensure
process.stop                private-workspace.remove
```

`baton-action.apply` records the exact facade action request, complete captured
snapshot, prepared result, and atomic ref transaction as one externally
observable Git edge. It does not expose an arbitrary workflow language.

## Exact authority refs and Git actions

The admitted plan must name:

```text
refs/heads/release-wt/<release>
refs/heads/track/<release>/<track>
```

These are the only Baton release and track authority refs. Private
`refs/sworn/...` refs may retain imported, quarantined, or attempt objects, but
they never replace an authority ref, select authoritative status, prove
composition, or satisfy a Baton expected-head check. The target is the exact
full branch ref in the approved plan.

Plan installation writes the exact plan and every baseline status in one
record-only commit from the target, then atomically creates only the release
ref while verifying the target and every track/owner ref absent. It creates no
track ref. Later, `materializeTrack` admits one dependency-ready track, makes
one record-only marker containing the release base and dependency heads, then
in one `update-ref --stdin` transaction updates the release ref and creates
only that track ref at the same marker while verifying all other bound heads.
A partial materialisation cannot be observed as success.

Work advances on the track ref through product-only candidate commits and
record-only lifecycle commits. The passed frozen track head remains immutable.
Composition is exactly either:

- a fast-forward to that eligible frozen track head; or
- a deterministic two-parent commit whose ordered parents are expected
  release head then frozen track head, and whose tree is the exact deterministic
  merge tree.

After composition, one record-only release commit transfers every work status
together to `merge / complete / none`, binds the frozen head, expected release
head, composition result, and transfer commit, and advances only the release
authority in an atomic transaction that verifies target and every track head.
Partial work transfer is invalid.

Assembly preparation writes Merge-produced proof plus
`verify / ready / verifier` status in one record-only release commit. It binds
the pre-preparation release head as both base and candidate and binds every
ordered frozen component. A fresh read-only Verifier must admit `PASS`.

Release integration again uses exact fast-forward or deterministic two-parent
composition between the expected target and passed assembly candidate. It also
prepares the final record-only release commit with assembly
`merge / complete / none`, outcome `merged`, and exact expected/result binding.
One atomic ref transaction updates both target to the composition result and
release ref to the final status commit while verifying every track head. Thus
product integration and terminal record publication succeed together or
neither ref moves.

All Git writes use the sanitized fixed executable, full refs, literal OIDs,
`--no-deref`, and exact old values. Retry first captures all refs:

- if every ref is at the old values, recompute the exact prospective objects
  and apply once;
- if every affected ref is at the exact new values and prospective commits
  reproduce byte-for-byte, return the original durable result with no commit;
- if an action's atomic transaction left all refs old, record not performed
  and create a new effect only after reconciliation;
- any mixed, symbolic, missing, stale, or third value is inconsistent and
  disables dispatch.

Deterministic commit identity fixes author/committer identity, timestamps,
message, parent order, and tree. A composition conflict never triggers model
resolution, merge, rebase, squash, force update, or retry with new inputs.

## Worktree and process containment

Each driver attempt receives a fresh private repository and detached worktree
seeded only with exact required objects. It has no remote, alternate, canonical
object path, control ref, release worktree, journal, credentials, or sibling
workspace. A mutating role may write only there. Candidate objects enter a
quarantine, pass parent/tree/path/history checks, and are imported
content-addressably before an authority action may reference them.

Every verification receives a different private repository at the exact
candidate, staged read-only for a fresh process. Sworn proves after exit that
OID, tree, manifest, status, and relevant refs did not change. Failure of OS
enforcement or post-proof is operational `NO_VERDICT`, never a verdict.

Every fake or later real driver is a new bounded process with fixed clean
environment, bounded stdin/stdout/stderr, deadline, cancellation, resource
limits, descendant ownership, and invocation-bound service identity. Result
and submission receipts are fsynced and atomically renamed before SQLite bind.

Those are bounded live pipes, not retained logs. The only durable out-of-band
raw process output is stdout/stderr from deterministic local checks under their
content-bound check receipt. Sworn never persists or exports raw agent/provider
stdin, stdout/stderr, prompts, completions/model text, credentials, raw argv,
tool arguments/results, or out-of-band copies of source and diffs. Candidate
Git objects and strictly admitted Baton plan/design/proof/status/review-evidence
bytes remain their intentional in-band authorities; they are not duplicated as
logs. Driver recovery retains only the normalized sealed handoff above,
canonical bounded receipts, byte counts/digests, transport/exit facts, and
sanitized bounded diagnostic codes/messages. Raw driver result text is
discarded after the process and submission checks.

S1 production execution is Linux-first and fail-closed. Bubblewrap supplies
mount/PID/network isolation and a transient systemd user service supplies
cgroup ownership, limits, `KillMode=control-group`, inspection, and cleanup.
Startup probes required behavior. Other systems may run pure/portable tests but
cannot claim autonomous S1 execution without an equivalent crash corpus.

On restart, a live or unidentified old process keeps its effect uncertain.
Recovery terminates and proves the entire process tree quiescent before reading
or removing writable roots. PID alone is never proof.

## Reconciliation and crash corpus

Every external edge has test cuts:

1. after durable intent/claim, before the call;
2. after start or partial progress;
3. after external result/ref transaction, before SQLite bind; and
4. after bind, before consumption and fresh fact capture.

SQLite cuts immediately before and after commit. The harness kills the real
binary without deferred cleanup, restarts on the same repository/journal,
runs recovery twice, and completes or reports the exact inconsistency.

The submission ABI has its own process-boundary corpus through the built fake:

- assert an `AF_UNIX` `SOCK_STREAM` endpoint at child FD3 and no endpoint leak
  to another child;
- fragment every prefix and payload boundary, then coalesce
  `describe`, `submit`, and exact replay in one write; responses must remain
  sequential and byte-identical to ordinary framing;
- reject zero and 4,194,305-byte lengths, noncanonical JSON/LF, a closed peer
  mid-prefix or mid-payload, and extra bytes inside one declared payload,
  without a seal or response;
- return the exact stored response for identical replay, return
  `submission_conflict` for a different replay without replacing the seal, and
  reject post-seal `describe`; and
- terminate cleanly on orderly half-close, child exit, and deadline with no
  blocked goroutine, inherited endpoint, or partial response treated as a
  submission.

A sentinel case places distinct markers in driver stdout/stderr, prompt,
completion, tool transport, out-of-band source/diff output, and
deterministic-check output. Excluding the canonical candidate and admitted
Baton handoffs, a post-run scan of the journal, engine private state, outbox,
and captured telemetry must find only sanitized driver receipts/digests and
the deterministic check's explicit raw log; none of the other sentinels may
persist or export.

Each accepted and sealed-rejection case then kills the real Sworn process at:

1. full first-submit receipt, before seal construction;
2. completed temporary seal-file fsync, before no-replace publication;
3. published seal and parent-directory fsync, before response;
4. response completion, immediately before SQLite journal bind;
5. immediately after journal bind, before fresh fact capture/admission;
6. after fresh admission, before `baton-action.apply`;
7. after the Baton ref transaction, before its SQLite receipt bind; and
8. after that bind, before scheduler consumption.

Every cut restarts twice. Before the durable publication point, a quiescent
exact temporary is removed and the old invocation is classified not performed;
only then may one new invocation identity be dispatched. At and after durable
publication, neither restart dispatches a driver. Matching facts produce one
SQLite driver receipt with the exact original response-frame bytes and at most
one Baton action/result; a completed Baton transaction is reconciled rather
than repeated. A variant moves one bound ref or changes plan, status, input,
candidate, approval, or protected evidence after sealing: both restarts retain
the seal, leave every Baton ref and record byte-for-byte unchanged from the
stale observation, emit no action, and require operator resolution. Replaying
the same journal command after either restart returns the identical receipt,
not merely an equivalent reconstruction.

| Edge | Performed | Not performed | Inconsistent |
| --- | --- | --- | --- |
| private workspace | exact registered identity/HEAD/mode | path and registration absent | foreign, dirty, replaced, canonical path exposed |
| driver/submission | sealed bound receipts and quiescence | quiescent, no receipts, disposable root | live/unidentified process, foreign receipt, changed candidate |
| candidate import | full expected object closure validates | no complete closure; residue disposable | malformed object or canonical ref/config mutation |
| Baton action | all affected refs exact new and objects replay | all affected refs exact old | mixed, third, symbolic, missing, or replay mismatch |
| cleanup | exact path and registration absent | exact quiescent workspace remains | replacement path, foreign registration, live process |

The corpus asserts one live writer per implemented scope, no uncertain retry,
late lease rejection, idempotent recovery, no synthesized status/verdict,
authority refs only at exact old/new values, unchanged verifier workspaces,
no leaked processes, and a target equal to the assembly-verified integration
result. New effect kinds fail tests until they have all cuts and a reconciler.

## Conformance truth

Portable-kit execution and autonomous-engine evidence are separate.

Portable-kit fixtures validate strict records, reference compatibility,
operations, installed assets, board/driver fixtures, dogfood, and release
overhead. Passing them against Go and the built fake proves compatibility only;
it never changes an autonomous case result. Their command logs, fixture
digests, and golden-vector diff are recorded as S0 evidence.

Autonomous cases run only through `baton.engine-conformance/v1` against the real
binary, journal, scheduler, driver, workspace, process, and Git boundaries.
The harness independently inspects its temporary repository and evidence
digests. All twelve are currently `NOT RUN`. The S0/S1 exit reporting is:

| Autonomous case ID | Current | S0/S1 exit |
| --- | --- | --- |
| `protected-external-approval` | NOT RUN | PASS only with real adapter evidence |
| `role-instruction-credential-workspace-process-isolation` | NOT RUN | PASS only with real adapter evidence |
| `clean-read-only-fresh-verifier-dispatch` | NOT RUN | PASS only with real adapter evidence |
| `one-writer-per-track-with-independent-track-concurrency` | NOT RUN | NOT RUN until S3 |
| `durable-invocation-attempt-and-effect-identity` | NOT RUN | PASS only with real adapter evidence |
| `crash-recovery-at-every-effect-boundary` | NOT RUN | PASS only with real adapter evidence |
| `timeout-cancellation-cleanup-and-bounded-retry` | NOT RUN | PASS only with real adapter evidence |
| `dependency-scheduling-and-one-serial-worker-per-track` | NOT RUN | NOT RUN until S3 |
| `exact-track-composition-and-ownership-transfer` | NOT RUN | PASS only with real adapter evidence |
| `fresh-assembly-verification` | NOT RUN | PASS only with real adapter evidence |
| `moved-target-compare-and-set-refusal` | NOT RUN | PASS only with real adapter evidence |
| `exact-release-integration` | NOT RUN | PASS only with real adapter evidence |

Until each applicable case actually runs, its status remains `NOT RUN`; a design
target is not a result. `FAIL` means it ran and an observation failed. Missing
support, timeout before start, absent credentials, model output, portable
fixture success, or adapter assertion cannot produce `PASS`.

## Implementation slices

| Slice | Owner paths | Acceptance |
| --- | --- | --- |
| R0 reset/admission | `AGENTS.md`, deleted old tree, module files, CI | exact RC2 identity/assets; old DB rejected |
| R1 Go Baton compatibility | `internal/baton/**` | all seven actions, validators, goldens, portable fixtures |
| R2 journal | `internal/journal/**` | strict schema, idempotency, claims, uncertainty |
| R3 Git boundary | `internal/gitx/**` | isolation, object identity, atomic old/new/third corpus |
| R4 proxy/process/fake | `internal/driver/**` | canonical envelope, Planner proposal, external fake, containment, receipts |
| R5 walking skeleton | `internal/runtime/**`, `cmd/sworn/**` | approval/install edge, one-track authority, assembly, integration |
| R6 crash/conformance | package tests, optional `test/e2e/**` | crash cuts and applicable autonomous evidence |

R1-R4 may proceed independently after R0. R5 owns composition. A failed R6
case returns to its owning slice rather than creating a cross-package repair.
No production behavior precedes its focused failing boundary test.

Test order is:

1. release/assets admission and startup self-check;
2. strict parser, schema, transition, evidence, owner, and seven-action goldens;
3. new SQLite identity, transaction, lease, receipt, uncertainty, and outbox;
4. repository binding, quarantine, product identity, exact composition, and
   atomic multi-ref retry;
5. read-only input projection/order, common submission envelope, FD3 framing,
   Planner proposal, and external fake containment;
6. seal/replay/restart cuts, approval wait/install, role output admission,
   missing/invalid submission, stale capability, and privacy sentinels;
7. one-track materialisation, work, composition, assembly, and integration;
8. generated crash matrix with restart-twice recovery;
9. portable-kit suite and real autonomous adapter cases applicable to S0/S1;
10. full tests, race tests, vet, formatting, size/dependency measurements, and
    `git diff --check`.

## Adjusted budgets and stop gates

The complete Go Baton contract makes the previous 6,000-line and 18 MiB targets
unrealistic. The reset remains small relative to the 20,443-line kernel:

| Measure | S0/S1 target | Mandatory stop |
| --- | ---: | ---: |
| production packages | 6 | 6 |
| handwritten production Go | <= 10,000 lines | 12,500 |
| `internal/baton` handwritten Go | <= 4,000 lines | 5,000 |
| production SQL | <= 350 lines | 500 |
| direct dependencies | 1 (`modernc.org/sqlite`) | 2 without ADR |
| production file length | <= 500 lines | 700 |
| stripped Linux binary | <= 25 MiB | 30 MiB |
| journal tables | 7 | 7 without Captain review |
| effect kinds | only S1 observed edges | any without reconciler/crash cuts |

Embedded released assets and checked-in golden vectors are reported separately
from handwritten Go. There is no provider SDK, Git library, workflow framework,
ORM, migration framework, JavaScript runtime, generated compatibility layer,
or production Node dependency. Exceeding a stop gate pauses for deletion or
fresh Captain review; it does not invite compressed code.

## Decisions preserved

1. Reset all old production packages; port only tested invariants.
2. Baton records/refs own lifecycle; SQLite owns runtime recovery only.
3. Implement the complete RC2 deterministic contract in pure Go and verify it
   against exact released assets/goldens.
4. Keep one custom Go command service and seven-table SQLite journal.
5. Reject old databases rather than migrate them.
6. Keep serial rollback journal and one writer until measurement says otherwise.
7. Use finite SQLite claims plus Git expected-head checks and OS quiescence.
8. Keep the fake external and role-neutral.
9. Require fresh read-only verification and separate assembly verification.
10. Keep Merge mechanical and engine-owned, including exact composition and
    atomic terminal record publication.
11. Keep the outbox optional, bounded, lossy, and non-controlling.
12. Keep Linux systemd/Bubblewrap containment for production S1.
13. Defer independent-track concurrency and dependency scheduling evidence to
    S3 without weakening single-track authority semantics.

## Readiness gate

Implementation may begin only after:

- the intended annotated RC2 release and exact assets are admitted;
- Planner produces the final exact Baton plan and protected external approval
  binds its complete raw digest without Planner or Implementer self-approval;
- the runtime journals one canonical `installApprovedPlan` effect and resulting
  receipt; its one mutation installs that unchanged plan and every strict
  baseline `status.json`, creates only the release ref, and atomically verifies
  every track ref absent; exact reconciliation may return the identical
  canonical `changed:false` receipt without another mutation;
- `materializeTrack` creates only dependency-ready
  `refs/heads/track/sworn-v0.3.0/T0-reset`, while every other planned track ref
  remains absent until its dependencies make it eligible;
- the assigned T0 Implementer writes the plan-bound design, then a distinct
  Captain binds that exact plan/design pair and returns `PROCEED`;
- the approved plan's non-overlapping R0-R6 assignments, target, release and T0
  refs, every still-absent later track ref, and all baseline/owner statuses are
  recaptured and match exactly;
- initial tests enumerate every S1 effect, action, failure, and reconciler; and
- base, target, release, track, evidence, and package bindings are recaptured
  immediately before the first source-changing commit.

At this design head no `.baton/releases/**/plan.md` or `status.json` exists.
That absence is an implementation stop for R0-R6, not permission to seed
placeholder lifecycle records. Planning may proceed; a Baton Captain decision
and production-source implementation may not.

If released bytes change package ownership, action semantics, record validation,
composition, or the shared proxy boundary, stop for an authorised plan
revision. Field-name translation that preserves exact RC2 behavior stays inside
`internal/baton`; weakening or partially implementing the contract does not.
