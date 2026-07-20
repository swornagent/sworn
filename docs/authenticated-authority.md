# Exact plan and authenticated authority

Sworn has one internal authenticated plan-approval boundary. Check dispatch and
reviewable admission consume its durable historical facts, but that history is
not a current effect permit or a fresh authority decision.

## Boundary

`protocol.ParseDeliveryPlan` strictly parses the complete `delivery-plan-v1`
and returns an opaque `ExactPlan`. Plan, authority, and work-contract digests
cover their complete RFC 8785 objects. SQLite retains the canonical plan and
reparses it whenever a later transaction needs the exact facts.

The engine constructs one `policy.Authority` service at startup with fixed
Ed25519 trust roots, resolver, ledger, and production UTC clock. An approval
operation accepts only an `ExactPlan`. Each root pins one source reference and
authorizer identity to a public key whose identifier is derived from the key.
Resolver output cannot select that key, and per-operation input cannot replace
the service configuration.

The resolved source envelope is Sworn-specific policy based on Baton's example
authority source; it is not another Baton record schema. It carries a monotonic
version, status, target, maximum grant set, authorizer, and validity window. A
detached `sworn-authority-proof-v1` binds its canonical digest and version to the
exact plan and authority digests, key identifier, and approval time. Ed25519
signs a domain-separated RFC 8785 encoding of those fields.

The signature therefore approves the whole exact plan. The source's maximum
grants are only a ceiling, and may be empty for total revocation. The generated
Baton receipt preserves the plan's grant order.

## Durable historical truth

One SQLite transaction retains the authenticated source/proof observation,
exact plan, and canonical Baton `authority_approval` receipt. A correctly signed
revoked, expired, or grant-reducing source is recorded before approval is
denied, so a newer observed version blocks an older source. The first observed
positive version becomes the high-water mark; an unseen lower version or a
same-version canonical fork fails atomically. A still-newer active version may
explicitly reauthorize work when signed by the configured root.

Raw source and proof bytes are retained for audit, while canonical digests own
semantic identity. Whitespace-only formatting differences can coexist without
changing approval identity. Legacy structural receipts cannot reserve or
preempt authenticated authority identities.

After commit, the service returns a distinct `HistoricalApproval`. Later check
dispatch and admission transactions do not restore a free-standing authority
capability. They reload the exact plan, require the immutable approval row to
join its source snapshot and authenticated proof observation, and validate the
canonical receipt against the exact plan and builder. The original approval
transaction owns signature verification; the control database is trusted only
under Sworn's single-writer boundary, never through a stored boolean.

Historical approval proves what was approved at the recorded time. It does not
claim the source is current.

## Current build authority

The internal builder controller re-resolves the configured source before
scheduling a ready build and again before claiming its pending effect. Every
authenticated non-rollback, non-fork source is durably observed first,
including a revoked, expired, or grant-reducing source. A future-dated approval
fails authentication before persistence; a rollback or fork is rejected
atomically. Resolver failure creates no source claim; persistence failure
creates no permit.

“Current” has a deliberately exact local meaning: the source was freshly
returned and authenticated for this gate, and it is not below the highest
version this Store has observed. It cannot prove that a resolver withheld a
newer remote version which Sworn has never seen. The Store reasserts the
permit's version and digest against that durable high-water mark inside each
dispatch and claim transaction, so a locally observed later revocation wins.

An active source may mint one opaque `CurrentBuildPermit`. It is bound to the
exact Authority instance, controller, delivery run, state revision, plan,
work, work attempt, work-contract digest, builder profile, source version, and
source digest. It expires at 30 seconds or at the source validity boundary,
whichever comes first. The exact plan and current source must both contain the
workspace `inspect`, `edit`, `execute`, and `commit` grants. A public facts view
cannot reconstruct the capability.

The permit is process-local and is not durable authority. The state revision
makes the pre-scheduling permit unusable after dispatch; a restart must resolve
authority again before claiming the pending build. The Store revalidates the
permit and durable source head in the successful preparation transaction
immediately before `BuilderService` invokes agent code. That commit is the
logical build-start authorization and linearization point in the shipped
sequence. Store then replaces the expiring permit with an opaque
prepared-attempt capability. A legitimately long-running build may therefore
bind and complete its exact attempt after 30 seconds, but that capability does
not prove the runner executed an instruction and cannot claim or start another
effect. The raw internal worker remains a privileged trusted-computing-base seam
until its execution and recovery methods consume one-shot Store capabilities.

Check execution, verifier dispatch, accepting `PASS`, and integration still
require their own short-lived gate-specific revalidation. Check scheduling and
admission currently use the approval receipt only as historical provenance and
remain unreachable from a public autonomous controller. See [ADR
0006](adr/0006-current-authority-controller.md).
