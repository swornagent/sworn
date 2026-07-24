---
operation: baton-verify
version: baton.operation/v1
---

## Purpose

Independently verify either one work candidate or the complete assembled
release against exact approved evidence.

## Inputs

- Scope: `work` with a work identity, or `assembly` without one.
- The admitted plan, authoritative status, exact proof bytes, and candidate.
- Protected clean, read-only dispatch evidence for this invocation.
- Required checks and raw evidence references.

## Authority

Begin in fresh context with read-only candidate access. For work, the Verifier
must differ from the design producer, Implementer, and Captain. For assembly,
it must differ from the Merge proof producer. Verification binds the current
plan, proof, candidate, product tree, and dispatch evidence.

## Actions

1. Re-select authoritative state from captured refs and validate every current
   binding before inspecting the candidate.
2. Read only the approved plan, status, proof, candidate, and necessary live
   repository evidence.
3. Re-run required checks and test each acceptance claim at the boundary it
   describes. For assembly, verify every exact composed component and the
   complete product together.
4. Choose exactly one Baton verdict: `PASS`, `FAIL`, or `BLOCKED`.
5. Construct the exact next status and record that result through
   `recordTransition`.
6. If execution, transport, or persistence fails before a verdict, return
   `NO_VERDICT` operationally and leave durable status byte-for-byte unchanged.

## Required output

Return the scope, verdict, numbered evidence or violations, bound identities,
resulting projection, and action receipt. On operational failure, return the
failure and unchanged-state evidence, not a verdict.

## Stop conditions

Stop on contaminated or writable context, stale bindings, changed candidate,
missing proof, untrusted dispatch evidence, unavailable required evidence, or
any action error. Absence of evidence cannot become `PASS`.

## Next handoff

Work `PASS` hands to `baton-merge track`; work `FAIL` returns to
`baton-implement`; `BLOCKED` or assembly `FAIL` hands to `baton-plan`. Assembly
`PASS` hands to `baton-merge release`.
