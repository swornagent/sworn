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

Work inside the approved slice and attempt. Implementation requires `PROCEED`
bound to this plan, slice, and design. `REVISE` adds a design attempt; `FAIL`
adds an implementation attempt. Neither replaces the slice.

Scope commits behavior and product, not exhaustive paths. Ancillary support
paths and extra checks are evidence unless they change behavior, consumed
product, contract, authority, or an external decision.

Product code, build, test, package, deploy, hooks, and runtime MUST NOT read or
depend on reserved `.baton/releases`; do not modify it.

## Actions

1. Before design, use the current release plan and exact engine-prepared
   consumed `PASS` base; track-local records are history. Return a concise
   design TL;DR covering approach, risks, and evidence. Stop.
2. After `PROCEED`, require that base again. Build the approved outcome, apply
   bounded corrections, preserve the reserved record root exactly, and repair
   prior `FAIL` on the same stable slice.
3. Run required and useful extra checks. Inspect the actual diff, candidate,
   product identity, and evidence.
4. Return acceptance-linked evidence over the exact candidate, including
   discovered support paths and extra results. Stop.

## Required output

Design output: plan revision, slice, attempt, TL;DR, exact binding, and Captain
handoff. Implementation output: attempt, candidate and product identities,
checks, evidence, deviations, and Verifier handoff. Never write receipts or
claim `PASS`.

## Stop conditions

Stop on missing approval, ambiguous eligibility, unmet dependencies, changed
consumed inputs, missing `PROCEED`, failed required checks, an ambiguous
candidate, a hard exclusion or approved product-boundary violation, or a
material behavior, contract, authority, or external-decision change. Report
operational failure without inventing a Baton outcome.

## Next handoff

Send a design to `baton-design-review`; send an implemented candidate and
evidence to a fresh `baton-verify`.
