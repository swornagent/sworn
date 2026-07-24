# Sworn S0/S1 runtime reset design

Date: 2026-07-24
Status: Captain-approved implementation design; Baton package admission blocked
until publication
Scope: S0 reset/conformance seam and S1 single-track walking skeleton
Worktree: `/home/brad/projects/sworn-worktrees/v0.3.0-P1-runtime-reset`

## Captain outcome

Replace the current production kernel. Do not refactor it into the new runtime.
Keep the one SQLite dependency and port only failure-preventing invariants,
each introduced by a new focused test. The S0/S1 implementation has six
production packages, one command service, one SQLite journal, one scheduler
loop, one external-process boundary, and no mutable copy of Baton's lifecycle.

S0 may start destructive source replacement only after the intended Baton RC2
has both an annotated tag and a published release. Admission must bind the tag
object, peeled commit, source tree, package bytes, package digest, and operation
contract supplied by that release. This capture deliberately names no RC2 tag,
commit, schema digest, or conformance claim.

S1 proves one approved, one-track delivery through a deterministic external
fake: derive the next Baton operation, claim one track, use isolated Git
worktrees, obtain a fresh track verification, compose with expected-old-value
checks, obtain a separate fresh assembly verification, and move the exact
target ref with compare-and-swap. Every external-effect edge is crash-cut and
reconciled before any retry.

## Audited baseline

### Exact refs and trees

The audit was read-only and limited to this checkout's Git object database and
working tree.

| Fact | Exact audited value |
| --- | --- |
| current branch | `refs/heads/prep/v0.3.0-runtime-reset` |
| current/base commit | `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f` (`docs: add Sworn site release cutover`) |
| base tree | `88b1174df81be536e7c0e1cb0051fabeb3166b77` |
| local `release/v0.3.0` | `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f` |
| local `origin/release/v0.3.0` | `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f` |
| `origin/main` | `009ea6476d75376975a75907e6c69686a5f6d6b9` |
| `origin/main` tree | `409a98b3052fb5ceb254fc2a257ed21dc7e5ac2e` |
| annotated `v0.2.0` tag object | `fc9ce1cf7b0a70addb855787dc3803cbf1a624ba` |
| `v0.2.0` peeled commit | `009ea6476d75376975a75907e6c69686a5f6d6b9` |
| annotated archive tag object | `7ec0df2d737c153ef0eca00f0318332f6862e00f` |
| archive tag | `refs/tags/archive/pre-baton-rc2-kernel-2026-07-24` |
| archive peeled commit | `ae387d22691b7aadfac392fa6ec44ff65fd700b0` |

The base differs from `origin/main` by only
`docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md`. Its four commits
after `origin/main` are:

1. `969157ad77db543ae6a50ca7bab0ee3104c1f55f` —
   `docs: define the lean Sworn v0.3 rebuild`;
2. `ccde1ebe9d55212003cc9899952ea5759edd41c6` —
   `docs: link the v0.3 rebuild epic`;
3. `aa95b3da022d43915b49c28ef7b073967d6a31ef` —
   `docs: bind Codex automation flags`; and
4. `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f` — the current base.

Only the current full commit is an implementation input; abbreviated hashes
above identify adjacent documentation history, not pinned dependencies.

No local tag matched `*rc.2*` at audit time. No remote release lookup or package
admission was performed, so this is not a claim about publication state outside
the audited object database.

### Code, tests, schema, and dependencies

At the base:

- 67 non-test production Go files contain 20,443 physical lines;
- 66 Go test files contain 22,523 physical lines and 336 named tests;
- 16 production packages exist: `cmd/sworn` plus 15 `internal/*` packages;
- four production files exceed 700 lines, led by
  `internal/store/effects.go` at 1,271 and
  `internal/policy/authority.go` at 1,236;
- eight migrations total 753 SQL lines and leave 11 durable domain/journal
  tables;
- `go.mod` uses Go 1.26.5 and has one direct dependency,
  `modernc.org/sqlite v1.54.0`, nine explicitly listed indirect dependencies,
  and 26 modules in the resolved build list; and
- `go.sum` has 51 lines.

`GOFLAGS=-buildvcs=false go test ./...` passes all packages in this linked
worktree. The flag is needed by tests that launch nested `go build` commands;
it changes VCS stamping only, not test semantics.

The present package graph makes Sworn own plan parsing, canonical JSON,
authority grants, assurance selection, work phase/state, submission admission,
board projection, builder/check lifecycle, workspace artifacts, executor
capabilities, and adapter policy. Those are the wrong ownership seams for the
approved architecture: Baton now owns protocol and lifecycle truth, while
Sworn owns runtime effects and recovery.

### Relevant archaeology

These commits were inspected for failure statements, not as code sources:

