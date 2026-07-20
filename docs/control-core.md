# Transactional control core

The control core separates decisions from external reality with one short
algorithm:

1. Bind a deterministic, length-delimited encoding of the command fields and
   exact payload bytes to a globally unique idempotency key.
2. In one SQLite transaction, load the current revision and invoke the pure
   reducer.
3. Commit the command result, next state snapshot, immutable event, and any
   pending effect requests together.
4. Only after commit may the controller claim a pending effect. Native build and
   local-check effects cannot be claimed through the generic journal surface.
5. A controlled claim returns an opaque lease bound to that Store instance,
   effect, owner, and monotonically increasing attempt. In the same transaction
   it records a canonical attempt identity before external work may start. The
   builder identity binds its execution profile; the check identity binds its
   content runtime. Store reloads the exact running row and attempt witness and
   issues a prepared capability. Its shared atomic state admits exactly one
   worker entry and retains controller ownership across the whole synchronous
   callback. Result binding and completion require that consumed capability.
6. Bind one kind-specific typed result to the effect row under that live lease,
   then complete the effect in a separate compare-and-swap. The result slot may
   move once from empty to populated and is immutable thereafter.
7. Record each claim, interruption, reconciliation, and terminal outcome as an
   immutable effect observation.
8. If a process ends while an effect is running, change it to `unknown`.
   Bound-result recovery may close only the exact attempt's already-bound typed
   result. An unbound build may return to pending only after proof of no Git
   publication plus complete writable cleanup. An unbound local check may return
   only after its exact content-bound unit is quiescent and its deterministic
   runtime and materialization roots are absent. Store prevalidates each claim,
   issues a one-shot recovery capability, and seals the opaque lower proofs into
   a kind-specific Store-owned retry proof.

Deterministic command rejections are stored and replayed. Infrastructure errors
which leave no durable command remain errors; they are not converted into domain
outcomes. Reuse of an idempotency key with different request bytes fails closed.
A controlled build caller must therefore keep one command ID stable for the
same logical dispatch across an ordinary retry and process restart; generating
a new ID cannot converge an earlier ambiguous commit.

Before seeking fresh authority, the controller's guarded build dispatch probes
that command ID under active ownership. A missing command row is the only
`not found` result. An occupied ID is decoded as its strict stored command and
stored result, its request digest is recomputed, and an applied result must close
over exactly one byte-matching `build.dispatched` event and one derived native
build effect. That closure rejoins the selected run, work, configured builder,
exact plan and contract, and current intended attempt; another outcome or a
foreign, incomplete, corrupt, stale, or differently used ID fails closed.
`Replayed` is set only on the returned copy.

Only an absent command proceeds to fresh authority and the mutating dispatch
transaction. If that apply returns an error, the controller performs one
five-second convergence probe using a context detached from caller cancellation.
An exact durable result wins the commit-ambiguity window; absence returns the
original apply error, and a probe failure is joined to it. This is bounded
convergence, not a retry policy or loop.

The Store-derived effect ID is also the stable Baton builder or producer run
ID. `effects.run_id` remains the enclosing delivery-engine run in SQLite and is
called `delivery_run_id` by Go and JSON APIs. Native build request v2 keeps
`dispatch_digest` as the exact work-contract digest and separately binds
`builder_dispatch_digest` to process configuration. Store derives a builder
invocation identity from effect ID, attempt, and that profile digest. A local
check derives a separate executor invocation identity from effect ID, attempt,
and content-runtime digest while retaining the effect ID in its Baton receipt.
Completion compare-and-swaps effect ID, owner, and attempt together, so an old
worker cannot complete a reconciled retry even when the same owner name is
reused.

`BindEffectResult` validates and stores the typed result before
`CompleteEffect` may close the lease. Normal completion and reconciliation to
`succeeded` use the same kind-specific validator and artifact-closure checks.
A missing result is not evidence that an external action did not happen, and an
orphaned content-addressed artifact is not an effect result. Neither condition
by itself permits successful recovery or retry. There is no manual
`not_applied` or interrupted-failure transition. Schema v7 admits the exact
native-build `unknown -> pending` path. Schema v8 adds only the corresponding
local-check path: Store validates the exact claim, a one-shot worker capability
proves content-bound quiescence and cleanup, and Store atomically persists the
byte-identical attempt-identity witness. Arbitrary text, orphan artifacts, and
legacy receipts grant no authority.

