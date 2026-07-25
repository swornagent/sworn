# Approach

## Authority and scope

This revision is for `sworn-v0.3.0-baton-rc3 / T2-driver /
W2-driver-core`, produced by
`codex:/root/w2-rc3-design-implementer/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/design/2`.
It binds plan
`sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`,
approval
`sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`,
authoritative owner head
`4362f5a0ed44a004981495c858e94af505c1b769`, materialization marker
`5d4c5eb117087e306be6868103891dc685319528`, base
`c846e8d8b9c1e054657e4b94dd586c4b8e7afac7`, and frozen T0 head
`7d925851dc91a4ee324d9fe29c33d631f44d1a56`.

The owner status records v1 design
`sha256:02e2a0fe66ad4271eb15309b6b230bd7e348639badfb854399614450118e2bd2`
and Captain `REVISE` invocation
`codex:/root/w2-rc3-design-captain/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/review/1`.
This revision preserves v1's sound contract, scope, isolation, fake,
submission, usage, and evidence decisions while replacing the four rejected
bindings.

Product and test changes are confined to `internal/driver`. No Baton record,
journal, resolver, runtime, Git, command, module, or configuration surface is
part of W2. W2 owns the common role-neutral contract, common
containment/process boundary, deterministic fake, sealed submission, and
usage semantics. Production provider/profile runtime-closure discovery,
credential containment, capability inspection, and certification belong to
W5 and are not claimed by this work.

The RC2 product commits are feasibility input only, and may not be applied
until a Captain has returned `PROCEED` over these exact revision bytes:

- `3b30295e6f8d04963266f9b98880df434f9c4f6c`, full-index patch
  `sha256:aa00493645b22d5e90ec0199ee5e93bc54b8107dc6744286a3cdf4350a43a751`;
- `287f83d05dff3540831996553c5610002c92f3da`, full-index patch
  `sha256:37a2ee63a67e5554d8070cfc110486b3871015cbcf4f31761a08cfac4523bf93`;
- `0136a96c4355e60c815b5cab043b54e860d00062`, full-index patch
  `sha256:ade0486ab9784047026475e80d127ae82382c0f23d2c8254c7204afb4807bd44`.

The W0-to-final `internal/driver` full-index patch is
`sha256:9bcaa63667fc4e0ac159439307c82a5c9192de538a7b76203db0898f600b3d52`
and its final subtree is `85eed1c35838c241f7edf6a3331cb771c59c66ce`.
These identities permit exact inspection and scoped replay; they do not admit
an RC2 design, proof, status, verdict, PASS, history, or authority.

## Contract and selection

Keep one closed, strict `baton.driver/v1` implementation for all model-facing
roles. `Request`, `Result`, `DriverInfo`, operation, workspace, limits, input,
and result codecs reject duplicate names, trailing JSON, unknown fields,
unsafe integers, invalid Unicode, non-canonical paths, over-limit bytes, and
binding drift. Each Planner, Implementer, Captain, and Verifier request names
one configured driver and one non-empty model. There is no default, fallback,
rotation, role-specific driver method, lifecycle decision, retry, or provider
orchestration.

Retain `merge` and deliberate `model:null` only in the portable Baton wire
codec and fixture coverage. Production selection, permission construction,
and invocation refuse Merge before process start. Merge remains deterministic
and engine-owned.

Canonical operations come only from the admitted embedded Baton package. The
driver checks the package's public RC3 identity (`1.0.0-rc.3`, annotated tag
object `34324784694696a38d951061c2313363b405c1e4`, peeled commit
`affaf16cc37f845b5dc43b22988d8b680ff1f212`, and manifest
`sha256:b58b3c2f844c0c0b07195c3ef5af8c1819871f11b51d697e06207ad0d4b2ec9c`)
before exposing an operation. The complete operation tuple must match those
admitted bytes. Equal operation bytes from an older package do not confer
authority: a submission permission also binds the admitted RC3 identity, the
RC3 invocation and request digests, and the ordered plan/status/evidence input
digests. An old-lineage request or seal therefore cannot replay as RC3.
Rename the stale tests to
`TestCanonicalOperationsBindExactRC3Package` and
`TestRC3WorkspaceAndRepositoryPathBoundaries`.

