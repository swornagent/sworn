---
operation: baton-merge
version: baton.operation/v1
---

## Purpose

Perform deterministic track composition, assembly preparation, or final release
integration without discretionary conflict resolution.

## Inputs

- Scope: `track`, `assembly`, or `release`.
- The admitted plan and one captured authoritative ref snapshot.
- For track scope, the track identity and every ordered work status.
- For assembly scope, the exact assembled proof bytes and producer invocation.
- For release scope, the assembly status covered by `PASS`.

## Authority

Use only `composeTrack`, `prepareAssembly`, and `integrateRelease` from the
admitted action surface. Those actions derive refs, candidates, targets,
topology, commit messages, and compare-and-set expectations from the plan.
Merge never invents a verdict or resolves a product conflict.

## Actions

1. For `track`, require every ordered work item at `merge / ready / merge`,
   freeze the exact track head, call `composeTrack`, and collectively transfer
   all work statuses to the release lineage in one record-only update.
2. After every planned track transfer is complete, use `assembly` to render
   `proof.md` for the complete product and call `prepareAssembly`. This
   atomically records the exact candidate, ordered component heads, proof, and
   initial `verify / ready / verifier` status.
3. Stop for a fresh `baton-verify assembly` invocation.
4. For `release`, require assembly `PASS` over the unchanged candidate and call
   `integrateRelease` against the exact expected target.
5. On an exact retry, return the existing canonical receipt without another
   commit.

## Required output

Return the scope, expected and observed heads, frozen component identities,
composition or integration result, transfer or preparation commit, and action
receipt.

## Stop conditions

Stop before mutation on incomplete work, absent prerequisites, conflict, stale
heads, changed candidates, moved targets, foreign records, unexpected parents
or trees, invalid assembly evidence, or any action error. Never report partial
success.

## Next handoff

Completed track scope waits for remaining tracks or proceeds to `assembly`.
Prepared assembly hands to `baton-verify assembly`. Completed release scope is
terminal for Baton delivery; deployment state is external.