Native build completion and recovery additionally reparse the bound result
through the configured repository. They require request v2, both exact digests,
candidate, repository binding, target, plan, and current work attempt to match
the delivery journal. Store then establishes the deterministic candidate ref
and attempt ref before committing success. Publication rederives the Git
objects, parent, trees, and changed paths, repairing a missing ref but rejecting
a collision or mutation. A crash after either ref but before SQLite commit is
safe because the typed result was already bound; replay finishes the same
postcondition. Legacy v1 bound results remain repairable archaeology but cannot
feed native checks or admission. Local-check recovery repeats its complete
artifact and semantic closure.

Effect ID plus attempt is the replay identity; the reconciler ID attributes the
process that wins the transition and is not part of idempotency. Replay repeats
kind-specific validation and any required external repair but adds no second
observation. Schema v6 refuses earlier manual requeues, schema v7 refuses unsafe
pre-v7 build history, and schema v8 refuses an ambiguous pre-v8 unbound running
or unknown local check rather than reinterpret history as machine authority.

`internal/control` is the shipped sequencing join. `BuilderService` validates
that Store and worker share the exact builder profile and repository, then fixes
`Store preflight -> run -> bind -> Store publish -> complete`. `CheckService`
fixes the corresponding content-bound check order without exposing a raw worker
entry point. `Controller` owns the startup recovery barrier and the sole bounded
builder-to-reviewable convergence. A foreign, stale, changed, or already-bound
lease stops before external code can run.

`internal/store` now supplies the ownership which that barrier previously
assumed. On Linux, before SQLite first connects it retains the exact database
and parent directory identities; a controller nonblockingly locks both retained
objects, never a later pathname lookup. The Store requires the parent to have no
group or world write bits and rejects replacement, symlinks, unsafe permissions,
hard links, foreign or copied handles, and contention. The kernel releases both
locks on ordinary close or process death. Controller ownership fails closed on
other platforms. The containing namespace must remain cooperative and
owner-controlled; an arbitrary same-UID filesystem adversary is outside this
process-ownership boundary.

Ownership begins in a recovery-only phase. Store recovery mutations require
that exact capability, and activation transactionally proves that no running
or unknown effect remains. Only the resulting active capability can dispatch,
claim, prepare, bind, or complete a native build or local check. Raw
`build.dispatch` is rejected, generic claims skip both controlled effect kinds,
and generic bind, completion, failure, and recovery APIs cannot consume their
leases.

The controller observes an exact durable dispatch outcome without re-resolving
authority because replay neither mutates state nor grants effect authority. A
new dispatch still re-resolves gate-specific current authority, and claiming its
pending builder resolves authority again after the state revision has advanced.
A revocation may therefore leave the historical dispatch replayable while
correctly preventing its pending effect from being claimed. Store selects only
the exact permitted work and attempt, then rechecks ownership, permit, durable
source head, state, contract, command, and typed request at successful
preparation. That transaction is the logical build-start authorization and
linearization point; `BuilderService` invokes the worker immediately afterward.
Every copy of the resulting
`PreparedAuthorizedBuildLease` shares one atomic phase: `BuilderWorker.Run`
must consume its sole worker entrance before any side effect. Its Store-gated
synchronous callback retains active ownership until the raw operation returns,
so `Close` and successor recovery cannot enter between consumption and actual
work. Store permits result binding or completion only after that consumption.
Preparation therefore remains the durable authorization and linearization
point, while consumption is the distinct process-local proof that worker entry
began. The consumed capability carries that exact attempt through result
binding and completion without turning the 30-second permit into a runtime
limit.

Check dispatch is a deterministic historical transaction and therefore does
not mint a permit. Before each exact pending check is claimed, the controller
reloads its policy-ordered request and freshly re-resolves authority. The
short-lived permit binds controller, run and revision, plan, work attempt and
contract, builder and check effect IDs, check definition, content runtime, and
current source. Store rejoins those facts to active ownership, the durable
source high-water mark, and the exact next pending row before issuing one
prepared check lease. The plan and current source need only the check's
`inspect` and `execute` grants. Admission remains an effect-free historical
transaction over completed evidence and does not reuse an execution permit.

Build recovery uses the same contraction. Store issues a one-shot
`BoundBuildCleanupLease` only for an exact unknown attempt with a valid durable
result, and a one-shot `BuildRecoveryLease` only for an exact unknown unbound
attempt. Cleanup and reconciliation consume their respective capabilities
before touching external state. After the latter produces matching opaque
repository unpublished and executor cleanup proofs, its Store issuance seals a
concrete `BuildRetryProof`. The proof remains replayable so the journal can
converge after a retry-transaction ambiguity without repeating external
cleanup; it cannot be constructed for another Store, attempt, or recovery
issuance. Recovery one-shot semantics are per issuance: exact idempotent cleanup
or reconciliation may be reminted only after a fresh Store preflight while the
attempt remains unknown, which permits convergence after a partial failure.
Each callback retains recovery ownership through cleanup and proof sealing. The
raw algorithms are package-private behind these gated entry points.

