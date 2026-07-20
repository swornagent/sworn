# Exact plan and authenticated authority

Sworn now has an internal authenticated plan-approval boundary. Other internal
engine paths can retain that historical fact while structurally scheduling
effects, but this boundary does not make a submission reviewable or issue a
current effect permit.

## Boundary

`protocol.ParseDeliveryPlan` strictly parses the complete `delivery-plan-v1`
and returns an opaque `ExactPlan`. Plan, authority, and work-contract digests
cover their complete RFC 8785 objects. SQLite retains the canonical plan; a
restart reparses it before restoring the structural capability.

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

After commit, the service returns a distinct `HistoricalApproval`. On restart,
the store reloads every retained byte, reparses the plan, and re-verifies the
signature and receipt against the configured root; it never trusts a stored
boolean. Historical approval proves what was approved at the recorded time. It
does not claim the source is current.

Builder execution, check execution, verifier dispatch, accepting `PASS`, and
integration will require separate short-lived gate-specific revalidation. No
such permit exists yet. The internal check-scheduling transaction requires its
receipt digest to identify an immutable authenticated historical approval for
the exact plan and verifies that the receipt precedes the succeeded builder; this
is provenance, not freshness. Prepared-submission construction consumes the
opaque `ExactPlan` directly and structurally compares its approval receipt with
that plan, but still cannot authenticate current authority or make the
submission reviewable.
