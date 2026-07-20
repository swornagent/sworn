# ADR 0004: Recover known results before proving retries

- Date: 2026-07-20
- Status: accepted

## Context

ADR 0003 required another architecture stop before further net production
growth. The next roadmap item appeared to be interrupted workspace and Git
reconciliation, but the kernel has no native builder adapter, exclusive command
service, or durable pre-execution witness yet. Designing a generic retry proof
without the real invocation, workspace-export, and Git-publication ordering
would turn guesses into a second recovery architecture.

The existing Store API also exposed `not_applied`, `succeeded`, and `failed` as
caller-selected reconciliations. Only `succeeded` had machine-verifiable input:
the exact attempt's immutable typed result. `not_applied` accepted audit prose,
which cannot prove that a process did not start or distinguish a late result
from an earlier attempt. An interrupted failure was equally caller-selected.

## Decision

Contract recovery to one currently provable operation:

1. Replace generic reconciliation with `RecoverBoundEffect(effect, attempt,
   reconciler)`. It transitions only an exact `unknown` attempt with its
   immutable typed result, repeats the same kind-specific validation used by
   ordinary completion, and commits `succeeded` plus one observation atomically.
   An already-succeeded exact attempt is accepted only as the replay in item 3.
2. For a bound build, require the immutable configured repository and call
   `EnsureCandidate` before the journal transition. The effect request,
   candidate, configured binding, delivery repository/target, and current work
   attempt must agree before Git is touched. Recovery then rederives Git object,
   parent, tree, changed-path, and retention facts; it may repair only the
   deterministic missing candidate ref.
3. Make successful replay by effect ID and attempt idempotent. The reconciler ID
   is audit attribution for the process that wins the transition, not a second
   idempotency identity. Replay repeats validation and external Git repair but
   cannot create another observation or alter the result.
4. Remove the Go paths for manual `unknown -> pending` and `unknown -> failed`.
   Migration 006 removes those transitions from the current SQL trigger as
   well. It fails closed if v5 already contains a manual-requeued `pending`
   effect with a positive attempt, leaving that database and its observations
   unchanged at v5 instead of making the effect claimable. Historical
   observation values remain readable archaeology.
5. Leave every unbound attempt unknown. Orphan CAS artifacts, missing processes,
   workspace residue, candidate refs, and free-form text are not retry proof.

The native builder slice must define the remaining protocol against its real
ordering. At minimum it needs an exact attempt identity, a durable witness
before target execution, restart-discoverable workspace/export ownership, and
an attempt-bound Git publication point. Only positive proof that an attempt did
not publish or start may authorize `unknown -> pending`; ambiguous attempts
remain stopped.

## Budget gate

This slice must be net-negative in production Go. It removes a generic recovery
surface rather than adding a recovery framework, state machine, scheduler, or
new dependency. Tests and the forward-only SQL migration may grow to prove the
narrower invariant; the semantic kernel must finish at or below the 10,083
nonblank, noncomment and 11,083 physical-line merged baseline.

The completed slice is 10,079 nonblank, noncomment and 11,078 physical
production Go lines: 4 semantic and 5 physical lines below that baseline.

## Consequences

Restart can truthfully finish a result already made durable, including repairing
its exact Git retention ref. It still cannot retry an unbound effect, so this
ADR does not complete the roadmap item or enable an autonomous claim loop.

The later loop must first acquire exclusive controller ownership, mark orphaned
running attempts unknown, reconcile every known result and every attempt-owned
residue, and only then claim new work. No generic provider, recovery workflow,
or operator-text escape hatch is introduced.
