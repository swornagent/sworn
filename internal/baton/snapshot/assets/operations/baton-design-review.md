---
operation: baton-design-review
version: baton.operation/v1
---

## Purpose

Make the distinct Captain decision over one exact plan and design before
implementation begins.

## Inputs

- The admitted plan and authoritative work status.
- The exact `design.md` bytes and their recorded digest.
- Relevant live repository facts needed to judge the proposed approach.
- The Captain invocation identity.

## Authority

Review only `design / ready / captain`. The Captain invocation must differ from
the design producer and bind the current plan and design digests. The decision
does not alter approved scope or approve a plan.

## Actions

1. Confirm the design digest matches the current bytes and the work remains
   authoritative on its owning ref.
2. Check that the approach covers acceptance, respects scope and dependencies,
   identifies consequential decisions and risks, and proposes credible
   evidence.
3. Choose exactly one result:
   - `PROCEED` when implementation may begin under this design.
   - `REVISE` when the Implementer must produce new design bytes.
   - `ESCALATE` when new planning authority or an external decision is needed.
4. Construct the exact next status and record the chosen result through
   `recordTransition`.

## Required output

Return only the decision, plan digest, design digest, Captain invocation,
resulting durable projection, and action receipt. Include concise reasons as
review evidence outside the status.

## Stop conditions

Stop without a decision on missing or changed bytes, stale authority, an
invalid invocation boundary, absent evidence needed for review, or any action
error. Do not implement, verify, or silently expand scope.

## Next handoff

`PROCEED` hands back to `baton-implement`; `REVISE` hands back for a new design;
`ESCALATE` hands to `baton-plan` for new authority.
