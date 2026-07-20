# Transactional control core

The control core separates decisions from external reality with one short
algorithm:

1. Bind a deterministic, length-delimited encoding of the command fields and
   exact payload bytes to a globally unique idempotency key.
2. In one SQLite transaction, load the current revision and invoke the pure
   reducer.
3. Commit the command result, next state snapshot, immutable event, and any
   pending effect requests together.
4. Only after commit may an executor claim a pending effect.
5. A claim returns an opaque lease bound to that store instance, effect,
   owner, and monotonically increasing attempt. A native build claim also
   records its canonical attempt identity in the same transaction, before the
   builder may start. Immediately before native execution, Store reloads the
   exact current running row and attempt witness through that lease. Only that
   exact lease may cross the builder boundary, bind, or complete the effect.
6. Bind one kind-specific typed result to the effect row under that live lease,
   then complete the effect in a separate compare-and-swap. The result slot may
   move once from empty to populated and is immutable thereafter.
7. Record each claim, interruption, reconciliation, and terminal outcome as an
   immutable effect observation.
8. If a process ends while an effect is running, change it to `unknown`.
   `RecoverBoundEffect` may close only the exact attempt's already-bound result.
   An unbound native build may return to pending only after Store prevalidates
   its claim witness and consumes the configured worker's composite proof of no
   Git publication, executor quiescence, and attempt-workspace cleanup.

Deterministic command rejections are stored and replayed. Infrastructure errors
roll back and remain errors; they are not converted into domain outcomes. Reuse
of an idempotency key with different request bytes fails closed.

The store-derived effect ID is also the Baton builder or producer run ID.
`effects.run_id` remains the enclosing delivery-engine run in SQLite and is
called `delivery_run_id` by Go and JSON APIs. A dispatch payload cannot choose a
second invocation identity. Native build request v2 keeps `dispatch_digest` as
the exact work-contract digest and separately binds `builder_dispatch_digest`
to process configuration. Store derives the invocation identity from effect ID,
attempt, and that builder digest. Completion compare-and-swaps effect ID, owner,
and attempt together, so an old worker cannot complete a reconciled retry even
when the same owner name is reused.

`BindEffectResult` validates and stores the typed result before
`CompleteEffect` may close the lease. Normal completion and reconciliation to
`succeeded` use the same kind-specific validator and artifact-closure checks.
A missing result is not evidence that an external action did not happen, and an
orphaned content-addressed artifact is not an effect result. Neither condition
by itself permits successful recovery or retry. There is no manual
`not_applied` or interrupted-failure transition. Schema v7 admits one
kind-specific `unknown -> pending` path: Store must first validate the exact
native build claim, then the worker must mint opaque attempt-bound Git and
writable-cleanup proofs, and Store must atomically persist the byte-identical
`not_applied` witness. Arbitrary text and legacy receipts grant no authority.

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
kind-specific validation and Git repair but adds no second observation. Schema
v6 refuses earlier manual requeues, while schema v7 refuses any pre-v7
`not_applied`, non-NULL claim witness, or live legacy build rather than
reinterpret history as machine authority.

`internal/control.BuilderService` is the only current sequencing join. It
validates that Store and worker share the exact builder profile and repository,
then fixes `Store preflight -> run -> bind -> Store publish -> complete`. A
foreign, stale, changed, or already-bound lease stops before agent code can run.
Its startup barrier first marks interrupted work unknown and resolves every
stopped effect under exclusive controller ownership. It does not claim work,
choose retry policy, or own a public loop.

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
build-witness pair introduced in schema v7. Partial unique indexes permit only
one claim and one retry witness per effect attempt. Store validation, not SQL
shape alone, proves the typed result, opaque cleanup capabilities, and external
Git closure.
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

Reviewable is not `PASS` and historical authenticated approval is not a current
execution permit. A native v2 builder result must match both its exact work
contract and the configured builder profile before it can feed checks or
admission. The public binary still has no mutating command service, claim loop,
native CLI adapter, verifier verdict, retry policy, or integration path.
`sworn board` remains a read-only projection of committed engine truth. See [ADR
0005](adr/0005-native-builder-recovery.md).
