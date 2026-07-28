---
operation: baton-implement
version: baton.operation/v2
---

## Purpose

Design or implement one eligible slice without crossing the Captain or
Verifier boundary.

## Inputs

- The applicable approved plan revision and stable slice identity.
- The current attempt, dependencies, consumed inputs, scope, acceptance,
  checks, constraints, and exclusions.
- The applicable Captain decision and prior Verifier decision when present.

## Authority

Work only inside the approved slice and current attempt. Implementation
requires an applicable `PROCEED` bound to this plan revision, slice, and design
attempt. A `REVISE` adds a design attempt; a `FAIL` adds an implementation
attempt. Neither replaces the slice.

## Actions

1. Before design, require the engine-prepared exact current consumed `PASS`
   base. Then inspect the work and return a concise design
   TL;DR covering approach, touched surfaces, consequential decisions, risks,
   and evidence. Stop.
2. After applicable `PROCEED`, require the exact current consumed `PASS` base
   again, then build only the approved product scope.
3. Run required checks and inspect the real diff and candidate.
4. Return acceptance-linked evidence over the exact candidate and stop.

## Required output

For design, return the plan revision, slice, design attempt, TL;DR, exact
binding, and Captain handoff. For implementation, return the implementation
attempt, candidate and product identities, check results, evidence references,
deviations, and Verifier handoff. Do not write protocol receipts or claim
`PASS`.

## Stop conditions

Stop on missing approval, ambiguous eligibility, unmet dependencies, changed
consumed inputs, missing `PROCEED`, scope escape, failed required checks, or an
ambiguous candidate. Report operational failure without inventing a Baton
outcome.

## Next handoff

Send a design to `baton-design-review`; send an implemented candidate and
evidence to a fresh `baton-verify`.