| Commit | Useful evidence |
| --- | --- |
| `08d5a14746bfd62072e69c3fa806d9b149dc43e0` | atomic command/effect transaction and idempotency |
| `8c15da153a45c33604a5236b0c3e0afca0670f62` | exact candidate objects and ref CAS |
| `fb0593e31b060b3c42431569c4676ed22f28c667` | contained read-only subprocess boundary |
| `056d7bb8a2f016eb75585a4562b54ce0eb1f2cbf` | no blind retry of interrupted effects |
| `0cb058d47332809fd40d6ba67443572468bfb933` | writable-process quiescence before cleanup |
| `3601bb82bb1c53cf8b1eeb263fea73022cb6577d` | native CLI/tool capability split |
| `221ee4c8c43dd7515cb99a366805370e4ad25d66` | bounded real-binary vertical |
| `89d3966782a4f7eb162eb46bb3b60e34ab65ef0e` | credential-free read-only verifier boundary |

The archived construction adds four post-base commits and changes 62 files
with 15,964 insertions and 542 deletions. In particular,
`bd7a9c0c407faee56e5b6f85181d267f5614d9e4` and
`74e0dd38e3a433b6b0be7f738224e9ae97473ff3` add a second verifier lifecycle
through Store, engine, policy, protocol, effects, and adapter layers. That is
evidence that extending the current abstractions increases duplicated Baton
state; it is not a base for S1.

## Delete, retain, and rebuild

No current production Go package is reused verbatim.

| Current surface | S0 action | What survives |
| --- | --- | --- |
| `internal/protocol`, embedded RC1 snapshot | delete once the published replacement is admitted | strict-package and digest tests rewritten against released Baton bytes |
| `internal/engine`, `internal/control` | delete | atomic command/effect and stop-before-retry invariants only |
| `internal/store`, all eight migrations | delete; start a new incompatible database identity and schema | private file, `application_id`, `user_version`, `foreign_keys`, `trusted_schema=OFF`, `synchronous=FULL`, one-connection tests |
| `internal/repo`, `internal/workspace` | delete | Git object validation, sanitized Git environment, literal path scope, readback, and `update-ref --no-deref NEW OLD` tests |
| `internal/executor`, `internal/effects` | delete | bounded output, cancellation, child-tree quiescence, read-only workspace, and stale-owner tests |
| `internal/adapter`, `internal/producer` | delete | one role-independent invocation/result shape; no provider code in S0/S1 |
| `internal/policy`, `internal/config` | delete | explicit configured runner/model and fail-closed unknown capability, implemented at the new seam |
| `internal/board` | delete | Baton board is called as a read-only projection; runtime overlay is deferred |
| `internal/app`, `internal/buildinfo` | delete | thin CLI lifecycle and version output rebuilt in `cmd/sworn` |
| existing white-box tests | delete with owners | only named black-box failure cases are rewritten before production behavior |
| `go.mod`, `go.sum` | retain module identity; minimize after deletion | Go directive and the single SQLite direct dependency |
| CI/release workflows, licence, release docs, historical ADRs | retain | history and repository policy, not runtime authority |

Focused tests should preserve these old failure statements: idempotency conflict,
atomic effect insertion, immutable result binding, interrupted effects never
retry blindly, late leases cannot publish, completion/recovery races converge,
candidate ref collision fails closed, target movement blocks integration,
read-only database open never migrates, process cancellation kills descendants,
writable recovery waits for quiescence, and path replacement/symlink attacks
fail before mutation. Test code and fixtures may be newly expressed; production
implementations may not be copied.

## S0/S1 product cut

### S0: reset and immutable Baton seam

S0 does only the following:

1. Verify that the selected Baton release has an annotated tag and release
   publication, then record its exact tag object, peeled commit, tree, package
   bytes, package digest, operation contract identity, and fixture manifest.
2. Replace the old production tree and database schema.
3. Embed or otherwise make immutable the exact released package selected by
   the audited release mechanism. Startup recomputes and checks its digest.
4. Expose Sworn version plus Baton identity/digest without describing the
   package as Baton v1.0.0 final.
5. Run Baton's portable driver and engine fixtures through the real built
   `sworn` harness. Claims list only fixtures actually passed.

If the release package shape differs from the seam below, S0 adjusts the
adapter after publication; it does not translate or weaken the package.

### S1: one-track walking skeleton

S1 begins with an already externally approved Baton plan and exact approval
evidence. It supports one run and one track but uses track and release claims
that remain valid when concurrency is added later. The end-to-end sequence is:

```text
read exact committed records and refs
  -> Baton derives one exact operation
  -> accept operation + create effect + claim track atomically
  -> execute one external edge outside SQLite
  -> bind or reconcile an immutable result
  -> re-read records and refs and derive again
  -> fresh read-only track verification
  -> expected-old-value release composition
  -> separate fresh read-only assembly verification
  -> exact target update-ref compare-and-swap
```

Planner/design/Captain/Implementer details remain opaque Baton operations.
Sworn dispatches the operation and enforces its declared role, workspace
access, freshness, and limits. It does not store a mutable enum saying that a
work item is designed, approved, implemented, verified, or merged.

