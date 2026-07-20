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
- `submission_records` — immutable global submission and work-attempt identity
  reservations bound to canonical record digests, written only by the future
  final admission transaction.

SQL triggers forbid mutation of immutable history, illegal revision jumps,
effect request rewrites, effect deletion, and invalid effect-state transitions.
The partial unique target index prevents two non-terminal runs from owning the
same repository and target.

## Current boundary

The journal now understands typed builder results and typed `check.local`
results. The local-check worker accepts its candidate only through the succeeded
builder effect, invokes only the content-bound executor, and returns the minimal
outcome and receipt reference. The worker materializes the builder candidate
from Git, while the executor stages and remeasures the candidate workspace and
content runtime before execution.

The store performs a narrower durable rebind. It matches the receipt's candidate
identifiers to the succeeded builder result; resolves and validates the receipt,
definition, environment, stdout, and stderr CAS objects; matches the exact
definition fields and measured runtime/output ceilings; and requires the
environment's runtime-manifest digest to equal the request. It does not
rematerialize Git, remeasure a workspace or runtime, or compare the
environment's embedded protocol-snapshot digest. Final admission must close
those remaining protocol and submission checks.

The reducer now has one bounded, batched `checks.dispatch` edge. Its payload is
untrusted: before persistence, the store transaction reparses the exact plan,
resolves its canonical policy and every selected definition, requires their
ordered selection to match, and rebinds the configured runtime and succeeded
builder attempt. It also requires the active authority receipt to be an immutable
authenticated historical approval for that plan and to precede the builder.
Only then does work move from `active` to `checking` and the complete ordered
batch become pending atomically. Within that command, a later ordinal cannot be
leased until every earlier effect has succeeded. A precondition failure records
no command, event, state change, or effect.

This is structural scheduling, not current execution authority. Historical
approval does not prove that its source is still current, and Baton requires
gate-specific revalidation before builder or check execution. The public binary
still opens only the read-only board path: there is no mutating CLI, command
service, claim loop, or native runner adapter. `sworn board` remains read-only,
and no path integrates a target.

Structural submission persistence and its parallel builder/producer identity
registry have been removed. A future atomic admission transaction will recheck
the journal and artifact closure and become the sole submission writer.
