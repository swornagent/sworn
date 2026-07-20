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
   owner, and monotonically increasing attempt. Only that exact lease may
   complete the effect.
6. Bind one kind-specific typed result to the effect row under that live lease,
   then complete the effect in a separate compare-and-swap. The result slot may
   move once from empty to populated and is immutable thereafter.
7. Record each claim, interruption, reconciliation, and terminal outcome as an
   immutable effect observation.
8. If a process ends while an effect is running, change it to `unknown`.
   Successful reconciliation requires the already-bound result. Requeueing via
   `ReconcileNotApplied` is an explicit manual assertion, not autonomous proof.

Deterministic command rejections are stored and replayed. Infrastructure errors
roll back and remain errors; they are not converted into domain outcomes. Reuse
of an idempotency key with different request bytes fails closed.

The store-derived effect ID is also the eventual Baton builder or producer run
ID. `effects.run_id` remains the enclosing delivery-engine run in SQLite and is
called `delivery_run_id` by Go and JSON APIs. A dispatch payload cannot choose a
second invocation identity. Completion compare-and-swaps effect ID, owner, and
attempt together, so an old worker cannot complete a reconciled retry even when
the same owner name is reused. Reconciliation compare-and-swaps the observed
unknown attempt for the same reason.

`BindEffectResult` validates and stores the typed result before
`CompleteEffect` may close the lease. Normal completion and reconciliation to
`succeeded` use the same kind-specific validator and artifact-closure checks.
A missing result is not evidence that an external action did not happen, and an
orphaned content-addressed artifact is not an effect result. Neither condition
by itself permits successful reconciliation or autonomous retry.
`ReconcileNotApplied` may manually requeue such an attempt, but its required
detail is an audit note, not machine proof. No autonomous path may select it
until the effect kind supplies attempt-bound external evidence of non-application.
This matters because an effect ID is stable across retries: arbitrary text
cannot safely separate a late result from an earlier attempt.

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
effect request rewrites, effect deletion, and invalid effect-state transitions.
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
execution permit. The public binary still has no mutating command service,
claim loop, native agent adapter, verifier verdict, retry policy, or integration
path. `sworn board` remains a read-only projection of committed engine truth.
