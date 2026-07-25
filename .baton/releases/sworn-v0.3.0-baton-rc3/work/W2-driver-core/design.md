# Approach

## Authority and scope

This design is for `sworn-v0.3.0-baton-rc3 / T2-driver /
W2-driver-core`, produced by
`codex:/root/w2-rc3-design-implementer/sworn-v0.3.0-baton-rc3/T2-driver/W2-driver-core/design/1`.
It binds plan
`sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`,
approval
`sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`,
owner/materialization head
`5d4c5eb117087e306be6868103891dc685319528`, base
`c846e8d8b9c1e054657e4b94dd586c4b8e7afac7`, and frozen T0 head
`7d925851dc91a4ee324d9fe29c33d631f44d1a56`.

Product and test changes are confined to `internal/driver`. No Baton record,
journal, resolver, runtime, Git, command, module, or configuration surface is
part of W2.

The RC2 product commits are feasibility input only, and may not be applied
until a Captain has returned `PROCEED` over these exact design bytes:

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
one configured provider/driver and one non-empty model. There is no default,
fallback, rotation, role-specific driver method, lifecycle decision, retry, or
provider orchestration.

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
PID/process tree, no capabilities, and only the pinned executable, required
read-only runtime closure, role workspace, and input projection. Network is
shared only when the selected, validated profile explicitly requires it; the
fake always has none. Non-Linux execution fails closed as unsupported.

The model-visible workspace overlay must not expose `.git`,
`.baton/releases`, the journal/database, approval resolver or evidence store,
canonical host worktree paths, engine logs, or ambient user configuration.
Reserved paths are masked or absent in the namespace, canonical host paths are
not placed in the request, argv, environment, diagnostics, or guest mount
names, and hostile canaries at every excluded surface must be unreadable.
Inputs are staged in a private temporary directory outside the source
worktree, opened by descriptor, mounted read-only at a fixed guest input path,
and removed after the complete process tree is quiescent. No placeholder,
input byte, or cleanup marker is created in the source worktree.

`read_only` captures a source manifest before dispatch and again after
process-tree cleanup, including paths, types, modes, symlink targets, sizes,
and regular-file digests. Any source mutation, including an external or
symlink/mode-only mutation, suppresses all handoff bytes. A Verifier is
admissible only with both `workspace.access=read_only` and
`fresh_context=true`; writable or non-fresh Verifier requests fail before
staging or process start. All invocations still receive a fresh process,
regardless of the request flag.

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
Inside a live process, the first submit frame closes admission permanently and
no late or second frame can alter the seal.

## Serialized terminal arbiter

Replace independent stream and endpoint completion decisions with one
serialized invocation arbiter. It alone orders stdout chunks, stderr chunks,
submission attempts, endpoint/protocol faults, parent cancellation, timeout,
process exit, post-check and cleanup facts, and owns one terminal latch.

Stdout, stderr, result text, input, submission, and frame bytes each have
explicit hard counters. Raw stdout is capped by the lesser of the protocol
maximum and the configured text limit plus a fixed maximum result-envelope
allowance; strict decoding then enforces `text <= output_bytes`. Stderr has a
separate small hard maximum. Crossing either stream limit latches overflow,
closes endpoints, terminates the process group, waits for it, and returns no
handoff. Bounded diagnostic retention may be shorter than the hard limit, but
overflow is failure, never successful truncation.

Cancellation, timeout, non-zero exit, malformed or extra stdout, typed
non-completed transport status, endpoint fault, source mutation, or cleanup
failure likewise latch failure, terminate the full group with bounded
SIGTERM/SIGKILL escalation, wait, and release no artifact or decision.
Transport failure is operational only and never becomes `FAIL`, `BLOCKED`,
`REVISE`, or another Baton outcome.

Any submission attempt also latches the invocation terminal, returns its one
seal response when possible, closes the endpoint, kills the complete process
group, and waits; this prevents a later model or tool turn. An accepted seal is
provisional until process-tree quiescence and all post-checks. Parent
cancellation or timeout already observed, concurrently observable before
quiescence, or ordered before publication suppresses it. Deterministic
precedence is cancellation/timeout, overflow/protocol/transport failure,
submission attempt, then ordinary clean exit. A late exit, stdout chunk,
frame, or replay cannot revive a failed invocation. Race tests synchronize
each ordering rather than depend on timing.

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

Usage has an explicit `reported` or `unavailable` state. Reported token fields
are non-negative safe integers, so reported `(0,0)` remains a legitimate zero;
unavailable carries no token fields. Provider cost is independently either
reported with non-negative micro-units, currency and
`provider_reported` source, or unavailable. Partial states, inferred values,
pricing tables, estimates, and converting transport failure into zero are
rejected.

## Implementation sequence

1. Reconfirm the exact owner head, clean worktree, eligible status, plan,
   approval, dependency, this design digest, and a current Captain
   `PROCEED`. Stop before using RC2 product input if any binding differs.
2. Recompute all three full-index patch digests, the one-shot patch digest,
   and final subtree. Replay only `internal/driver` product bytes onto the RC3
   materialization; import no old record or evidence bytes.
3. Bind canonical operations and permissions to the admitted public RC3
   identity; generalize stale RC2 names; enforce explicit four-role selection
   and production Merge refusal.
4. Implement the outside-worktree input projection, fixed guest mount graph,
   forbidden-surface canaries, Verifier admission checks, and read-only source
   mutation detection.
5. Implement the first-attempt sealed endpoint and serialized terminal
   arbiter, including hard stdout/stderr limits, process-group cleanup and
   deterministic cancellation/submission precedence.
