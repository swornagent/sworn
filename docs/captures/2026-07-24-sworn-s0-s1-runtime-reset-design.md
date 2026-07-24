# Sworn S0/S1 runtime reset design

Date: 2026-07-24
Status: corrected after Captain REVISE; pending fresh Captain and Verifier review
Scope: S0 reset/conformance seam and S1 single-track walking skeleton
Implementation base: `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f`
Baton authority: local commit
`893f6fe8b6a52cebc8e7ccecc745ed5d138f3184` (1.0.0-rc.2)

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

This document is not Captain-approved. Its implementation gate remains closed
until a fresh Captain reviews these exact bytes and an independent Verifier
reviews the resulting candidate and proof.

## Binding authority and baseline

The protocol contract is the exact bytes at Baton commit `893f6fe…`:

- `baton/PROTOCOL.md`;
- `reference/records/actions.mjs`;
- `reference/records/transition.mjs`;
- `reference/records/records.mjs`;
- `reference/records/git.mjs`;
- `reference/driver/contract.md`;
- `conformance/manifest.json`; and
- `conformance/engine-adapter.md`.

S0 records the released annotated tag object, peeled commit, tree, package
archive digest, every embedded asset digest, operation-document digests, schema
digest, and conformance manifest digest. Startup checks the compiled asset
manifest before admitting a run. A missing publication, lightweight tag,
changed byte, incomplete asset set, unknown schema/operation version, or
digest mismatch fails closed.

The greenfield scope is the exact capture at base `6ab7dc…`. The v0.2 kernel
and archived v0.3 construction are archaeology, not sources. Useful invariants
to restate in new tests are:

- atomic command/effect insertion and idempotency conflict;
- immutable results and stop-before-retry for uncertain effects;
- exact candidate objects, expected revisions, and ref compare-and-set;
- private read-only verification and contained child processes;
- no cleanup until a writable process tree is proven quiescent;
- old/foreign SQLite databases never migrate on open; and
- paths, symlinks, replacement objects, hooks, config, credentials, and
  canonical Git metadata fail closed at process and worktree boundaries.

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

S1 begins with an externally approved strict Baton plan. It supports one run
and one track, but uses the exact release and track authority refs required by
Baton. It performs:

