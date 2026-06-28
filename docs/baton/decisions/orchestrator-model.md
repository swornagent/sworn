---
title: 'Orchestrator model — deterministic Go engine vs. agentic LLM'
description: 'Type-1 design decision: SwornAgent chose a deterministic Go binary as the release-loop orchestrator over an agentic LLM-driven orchestrator. Formal record per Rule 9 (Design Fidelity).'
---

# Design Decision: Orchestrator Model

- **Decision ID:** `orchestrator-model`
- **StakeClass:** Type-1 (hard to reverse — a switch to an agentic orchestrator
  would require re-architecting the entire loop, the role dispatch, and the
  state machine)
- **Decided by:** Brad Sawyer
- **Decided on:** 2026-06-27
- **Status:** Accepted

## Context

SwornAgent's release loop must coordinate the Planner, Implementer, Verifier,
and Captain roles across every slice in a release. The loop drives state
transitions (planned → in_progress → implemented → verified → merged),
routes on verdicts (PASS/FAIL/BLOCKED), and enforces the Baton safety
invariants (sequential track order, touchpoint disjointness, dependency
gates).

Two implementation models were considered for the coordination layer.

## Options considered

### (a) Deterministic Go engine

The Orchestrator is a Go binary compiled into the `sworn` CLI. It reads the
board oracle (`status.json` + `index.md` frontmatter), walks a deterministic
state machine, and dispatches role agents at each transition. Routing is
rule-based: a BLOCKED verdict always routes to `/replan-release`, a FAIL
always routes back to the Implementer, a dependency gate always blocks
until the predecessor merges.

### (b) LLM-driven orchestrator agent

The Orchestrator is an agentic LLM session (the "Coordinator model") that
reads the same board artefacts, decides what to dispatch next, and interprets
outcomes. This is the approach described in the `CLAUDE.md` "Three Driver
Model" (Driver 3) and the `project_dynamic_workflows_future_direction.md`
memory entry — a hosted-layer orchestrator that accepts human-language
release steering.

## Decision

**Option (a): Deterministic Go engine.**

## Rationale

1. **Deterministic is auditable.** Every state transition is reproducible
   from the same board state. Tests can assert exact routing paths.
   Debugging is `go test` and `dlv`, not prompt-inspection.

2. **Deterministic is cheaper.** No model call per state transition. The
   core loop — oracle read → pick next slice → dispatch → read verdict →
   route — runs in single-digit milliseconds. An agentic orchestrator would
   spend tokens on every routing decision.

3. **Deterministic is reliable.** No prompt-drift across model versions.
   No non-deterministic routing that manifests as "the loop sometimes stalls
   on the same board." The failure modes are Go errors, not model
   hallucinations.

4. **The Baton spec is implementation-neutral on coordination.** Baton
   specifies *what* the coordinator must enforce (invariants, state
   machine, fail-closed verdict) but not *how*. A deterministic engine
   satisfies the spec. Nothing in the spec mandates an agentic coordinator.

5. **The LLM orchestrator remains the long-term hosted-layer direction,**
   but it is out of scope for the open-source `sworn` binary. Driver 3
   (human-language release steering, multi-release coordination,
   cross-project analytics) is the hosted product's surface. The open
   binary ships the deterministic core and leaves the agentic layer to
   the hosted product.

## Consequences

- The Orchestrator will never make a Type-1 decision — those escalate to
  the Coach.
- The Orchestrator will never interpret ambiguous outcomes — an
  unparseable verdict is a BLOCKED, not an inference call.
- The agentic verifier (S11) and the LLM interpreter (S01) are the two
  places where LLM calls enter the loop, each with a well-defined
  contract. The Orchestrator itself stays deterministic.
- If the hosted-layer agentic orchestrator is built later, it wraps the
  deterministic core rather than replacing it — same oracle, same state
  machine, same gates, with an LLM-driven scheduling policy on top.