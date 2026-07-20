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

## Configured file-bundle source

Startup configuration maps each exact, logical source reference to one
authorizer identity, Ed25519 public key, and absolute trusted bundle directory.
The key identifier remains derived from the public key. Configuration cannot
carry private signing material, and neither a delivery plan nor a per-operation
input can replace this mapping.

On Linux, the resolver retains the identity of the configured absolute, clean
directory instead of reopening a caller-selected path. It derives the bundle
selection from the exact plan digest; the source reference is never interpreted
as a filename. The writer must atomically publish one regular-file bundle
containing the exact source and detached proof bytes. The resolver opens that
file once, so one read cannot mix two publication generations. Every
current-authority gate opens and reads the bundle again with strict fields and
bounded envelope, source, and proof sizes. Missing, malformed, oversized,
symlinked, non-regular, or multiply linked bundles fail closed without a cache
or fallback.

The transport contract is intentionally small. The filename is the plan's 64
lowercase digest characters followed by `.json`; its strict I-JSON object has
exactly these fields:

```json
{"proof":"<unpadded-base64url exact proof bytes>","schema_version":"sworn-authority-bundle-v1","source":"<unpadded-base64url exact source bytes>"}
```

The envelope is capped at 108 KiB and preserves the exact signed source and
proof bytes rather than normalizing either artifact.

Closing the configured authority is ordered resource cleanup, not revocation.
Composition must first stop dependent controllers and wait for authority calls
to finish. A signed newer source and the Store high-water assertion remain the
only way to revoke current authority; closing a directory handle does not
invalidate a permit which was already minted.

Deployment must keep the startup configuration and bundle directory outside the
write authority of the caller, builder, and verifier. The resolver validates
its filesystem shape; it does not infer a repository root or prove that the
chosen directory satisfies this deployment boundary. Sworn neither prompts for
approval nor holds a signing key, and the resolver accepts no authority content
from environment values, stdin, TTY identity, or arbitrary helper commands. A
later external interactive or remote authorizer may publish a valid bundle, but
its signing capability and approval policy remain outside Sworn.

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

“Current” has a deliberately exact local meaning: the source was freshly read
and authenticated for this gate, and it is not below the highest version this
Store has observed. It cannot prove that the configured source withheld a newer
version which Sworn has never seen. The Store high-water mark rejects rollback
only after a later authenticated version has been observed. It reasserts the
permit's version and digest inside each dispatch and claim transaction, so a
locally observed later revocation wins.

Dispatch convergence is historical rather than current authority. Under active
ownership, `DispatchBuild` probes the caller's stable command ID before
resolving the source and once, bounded, after an apply error. Store returns only
an exact command/result and causal journal/plan closure; a mismatched occupied
ID fails closed and only absence reaches fresh authorization. Replay mints no
permit. A newer revocation therefore leaves committed history observable while
still preventing a fresh dispatch or pending-build claim.

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
`PreparedAuthorizedBuildLease` whose value copies share one atomic phase.
`BuilderWorker.Run` consumes its single worker entrance and retains active
ownership across the complete synchronous callback before any executor, Git,
or attempt-workspace side effect. `Close` and successor recovery therefore
cannot overlap that operation. Store accepts result binding and completion only
from the consumed capability. Preparation is the durable authorization and
linearization point, while consumption separately proves process-local worker
entry. A legitimately long-running build may bind and complete its exact
attempt after 30 seconds, but the capability cannot claim or start another
effect.

Recovery authority is equally narrow. Store issues a one-shot bound-cleanup
capability only after validating the exact unknown attempt and its durable
result, or a one-shot unbound-reconciliation capability after validating the
exact unbound attempt. Each is tied to the recovery owner and consumed before
external work. For an unbound attempt, Store seals the exact opaque repository
unpublished and executor-cleanup proofs into its own `BuildRetryProof` only
after the recovery capability was consumed. That proof is replayable for
journal convergence, but cannot authorize another Store, attempt, or recovery
issuance. The worker's raw algorithms are package-private behind these gates.
Cleanup and reconciliation are one-shot per issuance, not per durable attempt;
a partial failure may obtain a fresh issuance only after Store repeats the exact
unknown-attempt preflight. Their synchronous callbacks retain recovery
ownership through external cleanup and proof sealing.

## Current check authority

Check dispatch remains a deterministic historical transition: it derives the
complete ordered batch from the exact plan and does not grant execution
capability. Immediately before each pending `check.local` claim, the controller
reloads that exact dispatch and asks the same Authority service to resolve and
authenticate the source again. The plan and current source must both grant
workspace `inspect` and `execute`; builder-only `edit` and `commit` grants are
not required for a read-only check.

One opaque `CurrentCheckPermit` binds the Authority instance, controller,
delivery run and state revision, exact plan, work attempt and contract, succeeded
builder effect, pending check effect, check identity and definition digest,
content-runtime digest, source version, and source digest. It has the same short
current-authority lifetime as a build permit. Store rejoins every fact to active
ownership, the durable source high-water mark, the exact next policy-ordered
effect, and immutable runtime configuration before it atomically records the
attempt identity and issues a one-shot worker capability.

Bound-result convergence and unbound cleanup recovery are historical facts and
do not mint another current permit. A retry receives fresh authority only when
the recovered effect is claimed again. Submission admission likewise consumes
completed authenticated provenance and evidence without pretending to be an
execution gate.

Verifier dispatch, accepting `PASS`, and integration still require their own
short-lived gate-specific revalidation. The bounded `sworn run` command stops at
reviewable and supplies no signing capability or external authorizer transport.
See [ADR 0006](adr/0006-current-authority-controller.md) and [ADR
0008](adr/0008-builder-to-reviewable-production-vertical.md).