```text
capture committed plan, records, approval evidence, and all authority heads
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

Planner, Implementer, Captain, and Verifier author bytes or decisions through
the submission proxy. Merge is deterministic engine-owned action execution;
no model chooses refs, parents, tree, merge mode, target, or commit message.

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
| `installApprovedPlan` | install exact plan and baseline statuses |
| `reboundPristinePlan` | replace only a pristine unmaterialised plan |
| `recordTransition` | validate and record ordinary work/assembly transition |
| `materializeTrack` | create owner marker and atomically move release/create track |
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

### `sworn.submission/v1` action proxy

Sworn owns one strict, versioned logical envelope for external fake, native
CLI, and HTTP tool-loop drivers. It is transported on a control endpoint
outside the candidate workspace:

- fake and native CLI processes receive an inherited, invocation-bound control
  descriptor whose address and credentials are not placed in model text;
- HTTP tool loops expose the same operation as an allowlisted server-side tool
  bound to the invocation; and
- all transports decode to the same Go `sworn.submission/v1` value before any
  runtime or Baton code sees it.

The closed envelope binds:

```text
schema version
invocation id
ordered proposed artifacts: kind, canonical Baton path, digest, exact bytes
optional role decision and exact bounded evidence bytes
one requested Baton action plus only its logical RC2 arguments
```

Exact bytes use one canonical binary encoding in the envelope and are hashed
after decoding. The only artifact kinds are design, work proof, assembly proof,
work status, and assembly status. Decisions are closed to Captain outcomes,
Verifier outcomes, and `NO_VERDICT` where the current responsibility permits
them. Action requests are closed to the seven facade methods and their RC2
logical parameters; refs, object IDs chosen by the caller, Git commands,
paths outside the admitted record paths, commit messages, merge modes, and
arbitrary effect descriptions are forbidden.

Before dispatch, runtime mints an invocation capability bound to role,
operation tuple, current authority ref/head, fact digest, plan and approval,
work/track/release identity, candidate/product identity, permitted artifact
kinds, permitted decision/action, workspace mode, freshness, and limits. The
capability is held by the control endpoint, not serialized into the workspace,
prompt, input record, model output, or candidate. The proxy accepts at most one
final submission and binds its digest to the effect receipt.

Admission is:

1. validate strict envelope syntax, size, uniqueness, byte digests, invocation,
   and invocation capability;
2. reread current canonical facts and reject any changed binding;
3. resolve protected evidence outside the workspace;
4. pass exact proposed records and opaque admission to the Go Baton facade;
5. prospectively validate every transition, commit, and ref transaction; and
6. only then journal and execute the permitted action.

For Captain or Verifier, invalid/missing submission yields operational
`NO_VERDICT`: durable Baton status remains byte-for-byte unchanged. For an
authoring or mechanical responsibility it is an operational failure with no
Baton transition. Transport failure is never converted to `FAIL`, `BLOCKED`,
`PASS`, or `MERGED`.

## Trusted evidence and opaque admission

The evidence resolver is engine-owned and never mounted or described to an
agent. Given an exact status and execution profile, it reads protected approval
and Verifier-dispatch evidence by canonical reference, verifies exact bytes,
digest, provenance, candidate read-only access, clean/fresh context,
invocation separation, and profile-specific isolation, then returns immutable
bytes plus provenance to `internal/baton`.

The facade mints an opaque, unforgeable in-process admission bound to:

- exact status bytes/digest and action;
- plan, approval, authority ref/head, target and component heads;
- execution profile (`guided` or `autonomous`);
- exact resolved evidence bytes/digests and provenance;
- candidate, candidate tree, product tree, workspace mode, and freshness; and
- current record-root behavioral-inertness result.

Admissions are single-snapshot values, never accepted from JSON, the journal,
the board, a candidate file, or a driver. They are not visible to agents and
cannot be copied to a changed status or profile. Recovery resolves fresh
evidence after recapturing refs; a stored evidence digest is diagnostic and
does not recreate admission.

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
baton-action.apply          verifier-workspace.ensure
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

Plan installation makes a record-only commit from the target and atomically
creates the release ref while verifying target and absent owner refs.
Materialisation makes one record-only marker containing the release base and
dependency heads, then in one `update-ref --stdin` transaction updates the
release ref and creates the track ref at that same marker while verifying all
other bound heads. A partial materialisation cannot be observed as success.

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
| R0 reset/admission | deleted old tree, module files, CI | exact RC2 identity/assets; old DB rejected |
| R1 Go Baton compatibility | `internal/baton/**` | all seven actions, validators, goldens, portable fixtures |
| R2 journal | `internal/journal/**` | strict schema, idempotency, claims, uncertainty |
| R3 Git boundary | `internal/gitx/**` | isolation, object identity, atomic old/new/third corpus |
| R4 proxy/process/fake | `internal/driver/**` | shared envelope, external fake, containment, receipts |
| R5 walking skeleton | `internal/runtime/**`, `cmd/sworn/**` | exact one-track authority, assembly, integration |
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
5. common submission envelope and external fake containment;
6. role output admission, missing/invalid submission, and stale capability;
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
- a fresh Captain returns `PROCEED` for this corrected design;
- the approved plan assigns the non-overlapping slices and exact authority refs;
- initial tests enumerate every S1 effect, action, failure, and reconciler; and
- base, target, release, track, evidence, and package bindings are recaptured
  immediately before the first source-changing commit.

If released bytes change package ownership, action semantics, record validation,
composition, or the shared proxy boundary, stop for an authorised plan
revision. Field-name translation that preserves exact RC2 behavior stays inside
`internal/baton`; weakening or partially implementing the contract does not.
