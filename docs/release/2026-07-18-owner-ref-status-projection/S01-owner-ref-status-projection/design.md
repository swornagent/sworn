# Design: owner-ref status projection

## Outcome

Keep `DiscoverCatalog` as the single board projection authority, but replace its
cross-ref lifecycle election with the established ownership chain for every
topology-declared slice.

## Decisions

1. Derive the local owner ref as
   `refs/heads/track/<release>/<topology-track>`. A valid status there wins.
2. If the owner status is absent or invalid, use the already-selected topology
   source ref as the committed fallback.
3. Probe both supported Git-tree documentation prefixes. Do not traverse the
   logical symlink in the filesystem.
4. Do not admit launch working-tree bytes or statuses from non-owner refs.
5. Carry the selected CatalogRecord into named rendering so both JSON modes and
   text can identify topology provenance.

## Type-1 choices

None. The owner-track-first rule was already established by #81; issue #124 is a
compatibility correction to the #123 catalog implementation.

## Verification shape

The integration fixture executes the compiled command in a temporary consumer
repository. It commits the real symlink topology and creates deliberately
divergent release, owner, ghost, and dirty status records so the assertion fails
at the public CLI boundary if any competing authority participates.

## Recovery note

This design record was reconstructed during Baton artefact recovery after the
two implementation checkpoints had already landed. The immutable implementation
boundary remains the exact `release/v0.2.0` base through commit `60ff1e59`; the
fresh verifier must judge that complete range and must not treat this prose as
certification.
