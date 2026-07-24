---
operation: baton-plan
version: baton.operation/v1
---

## Purpose

Create a new approved delivery plan, or replace a pristine unmaterialised plan
under new external approval. Planning proposes authority; it never grants its
own approval.

## Inputs

- The requested release outcome, repository, target ref, and release ref.
- Ordered tracks and work, dependencies, touch surfaces, acceptance criteria,
  checks, constraints, and excluded paths.
- The protected approval reference and, after approval, its exact evidence
  digest.
- For a replan, the previously admitted plan and authoritative repository
  state.

## Authority

Use the strict plan parser and the admitted action surface supplied by the Baton
package. The external approval evidence must bind the raw digest of the complete
plan. Repository refs and records, not conversation, determine whether a plan
is pristine or materialised.

## Actions

1. Render `templates/plan.md` from byte zero. Keep its closed JSON metadata and
   Markdown consistent, then parse and validate the exact bytes.
2. Present those bytes for external approval without editing the proposal.
3. For a new release identity, call `installApprovedPlan` with the protected
   approval digest. This creates the plan and all baseline work statuses.
4. For an authorised revision, call `reboundPristinePlan` only when the prior
   namespace is exact, unmaterialised, and topologically identical.
5. If materialisation or durable handoffs exist, preserve that lineage and plan
   new work and release identities instead of clearing prior gates.

## Required output

Return the plan digest, approval reference and digest, release head, ordered
work, and the action receipt. A retry must identify the same durable result
without another commit.

## Stop conditions

Stop on missing approval, invalid metadata, overlapping independent work,
unsafe refs or paths, stale heads, foreign records, a non-pristine rebound, or
any action error. Do not partially install or rewrite history.

## Next handoff

Hand each ready work item to `baton-implement`, beginning with the first
dependency-ready work in each independent track.
