# ADR 0003: Contract before reviewable admission

- Date: 2026-07-20
- Status: accepted

## Context

The exact-plan authority slice brought the walking skeleton to 9,931 physical
production Go lines. That is inside ADR 0001's 8–10k estimate, but close enough
that the mandatory architecture stop applies before another feature is added.

The remaining milestone-2 boundary is also indivisible as a claim. A Baton
submission is `reviewable` only when its exact plan and policy, authenticated
approval, builder and producer journal history, runtime, artifacts, and reducer
state all agree. Persisting separate partial approvals or projecting any one of
those prerequisites as reviewable would create another truth.

The temporary measured-submission path still accepts caller-projected plan and
policy facts through `StructuralWork`, permits structural submission storage,
and reserves builder and producer identities separately from the effect journal.
Those seams were useful to test construction, but they must not become a second
admission architecture.

## Decision

Contract the existing path in three ordered changes:

1. Submission construction derives delivery, target, work contract, scope,
   acceptance, assurance, authority, and digests from one `ExactPlan`. It
   strictly resolves the plan's exact `assurance-policy-v1` registry and every
   selected Standard check definition by digest. Caller-projected equivalents
   are removed.
2. Builder and check results become typed effect-journal facts. Admitted checks
   use an engine-configured runtime tree staged and remeasured by the existing
   manifest boundary; its digest is bound through dispatch, environment, and
   result. The unmeasured host `/usr` path remains useful for evaluation but
   cannot support reviewable admission.
3. One gated admission transaction replaces structural submission persistence.
   It verifies locally persisted authenticated approval and all successful
   effect results, writes the submission and run bindings, applies exactly one
   reducer transition and event, and exposes `reviewable` only after commit.

The first two changes remain explicitly non-reviewable. The final transaction
must satisfy every gate together; authority, journal, runtime, and record
persistence do not become independent reviewability states.

Admission reuses the existing protocol, reducer, effect, store, and board
owners. It adds no admission service package, provenance framework, workflow
engine, database, or dependency. `submission_records` and the effect journal
remain the durable identities; builder and producer ownership moves to exact
effect IDs instead of a parallel identity namespace.

## Budget gate

Admission must replace temporary scaffolding rather than layer beside it. In
particular:

- remove caller-selected `StructuralWork` facts;
- replace exported structural `PutSubmission` with the atomic admission path;
- reuse strict JSON, record, artifact, authority, and effect primitives;
- keep the board a projection of committed engine truth; and
- keep the completed walking-skeleton change around 10k physical production
  lines and below 10k nonblank, noncomment lines; stop again if it materially
  exceeds either boundary or introduces a competing owner.

Physical line count remains a warning, not a target to game. Strict protocol,
Git, containment, and persistence checks are retained when deletion would move
the proof burden into convention or prompt text.

## Consequences

Reviewable admission lands later than a purely additive implementation, but its
trusted surface becomes smaller. A crash during either prerequisite leaves the
row `active`; a crash during admission leaves either the complete transaction or
none of it. Current authority re-resolution before dispatch, verifier verdicts,
`PASS`, repair and retry routing, and integration remain later milestones.