Inputs remain an ordered list, not a map. Names and repository-relative paths
are unique, and each supplied `InputContent` must occupy the same index and
match the declared raw-byte digest. Reordering, substitution, omission,
duplication, path aliasing, or digest mismatch fails before process start.

## Workspace and process boundary

Every call starts a new bounded process; no process, context, home, session,
environment, or submission endpoint is reused. The private host workspace is
opened and pinned by file descriptor, while the request and child see only a
stable guest path. A Linux Bubblewrap mount namespace uses an empty
environment, fixed `HOME`, `TMPDIR`, locale, time zone and `PATH`, a private
PID/process tree, no capabilities, and only the pinned executable, fixed
W2/fake test-system mounts, role workspace, and input projection. Network is
shared only when the already selected invocation policy explicitly requires
it; the fake always has none. Non-Linux execution fails closed as unsupported.

W2 validates the common mount shape, trusted root-owned Bubblewrap binary and
required namespace/process features. It does not discover or certify a
production profile's executable closure, toolchain, libraries, CA, DNS, NSS,
credentials, or provider capabilities. W5 must produce and certify those
profile-specific inputs before using this boundary.

The model-visible workspace overlay must not expose `.git`,
`.baton/releases`, the journal/database, approval resolver or evidence store,
canonical host worktree paths, engine logs, or ambient user configuration.
The namespace has fixed descriptor-based mount destinations. Reserved paths
are covered by empty namespace-owned read-only masks or are required absent;
no host path is accepted as a guest destination. Before dispatch, reject
workspace symlinks that are absolute, escape the pinned workspace, alias a
reserved path, or reach a separately mounted system/input path. Masking is
applied after the workspace bind so aliases cannot bypass it. Canonical host
paths are not placed in the request, argv, environment, diagnostics, or guest
mount names, and hostile canaries at every excluded surface must be
unreadable.

Inputs are staged in a private temporary directory outside the source
worktree, opened by descriptor, mounted read-only at a fixed guest input path,
and removed after the complete process tree is quiescent. No placeholder,
input byte, or cleanup marker is created in the source worktree.

For `read_only`, bind the pinned source descriptor read-only at the VFS mount
boundary; model writes must fail before reaching the source. A pre-dispatch
manifest and a post-quiescence manifest compare paths, types, modes, symlink
targets, sizes, and regular-file digests. This comparison detects persistent
changes present at either observation, including mode-, symlink-, and
content-only differences. It does not prove that an external host actor did
not mutate and restore bytes between observations. That limitation is
explicit: prevention of model-caused writes comes from the read-only
descriptor mount plus reserved-path/symlink controls, while the manifests are
post-condition evidence and concurrent persistent-mutation detection, not a
complete write audit. If descriptor identity, symlink confinement, masking,
read-only mount state, or post-check cannot be proved, fail before publication.

A Verifier is admissible only with both
`workspace.access=read_only` and `fresh_context=true`; writable or non-fresh
Verifier requests fail before staging or process start. All invocations still
receive a fresh process, regardless of the request flag.

## Sealed role output

Keep one canonical, bounded submission envelope and one engine-owned
permission. The permission binds the admitted RC3 package identity,
invocation/request digest, exact role and operation, provider key, driver ID
and version, executable digest, explicit model, network policy, access,
freshness, containment profile, ordered-input digest, and one responsibility.
The accepted shapes are closed and ordered:

| Responsibility | Exact artifacts | Exact decision |
| --- | --- | --- |
| Planner proposal | plan | none |
| Implementer design | design, work status | none |
| Implementer implementation | work proof, work status | none |
| Captain review | work status | `PROCEED`, `REVISE`, or `ESCALATE` |
| Work verification | work status | `PASS`, `FAIL`, or `BLOCKED` |
| Assembly verification | assembly status | `PASS`, `FAIL`, or `BLOCKED` |

There is no Merge permission. Artifact and evidence bytes carry exact byte
counts and SHA-256 digests; strict canonical framing, per-kind, aggregate, and
frame limits apply.