Deferred from S0/S1 are real model drivers, parallel dispatch, user-facing
pause/resume/cancel/retry/takeover, HTTP providers, telemetry export, cockpit,
and DBOS. The schema has no speculative columns for them.

## Production packages and interfaces

The walking skeleton has exactly these six production packages:

```text
cmd/sworn         parsing, signals, process lifetime, version, real-binary harness
internal/baton    admitted package identity, validation, operation derivation
internal/runtime  command service, scheduler, recovery, effect dispatch
internal/journal  SQLite transactions, claims, receipts, events, outbox
internal/gitx     repository identity, worktrees, object facts, exact ref CAS
internal/driver   role-independent contract, contained process, deterministic fake
```

There is no shared `model`, `types`, `effects`, `policy`, `executor`, or
`util` package. Types live with their owner; consumer-side narrow interfaces
are declared only where a focused fake is needed.

### `internal/baton`

The concrete package adapter owns:

```go
type PackageIdentity struct {
    TagObject, Commit, Tree, PackageDigest, OperationContract string
}

type Facts struct {
    RepositoryID string
    Refs         []BoundRef
    Records      []BoundRecord
    PolicyDigest string
}

type Decision struct {
    OperationBytes  []byte
    OperationDigest string
    TrackID         string
    Role            string
    Workspace       WorkspaceRequirement
    FreshContext    bool
    Terminal        bool
}

func OpenPinned() (*Package, error)
func (p *Package) Derive(Facts) (Decision, error)
func (p *Package) ValidateResult(Decision, []byte) (ValidatedResult, error)
```

`Facts` are assembled from Git object IDs and exact record bytes, sorted and
domain-separated before hashing. `Decision.OperationBytes` are the exact
package-produced driver input. The adapter makes defensive copies. Unknown
fields, capabilities, roles, workspace modes, result kinds, or terminal facts
fail before an effect is inserted. The final mapping to published Baton types
is an S0 task; the names above are Sworn-internal and claim no unpublished
schema.

`Derive` is pure and repeatable for identical facts. The decision is never
updated in place. A later pass always rereads Git and derives anew.

### `internal/journal`

`Store.Apply` is the only state-changing entry point used by the command
service. Narrow query/claim methods return copied values and opaque lease
tokens:

```go
func (s *Store) Apply(context.Context, Command) (Receipt, error)
func (s *Store) Claim(context.Context, ClaimRequest) (Lease, error)
func (s *Store) Renew(context.Context, Lease) error
func (s *Store) Bind(context.Context, Lease, EffectReceipt) error
func (s *Store) Events(context.Context, RunID, AfterSequence) ([]Event, error)
```

`Bind`, renewal, expiry, and reconciliation are internally represented as
closed typed commands and pass through the same transaction/reducer as
operator commands. The exported methods do not contain a second reducer.

### `internal/gitx`

`Repository` is concrete and admits one canonical common Git directory, object
format, exact `git` executable identity, and repository ID. Its small surface is:

```go
func Open(Binding) (*Repository, error)
func (r *Repository) ReadFacts(context.Context, FactRequest) (baton.Facts, error)
func (r *Repository) EnsureWorktree(context.Context, WorktreeIntent) (Observation, error)
func (r *Repository) ReconcileWorktree(context.Context, WorktreeIntent) (Observation, error)
func (r *Repository) Compose(context.Context, CompositionIntent) (Observation, error)
func (r *Repository) UpdateRefCAS(context.Context, RefIntent) (Observation, error)
```

