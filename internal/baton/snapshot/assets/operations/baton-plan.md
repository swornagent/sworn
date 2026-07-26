---
operation: baton-plan
version: baton.operation/v2
---

## Purpose

Propose a bounded delivery plan or forward-only revision for external approval.
Planning never grants its own authority.

## Inputs

- The goal, repository, target, and external authorizer.
- Stable tracks and slices with scope, acceptance, checks, constraints,
  dependencies, consumed inputs, and exclusions.
- The prior approved revision and current repository facts when revising.

## Authority

Keep one release identity while its goal, target, and authority remain the same.
Retain unchanged slices. Change only the slices whose contracts changed and
identify the dependency closure whose consumed inputs changed. Approval must
bind the exact proposed bytes.

## Actions

1. Inspect current Git and the applicable plan revision.
2. Propose the smallest complete next revision using `templates/plan.md`.
3. Preserve stable slice identities; add or explicitly retire slices when the
   approved outcomes change.
4. Present the exact bytes for external approval and stop.

## Required output

Return the proposed plan bytes, release and revision, retained/changed/added/
retired slices, invalidated dependency closure, and concise approval handoff.
Do not write an approval or receipt.

## Stop conditions

Stop on ambiguous goal, target, authority, scope, dependencies, consumed
inputs, or acceptance. Do not guess approval or silently widen delivery.

## Next handoff

After external approval is durably bound to the exact plan, hand each
dependency-ready slice to `baton-implement`.