The submission server seals on the first attempted body, accepted or rejected.
An exact replay produces the same immutable seal for reconciliation; a
different body is a conflict; a seal from another invocation is refused.
Inside a live process, the first complete submit frame closes admission
permanently and no late or second frame can alter the seal.

## Serialized terminal arbiter

One serialized invocation arbiter owns stdout chunks, stderr chunks,
submission frames, endpoint/protocol faults, parent cancellation, deadline,
process exit, post-check, cleanup, one monotonically increasing event
sequence, and the only publication gate. Event producers may run
concurrently, but they can change state only while holding the arbiter mutex;
acquiring that mutex and allocating the event sequence is the event's
linearization point.

Stdout, stderr, result text, input, submission, and frame bytes each have
explicit hard counters. Raw stdout is capped by the lesser of the protocol
maximum and the configured text limit plus a fixed maximum result-envelope
allowance; strict decoding then enforces `text <= output_bytes`. Stderr has a
separate small hard maximum. Crossing either stream limit records a fatal
event, closes endpoints, terminates the process group, waits for it, and
returns no handoff. Bounded diagnostic retention may be shorter than the hard
limit, but overflow is failure, never successful truncation.

### Normal result path

With no submission attempt, success requires process exit zero, exactly one
strict bound result, `transport_status=completed`, source post-check and
cleanup success. Non-zero exit, malformed/extra stdout, typed non-completed
transport status, endpoint fault, cancellation, timeout, overflow, source
post-check failure, cleanup failure, or a process group that is not quiescent
is fatal and releases no artifact or decision. Transport failure is
operational only and never becomes `FAIL`, `BLOCKED`, `REVISE`, or another
Baton outcome.

### Submission terminal path

The exact successful accepted-submit sequence is:

1. The endpoint reads one complete canonical submit frame. Under the arbiter
   mutex, it allocates the submit event sequence, closes admission, validates
   the body against the invocation permission, and records the immutable
   accepted seal as provisional.
2. If no fatal event has already linearized, the endpoint writes exactly one
   accepted seal response. A response write failure is a protocol/transport
   failure and suppresses the seal.
3. After the acknowledgement write completes, the arbiter closes the
   endpoint, records `engine_stop_after_submit`, sends SIGTERM to the complete
   process group, escalates to SIGKILL after the fixed grace period, and waits.
   This engine-caused stop is the required successful termination mechanism:
   a clean exit after endpoint closure or the exact SIGTERM/SIGKILL issued by
   this stop is classified `engine_terminated_after_submit`, not as a runner
   non-zero exit or transport failure. A spontaneous non-zero exit or other
   signal that linearized before the engine stop remains transport failure.
4. The arbiter proves the process group and descendants quiescent, performs
   the source post-check, removes the outside-worktree input projection, and
   joins every stream, endpoint and wait producer. The parent
   cancellation/deadline watcher remains armed through publication.
5. The arbiter takes the publication mutex, performs a final synchronous
   parent-context/deadline check, and linearizes either failure or publication.
   On publication it records `published`, closes the watcher through its
   separate stop channel, releases the mutex, and joins the watcher. A watcher
   that already observed cancellation contends for the same mutex: watcher
   first records fatal failure; publication first means completion linearized
   before that later cancellation. Only an accepted provisional seal with
   successful acknowledgement, the expected engine-caused termination
   classification, quiescence, post-check and cleanup becomes a
   `SealedHandoff`. No driver `completed` result is synthesized; usage is
   unavailable unless it was independently present in an already validated
   typed observation allowed by the contract.

A rejected first submission follows the same acknowledge, close,
engine-stop, wait and cleanup sequence but publishes no handoff or Baton
decision. Rejection is not converted into a transport verdict.

Any fatal event that linearizes before the publication point suppresses an
accepted provisional seal, even when submit and acknowledgement happened
first. Fatal classes have diagnostic precedence
`parent cancellation > timeout > stdout/stderr overflow > endpoint/protocol
failure > driver transport/process failure > source/post-check/cleanup
failure`; every class has the same no-handoff effect. A submit event is below
all fatal classes and above ordinary clean exit.

Concurrent sources are resolved by the same mutex and publication point:

- cancellation, timeout, overflow, protocol or transport failure that
  linearizes before publication wins and suppresses the seal;
