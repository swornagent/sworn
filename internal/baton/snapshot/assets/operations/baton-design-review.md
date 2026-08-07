---
operation: baton-design-review
version: baton.operation/v2
---

## Purpose

Check one proposed approach before implementation starts.

## Inputs

- The approved plan revision and stable slice contract.
- The design TL;DR and exact saved object it covers.
- The exact consumed product base and product fingerprints.
- Relevant repository facts and the Captain invocation.

## Authority

Review only this design attempt. The Captain must differ from its producer and
cannot change scope, approve the plan, implement, or issue a delivery verdict.
It may include a bounded correction with `PROCEED` only when the approved
contract and authority stay unchanged. The Captain never widens its own remit.

An exact recorded delegation may let the Captain authorize or revise an exact
existing proposal without pausing for a person, but only for the decisions that
delegation names. New scope, a changed target, destructive or high-stakes work,
genuine ambiguity, a protocol migration, or any wider remit still needs
explicit human approval, and refusing on those grounds is reported to the
operator.

## Actions

1. Confirm the plan, slice, design attempt, and exact binding agree.
2. Independently try to disprove the design: attack the promised outcome, each
   acceptance boundary, the exclusions, dependencies, consumed inputs,
   touchpoints, claimed reuse of existing owners, stated risks, and the
   proposed proof. Check the product, not the prose — a claim that an owner
   exists and behaves as assumed must be verified in the code.
3. Return exactly one decision, inside the exact approved contract:
   - `PROCEED` when implementation may begin, including with named bounded
     corrections inside the approved contract;
   - `REVISE` when a material design change needs another attempt on the same
     slice; or
   - `ESCALATE` naming the one precise decision that sits outside the recorded
     remit, when behavior, contract, authority, or an external decision
     requires revised approval.

## Required output

Lead with the decision and plain reason, then say what happens next. Put exact
bindings and the Captain invocation under technical details. Do not write the
Captain receipt.

## Stop conditions

Stop without a decision when approval, scope, authority, design identity, or
evidence is unclear. A tool or save failure is operational and creates no
Captain decision.

## Next handoff

`PROCEED` returns to `baton-implement` for implementation or bounded repair;
`REVISE` starts another design attempt on the same slice; `ESCALATE` hands to
`baton-plan`.