6. Make usage availability explicit and extend the shared fake and adversarial
   process helpers.
7. Run the focused, adversarial, race, shuffle, repeat, full, vet, build,
   formatting, scope, history, budget, reproducibility and proof gates. Create
   one product-only candidate only when all pass.

# Surfaces

Product files are limited to:

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

No configuration or Baton record changes are designed. Preserve the approved
post-W2 composition at exactly 29 non-generated runtime-production Go files
and 10,464 physical lines under `cmd/**` and `internal/**`, using the approved
classifier. Added arbiter/isolation lines must be offset by removing duplicate
RC2 machinery or prose; tests and fixtures are reported separately and cannot
offset production lines.

# Consequential decisions and risks

- **RC2 replay could masquerade as authority.** Mitigation: gate replay on a
  fresh RC3 Captain decision, verify exact product patches/tree, bind the
  admitted RC3 package plus current request/input identities, and exclude all
  old control history.
- **Equal operation bytes could hide a stale package.** Mitigation: validate
  the public RC3 tag/commit/manifest identity independently of operation
  bytes, and test an equal-byte old-lineage replay refusal.
- **Mount-path or symlink tricks could expose authority or mutate source.**
  Mitigation: descriptor-pinned regular files/directories, canonical paths,
  closed guest mounts, outside-worktree input staging, forbidden canaries, and
  before/after manifests.
- **Submission/cancellation races could release a decision after failure.**
  Mitigation: one arbiter and terminal latch, deterministic precedence,
  provisional seals, process-tree quiescence before publication, and
  barrier-controlled race tests.
- **Output flooding could bypass memory bounds or look successful after
  truncation.** Mitigation: hard per-stream counters trigger termination;
  truncation is never a success path.
- **A rejected submit could continue model execution.** Mitigation: every
  attempted submit closes admission and terminates/waits for the process tree.
- **Bubblewrap or process-group behavior may differ by host.** Mitigation:
  require the trusted root-owned `/usr/bin/bwrap` feature set, prove Linux
  mount/network/process behavior, and fail closed elsewhere.
- **The stronger boundary could exceed the ratified baseline.** Mitigation:
  keep the existing file inventory, consolidate duplicated parsing/control
  code, and hard-stop unless the exact 29-file/10,464-line composition is
  reproduced.

# Evidence plan

## Acceptance mapping

- **A-W2-contract:** strict codec and canonical-operation tests; exact RC3
  identity test; explicit per-role driver/model resolution; production Merge
  refusal with portable `merge/model:null` fixture; ordered-input
  mutation/reorder tests; guest visibility canaries; role-shape table tests;
  replay/conflict/cross-invocation seal tests; shared fake conformance.
- **A-W2-isolation-usage:** writable and non-fresh Verifier pre-start refusal;
  read-only content/mode/symlink/source-mutation cases; outside-worktree input
  and cleanup checks; stdout and stderr flood termination; timeout,
  cancellation, descendant cleanup, malformed transport and no-handoff
  checks; accepted/rejected submit termination; synchronized
  cancel-before/during/after-submission races; reported non-zero, reported
  zero, unavailable and invalid partial usage/cost cases.
- **A-W2-rebind:** exact RC2 patch/subtree verification log, RC3-only changed
  paths, renamed/generalized tests, old-lineage/equal-operation-byte replay
  refusal, fresh candidate/proof identities, and no reference to an RC2
  verdict or proof as acceptance.

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

The process tests must explicitly report Bubblewrap feature/provenance,
workspace/input mount visibility, process-group descendants after
termination, hard stdout/stderr counters, terminal event order, and absence of
a handoff on every failure path.

Scope evidence lists every path and proves the candidate delta is wholly under
`internal/driver`, with no `.baton/releases`, module, generated, or
out-of-scope change. History evidence binds the materialization head, fresh
Captain result, implementation start, candidate ancestry, and proves no
product commit preceded `PROCEED` and no old lineage or hidden post-candidate
product bytes were imported. Budget evidence records the byte-defined
classifier and exact composed 29-file/10,464-line result. Run the official
build twice from independent product-only copies with fresh caches and compare
binary digests.

Fresh `proof.md` must bind repository, materialization base, frozen dependency,
candidate commit/tree, product-tree digest, plan, approval, this design,
Captain and Implementer invocation identities, admitted RC3 package identity,
ordered input/request/permission digests, changed-file and physical-line
manifests, check-output digests, and the adversarial observation matrix. It
must state that RC2 records/design/proof/PASS/history were not reused.

# Stop conditions

Stop without a candidate or transition on any stale/foreign status, head,
plan, approval, dependency, design or Captain binding; dirty initial
worktree; absent `PROCEED`; RC2 patch/tree mismatch; imported old control
bytes; out-of-scope path; public RC3 identity mismatch; operation/input/order
drift; production Merge dispatch; missing explicit model; writable or
non-fresh Verifier; unavailable or incomplete isolation; forbidden canary
visibility; source mutation; incomplete process-tree cleanup; overflow that
does not terminate; any transport failure releasing a handoff; late/replayed
submission changing a seal; flaky/racy/failed check; non-reproducible build;
or a composed result other than exactly 29 production files and 10,464
production lines.

# Revisions

Initial RC3 design. It uses the audited RC2 product tree only to establish
feasibility and explicitly adds RC3 package/lineage binding, outside-worktree
input projection, Verifier admission, a serialized terminal arbiter, hard
stderr handling, submission-attempt termination, race canaries, and exact
post-W2 budget preservation. No RC2 design bytes were reused.