- submit may linearize first and cause the intentional stop, but remains
  provisional, so a later fatal event before publication still wins;
- at the final gate, cancellation and publication contend on the same mutex;
  cancellation first means failure, publication first means the invocation
  completed before that later cancellation;
- stream, endpoint and wait producers are joined before the gate, while the
  cancellation/deadline watcher stays armed and uses the publication mutex,
  so no pre-publication event can arrive unobserved afterward;
- late frames, stdout, exits, exact replays, or conflicting bodies cannot
  reopen admission or revive a failed invocation.

Barrier-controlled tests exercise each pairwise ordering, including
cancel-before-submit, cancel-after-seal-before-ack, cancel-after-ack-before
quiescence, cancel racing publication, overflow racing submit, process exit
racing engine stop, and late replay/conflict.

## Fake and usage

The deterministic fake implements the same `info`, `run`, request/result
codec, process boundary, selection, submission, and permission paths as later
profiles. Shared conformance covers every transport status, all four
model-facing roles with explicit models, portable `merge/model:null`,
canonical operation bytes, ordered inputs, empty completed text, zero usage,
unavailable usage, and sealed shapes. Scripted helpers additionally produce
valid/rejected/conflicting submissions, parent/child blocking, stdout flood,
stderr flood, malformed/late frames, workspace writes, forbidden-surface
reads, and cancel-after-submission races.

The fake's minimal executable/system mount map proves W2's common containment
mechanics only. It is not evidence that any W5 production profile, runtime
closure, credential path, or capability set is certified.

Usage has an explicit `reported` or `unavailable` state. Reported token fields
are non-negative safe integers, so reported `(0,0)` remains a legitimate zero;
unavailable carries no token fields. Provider cost is independently either
reported with non-negative micro-units, currency and
`provider_reported` source, or unavailable. Partial states, inferred values,
pricing tables, estimates, and converting transport failure or an
engine-caused submit stop into zero are rejected.

## Implementation sequence

1. Reconfirm the exact owner head, clean worktree, eligible `REVISE` status,
   plan, approval, dependency, v1 design/Captain binding, these revision
   bytes, and then a current Captain `PROCEED`. Stop before using RC2 product
   input if any binding differs.
2. Recompute all three full-index patch digests, the one-shot patch digest,
   and final subtree. Replay only `internal/driver` product bytes onto the RC3
   materialization; import no old record or evidence bytes.
3. Bind canonical operations and permissions to the admitted public RC3
   identity; generalize stale RC2 names; enforce explicit four-role selection
   and production Merge refusal.
4. Implement the outside-worktree input projection, fixed descriptor mount
   graph, reserved/symlink masks, Verifier admission checks, read-only
   prevention, and honest persistent-delta post-check.
5. Implement the first-attempt sealed endpoint and serialized terminal
   arbiter, including the acknowledged engine-stop path, hard stdout/stderr
   limits, process-group cleanup, event linearization and publication gate.
6. Make usage availability explicit and extend the shared fake and adversarial
   process helpers without adding W5 profile certification.
7. Run the focused, adversarial, race, shuffle, repeat, full, vet, build,
   formatting, scope, history, independent-budget, aggregate-budget,
   reproducibility and proof gates. Create one product-only candidate only
   when all pass.

# Surfaces

The final W2 production surface is exactly these 11 files:

- `internal/driver/contract.go`
- `internal/driver/control.go`
- `internal/driver/doc.go`
- `internal/driver/fake.go`
- `internal/driver/invoke.go`
- `internal/driver/process_linux.go`
- `internal/driver/process_other.go`
- `internal/driver/projection.go`
- `internal/driver/selection.go`
- `internal/driver/submission.go`
- `internal/driver/usage.go`

Test and fixture files are limited to:

- `internal/driver/conformance_test.go`
- `internal/driver/contract_test.go`
- `internal/driver/invoke_linux_test.go`
- `internal/driver/projection_test.go`
- `internal/driver/selection_usage_test.go`
- `internal/driver/submission_test.go`
- `internal/driver/testdata/fake/main.go`
- `internal/driver/testdata/process/main.go`

No configuration or Baton record changes are designed. Using the approved
physical-line classifier:

