# Design review: owner-ref status projection

## Verdict

PROCEED

## Pins

1. The owner ref must be derived from board topology, never from the stale
   `track` value of a competing status copy.
2. Release fallback must use the selected CatalogRecord source ref so remote or
   noncanonical selected topology continues to resolve consistently.
3. Canonical documentation paths must be probed as Git objects; filesystem
   symlink traversal is not authoritative.
4. Named and aggregate CLI tests must assert source and durability, not lifecycle
   state alone.
5. Fresh verification must inspect the full implementation range beginning at
   `68a578b10d9c8c69632aad96301e4fc04dff0de0` and independently rerun the public
   CLI fixture.

No constitutional conflict or open design choice remains. This is a recovered
mechanical design review, not an implementation verdict.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The owner-track authority is already ratified and the design preserves one catalog projection boundary with integration-level proof.
-->
