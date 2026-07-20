# ADR 0005: Make the first native effect attempt-bound

- Date: 2026-07-20
- Status: accepted

## Context

ADR 0004 deliberately stopped at recovering an already-bound result. An
unbound build could not be retried because absence of a result says nothing
about whether a process ran, a writable tree remained live, or Git publication
occurred. Solving that generically before a real native effect existed would
have created a recovery framework whose claims were stronger than its facts.

The native builder is the first complete external-effect boundary. It must join
the exact Baton work contract, process configuration, executor lifetime,
attempt-owned workspace, candidate objects, Git publication, journal binding,
and restart behavior without adding another durable owner. The v1 build-request
schema already used `dispatch_digest` for the exact work contract, so changing
that field's meaning to process configuration would also make old journal bytes
ambiguous.

## Decision

Add one kind-specific vertical slice with this fixed ordering:

1. A v2 build request preserves `dispatch_digest` as the exact work-contract
   digest and adds `builder_dispatch_digest` for the complete native execution
   profile. The profile binds the executor configuration, repository, workspace
   root, agent, argv, environment names, timeout, network, and writable access.
2. Claiming a native build atomically records a canonical attempt identity
   derived from effect ID, monotonically increasing attempt, and builder
   dispatch digest. Legacy v1 claims retain a NULL witness and can never enter
   native execution, downstream checks, admission, or autonomous retry.
3. Before agent code can run, Store reloads the exact current running lease and
   its durable attempt witness. A foreign, stale, changed, or already-bound
   lease stops. The builder worker then runs through the existing contained
   executor, validates the exact plan and contract, and prepares candidate Git
   objects without publishing a ref. It neither claims work, writes the
   journal, nor decides success.
4. The single `internal/control` composition service fixes one order: Store
   preflight, run, bind typed result, publish, then succeed. Store owns the first
   and last two operations. While the result is durably bound, it validates the
   exact delivery, plan, work attempt, process configuration, repository,
   target, and candidate, then publishes both the deterministic candidate ref
   and `refs/sworn/v1/attempts/<invocation-id>` before committing `succeeded`.
5. Writable executor ownership uses deterministic invocation paths and a
   process-shared lock. Reconciliation proves the matching systemd unit is
   quiescent before removing runtime and workspace residue. A second executor
   instance cannot clean the pre-launch window of a live owner.
6. Startup recovery is a barrier under exclusive controller ownership. Store
   first validates the exact unknown build and its durable claim witness and
   issues an opaque recovery lease with a fresh challenge. Only then may the
   worker prove the attempt ref absent and reconcile every attempt-owned
   writable resource. Store accepts only a composite opaque proof bound to that
   exact Store lease and challenge, then atomically records the canonical
   `not_applied` witness while returning the same attempt to `pending`.
7. A bound result is never retried. Recovery idempotently establishes its Git
   refs and closes it as succeeded. An unbound attempt with missing, legacy,
   corrupt, stale, cross-Store, or mismatched proof remains stopped.
8. Migration 007 refuses to reinterpret archaeology. It fails atomically if a
   pre-v7 database contains a `not_applied` observation, a non-NULL claim
   receipt, or a live legacy build. Exact unique claim/retry witnesses and the
   guarded build-only `unknown -> pending` transition exist only after that
   cutoff.

The slice adds no provider SDK, LangChain or LangGraph runtime, scheduler,
workflow table, second state machine, public mutation command, claim loop,
verifier, retry policy, or operator-text escape hatch. `internal/control` is
only the one process-local sequencer around the existing Store and effect
worker; SQLite remains the sole durable control truth.

## Budget gate

This is an explicit architecture stop and the first accepted expansion beyond
the original 8–10k walking-skeleton estimate. The merged base was 10,079
nonblank, noncomment and 11,078 physical production Go lines. The completed
native-builder boundary is 11,518 and 12,675 respectively: an increase of 1,439
semantic and 1,597 physical lines.

The increase is accepted as one indivisible real boundary, not as a new rate of
growth. Of the semantic increase, 588 lines are the native worker and its
attempt cleanup, 367 are journal execution/publication/recovery gates, 201
harden the existing executor, 125 form the narrow composition service, 82 add
attempt-scoped Git publication, 74 version and bind engine requests, and 32
consolidate strict protocol validation. Shared producer code contracts by 30
lines; there is no new production dependency.

The budget is evidence, not permission to weaken a proof. This slice removed a
redundant Linux-executor adapter and reused the existing reducer, journal,
repository, executor, plan, candidate, and admission owners. Further production
growth requires another architecture stop and must either close the next real
end-to-end boundary or replace existing surface; generic orchestration
abstractions remain out of scope.

## Consequences

Sworn can now execute and safely recover one native builder effect internally.
Every crash cut converges to one of two machine-supported outcomes: a bound
candidate is published and completed, or an exact unpublished attempt is
cleaned and requeued. Ambiguity, legacy state, and configuration drift stop.

This does not yet create an autonomous product loop. No public command acquires
exclusive controller ownership, resolves current authority, claims work, runs
checks, obtains an independent verdict, accepts `PASS`, or integrates a target.
Those capabilities must compose around this boundary rather than introduce a
parallel runner or recovery engine.
