---
operation: baton-implement
version: baton.operation/v2
---

## Purpose

Explain an approach or build one ready slice without doing the Captain's or
Verifier's job.

## Inputs

- The approved plan revision and stable slice identity.
- The attempt, dependencies, consumed inputs, scope, acceptance,
  checks, constraints, and exclusions.
- The Captain decision and prior Verifier decision when present.

## Authority

Stay inside the approved slice and attempt. Building requires `PROCEED` for
this plan, slice, and design. `REVISE` adds a design attempt; `FAIL` adds an
implementation attempt. Neither replaces the slice.

Scope commits behavior and product, not exhaustive paths. Ancillary support
paths and extra checks are evidence unless they change behavior, consumed
product, contract, authority, or an external decision.

Product code, build, test, package, deploy, hooks, and runtime MUST NOT read or
depend on reserved `.baton/releases`; do not modify it.

## Actions

1. Before design, use the current release plan and the exact base the engine
   prepared from consumed work that passed. Older track-local records are
   history. Return a design TL;DR covering approach, risks, and evidence. Stop.
2. After `PROCEED`, check that base again. Build the approved result, apply
   bounded corrections, preserve the reserved record root exactly, and repair
   prior `FAIL` on the same stable slice.
3. Run required and useful extra checks. Inspect the diff, candidate,
   product identity, and evidence.
4. Return acceptance-linked evidence over the exact candidate, support paths,
   and extra results. Stop.

## Required output

Lead with the result, meaning, and next step. Design output includes the TL;DR
and Captain handoff. Implementation output includes checks, evidence,
deviations, and Verifier handoff. Put plan revision, slice, attempt, exact
binding, candidate, and product identities under technical details. Never
write receipts or claim `PASS`.

## Stop conditions

Stop on missing approval, unclear eligibility, unmet dependencies, changed
consumed inputs, missing `PROCEED`, failed required checks, an unclear
candidate, a hard exclusion or approved product-boundary violation, or a
material behavior, contract, authority, or external-decision change. Report
operational failure without inventing a Baton outcome.

## Next handoff

Send a design to `baton-design-review` and candidate evidence to
`baton-verify`. The engine applies the Protocol's direct-repair rule and
assurance policy.
