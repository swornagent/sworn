---
operation: baton-verify
version: baton.operation/v2
---

## Purpose

Independently check finished work against approval.

## Inputs

- Work identity, approved plan, exact candidate, and slice Captain decision.
- Required checks, observable evidence, and proof this thread and invocation
  are permitted and read-only.

## Authority

Start threads fresh and separate from delivery roles. Reuse one only for its own
recorded `FAIL`'s direct repair while approved bindings stay unchanged and no
later verdict exists. Keep every invocation read-only; bind its identity, exact
candidate, product identity, and evidence.

Judge the candidate against the approved commitment. Support paths and extra
checks are evidence, not scope failures by themselves. They cannot excuse a
material behavior, consumed product, contract, or authority change.

Ensure product code, build, test, package, deploy, hooks, and runtime neither
read nor depend on reserved `.baton/releases`; verify the candidate preserves it
from its exact implementation base.

## Actions

1. Recheck every trust fact from immutable saved evidence.
2. Inspect the exact candidate and full diff; rerun required checks, use helpful
   extra evidence, and test each acceptance claim where it matters. After
   repair, recheck the whole candidate and earlier findings.
3. For assembly, check every component and whole product.
4. Return exactly one verdict:
   - `PASS` when the exact candidate satisfies the contract;
   - `FAIL` when the contract is adequate but candidate or evidence needs
     correction;
   - `BLOCKED` when safe progress requires changed behavior, consumed product,
     authority, contract, or an external decision.

## Required output

Lead with verdict, reason, and next step; number evidence or violations. Put
exact bindings and invocation under technical details. Never write the receipt.
An operational failure returns no verdict. Make `FAIL` useful to the
Implementer and a fresh fallback; private context grants no authority.

## Stop conditions

Never return `PASS` with inherited implementation context, writable candidate
access, missing approval, a stale Captain decision, candidate movement after
dispatch, unclear evidence, or unavailable required checks.

## Next handoff

`PASS` hands to `baton-merge`; `FAIL` returns the same stable slice to
`baton-implement`; `BLOCKED` hands to `baton-plan`. The engine chooses
continuation under Protocol and policy. Assembly `PASS` permits final Merge.