This is not a general Git library. `CompositionIntent` is created only from a
validated Baton decision and binds source commit/tree, destination old
commit/tree, exact operation, output ref, and allowed paths. `RefIntent` always
contains a full ref, expected old OID (or the repository's zero OID), and new
commit OID.

### `internal/driver`

There is one role-independent boundary:

```go
type Request struct {
    InvocationID, OperationDigest, Role, RepositoryID string
    OperationBytes []byte
    CandidateOID   string
    Workspace      Workspace
    FreshContext   bool
    Driver, Model  string
    Limits         Limits
}

type Result struct {
    Driver, ObservedModel string
    Transport             TransportOutcome
    BatonBytes            []byte
    ExitCode              int
    Duration              time.Duration
    Diagnostic            string
    Capabilities          []string
}

type Driver interface {
    Invoke(context.Context, Request) (Result, error)
    Reconcile(context.Context, InvocationIdentity) (ProcessObservation, error)
}
```

The S1 fake implements this interface as a separate bounded subprocess, not an
in-memory shortcut. The same fake executable can deterministically edit,
commit, pass, fail, emit `NO_VERDICT`, block, hang, exit, or crash at a named
step. Transport outcome and Baton outcome are distinct. An invocation error
never becomes a Baton verdict.

The contained-process implementation is private to `internal/driver`; it is
not another public package or driver-specific agent loop.

### `internal/runtime`

`Service` composes one admitted Baton package, journal, repository, and driver
set. Its only loop is:

```go
func (s *Service) Tick(context.Context, RunID) error
func (s *Service) Recover(context.Context, RunID) error
func (s *Service) Apply(context.Context, Command) (journal.Receipt, error)
```

`Tick` refuses to derive while the run has an unresolved uncertain effect or a
disabled dispatch gate. It snapshots Git facts, derives, revalidates the
decision against the package, then submits a typed accept/claim command with
the fact and operation digests. After each result it rereads facts; it never
advances a cached operation.

## Minimal SQLite journal

SQLite remains behind `database/sql` with `modernc.org/sqlite`. Use one
serialized connection, rollback journal mode, `synchronous=FULL`,
`foreign_keys=ON`, `trusted_schema=OFF`, DQS disabled, a bounded busy timeout,
a new application ID, and schema version 1. A read-only open uses
`mode=ro,query_only=ON`, never creates, and never migrates.

There is no migration from the v0.2 database. Opening an old or foreign
application ID fails with an explicit diagnostic and leaves the file
untouched.

The initial schema has seven tables:

| Table | Minimal columns and key | Ownership |
| --- | --- | --- |
| `runs` | `run_id` PK; immutable repository/package/plan/target/release bindings; `revision`; `dispatch_enabled`; bounded reason/timestamps | current Sworn runtime control only |
| `commands` | `command_id` PK; `run_id`; closed `kind`; `expected_revision`; exact payload bytes and digest; accepted time | append-only request identity |
| `effects` | `effect_id` PK; command + ordinal; operation/facts digests; scope; closed kind; exact request + digest; state; attempt; result receipt FK; timestamps | mutable external-effect journal, not Baton state |
| `claims` | PK `(run_id, resource_kind, resource_id)`; owner; random lease token digest; generation; effect/attempt; acquired/renewed/expires times | finite one-writer lease for track, release, target, effect, or outbox row |
| `receipts` | `receipt_id` PK; run; exactly one command/effect origin; attempt; closed outcome; exact body + digest; time; unique command result and effect-attempt result | append-only idempotent outcomes |
| `events` | monotonic integer sequence PK; stable event ID; run; optional command/effect; closed kind; exact body + digest; time | append-only operator/runtime history |
| `outbox` | `outbox_id` PK; unique `(event_sequence,sink)`; state; attempts; next time; claim generation; bounded last error | lossy asynchronous projection only |

All tables are `STRICT`. IDs, digest syntax, non-negative revisions/attempts,
bounded byte lengths, closed enums, origin exclusivity, and timestamp ordering
have `CHECK` constraints. Immutable tables have no-update/no-delete triggers.
`effects`, `runs`, `claims`, and `outbox` have triggers allowing only the
enumerated changes below. Every accepted or deterministically rejected command
gets exactly one immutable receipt. Infrastructure failures do not.

An outbox row is inserted in the same transaction as its event only when a
configured sink exists. Outbox loss, backoff, duplicate delivery, or permanent
failure cannot alter a run, command, effect, verdict, or exit status. The local
board/event stream reads `events`, not the outbox.

### Journal invariants

1. Reusing a command ID with identical bytes returns the original receipt;
   different bytes return `ErrIdempotencyConflict` without mutation.
2. A command transition, effects, receipt, event, and enabled outbox rows commit
   in one `BEGIN IMMEDIATE` transaction.
3. `(run, track)` has at most one live writer claim. Release composition and
   target movement use distinct singleton claims and still rely on Git CAS.
4. Every lease is store-instance, resource, generation, owner, effect-attempt,
   and expiry bound. A late or copied lease cannot bind.
5. An effect request and result are immutable bytes with separate digests.
   Result binding cannot change request identity or attempt.
6. An expired or ownerless executing effect becomes uncertain, never pending.
7. An uncertain effect blocks dispatch for its scope until reconciliation
   proves `performed`, `not_performed`, or an explicit inconsistency.
8. `not_performed` closes that attempt; retry creates a new effect identity.
9. Git records/refs and Baton records remain truth. `runs` and `effects` store
   only Sworn control and recovery facts.
10. Receipt or event text cannot manufacture `PASS`, `FAIL`, `BLOCKED`, or
    integration authority; only validated Baton bytes bound to current facts
    can do so.

## Command and effect transitions

S0/S1 command kinds are closed: create run, accept derived operation, claim,
renew, bind result, mark uncertain, record reconciliation, apply observed
result, and disable/enable dispatch. Only create run and run-to-completion are
initial CLI surfaces; the rest are internal commands with the same durable
receipt rules.

Run revision changes only through `Apply`:

```text
valid command + expected revision
  -> reducer validates current runtime projection
  -> one transaction writes command, new projection, effects, receipt, event
stale but valid command
  -> one transaction writes command + rejected receipt + event; no projection/effect
malformed or unknown command
  -> fail before write
```

Effect states are operational:

```text
planned   --claim + track lease------------------------> executing
executing --valid immutable result---------------------> observed
executing --owner death / expired lease---------------> uncertain
uncertain --external proof + reconciliation receipt---> observed
observed  --validate and consume receipt---------------> consumed
```

`observed` receipts distinguish performed success, performed operational
failure, proved not-performed, transport failure, cancellation, `NO_VERDICT`,
and inconsistent external state. Inconsistent state atomically disables
dispatch and remains visible; it is never converted to pending. `consumed`
means the runtime observation was applied and a fresh Baton derivation may
occur, not that Baton work passed.

The S1 effect-kind registry is closed:

```text
attempt.git.ensure
driver.invoke
candidate.import
track-ref.publish
release-worktree.ensure
release.compose
release-ref.publish
target-ref.publish
worktree.remove
```

The registry is a fixed switch, not a workflow language. A mutating operation
uses ensure, invoke, import, track publication, and cleanup. A verification
uses ensure, invoke, and cleanup against a private read-only worktree.
Composition uses the release-worktree, compose, release-publication recipe.
Exact Merge uses only target publication. Each next edge is derived from the
validated Baton operation plus immutable prior receipts; no generic graph or
user-defined step vocabulary exists.

A healthy worker renews its effect and scope claims while the subprocess runs.
Cancellation stops new dispatch, terminates the contained process tree, waits
for quiescence, and then binds cancellation. It does not edit Baton records.

## Worktree, process, and Git boundaries

### Repository and refs

Run creation binds the canonical common Git directory, object directory,
object format, sanitized exact Git executable, target ref/OID, release ref/OID,
approved-plan digest, and repository ID. Every engine use revalidates the
binding. The canonical common directory and object directory are never visible
in a driver child. Git commands use no shell, no prompts, no hooks, no
global/system config, no replacement objects, no lazy fetch, no credential
helper, and bounded output.

Engine refs use one validated namespace derived from the run ID:

```text
refs/sworn/runs/<run>/tracks/<track>
refs/sworn/runs/<run>/release
```

The target remains the approved full ref. All engine and target writes use
`git update-ref --no-deref <ref> <new> <expected-old>` followed by an exact
readback. No force update, symbolic target, remote push, merge command, squash,
or branch checkout is an S1 integration primitive.

### Worktrees

- Each driver attempt gets a fresh private Git repository with its own metadata
  and refs, seeded by the engine with only the objects needed for the exact
  input commit, plus a detached worktree. It has no remote or alternate path
  back to the canonical repository.
- A mutating role may write only its private attempt worktree and Git metadata.
  Its commits cannot update canonical objects or refs.
- Each verification gets a different private repository/worktree at the exact
  candidate commit, mounted or staged read-only for the child.
- The release has one dedicated engine-only detached composition worktree.
  Only this worktree may share canonical metadata, and no driver process can
  see it.
- Worktree paths are engine-derived beneath the private run root. Existing,
  symlinked, replaced, foreign, dirty, or wrong-HEAD paths fail reconciliation.
- Agents never receive the journal, canonical Git directory/object store,
  control refs, release worktree, other track worktrees, credentials, or
  another checkout.
- Agent-written files and commits are observations only. Sworn validates object
  parent/tree/path facts in a quarantine, imports content-addressed objects
  without accepting refs/config/hooks, and publishes an exact retention ref by
  CAS before the candidate can be used.

Seeding and import are engine-only Git operations. The implementation may use a
temporary bundle, pack, or quarantine repository, but the selected mechanism
must prove that no canonical ref, config, hook, index, or worktree is writable
through the attempt. Candidate import is its own journalled, idempotent effect:
all expected objects validate and exist canonically, or the import is not
complete; a partial temporary pack is disposable and never evidence.

`EnsureWorktree` is idempotent only for an exact private repository, registered
worktree path, HEAD, gitdir, mode, and clean-state observation. Otherwise
recovery stops for operator attention. Cleanup happens only after process
quiescence and retained Git facts make the workspace disposable.

### Driver processes

Every role, including the fake, runs as a new OS process in an attempt-specific
private root with a clean fixed environment, bounded stdin/stdout/stderr,
deadline, resource bounds, cancellation, and descendant-tree ownership.
Invocation identity and process-supervisor identity are written before launch.
Completion is first sealed to an fsynced, atomically renamed spool receipt,
then bound to SQLite.

S1 production execution is Linux-only and fail-closed. R4 rebuilds the narrow
proven shape with Bubblewrap for mount/PID/network isolation and a transient
systemd user service for cgroup ownership, limits, `KillMode=control-group`,
`Restart=no`, inspection, and kill-after-engine-death. Startup probes exact
configured executables, required namespace/mount behavior, cgroup v2
controllers, and the user manager before accepting a run. Exact minimum
versions are set by those new tests, not copied from the old package. Other
platforms may run pure package/unit fixtures but cannot claim S1 contained
delivery until they provide the same process and crash corpus.

On engine death, recovery proves the old process tree is absent or terminates
and waits for it before inspecting or deleting the attempt root. A live or
unidentifiable process keeps the effect uncertain. PID alone is not proof;
the implementation uses a deterministic invocation-bound service unit,
recorded unit properties, and an attempt lock proven by the process-boundary
tests.

Verifier requests must set fresh context and read-only workspace. They receive
only exact operation/record bytes and the candidate under review, with empty
home/config/session directories and no inherited rules, memory, conversation,
or writable Git metadata. After exit Sworn proves the candidate OID/tree,
worktree manifest/status, and relevant refs did not change. Failure of either
OS enforcement or after-the-fact proof is an operational failure, never a
verdict.

Track verification and assembly verification are separate invocations with
different invocation IDs and operation digests. Assembly verification always
runs, even with one track or when composition yields the same commit OID. Its
verdict binds the composed release candidate and the still-current expected
target.

### Composition and exact Merge

The Baton decision supplies the exact composition inputs and ordering. Sworn
checks source candidate and destination release old values, performs the
specified Git object operation in the release worktree, validates output
parent/tree/path facts, then CAS-updates the release ref. A conflict or third
value disables dispatch; Sworn does not invent conflict resolution.

After a fresh assembly `PASS` is admitted by Baton and a final derive still
requests integration, exact Merge is:

```text
git update-ref --no-deref <target-ref> <verified-release-oid> <expected-target-oid>
```

Readback must equal the verified release OID. This creates no new commit. A
target move is a stale candidate, not a reason to merge, rebase, or retry.

## Uncertain-effect reconciliation

Reconciliation runs before normal scheduling on every activation and whenever
a claim expires. It uses the same typed commands and receipts as normal
completion.

| External edge | `performed` proof | `not_performed` proof | Inconsistent/blocked |
| --- | --- | --- | --- |
| create private repository/worktree | registered exact path/gitdir/HEAD/mode exists; canonical store is unreachable | path and registration both absent | foreign, dirty, replaced, mismatched, or canonical path exposed |
| mutating driver | sealed receipt matches invocation; process quiescent; candidate facts validate | process quiescent and no sealed receipt; discard isolated root | live/unidentifiable process, foreign receipt, unverifiable Git object |
| read-only verifier | sealed receipt matches invocation and read-only post-proof passes | process quiescent and no sealed receipt; fresh rerun allowed | mutation, live process, identity mismatch, result bound to wrong candidate |
| import candidate objects | every expected object validates in canonical store; an exact partial content-addressed import may be completed by reconciliation; no ref changed | no complete candidate closure is present and temporary residue is safely removed; unreachable valid objects are harmless | malformed object, unexpected ref/config change, or source identity loss during partial import |
| publish track ref | ref equals requested new OID | ref equals exact expected old OID | any third OID, symbolic ref, invalid object |
| compose release | sealed composition receipt and output OID validate against exact source and destination old OIDs; worktree is clean at output | no sealed receipt and engine worktree is clean at the exact destination old OID | conflict, dirty/foreign worktree, or unverifiable output |
| publish release ref | ref equals requested output OID | ref equals exact expected old OID | third value or object mismatch |
| assembly verifier | same as read-only verifier, bound to release OID and target old | same as read-only verifier | mutation or stale candidate/target binding |
| exact target Merge | target equals verified release OID | target equals expected old OID | any third OID or symbolic target |
| remove worktree | registration and exact path absent | exact worktree still present and quiescent, so removal may run | replaced path, foreign registration, active process |

For driver effects, absence of a result is not automatically `not_performed`.
Quiescence plus absence of an authoritative publication and disposal of the
isolated attempt resolves the uncertainty. The retry is a new invocation and
new worktree. For Git ref effects, only the expected old/new/third-value
trichotomy is accepted. S0/S1 configure no outbox sink, so there is no outbox
send effect in the crash registry.

## Crash-injection contract

Every external edge above exposes test-only cuts at the same four points:

1. after durable intent/claim commit, before the external call;
2. after the external call starts or makes partial progress;
3. after the external result or Git mutation, before SQLite result binding; and
4. after immutable result binding, before apply/rederive.

SQLite transitions also cut immediately before and after transaction commit.
The harness kills the real `sworn` process with no deferred cleanup, restarts a
new process on the same journal/repository, runs recovery twice, then completes
the delivery. For every cut it asserts:

- at most one live track writer and one release writer;
- no uncertain attempt is redispatched;
- stale workers and leases cannot bind;
- recovery is idempotent;
- no Baton status or verdict is synthesized;
- only the exact expected refs move;
- verifier worktrees remain unchanged;
- no live process or disposable worktree remains; and
- the final target OID is exactly the assembly-verified OID.

A failpoint is an injected dependency at the edge, not production branching or
environment-variable behavior. The crash corpus enumerates all registered
edges and fails if a new external-effect kind lacks the four cuts and a
reconciler.

## Failure and recovery matrix

| Failure | Immediate durable state | Recovery/next action |
| --- | --- | --- |
| crash before command commit | no command/effect, or complete prior transaction | replay same command ID |
| crash after command commit, before claim | `planned` effect | claim once after fresh fact validation |
| crash after claim, before call | `executing`, later `uncertain` | prove edge not performed, close attempt, create new effect |
| process hangs or engine is cancelled | executing with live supervisor | stop tree, prove quiescence, bind cancellation |
| process exits without sealed receipt | uncertain | prove quiescence/publication absence, discard root, record not-applied |
| result sealed, DB bind missing | uncertain | validate sealed receipt and bind idempotently |
| DB bind succeeds, apply missing | observed | apply same immutable receipt, reread facts |
| worker returns after lease loss | later generation owns scope | reject stale bind; reconcile old attempt |
| SQLite busy/IO/commit error | no inferred transition | stop; retry only the database command after inspecting receipt |
| worktree path replaced/symlinked | uncertain/inconsistent | disable dispatch; never remove the foreign path |
| candidate changes outside approved paths | operational failure receipt | Baton does not see a successful result; repair via new attempt if derived |
| track verification transport failure | transport failure, no Baton verdict | fresh derive decides whether another invocation is allowed |
| verifier emits malformed/unknown result | operational `NO_VERDICT` | no pass/fail mapping; fresh derive |
| release composition conflict | inconsistent; release dispatch disabled | external repair/replan; no automatic conflict resolution |
| target moves before Merge | exact Merge not applied | invalidate assembly assessment and rederive |
| crash after target CAS | uncertain Merge effect | read target; equal new means applied, equal old means not applied, third blocks |
| outbox unavailable/overflowing | delivery retry/drop event | bounded projection loss only; delivery continues |
| unknown Baton capability/state/field | no effect inserted | stop closed and require package/runtime update |

## Implementation slicing and touchpoints

These are ordered work items, not an admitted Baton plan. Their final plan bytes
and approval must be created only after the Baton release gate.

| Slice | Dependencies | Exclusive paths | Acceptance |
| --- | --- | --- | --- |
| R0 reset and release admission | published annotated Baton tag + release | all deleted old production/test paths; `go.mod`, `go.sum` | exact package identity recorded; old DB rejected; no old production package remains |
| R1 Baton seam/conformance | R0 | `internal/baton/**` | package self-check and portable fixtures pass through built harness |
| R2 journal kernel | R0 | `internal/journal/**` | schema, command idempotency, claims, receipts, uncertain barrier, read-only tests |
| R3 exact Git boundary | R0 | `internal/gitx/**` | isolated worktrees, candidate validation, old/new/third CAS corpus |
| R4 fake/process boundary | R0 | `internal/driver/**` | external fake, fresh read-only mode, limits, quiescence, sealed receipts |
| R5 runtime walking skeleton | R1-R4 | `internal/runtime/**`, `cmd/sworn/**` | one track reaches distinct assembly verification and exact Merge |
| R6 crash and real-binary proof | R5 | package-local tests plus one `test/e2e/**` harness if needed | every registered edge passes all crash cuts and restart-twice proof |

R1-R4 may run independently after R0 because their production paths do not
overlap. R5 is the sole composition owner for `cmd/sworn` and runtime wiring.
R6 does not make opportunistic cross-package fixes: a failure returns to the
owning slice, is repaired there, and is recomposed.

Shared touchpoints are deliberately few:

- R0 alone changes `go.mod`, `go.sum`, CI, application ID, and old-tree
  deletions.
- `internal/runtime` owns orchestration types; other packages expose their own
  values rather than editing a shared model package.
- each owner keeps its fixtures and tests beside its package;
- only `cmd/sworn` imports all runtime components; and
- the end-to-end harness consumes the binary and public CLI, not internal
  package hooks except the compiled test failpoint registry.

## Test-first build order

1. Release admission tests reject a lightweight tag, missing release evidence,
   wrong peeled commit/tree, altered package byte, digest mismatch, incomplete
   fixture manifest, and unknown operation contract.
2. A built-binary fixture proves the admitted Baton identity and one portable
   fake-driver operation; only then remove RC1 protocol code.
3. Journal tests define the seven-table schema, foreign/old DB rejection,
   private/read-only opens, strict commands, idempotent receipts, atomic effect
   creation, immutable events, and disabled outbox behavior.
4. Claim tests prove same-track exclusion, independent-scope eligibility,
   lease generation/expiry, late-owner rejection, and uncertain-before-retry.
5. Git tests define repository rebinding rejection, worktree identity, literal
   path scope, candidate parent/tree/path facts, and old/new/third-value CAS for
   track, release, and target refs.
6. Driver tests run the fake as a real child and prove clean environment,
   bounded output/time/resources, cancellation, descendant death, receipt
   sealing, writable isolation, and fresh read-only verifier mutation failure.
7. Runtime tests prove identical Git facts derive identical operation bytes,
   changed facts force rederive, unknown decisions dispatch nothing, and a
   transport failure cannot become a Baton verdict.
8. One-track integration proves mutating operation(s), track verification,
   release composition, separate assembly verification, and exact target CAS.
9. Generate the crash matrix from the effect registry and pass every four-cut
   edge test, including stale target and third-value conflicts.
10. Run portable Baton engine cases through the built binary, then run
    `go test ./...`, race tests for journal/runtime, `go vet ./...`, formatting,
    stripped-size measurement, direct-dependency count, and `git diff --check`.

Production code for each step starts only after its failing boundary test
exists. A copied old test that passes without new behavior is not evidence.

## Budgets and stop gates

S0/S1 budgets are tighter than the full-release limits:

| Measure | S0/S1 target | Mandatory stop |
| --- | ---: | ---: |
| production packages | 6 | 6 |
| non-generated production Go lines | <= 6,000 | 7,500 |
| production SQL | <= 350 lines | 500 lines |
| direct dependencies | 1 (`modernc.org/sqlite`) | 2 without a short ADR |
| production file length | <= 500 lines | 700 lines |
| stripped binary | <= 18 MiB target | 25 MiB |
| journal tables | 7 | 7 without Captain schema review |
| effect kinds | only edges exercised by S1 | any kind lacking reconciliation/crash corpus |

The immutable released Baton package and generated fixture bytes are measured
separately from handwritten Go, but their size and digest are reported. No
provider SDK, workflow framework, ORM, migration framework, Git library,
logging framework, UUID dependency, or generated compatibility layer is
admitted. IDs and digests use the standard library. SQLite remains the sole
direct dependency unless measured implementation evidence earns an ADR.

Exceeding a stop gate pauses implementation for deletion or Captain review; it
does not invite code compression.

## Explicit Captain verdicts

1. **Reset versus reuse — RESET.** Delete every current production Go package.
   Reuse invariant statements only through new focused tests.
2. **Baton RC2 identity — DEFER.** Do not pin, name, or claim it until both the
   annotated tag and release exist and their exact bytes pass admission.
3. **Lifecycle ownership — BATON ONLY.** Sworn stores operation/fact digests,
   runtime controls, claims, and effects; it does not store Baton work states.
4. **Runtime architecture — CUSTOM GO + SQLITE.** DBOS, Temporal, LangGraph,
   and an internal workflow DSL are rejected for S0/S1. The bounded DBOS
   experiment remains S5 work.
5. **Database compatibility — CLEAN SCHEMA.** Do not migrate or read the v0.2
   store as authority. Fail it closed and provide an explicit new-run path.
6. **SQLite mode — SERIAL ROLLBACK JOURNAL.** Keep one writer,
   `synchronous=FULL`, and rollback mode until contention measurements justify
   WAL and its checkpoint lifecycle.
7. **Claims — SQLITE TRUTH, OS QUIESCENCE PROOF.** Filesystem locks and process
   supervisors support liveness; they do not replace durable claims. Git CAS
   remains the final authority for refs.
8. **Effect granularity — ONE OBSERVABLE EDGE.** Do not journal an entire Baton
   operation as one ambiguous effect, and do not make a framework of internal
   syscalls. Journal process, worktree, composition, verification, ref, and
   cleanup edges that need independent reconciliation.
9. **Fake driver — EXTERNAL PROCESS.** An in-memory fake cannot prove
   containment, crash, receipt, or freshness boundaries and is rejected for
   shared conformance.
10. **Verifier — FRESH AND READ-ONLY.** A new process and a new read-only
    worktree are mandatory. Cleanup plus `git status` alone is insufficient.
11. **Assembly verification — ALWAYS DISTINCT.** A one-track release does not
    reuse its track verdict; composition is followed by a new invocation.
12. **Merge — EXACT REF CAS.** S1 creates no merge/squash/rebase commit and no
    PR. The verified release OID becomes the target OID or integration fails.
13. **Outbox — LOSSY PROJECTION.** It can duplicate, back off, or drop within a
    bound and can never control scheduling or recovery.
14. **Parallel-ready, not a framework — CLAIM KEYS ONLY.** S1 implements
    track/release/target claim keys now; dependency-ready parallel scheduling
    and operator controls remain S3.
15. **S1 containment platform — LINUX SYSTEMD + BUBBLEWRAP.** Rebuild only the
    contained capabilities needed by the fake and fresh Verifier inside
    `internal/driver`. Non-Linux execution fails before dispatch; platform
    generalisation waits for an equivalent crash corpus.

## Implementation readiness gate

Implementation may begin when:

- the intended Baton release publication gate is satisfied;
- R0 records the exact released package evidence without changing these
  ownership decisions;
- the externally approved Baton plan assigns the non-overlapping slices above;
- the initial tests name every S1 effect edge and reconciler; and
- the base ref and expected release head are rechecked immediately before the
  first source-changing commit.

If publication changes the Baton operation surface enough to alter package
ownership, effect granularity, composition semantics, or the test slices, stop
for an authorised plan revision. Field-name adaptation inside
`internal/baton` is not such a change; weakening or reimplementing the package
is.