- final W2 production is exactly 11 files and 3,277 physical lines;
- the W0 `internal/driver/doc.go` stub is 1 file and 3 physical lines, so W2
  adds exactly 10 files and 3,274 net production lines;
- the dependency-complete post-W1 composition must be exactly 19 production
  files and 7,190 physical lines, including the W0 stub; and
- applying W2's net delta must yield exactly 29 production files and 10,464
  physical lines.

Added arbiter/isolation lines must be offset by removing duplicate RC2
machinery or prose. Tests and fixtures are reported separately and cannot
offset production lines. Both the independent W2 result and dependency-complete
aggregate are binding gates.

# Consequential decisions and risks

- **RC2 replay could masquerade as authority.** Mitigation: gate replay on a
  fresh RC3 Captain decision, verify exact product patches/tree, bind the
  admitted RC3 package plus current request/input identities, and exclude all
  old control history.
- **Equal operation bytes could hide a stale package.** Mitigation: validate
  the public RC3 tag/commit/manifest identity independently of operation
  bytes, and test an equal-byte old-lineage replay refusal.
- **Mount-path or symlink tricks could expose authority or mutate source.**
  Mitigation: descriptor-pinned roots, fixed guest destinations, read-only
  VFS binding, reserved masks, symlink confinement, outside-worktree input
  staging, hostile canaries, and fail-closed mount proof.
- **Manifests could overclaim write detection.** Mitigation: state that they
  detect persistent pre/post deltas only; prevent model writes at the mount
  boundary and do not claim detection of host mutate-and-restore behavior.
- **Submission/cancellation races could release a decision after failure.**
  Mitigation: one arbiter, event sequence, provisional seal, exact
  acknowledge/engine-stop classification, joined producers, quiescence, and
  one linearized publication gate.
- **Output flooding could bypass memory bounds or look successful after
  truncation.** Mitigation: hard per-stream counters trigger termination;
  truncation is never a success path.
- **A rejected submit could continue model execution.** Mitigation: every
  attempted submit closes admission and intentionally terminates/waits for the
  process tree without publishing a decision.
- **Bubblewrap or process-group behavior may differ by host.** Mitigation:
  require the trusted root-owned `/usr/bin/bwrap` common feature set, prove
  W2 fake mount/process behavior, and fail closed elsewhere. W5 separately
  owns production profile closure/certification.
- **The stronger boundary could exceed the ratified baseline.** Mitigation:
  keep the exact W2 inventory, consolidate duplicated parsing/control code,
  and hard-stop on either the 11/3,277 independent or 29/10,464 aggregate
  mismatch.

# Evidence plan

## Acceptance mapping

- **A-W2-contract:** strict codec and canonical-operation tests; exact RC3
  identity test; explicit per-role driver/model resolution; production Merge
  refusal with portable `merge/model:null` fixture; ordered-input
  mutation/reorder tests; guest visibility canaries; role-shape table tests;
  replay/conflict/cross-invocation seal tests; shared fake conformance.
- **A-W2-isolation-usage:** writable and non-fresh Verifier pre-start refusal;
  read-only VFS write refusal; reserved-path and escaping-symlink refusal;
  persistent content/mode/symlink/source-delta cases plus an explicit
  mutate-and-restore limitation assertion; outside-worktree input and cleanup
  checks; stdout and stderr flood termination; timeout, cancellation,
  descendant cleanup, malformed transport and no-handoff checks;
  accepted/rejected submit acknowledgement and intentional termination;
  synchronized cancel/overflow/exit/submission/publication races; reported
  non-zero, reported zero, unavailable and invalid partial usage/cost cases.
- **A-W2-rebind:** exact RC2 patch/subtree verification log, RC3-only changed
  paths, renamed/generalized tests, old-lineage/equal-operation-byte replay
  refusal, exact 11/3,277 W2 and 10-file/3,274-line delta manifests, exact
  post-W1 19/7,190 input manifest, exact 29/10,464 aggregate manifest, fresh
  candidate/proof identities, and no reference to an RC2 verdict or proof as
  acceptance.

## Required checks and raw outputs

Run and retain separate raw, digest-addressed outputs outside model-visible
workspaces for:

