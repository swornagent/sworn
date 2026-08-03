---
operation: baton-merge
version: baton.operation/v2
---

## Purpose

Combine passed work or merge the exact complete release covered by current
`PASS`.

## Inputs

- The approved plan revision and one saved Git snapshot.
- For a track, the passed slices in approved order.
- For the release, the exact assembled candidate and fresh assembly `PASS`.

## Authority

Merge never invents a verdict or resolves a product conflict. Get candidates,
order, and expected targets from approved facts. Stop before an unsafe change;
the surrounding system may then rescan and recover.
Merge acts on the exact passed candidate, including verified support work, not
a predicted file list.

## Actions

1. For a track, prove every required slice has an applicable `PASS`, hold the
   exact candidate still, and combine it in the approved order.
2. After all tracks pass, assemble the latest target with those exact products
   and stop for fresh `baton-verify` assembly. A later target advance rebuilds
   and rechecks only this assembly; it does not reset unchanged slices.
3. For release Merge, recheck the assembly `PASS`, candidate, authority, and
   expected target immediately before integration.
4. Perform the exact Git change and read the resulting target.
5. On an exact retry, return the already observed canonical result without
   duplicating the effect.

## Required output

Lead with what happened, what it means, and what happens next. Put scope, pass
bindings, component candidates, expected and observed targets, and resulting
commit under technical details for the machine-written receipt. Never report
partial success as `MERGED`.

## Stop conditions

Stop before changing Git on missing `PASS`, a changed candidate, divergent
target history, conflict, unexpected history or tree, unclear authority, or an
unreconciled effect. A stale snapshot needs a rescan, not a Baton verdict.

## Next handoff

Completed track composition waits for other tracks or assembly verification.
An exact release merge finishes Baton; deployment happens elsewhere.
