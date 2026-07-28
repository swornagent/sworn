# Baton Core 1.0

Baton is a small protocol for delivering software autonomously without treating
an agent’s confidence as proof. It keeps five trust boundaries stable while
leaving scheduling, retries, recovery, worktrees, drivers, projections, and
telemetry to the system using it.

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative.

## B1 — Stay inside the agreed work

Autonomy begins with an exact externally approved plan. It states the goal,
scope, acceptance, checks, dependencies, constraints, target, and integration
authority.

Those fields are commitments, not an exhaustive implementation inventory.
Scope names the approved behavioral and product surfaces, checks are the
required minimum, constraints are semantic limits, and dependencies name real
product inputs. Ancillary support paths, additional checks, evidence detail,
and delivery bookkeeping MAY be discovered while implementing without
revising the plan when the approved commitment is unchanged.

The Planner may propose a forward-only revision but cannot approve it. A
material change requires approval of the revised plan. Unchanged slices retain
their identities and prior trusted facts unless their contract or consumed
inputs changed.

## B2 — Keep durable facts

Facts that decide what happens next MUST survive the conversation that produced
them. Baton keeps the applicable approved plan, stable slice and attempt
identities, exact candidates, evidence, role decisions, and Merge observations.
Compact machine-written receipts bind those facts to immutable Git objects.

The candidate diff, tests, code, comments, commits, and external evidence may
carry the detail. Baton does not require duplicate narrative artefacts.

Chat, mutable dashboards, timestamps, and recollection are not delivery truth.
A board is a read-only projection of the plan, receipts, and Git.

## B3 — Prove the real result

Each acceptance claim MUST link to falsifiable evidence at the boundary it
names. A leaf test cannot prove an assembled journey, a mock cannot prove a
real integration, and an unsupported statement is not evidence.

The repository, candidate, tree, changed product, checks, and evidence
references are independently observable. Missing, stale, fabricated, or
unreachable evidence cannot support completion.

## B4 — Use a fresh independent Verifier

No Implementer certifies its own work. Verification runs in a clean context
with no inherited implementation conversation, no authority to change the
candidate, and read-only access to the exact candidate and applicable facts.

The Verifier returns `PASS`, `FAIL`, or `BLOCKED`. A crash, timeout,
cancellation, malformed response, or bookkeeping failure creates no verdict.
A `PASS` binds the applicable plan, Captain decision, exact candidate,
evidence, and fresh Verifier invocation. Changing a bound fact invalidates it.

## B5 — Merge only what passed

Merge is mechanical. It rechecks current authority and integrates only the
exact candidate covered by `PASS` against the expected target.

A changed candidate, unsafe target movement, conflict, ambiguous composition,
or persistence ambiguity stops without claiming success. Multi-track delivery
requires fresh verification of the complete assembled product before final
Merge.

## The useful minimum

```text
approved plan
  -> Implementer design TL;DR
  -> Captain decision
  -> candidate and evidence
  -> fresh Verifier
  -> exact Merge
```

A procedural defect is not a Baton `BLOCKED` outcome when these facts can still
be established. The engine reconstructs, retries, or reports it operationally
without inventing a trust fact.
