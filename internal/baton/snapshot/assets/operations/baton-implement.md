---
operation: baton-implement
version: baton.operation/v1
---

## Purpose

Advance one authoritative work item through design or implementation without
crossing the Captain or Verifier boundary.

## Inputs

- The admitted plan and authoritative `status.json`.
- The work identity, owning track, approved acceptance criteria, checks,
  constraints, and touch surfaces.
- The current design, Captain result, proof, and candidate when present.
- The Baton package templates and admitted action surface.

## Authority

Follow the owner-aware record selected from captured refs. Only the next
eligible work in its track may advance. Use `materializeTrack` before the first
owned transition. Never supply refs, targets, commit messages, or arbitrary Git
effects to the action surface.

## Actions

1. If status is `design / ready / implementer`, inspect the approved scope,
   write or revise `design.md`, and record `DESIGN_WRITTEN` with the exact
   design bytes.
2. Stop for `baton-design-review`. A current `REVISE` returns to step 1; a
   current `ESCALATE` remains blocked for new planning authority.
3. Continue only from `implement / ready / implementer` with current
   `PROCEED`, or after a Verifier `FAIL` returns the work there.
4. Build only the approved product scope. Run required checks and make the
   final candidate commit product-only.
5. Render `proof.md` from live evidence, binding the exact base, candidate,
   candidate tree, product tree, plan, approval, design, Captain invocation,
   and Implementer invocation.
6. Record `IMPLEMENTED` with the exact proof bytes, then stop.

## Required output

For design, return its digest, transition receipt, and Captain handoff. For
implementation, return the exact candidate and product identities, proof
digest, check evidence references, transition receipt, and Verifier handoff.

## Stop conditions

Stop on stale or foreign status, unmet dependencies, changed approval or
design, missing `PROCEED`, scope escape, failed checks, dirty evidence, product
changes hidden after the candidate, or any action error. Never claim `PASS`.

## Next handoff

Send a completed design to `baton-design-review`; send an implemented candidate
and proof to a fresh `baton-verify` work invocation.
