---
operation: baton-verify
version: baton.operation/v2
---

## Purpose

Independently verify one slice candidate or the complete assembled product
against the applicable approved contract.

## Inputs

- Scope: one stable slice or the assembled release.
- The applicable approved plan revision and exact candidate.
- The applicable Captain decision for slice verification.
- Required checks, observable evidence, and protected fresh read-only dispatch
  evidence.

## Authority

Begin in fresh context with read-only candidate access. Differ from the
Implementer and Captain. Bind the decision to the exact plan revision,
candidate, product identity, evidence, and invocation.

Judge the actual candidate against the approved behavioral commitment.
Ancillary support paths and additional checks are evidence, not scope failures
by themselves. They cannot excuse a material behavior, consumed-product,
contract, or authority change.

Product code, build, test, package, deploy, hooks, and runtime MUST NOT read or
depend on reserved `.baton/releases`; verify the candidate preserves it from
its exact implementation base.

## Actions

1. Re-establish every trust-critical binding from immutable facts.
2. Inspect the real candidate and complete diff, rerun required checks, consider
   useful additional evidence, and test each acceptance claim at its named
   boundary.
3. For assembly, check every composed component and the complete product.
4. Return exactly one verdict:
   - `PASS` when the exact candidate satisfies the contract;
   - `FAIL` when the contract is adequate but candidate or evidence needs
     correction;
   - `BLOCKED` when safe progress requires changed behavior, consumed product,
     authority, contract, or an external decision.

## Required output

Return scope, verdict, exact bindings, numbered evidence or violations,
Verifier invocation, and concise reason. Do not write the Verifier receipt.
On operational failure, return the condition and no verdict.

## Stop conditions

Never return `PASS` for contaminated context, writable candidate access,
missing approval, stale Captain decision, changed candidate, ambiguous
evidence, or unavailable required checks.

## Next handoff

Slice `PASS` hands to `baton-merge`; `FAIL` returns the same stable slice
directly to `baton-implement` for another implementation attempt; `BLOCKED`
hands to `baton-plan`. Assembly `PASS` permits final Merge.
