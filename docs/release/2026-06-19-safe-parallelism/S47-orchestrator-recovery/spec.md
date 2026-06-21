---
title: 'S47-orchestrator-recovery — intelligent triage on non-PASS instead of fixed branches'
description: 'On a non-PASS verdict, sworn run runs a triage step that chooses the next action — resolve-in-place (S44 feedback, same model), escalate the model, or halt for a human — and assesses a BLOCKED verdict for resolvability (a spec defect the planner can fix) rather than hard-stopping. Restores the coach orchestrator/interpreter-talkback judgment to the product loop.'
---

# Slice: `S47-orchestrator-recovery`

## User outcome

When `sworn run` gets a non-PASS verdict, it **decides intelligently** what to do next instead
of running a fixed branch: resolve-in-place (retry the same model with the verifier feedback from
S44), escalate to the next model, or halt for a human — based on the verdict, the rationale, and
the attempt history. A BLOCKED verdict is assessed for **resolvability**: a spec defect the
implementer/planner can correct is surfaced as such (the coach-loop's BLOCKED→planner handoff),
rather than every BLOCKED hard-stopping the run.

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
  returns an action: `resolve_in_place` (re-run same model with S44 feedback),
  `escalate_model` (advance escalation slot), or `halt` (surface for human).
- Policy (first cut, deterministic + explainable): FAIL/Inconclusive → `resolve_in_place` for the
  first K attempts on the same model (default K=1), then `escalate_model`, then `halt` when the
  escalation list is exhausted. BLOCKED → classify spec-defect vs genuine: a spec-defect block
  surfaces an `awaiting_replan`-style outcome (carry the verifier's proposed amendment), a genuine
  block halts.
- A clear, logged rationale for each triage decision (auditability).

## Out of scope

- A full LLM-driven triage model call — the first cut is a deterministic, explainable policy with
  a single optional hook for an LLM assessment of *BLOCKED resolvability* only. A general
  LLM-orchestrator is **deferred** (Rule 2; why: prove the policy shape first; tracking: follow-up;
  ack: Coach 2026-06-21).
- Changing the escalation model list / rotation (S09 / existing) — S47 chooses *whether* to
  escalate, not the order.

## Design decisions (for the captain review to ratify)

- **Heuristic-first**: proposed deterministic policy above, with an LLM hook only for BLOCKED
  resolvability. Confirm vs a fuller LLM triage now.
- **`resolve_in_place` budget K**: proposed default 1 (one same-model feedback retry before
  escalating). Confirm.

## Planned touchpoints

- `internal/run/slice.go` (replace the verdict switch with the triage call)
- `internal/orchestrator/triage.go` (new — the policy + action type)
- `internal/orchestrator/triage_test.go` (new)
- `internal/state/state.go` (an `awaiting_replan`/spec-defect outcome, if not reusing `blocked`)

## Acceptance checks

- [ ] A FAIL on attempt 0 returns `resolve_in_place` (same model) and the retry carries the S44
  feedback; a second FAIL returns `escalate_model`
- [ ] Escalation list exhausted by FAILs returns `halt` (fail-closed to human), not a loop
- [ ] A BLOCKED classified as a spec defect surfaces the verifier's proposed amendment (does not
  hard-stop silently); a genuine BLOCKED halts with the reason
- [ ] Each triage decision logs an explainable rationale
- [ ] `go test -race ./internal/orchestrator/... ./internal/run/...` passes

## Required tests

- **Unit**: `internal/orchestrator/triage_test.go` — `TestFailResolvesThenEscalates`,
  `TestExhaustedEscalationHalts`, `TestBlockedSpecDefectSurfacesAmendment`,
  `TestBlockedGenuineHalts`.
- **Reachability artefact**: paste in `proof.md` the triage decision log across a scripted
  FAIL→resolve→FAIL→escalate→PASS sequence.

## Risks

- Must not introduce an infinite resolve-in-place loop — the K budget and the escalation
  exhaustion → halt are the guards; test both.
- Keep the BLOCKED LLM hook optional and bounded (S42 timeout) so triage never wedges the run.

## Deferrals allowed?

Yes, with Rule 2 — the full LLM-orchestrator and the interactive human-halt UX carry forward
with why/tracking/ack; the deterministic policy + BLOCKED-resolvability hook is the landing scope.
