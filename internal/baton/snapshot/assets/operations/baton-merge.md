---
operation: baton-merge
version: baton.operation/v2
---

## Purpose

Mechanically compose passed track candidates or integrate the exact assembled
candidate covered by current `PASS`.

## Inputs

- The applicable approved plan revision and one captured Git snapshot.
- For track composition, the ordered passed slice candidates.
- For release integration, the exact assembled candidate and fresh assembly
  `PASS`.

## Authority

Merge never invents a verdict or resolves a product conflict. Derive candidates,
topology, and expected targets from approved facts. Refuse unsafe mutation, then
allow the surrounding system to rescan and reconcile operational outcomes.
Merge acts on the exact passed candidate, including its verified support work;
it does not compare that candidate with a predicted path inventory.

## Actions

1. For a track, prove every required slice has applicable `PASS`, freeze the
   exact candidate, and compose it through the approved topology.
2. After all tracks are present, identify the exact assembled candidate and
   stop for fresh `baton-verify` assembly.
3. For release Merge, recheck the assembly `PASS`, candidate, authority, and
   expected target immediately before integration.
4. Perform the exact Git effect and observe the resulting target.
5. On an exact retry, return the already observed canonical result without
   duplicating the effect.

## Required output

Return the scope, applicable pass bindings, component candidates, expected and
observed targets, resulting commit, and concise outcome for a machine-written
receipt. Never report partial success as `MERGED`.

## Stop conditions

Stop before mutation on missing `PASS`, changed candidate, unsafe target
movement, conflict, unexpected ancestry or tree, ambiguous authority, or an
unreconciled effect. A stale snapshot is an operational rescan condition, not a
Baton verdict.

## Next handoff

Completed track composition waits for remaining tracks or assembly
verification. Exact release integration is terminal for Baton; deployment is
external.
