---
operation: baton-plan
version: baton.operation/v2
---

## Purpose

Turn a goal into small, checkable pieces of work for someone else to approve.
Planning never approves itself.

## Inputs

- The goal, repository, target, and person or system allowed to approve it.
- Stable tracks and slices: promised behavior, product scope, acceptance,
  minimum checks, constraints, real dependencies, consumed inputs, and
  exclusions.
- The current repository and prior approved plan when revising.
- Everything the working tree already answers: current Git state, the
  applicable plan and contracts, code, tests, documentation, and the history
  that explains why a surface looks the way it does.

## Authority

Keep the same release while its goal, target, and approval authority stay the
same. Keep unchanged slices. Change only slices whose contracts changed and
identify dependent slices whose consumed inputs changed. Approval must cover
the exact proposed plan bytes.

The plan is a commitment, not an inventory. Predicted paths, support work,
additional checks, evidence notes, scheduling, retries, worktrees, and
bookkeeping do not require revision when the promise is unchanged.

## Actions

1. Before asking a person anything, read the repository: current Git, the
   applicable plan and contracts, the code and tests the goal touches, the
   documentation that describes them, and any history that explains them.
   Never ask a person to restate a fact the repository already holds.
2. Present a short summary first: the result you intend to promise, its scope,
   acceptance, evidence, inputs, and limits. Ask a question only for a missing
   human choice that would materially change that promised result — one
   question, in your own words, no questionnaire.
3. Only after that summary or question is answered, propose the smallest
   complete plan or forward-only revision using `templates/plan.md`.
4. Give every slice one independently reviewable result, an acceptance
   boundary that can fail through the real product, the minimum end-to-end
   proof that runs the real built product, real dependencies and consumed
   products, and explicit touchpoint ownership.
5. Put only promised behavior, product surfaces, minimum proof, constraints,
   authority, and real product relationships into slice contracts. Reuse the
   product's existing owners; never invent a parallel function, field, schema,
   or component where one already exists.
6. Preserve stable slice identities; add or explicitly retire slices when the
   promised outcomes change.
7. Present the exact plan bytes for external approval and stop. Confirming
   your summary is not approval.

## Required output

Lead with what the plan will deliver, what changed, and what the approver should
do next. Put the exact plan, release, revision, retained/changed/added/retired
slices, and invalidated dependency closure under technical details. Do not
write an approval or receipt.

## Stop conditions

Stop when the goal, target, authority, scope, dependencies, consumed inputs,
acceptance, or a material contract change is unclear. Do not guess approval,
widen the work, or revise merely to record operational discovery. Do not
substitute a question for reading the repository.

## Next handoff

After external approval is durably bound to the exact plan, hand each
dependency-ready slice to `baton-implement`.
