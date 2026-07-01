---
title: 'S18 — Orchestrator role formally specified as a Sworn-side artefact'
description: 'Create docs/baton/roles/orchestrator.md naming the Orchestrator role; record the deterministic-vs-agentic design choice as a Type-1 decision in a status.json design decision record; distinguish Orchestrator (Sworn) from Coach/planner/implementer/verifier (Baton).'
---

# Slice: `S18-orchestrator-formalized`

## User outcome

`docs/baton/roles/orchestrator.md` exists in the repo and names the Orchestrator role, its responsibilities (coordinate execution, hold no authority, escalate Type-1 to the Coach), and its relationship to the existing Baton roles. The deterministic-vs-agentic design choice (why Sworn chose a deterministic Go engine over an agentic LLM orchestrator) is recorded as a Type-1 design decision in `docs/baton/decisions/orchestrator-model.md`.

## Entry point

`docs/baton/roles/` (new directory); `docs/baton/decisions/` (new or existing). This is a documentation slice with no production code changes.

## In scope

- `docs/baton/roles/orchestrator.md`: specification of the Orchestrator role:
  - Role name: Orchestrator (Sworn-side; contrasted with Coach, Planner, Implementer, Verifier, Captain which are Baton contract roles)
  - Responsibilities: read the board oracle, drive the loop (implement → verify → merge), route escalations, dispatch role agents, manage pause/resume, enforce invariants
  - Authority: zero — the Orchestrator executes; it never makes Type-1 decisions; those are escalated to the Coach via PAGE events
  - Design choice: deterministic Go binary (current) rather than an agentic LLM orchestrator; rationale = deterministic is auditable, reproducible, cheaper, and the Baton spec doesn't mandate how the coordinator is implemented
  - Relationship to captain.md: captain.md is the design-reviewer role (Baton); the release-orchestrator role described in captain.md is actually the Orchestrator (Sworn) plus the Coach's steering function
- `docs/baton/decisions/orchestrator-model.md`: Type-1 design decision record
  - Stakes: Type-1 (hard to reverse — a switch to an agentic orchestrator would require re-architecting the whole loop)
  - Options considered: (a) deterministic Go engine, (b) LLM-driven orchestrator agent
  - Decision: (a) deterministic Go engine
  - Rationale: deterministic is auditable, fits the Baton spec (spec says coordinator exists; doesn't mandate agentic), cheaper, more reliable; LLM-driven orchestrator is the long-term hosted-layer direction but is out of scope for the open binary
  - Decided by: Brad Sawyer, 2026-06-27
- Status.json for this slice: set `open_deferrals: []`, `state: "planned"`, `covers_needs: ["N-20"]`

## Out of scope

- Changing any production Go code
- The captain.md split (S19)
- Any changes to how the Orchestrator runs (it just gets a name and a decision record)

## Planned touchpoints

- `docs/baton/roles/orchestrator.md` (new)
- `docs/baton/decisions/orchestrator-model.md` (new)

## Acceptance checks

- [ ] `docs/baton/roles/orchestrator.md` exists and contains the word "Orchestrator" as a role name, its responsibilities, and its relationship to the Baton roles
- [ ] `docs/baton/decisions/orchestrator-model.md` exists and contains a StakeClass field with value "type-1" or equivalent language
- [ ] `docs/baton/decisions/orchestrator-model.md` explicitly names the decided option ("deterministic Go engine") and the human decision-maker ("Brad Sawyer") with a date
- [ ] `docs/baton/decisions/orchestrator-model.md` contains the rationale for rejecting option (b) LLM-driven orchestrator
- [ ] Both files are valid Markdown and pass `sworn lint design` without errors (if applicable — the design gate checks for design decisions in status.json, not standalone docs)

## Required tests

- **Reachability artefact**: manual smoke step: `cat docs/baton/roles/orchestrator.md` and `cat docs/baton/decisions/orchestrator-model.md` exist and are non-empty; the Orchestrator role is named and the Type-1 decision is recorded

## Risks

- The `docs/baton/` directory may already exist with content (ADRs etc.); the implementer must not overwrite existing files

## Deferrals allowed?

No.
