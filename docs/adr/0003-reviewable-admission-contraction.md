# ADR 0003: Contract before reviewable admission

- Date: 2026-07-20
- Status: accepted

## Context

At the architecture stop, the exact-plan authority slice had brought the
walking skeleton to 9,931 physical production Go lines. That was inside ADR
0001's 8–10k estimate, but close enough that the mandatory stop applied before
another feature was added.

The remaining milestone-2 boundary was also indivisible as a claim. A Baton
submission is `reviewable` only when its exact plan and policy, authenticated
approval, builder and producer journal history, runtime, artifacts, and reducer
state all agree. Persisting separate partial approvals or projecting any one of
those prerequisites as reviewable would create another truth.

The temporary measured-submission path accepted caller-projected plan and policy
facts through `StructuralWork`, permitted structural submission storage, and
reserved builder and producer identities separately from the effect journal.
Those seams were useful to test construction, but were removed rather than
becoming a second admission architecture.

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
   result. Remove the host-runtime producer and public read-only executor path;
   writable builder execution remains a distinct, non-evidence capability.
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

The completed slice is 10,083 nonblank, noncomment and 11,083 physical
production Go lines at this commit, compared with 9,966 and 10,964 respectively
at its merged base. The 117-line semantic increase leaves the kernel 0.83%
above the 10k nonblank, noncomment target. This is an accepted commit-specific
variance for the indivisible admission closure, not a new budget.

Before accepting that variance, the implementation removed the caller-owned
structural submission path, standalone historical-approval restore API,
host-runtime local producer and public read-only host executor entry point, and
duplicate artifact storage code. No competing admission owner remains. Any
further net production growth requires another architecture stop.

## Consequences

Reviewable admission lands later than a purely additive implementation, but its
trusted surface is smaller. A crash before check dispatch leaves work `active`;
an interrupted check remains journal truth while work is internally `checking`.
A crash during admission leaves either the complete transaction or none of it.
Current authority re-resolution before dispatch, verifier verdicts, `PASS`,
repair and retry routing, and integration remain later milestones.
