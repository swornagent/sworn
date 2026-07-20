# ADR 0006: Make controller ownership and build authority capabilities

- Date: 2026-07-20
- Status: accepted

## Context

ADR 0005 closed native-builder execution and recovery, but its startup method
trusted the caller's statement that it held exclusive controller ownership.
Authenticated plan approval was also intentionally historical: the source may
have expired, been revoked, or reduced its grant ceiling after activation.

Adding another native runner on top of either assumption would reproduce two
of v0's central failures: several paths believing they owned the loop, and old
approval data being treated as authority for a new effect. The next boundary
therefore has to make both conditions capabilities before adding adapters or a
verifier.

## Decision

Add one internal current-authority controller with this order:

1. On Linux, before SQLite first connects, Store retains descriptors for the exact
   database and its parent directory and revalidates both identities after
   connection and migration. `StartBuilderController` nonblockingly locks those
   retained objects parent-first; it never reopens a pathname to choose the
   lock. The Store requires a private single-link database in an owner-controlled
   parent with no group or world write bits and rejects replacement, symlinks,
   permission drift, copied handles, foreign owners, and contention. Close and
   process death release both kernel locks. The parent lock intentionally
   serializes every Store in that directory. Controller ownership fails closed
   on other platforms.
2. Ownership starts as a recovery-only capability. Every recovery mutation
   consumes it. Activation holds a SQLite read transaction while proving no
   `running` or `unknown` effect remains, then advances the same capability to
   active. Failure releases ownership and returns no usable controller; a
   successor must repeat the whole barrier.
3. `policy.Authority.AuthorizeBuild` re-resolves and authenticates the exact
   configured source. Every authenticated non-rollback, non-fork source is
   durably observed before current status, validity, or grants can permit or
   deny execution. A rollback or fork is rejected atomically. Resolver failure,
   signature failure, source rollback/fork, or persistence failure creates no
   permit.
4. One opaque permit binds the Authority instance, exact approval ledger,
   controller, delivery run, state revision, plan and authority digests, work,
   work attempt, work-contract digest, builder profile, source reference,
   source version, and source digest. The plan and current source must admit
   workspace `inspect`, `edit`, `execute`, and `commit`. The permit expires
   after 30 seconds or at source expiry.
5. Store rejects raw `build.dispatch`; generic effect claims skip builds; and
   generic prepare, result-binding, completion, and recovery calls cannot
   consume native-build capabilities. The guarded dispatch transaction rejoins
   active ownership, the live permit, the durable source high-water mark,
   current state, exact contract, command, and configured builder before it
   writes anything.
6. Dispatch advances the state revision, so its permit cannot authorize the
   pending effect. `ExecutePendingBuild` resolves authority again, and Store
   claims only the unique pending effect matching the exact run, work, attempt,
   contract, builder profile, and causal command. Selector mismatches leave the
   journal completely unchanged.
7. Store revalidates the permit and durable source head at the successful
   preparation transaction, which is the logical build-start authorization and
   linearization point in the shipped sequence. Store then issues a second
   opaque `PreparedAuthorizedBuildLease`. Every value copy shares one atomic
   phase. `BuilderWorker.Run` must consume the single prepared-to-consumed
   entrance before any executor, Git, or attempt-workspace side effect. The
   Store-gated synchronous callback retains active ownership from that CAS until
   the raw operation returns, preventing `Close` or successor recovery from
   entering in between. Store permits binding and completion only in the
   consumed phase. The preparation transaction remains the durable
   authorization and linearization point; consumption is the separate
   process-local worker-entry boundary. The capability remains valid when a
   legitimate build outlives the 30-second permit and authorizes no other
   effect.
