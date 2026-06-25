---
title: 'S47-orchestrator-recovery — intelligent triage on non-PASS instead of fixed branches'
description: 'On a non-PASS verdict, sworn run runs an intra-run triage that chooses the next action — resolve-in-place (S44 feedback, same model), escalate the model, or halt — then commits the resulting state and DELEGATES the lifecycle decision (BLOCKED→replan, failed_verification→redesign vs implement) to the ported router (S58). Restores the coach orchestrator''s judgment without duplicating the router''s state machine.'
---

# Slice: `S47-orchestrator-recovery`

> **Re-scoped 2026-06-23** (orchestration-core replan). S47 no longer reimplements the
> lifecycle/BLOCKED-resolvability decision — that is owned by the ported router
> **S58-slice-router** (T17), which maps `BLOCKED → replan-release` and `failed_verification →
> redesign | implement`. S47 keeps only the **intra-run** escalation budget the router does not
> cover: the router runs *between* dispatches off committed state; S47's triage runs *within* one
> `RunSlice` model-escalation loop. **Depends on T17-orchestration-core (S58).**

## User outcome

When `sworn run` gets a non-PASS verdict, it **decides intelligently** what to do next *within the
run attempt* instead of a fixed branch: resolve-in-place (retry the same model with the verifier
feedback from S44), escalate to the next model, or halt — based on the verdict, rationale, and
attempt history. On a BLOCKED verdict it commits the `blocked` state (carrying the verifier's
violations) and hands off; the **router (S58) routes `BLOCKED → replan-release`** — S47 does not
re-classify spec-defect vs genuine here (the verifier writes the diagnosis, the router routes it).

## Entry point

`sworn run` → a recovery-triage step in `RunSlice`, replacing the current deterministic
`switch lastVerdict.Verdict` (slice.go:199) with a triage decision.

## Background

Today `RunSlice` handles verdicts with fixed branches: PASS→verified, BLOCKED→`return error`
immediately, FAIL/Inconclusive→`continue` (escalate next model). This embodies none of the
coach orchestrator's judgement — notably, every BLOCKED hard-stops even when it's a resolvable
spec defect, and FAIL always escalates the model even when the *same* model with the S44 feedback
would resolve it. This slice adds the triage layer; S44 (feedback retry) is its in-place-resolve
primitive.

## In scope

- A triage step that, given `{verdict, rationale, attempt, escalationBudget, priorFeedback}`,
  returns an **intra-run** action: `resolve_in_place` (re-run same model with S44 feedback),
  `escalate_model` (advance escalation slot), or `halt` (commit the terminal state — `blocked` or
  `failed_verification` — and return control to the loop/router).
- Policy (first cut, deterministic + explainable): FAIL/Inconclusive → `resolve_in_place` for the
  first K attempts on the same model (default K=1), then `escalate_model`, then `halt` when the
  escalation list is exhausted. BLOCKED → `halt` immediately, committing `blocked` with the
  verifier's violations; the **router (S58) then routes it to `replan-release`** (S47 does not
  classify spec-defect vs genuine).
- A clear, logged rationale for each triage decision (auditability).

## Out of scope

- **Lifecycle routing and BLOCKED-resolvability classification** — owned by the ported router
  **S58-slice-router** (`BLOCKED → replan-release`; `failed_verification → redesign | implement`).
  S47 commits the state and halts; the router (driven by S59's worker loop) decides what runs next.
- A full LLM-driven triage model call — the first cut is a deterministic, explainable policy. A
  general LLM-orchestrator is **deferred** (Rule 2; why: prove the policy shape first; tracking:
  follow-up; ack: Coach 2026-06-21).
- Changing the escalation model list / rotation (S09 / existing) — S47 chooses *whether* to
  escalate, not the order.

## Design decisions (for the captain review to ratify)

- **Heuristic-first**: proposed deterministic intra-run policy above (no LLM call this slice — the
  BLOCKED-resolvability judgment moved out to the router/verifier). Confirm.
- **`resolve_in_place` budget K**: proposed default 1 (one same-model feedback retry before
  escalating). Confirm.
- **Reuse `blocked`, no new state**: S47 commits the existing `blocked` state on a BLOCKED halt
  and lets S58 route it; no `awaiting_replan` state is added. Confirm.

## Planned touchpoints

- `internal/run/slice.go` (replace the verdict switch with the triage call; on `halt`, commit the
  terminal state and return — the worker loop/router takes the lifecycle decision)
- `internal/orchestrator/triage.go` (new — the intra-run policy + action type)
- `internal/orchestrator/triage_test.go` (new)

## Acceptance checks

- [ ] A FAIL on attempt 0 returns `resolve_in_place` (same model) and the retry carries the S44
  feedback; a second FAIL returns `escalate_model`
- [ ] Escalation list exhausted by FAILs returns `halt`, committing `failed_verification` (fail-closed),
  not a loop
- [ ] A BLOCKED verdict returns `halt` immediately, committing `blocked` with the verifier's
  violations populated (S38) — and does NOT re-classify spec-defect vs genuine here (that routing
  is S58's `BLOCKED → replan-release`)
- [ ] Each triage decision logs an explainable rationale
- [ ] `go test -race ./internal/orchestrator/... ./internal/run/...` passes

## Required tests

- **Unit**: `internal/orchestrator/triage_test.go` — `TestFailResolvesThenEscalates`,
  `TestExhaustedEscalationHalts`, `TestBlockedHaltsCommitsBlocked` (asserts `blocked` committed
  with violations, no spec-defect re-classification).
- **Reachability artefact**: paste in `proof.md` the triage decision log across a scripted
  FAIL→resolve→FAIL→escalate→PASS sequence.

## Risks

- Must not introduce an infinite resolve-in-place loop — the K budget and the escalation
  exhaustion → halt are the guards; test both.
- **Layer confusion with S58.** S47 must NOT reimplement lifecycle routing — on `halt` it commits
  the terminal state and returns; the router (S58, driven by S59) decides what runs next. A
  re-introduced BLOCKED-classification here would duplicate the router and risk divergent verdicts.

## Deferrals allowed?

Yes, with Rule 2 — the full LLM-orchestrator and the interactive human-halt UX carry forward
with why/tracking/ack; the deterministic intra-run triage policy is the landing scope. Lifecycle
routing + BLOCKED-resolvability are not deferred — they are reassigned to S58 (T17).
