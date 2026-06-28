---
title: Orchestrator role
description: The Orchestrator (Sworn-side) coordinates execution of the Baton release loop — implement, verify, merge — as a deterministic Go engine. It holds no decision authority and escalates Type-1 choices to the Coach.
---

# Orchestrator

The **Orchestrator** is a SwornAgent role — the runtime engine that drives the Baton
release loop. It is a deterministic Go binary, not an agentic LLM, and it exists
at the **Sworn Layer-2** (runtime) rather than in the Baton contract layer.

## Relationship to Baton contract roles

Baton defines five contract roles; the Orchestrator lives outside them:

| Baton contract role | What it does | Relationship to Orchestrator |
|---------------------|-------------|------------------------------|
| **Coach** | Human-in-the-loop; owns strategy, product decisions, architectural authority | Orchestrator escalates Type-1 decisions to the Coach via PAGE events |
| **Planner** | Decomposes a release into slices; writes specs | Orchestrator dispatches the Planner at release start and on `/replan-release` |
| **Implementer** | Builds one slice against its spec; terminal state `implemented` | Orchestrator dispatches the Implementer per slice, in track order |
| **Verifier** | Fresh-context adversarial review; terminal state `verified` or `failed_verification` | Orchestrator dispatches the Verifier after `implemented`, reads the verdict, routes |
| **Captain** | Design-review gate (reads `design.md`, surfaces pins for the Coach); also the historical home of release-orchestration language | Orchestrator handles the workflow coordination the captain.md text once described; the Captain role's remaining function is `design-review` and captain-mode `/replan-release` spec-amendment ratification |

**The split:** The `captain.md` role prompt historically described both
design-review and release-orchestration as a single "on-field tactical leader"
role. In the formalised ontology, the Orchestrator absorbs the workflow
coordination (loop driving, role dispatch, state-machine routing); the Captain
retains only the design-review function (reading `design.md` before code,
surfacing pins). The Coach retains the human steering and authority the
captain.md text ascribed to itself.

## Responsibilities

The Orchestrator is responsible for:

1. **Reading the board oracle.** It consumes `status.json` files and the
   release `index.md` frontmatter to know the state of every slice and track
   in the release.

2. **Driving the loop.** The canonical implement → verify → merge cycle is
   the Orchestrator's main loop. For each actionable slice:
   - Dispatch the Implementer (`/implement-slice`), wait for terminal state
   - Dispatch the Verifier (`/verify-slice`), read the verdict
   - On PASS: advance slice to `verified`; on FAIL: route back to implement;
     on BLOCKED: route to `/replan-release`

3. **Routing escalations.** When the loop encounters a non-routine state
   (BLOCKED verifier verdict, track conflict, invariant violation), the
   Orchestrator routes to the correct resolver — the Planner for spec
   defects, the Coach for authority-boundary decisions.

4. **Dispatching role agents.** The Orchestrator selects and dispatches the
   correct role agent (Planner, Implementer, Verifier, Captain) at the
   correct state transition, with the correct artefacts as inputs.

5. **Managing pause and resume.** The Orchestrator supports stopping the
   loop mid-flight (at a committed slice boundary) and resuming from the
   durable track branch — see S07-pause-resume-committed.

6. **Enforcing invariants.** Before every state transition, the Orchestrator
   gates on the Baton safety invariants: sequential order within tracks,
   touchpoint disjointness across tracks, dependency-gate clearance,
   fail-closed verdict parsing.

## Authority

The Orchestrator has **zero decision authority**. It executes; it never chooses.

- **Type-1 decisions** (architecturally significant, hard to reverse) are
  escalated to the **Coach** via PAGE events. The Orchestrator never picks
  a Type-1 option on its own.
- **Type-2 decisions** (reversible, narrow blast radius) may be recorded
  with a noted default, but the Orchestrator itself does not originate them —
  the Implementer records those in the slice's `design.md` or `journal.md`.
- The Orchestrator does not amend specs, does not overrule verifier
  verdicts, and does not decide to skip a gate.

## Design choice: deterministic Go binary, not an agentic LLM

SwornAgent chose a **deterministic Go engine** over an agentic LLM orchestrator.
The rationale:

- **Deterministic is auditable.** An orchestrator that always makes the same
  decision on the same board state can be tested, replayed, and debugged.
- **Deterministic is cheaper.** No model call per state transition; the
  core loop runs in milliseconds.
- **Deterministic is reliable.** No prompt-drift, no model retraining
  breaking the loop, no non-deterministic routing.
- **The Baton spec does not mandate agentic coordination.** The spec says a
  coordinator exists and enforces the rules; it is implementation-neutral on
  whether the coordinator is deterministic or LLM-driven.
- **An agentic orchestrator remains the long-term hosted-layer direction**
  (Driver 3 — human-language release steering), but it is out of scope for
  the open-source `sworn` binary.

The formal Type-1 design decision record is at
[`docs/baton/decisions/orchestrator-model.md`](../decisions/orchestrator-model.md).

## What the Orchestrator is not

- **Not the Coach.** The Coach is the human who owns the team. The
  Orchestrator is a runtime binary.
- **Not the Captain.** The Captain reviews designs; the Orchestrator drives
  the loop. They share no code and no session.
- **Not a Planner.** The Orchestrator dispatches the Planner; it does not
  write or amend specs.
- **Not a Verifier.** The Orchestrator reads verdicts; it does not perform
  verification.