For a bound local check, the existing typed-result recovery path repeats the
complete receipt, artifact, definition, environment, and candidate closure. For
an unbound local check, Store issues a one-shot `CheckRecoveryLease` only after
validating the exact attempt identity and configured runtime. The worker proves
the deterministic content-bound unit quiescent, removes its runtime and
candidate materialization roots, and returns an opaque cleanup proof. Store
seals that proof to the issuance before schema v8 permits the exact attempt to
return to pending. Orphan CAS objects are harmless but never sufficient proof.

A failed controlled claim, or any failure after claim, releases ownership and
forces the next process through recovery. `AdvanceToReviewable` derives three
stable domain-separated command IDs from run, work, and attempt, then performs
one bounded builder, ordered-check, and admission convergence. It does not poll,
choose repair policy, advance another work item, obtain a verifier verdict, or
own a public loop. Controlled dispatch still converges an exact historical
command outcome before fresh authority and once more for up to five seconds
after an apply error. See [ADR 0006](adr/0006-current-authority-controller.md)
and [ADR 0008](adr/0008-builder-to-reviewable-production-vertical.md).

## SQLite ownership

`internal/store` is the only package that imports the SQLite driver. It uses a
single serialized connection, private database permissions, foreign keys,
rollback journaling, and `synchronous=FULL`. The file's `application_id` and
forward-only `user_version` must exactly match the binary. Read-only board opens
cannot create or migrate the file.

The database contains:

- `runs` — current snapshots indexed by immutable delivery, repository, target,
  plan, and revision facts;
- `commands` and `events` — append-only control history;
- `effects` — the current external-effect journal, including one immutable
  typed result slot per effect;
- `effect_observations` — append-only claim, interruption, reconciliation, and
  completion facts;
- `records` and `artifacts` — immutable content-addressed JSON and raw bytes;
- `submission_records` — immutable global submission and work-attempt identities
  bound to canonical records, the same delivery run, and the exact applied
  admission command.

SQL triggers forbid mutation of immutable history, illegal revision jumps,
effect request rewrites, effect deletion, invalid effect-state transitions, and
every `unknown -> pending` or `unknown -> failed` transition except the exact
machine-witnessed build and check paths introduced in schemas v7 and v8.
Partial unique indexes permit only one claim and one retry witness per effect
attempt. Store validation, not SQL shape alone, proves the typed result, opaque
cleanup capabilities, external Git closure, and content-bound check cleanup.
The partial unique target index prevents two non-terminal runs from owning the
same repository and target.

## Reviewable admission boundary

The reducer has two narrow internal edges after builder success:

- `checks.dispatch` reparses the exact plan, resolves its policy and ordered
  definitions, rebinds the succeeded builder and process-configured runtime,
  and atomically creates the complete serial check batch. Work moves from
  `active` to internal `checking`; Baton board projection remains `active`.
- `submission.admit` accepts only `{work_id}`. Ordinary reduction validates that
  intent and returns a sentinel requiring Store-derived facts. The Store anchors
  admission to the current dispatch event and complete effect batch, requires
  every result to be durably succeeded and semantically passing, then supplies
  the exact submission binding to the same pure reducer.

Admission repeats the full durable closure inside its SQLite transaction: exact
plan and policy, definitions, authenticated historical approval, builder
attempt, typed results, receipt/environment/output CAS, content runtime, Baton
snapshot, and configured-repository Git candidate and scope. It writes the
command, state, event, canonical submission, and run/command-bound identity
together and emits no effect. A preflight or write failure leaves `checking`
unchanged; exact command replay returns the committed result.

Schema v5 rebuilds `submission_records` without copying earlier structural
identities. Their records remain content-addressed archaeology, but no legacy
row is treated as journal-backed admission proof.

Reviewable is not `PASS`. A build permit grants nothing to checks; each pending
check receives a separate current permit bound to its exact effect and runtime.
Neither permit grants verification, `PASS`, or integration. A native v2 builder
result must match both its exact work contract and the configured builder
profile before it can feed checks or admission. The public binary exposes one
bounded mutating command, not a claim loop: `sworn run` converges only the
selected current work item to reviewable. There is still no verifier verdict,
repair policy, scheduler, or integration path. `sworn board` remains a read-only
projection of committed engine truth. See [ADR
0008](adr/0008-builder-to-reviewable-production-vertical.md).