```text
GOFLAGS=-buildvcs=false go test ./internal/driver/...
GOFLAGS=-buildvcs=false go test -race ./internal/driver/...
GOFLAGS=-buildvcs=false go test -shuffle=on -count=20 ./internal/driver/...
GOFLAGS=-buildvcs=false go test -race -shuffle=on -count=10 ./internal/driver/...
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race ./...
GOFLAGS=-buildvcs=false go vet ./...
CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -o /tmp/sworn-w2 ./cmd/sworn
git diff --check
```

The process tests must explicitly report trusted Bubblewrap common
feature/provenance, the fake-only mount map, workspace/input/reserved/symlink
visibility, process-group descendants after intentional and failure
termination, hard stdout/stderr counters, terminal event sequences,
publication decisions, and absence of a handoff on every failure path. They
must not label this evidence as production provider/profile runtime-closure
certification.

Scope evidence lists every path and proves the candidate delta is wholly under
`internal/driver`, with no `.baton/releases`, module, generated, or
out-of-scope change. History evidence binds the materialization marker,
recorded v1 `REVISE`, fresh v2 Captain result, implementation start, candidate
ancestry, and proves no product commit preceded `PROCEED` and no old lineage
or hidden post-candidate product bytes were imported.

Budget evidence uses the same byte-defined physical-line classifier and
separately records:

1. W0 driver stub: 1 file/3 lines;
2. final W2: 11 files/3,277 lines;
3. W2 delta: +10 files/+3,274 lines;
4. dependency-complete post-W1 input: 19 files/7,190 lines; and
5. aggregate post-W2 composition: 29 files/10,464 lines.

Run the official build twice from independent product-only copies with fresh
caches and compare binary digests.

Fresh `proof.md` must bind repository, materialization base, frozen dependency,
candidate commit/tree, product-tree digest, plan, approval, this revision,
Captain and Implementer invocation identities, admitted RC3 package identity,
ordered input/request/permission digests, changed-file and all five
physical-line manifests, check-output digests, terminal-arbiter race matrix,
and the adversarial observation matrix. It must state that RC2
records/design/proof/PASS/history were not reused and that W5 profile
certification is not claimed.

# Stop conditions

Stop without a candidate or transition on any stale/foreign status, head,
plan, approval, dependency, v1 design/Captain or v2 design/Captain binding;
dirty initial worktree; absent `PROCEED`; RC2 patch/tree mismatch; imported
old control bytes; out-of-scope path; public RC3 identity mismatch;
operation/input/order drift; production Merge dispatch; missing explicit
model; writable or non-fresh Verifier; unavailable or incomplete common
isolation; unproved descriptor/read-only/mask/symlink boundary; forbidden
canary visibility; persistent source mutation; incomplete process-tree
cleanup; overflow that does not terminate; unclassified submit-caused exit;
accepted seal published before acknowledgement, quiescence, post-check,
cleanup, producer join and final context check; any pre-publication fatal
event releasing a handoff; late/replayed submission changing a seal; any W2
claim of W5 provider/profile certification; flaky/racy/failed check;
non-reproducible build; final W2 other than exactly 11 files/3,277 lines; W2
delta other than exactly +10 files/+3,274 lines; post-W1 input other than
exactly 19 files/7,190 lines; or aggregate post-W2 other than exactly 29
files/10,464 lines.

# Revisions

- **Revision 1:** Initial RC3 design. It used the audited RC2 product tree
  only to establish feasibility and added RC3 package/lineage binding,
  outside-worktree input projection, Verifier admission, a serialized
  terminal arbiter, hard stderr handling, submission-attempt termination,
  race canaries, and aggregate post-W2 budget preservation. No RC2 design
  bytes were reused.
- **Revision 2:** In response to the recorded Captain `REVISE`, defines the
  exact accepted-submit acknowledgement, intentional engine-stop
  classification and linearized publication race semantics; narrows manifest
  evidence to persistent mutation while preventing model writes through
  descriptor/read-only/mask/symlink controls; binds W2 independently to
  11/3,277 and +10/+3,274 plus post-W1 19/7,190 and aggregate 29/10,464; and
  assigns production profile runtime-closure/provider certification solely to
  W5.
