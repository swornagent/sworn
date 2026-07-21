# Baton Core 1.0

Baton is a protocol for autonomous software delivery whose completion claims can
be trusted. It specifies the facts that must cross between a delivery engine, a
builder, a verifier, and an authorizer. It does not prescribe prompts, Git
commands, model providers, user interfaces, or project-management ceremony.

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative.

## B1 — Bounded Authority

Autonomy operates inside an approved delivery plan. The plan MUST state the
desired outcome, included and excluded scope, acceptance criteria, allowed
effects, assurance requirements, and integration authority.
Approval MUST be authenticated by an authorizer capability or trust root that is
unavailable to the autonomous caller, builder, and verifier. An identity claim
or policy file they can forge or replace is not authority.

An actor MUST NOT silently widen the outcome, scope, or effects it is authorized
to change. An engine-discovered authority failure MUST stop for authorizer
attention without inventing a verdict; a verifier-discovered authority or
contract failure is `SPEC_BLOCK`. A changed contract creates new work; it does
not retroactively authorize an attempt.

## B2 — Durable Truth

Claims that affect delivery MUST survive the session that made them. The plan,
submission, verdict, authority references, and relevant evidence MUST be stored
as validated records or independently addressable artifacts.

Chat transcripts, mutable labels, and an actor's recollection are not delivery
truth. A board is a projection of durable records and repository facts; it MUST
NOT be an independent state writer. Deferrals and exceptions are valid only
when the approved plan bounds them or a new authority record accepts them.
Completed effects remain historical truth after later authority or policy
changes; their receipts MUST preserve the facts and authorization used at the
time of the effect.

## B3 — Real Evidence

A submission MUST identify the exact candidate and provide falsifiable evidence
for every acceptance criterion. Evidence MUST come from an engine-registered
producer over the live candidate and MUST state the boundary and environment it
exercised. A builder's unsupported assertion is not evidence.

A leaf test cannot prove an assembled behavior. A mock cannot prove a real
integration. The strength of the evidence MUST match the claim. Missing,
stale, fabricated, or unreachable evidence cannot support `PASS`.

## B4 — Independent Verification

No actor may certify its own work. A `PASS` verdict MUST come from a verifier
run with fresh context, no inherited implementation transcript, and no authority
to alter the candidate it reviews.

Verifier instructions, configuration, and capabilities MUST come from an
engine-controlled context. Candidate-local agent instructions, plugins, hooks,
or tool configuration MUST NOT become verifier control input.

The verdict MUST bind the immutable submission, contract, policy, and exact
candidate. The verifier SHOULD actively try to disprove completion and MAY
raise the assurance level. It MUST NOT lower approved assurance requirements.

## B5 — Safe Composition

Only the exact candidate covered by a valid `PASS` may be integrated. A change
to candidate bytes, base revision, contract, or assurance policy invalidates the
verdict.

Integration MUST use an expected target revision and fail closed when the target
has moved. If independently verified work is composed into a new candidate, the
composition MUST itself be checked and verified before it is represented as
delivered. Predicted touchpoints are scheduling hints; actual repository facts
decide composition safety.

## The trust kernel

A conforming delivery therefore has one short causal chain:

> approved plan -> exact candidate -> real evidence -> fresh verdict -> safe integration

Everything else is an implementation choice or a risk-selected assurance pack.