8. Recovery has two separate one-shot entrances. Store issues a
   `BoundBuildCleanupLease` only after revalidating recovery ownership and the
   exact unknown attempt's durable typed result. It issues a
   `BuildRecoveryLease` only for an exact unknown unbound attempt. Cleanup and
   reconciliation consume their capability before touching external state.
   The unbound capability seals the exact opaque repository-unpublished and
   executor-cleanup proofs into a concrete Store-owned `BuildRetryProof` after
   attempt-workspace cleanup. That proof is replayable for journal convergence
   after commit ambiguity, but is bound to its Store, attempt, and recovery
   issuance. The worker's raw algorithms remain package-private behind
   these gates. Cleanup and reconciliation are one-shot per issuance; an exact
   idempotent operation may be reminted only after a fresh Store preflight while
   the attempt remains unknown. Each synchronous callback retains recovery
   ownership through external cleanup and, for an unbound attempt, proof
   sealing.
9. A failed controlled claim, or any failure after a successful claim, releases
   controller ownership. The journal remains truthful and the next owner must
   repeat the recovery barrier through ADR 0005.

The slice adds no durable owner row, heartbeat, scheduler, polling loop, public
mutation command, CLI adapter, verifier, generic permit framework, retry policy,
check runner, integration path, schema migration, or runtime dependency. SQLite
remains the only durable control truth. Ownership and permits are narrow
process-local capabilities. The one-shot worker gates are also process-local;
the Store-owned retry proof is replayable within its issuance, while durable
recovery truth remains in the effect journal.

## Budget gate

The merged base was 11,518 nonblank, noncomment and 12,675 physical production
Go lines. The final tree is 13,454 nonblank, noncomment and 14,860 physical
production Go lines: a delta of +1,936 and +2,185 respectively. The semantic and
physical deltas break down as follows:

- current-authority policy: +242 / +286;
- retained ownership, Store opening, and platform fail-closed paths: +600 /
  +685;
- guarded Store build lifecycle: +812 / +893;
- controller composition: +279 / +315; and
- builder configuration seam: +3 / +6.

This is the second explicit architecture stop and the minimum reviewed vertical
join from current source resolution through exclusive recovery, exact
scheduling, scoped claim, and the existing native builder. It adds no schema or
runtime dependency. Further generic controller or authority abstractions remain
out of scope.

The one-shot worker capability amendment starts from that merged 13,454
semantic / 14,860 physical production-line base. The final tree is 13,732
semantic / 15,177 physical lines, a delta of +278 / +317. It adds no schema
migration or runtime dependency.

## Consequences

Within the shipped Store and control composition, only the recovery-phase owner
can reconcile interrupted journal state. Only the active owner under freshly
authenticated exact authority can dispatch, claim, or successfully prepare a
native-builder attempt. The same active owner and exact prepared-attempt
capability can then bind, publish, and complete it without reinterpreting permit
expiry as evidence that the attempted external work did not happen. A historical
receipt, source facts, permit facts, a permit from another Authority instance,
or authority for another state revision cannot cross those boundaries. The raw
worker algorithms are package-private, while each effectful entry point consumes
its exact Store issuance before reaching external work and retains ownership
until that synchronous operation returns. A copied or concurrent capability
value cannot create a second entrance, successor recovery cannot overlap the
old callback, and bind or completion cannot precede execution consumption. The
Linux lock assumes a cooperative, owner-controlled filesystem namespace; it is
not a sandbox against arbitrary same-UID code or direct lower-level process
calls.

“Current” means freshly resolved and not below the highest authenticated source
version this Store has observed. It cannot prove that no unseen newer remote
version exists. If a later signed revocation reaches the ledger before dispatch,
claim, or successful build preparation, the transactional high-water assertion
rejects the older permit.

This is still not an autonomous product loop. Current authority has not been
extended to checks, verifier dispatch, accepting `PASS`, or integration. There
is no production authority-source configuration, native agent-CLI adapter,
fresh verdict, bounded outcome routing, public `sworn run`, or target update.
A controlled-dispatch commit ambiguity can leave only the truthful advanced
state and pending effect, but the call cannot yet recover that committed result
from its command ID once state has advanced. Durable command-outcome convergence
therefore remains a gate for the public loop.
