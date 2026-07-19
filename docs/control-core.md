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
5. Record each claim and outcome as an immutable effect observation.
6. If a process ends while an effect is running, change it to `unknown`. Never
   claim it again until a named reconciler supplies a JSON observation proving
   success, failure, or that the effect was not applied.

Deterministic command rejections are stored and replayed. Infrastructure errors
roll back and remain errors; they are not converted into domain outcomes. Reuse
of an idempotency key with different request bytes fails closed.

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
- `effects` — the current external-effect journal;
- `effect_observations` — append-only claim, interruption, reconciliation, and
  completion facts;
- `records` and `artifacts` — immutable content-addressed JSON and raw bytes;
- `submission_records` — immutable global submission and work-attempt identity
  reservations bound to canonical record digests; and
- `protocol_identities` — write-once approval, builder-run, and producer-run
  bindings for prepared submissions.

SQL triggers forbid mutation of immutable history, illegal revision jumps,
effect request rewrites, effect deletion, and invalid effect-state transitions.
The partial unique target index prevents two non-terminal runs from owning the
same repository and target.

## Current boundary

The transactional reducer still does not validate Baton plans, resolve current
authority, run checks or coding agents, inspect Git, execute effects, or mutate
a target. Later internal slices may use the same store for measured artifacts
and prepared records without creating a second control path. `sworn board` remains
the only new CLI command and opens the control store in read-only mode. The
reducer's activation and build-dispatch transitions exist to prove the
command/effect boundary; they are unreachable from the CLI.
