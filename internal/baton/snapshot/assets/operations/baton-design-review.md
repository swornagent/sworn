---
operation: baton-design-review
version: baton.operation/v2
---

## Purpose

Make the distinct Captain decision over one exact plan revision, slice, and
design attempt before implementation.

## Inputs

- The applicable approved plan revision and stable slice contract.
- The exact design TL;DR and immutable object it binds.
- The exact consumed product base and its product-tree pins.
- Relevant repository facts and the Captain invocation identity.

## Authority

Review only the presented design attempt. The Captain must differ from its
producer and cannot change approved scope, approve a plan, implement, or issue
a delivery verdict. It may carry a bounded correction with `PROCEED` only when
the correction leaves the approved contract and authority unchanged.

## Actions

1. Confirm the plan, slice, design attempt, and immutable binding agree.
2. Check acceptance coverage, scope, dependencies, the exact consumed product
   base,
   consequential decisions, risks, and proposed evidence.
3. Return exactly one decision:
   - `PROCEED` when implementation may begin, including with named bounded
     corrections inside the approved contract;
   - `REVISE` when a material design change needs another attempt on the same
     slice; or
   - `ESCALATE` when behavior, contract, authority, or an external decision
     requires revised approval.

## Required output

Return only the decision, exact bindings, Captain invocation, and concise
reason. Do not write the Captain receipt.

## Stop conditions

Stop without a decision when approval, scope, authority, design identity, or
evidence needed for review is ambiguous. An execution or persistence failure
is operational and creates no Captain decision.

## Next handoff

`PROCEED` returns to `baton-implement` for implementation or bounded repair;
`REVISE` starts another design attempt on the same slice; `ESCALATE` hands to
`baton-plan`.
