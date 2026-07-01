# Orchestrator Notes

This file documents the relationship between the `captain.md` role prompt's
historical "release orchestrator" function and the Sworn deterministic engine.

**The release-orchestrator function is realised by the Sworn engine, not by a
role prompt for a human or an LLM agent.**

## What happened

The original `captain.md` (now split — see `design-reviewer.md`) described the
Captain as the "release-level orchestrator — the on-field tactical leader for
one release in flight" who coordinates the work of Planner, Implementer, and
Verifier across every slice. This conflated two identities:

1. **Design Reviewer** — the Captain in its design-review capacity: reviewing
   `design.md` before code, surfacing pins for the Coach, running the six-step
   review. This is an LLM-role function and remains in
   [`design-reviewer.md`](./design-reviewer.md).

2. **Release Orchestrator** — the Captain in its workflow-coordination
   capacity: driving the implement → verify → merge loop, routing on verdicts,
   managing state-machine transitions. This is now realised by the Sworn
   deterministic Go engine (`internal/scheduler/`, `internal/orchestrator/`,
   and the `sworn run` loop).

## Where the orchestration lives

The workflow coordination the original `captain.md` described — routing,
scheduling, managing the release loop — is not a prompt at all. It is a
**deterministic Go binary**. The Sworn engine:

- Reads the board oracle (consumes `status.json` and `index.md`)
- Drives the implement → verify → merge cycle
- Routes on verdicts (PASS → advance, FAIL → retry, BLOCKED → replan)
- Manages pause/resume at committed slice boundaries
- Enforces invariants before every state transition

See the formal role documentation at:

- [`docs/roles/orchestrator.md`](../../../docs/roles/orchestrator.md) — the
  Orchestrator role specification (S18-orchestrator-formalized)
- [`docs/decisions/orchestrator-model.md`](../../../docs/decisions/orchestrator-model.md) —
  the Type-1 design decision record for the deterministic Go engine over an
  agentic LLM orchestrator

## What remains of captain.md

`captain.md` still exists as the embedded prompt served by `prompt.Captain()`.
It is vendored **verbatim** from upstream Baton, which has not adopted the
split — so it still conflates the design-review and release-orchestrator
functions, and any split-notice header added locally is clobbered by parity
re-vendors. The engine therefore does not dispatch it for design review: the
design-review stage (`internal/captain`) uses `prompt.DesignReviewer()`. The
canonical home for the design-review function is `design-reviewer.md`.

## For implementers

- If you are building a feature that needs the **design-review prompt**, load
  `design-reviewer.md`.
- If you are building workflow coordination (loop driving, role dispatch,
  state-machine routing), you are building into the Sworn engine — reference
  the Orchestrator role spec, not a markdown prompt.
- `prompt.Captain()` continues to work and returns the captain.md content
  (which now includes the split notice). No caller breakage.