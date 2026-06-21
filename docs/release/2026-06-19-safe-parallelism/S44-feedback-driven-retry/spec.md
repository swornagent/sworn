---
title: 'S44-feedback-driven-retry — feed the verifier''s FAIL rationale into the retry implementer instead of blind re-running'
description: 'On verify FAIL, the run loop clears verification and re-implements with the next model but never tells the implementer why it failed — a blind retry. S44 preserves the verifier''s FAIL rationale/violations and injects them into the next implement attempt''s prompt, so retry resolves the named problem instead of re-implementing from scratch. The most direct embodiment of "don''t fail what an intelligent agent could resolve."'
---

# Slice: `S44-feedback-driven-retry`

## User outcome

When `sworn run` retries a slice after a verifier FAIL, the next implement attempt is told
**exactly why the previous attempt failed** (the verifier's rationale and the specific gate
violations) and is instructed to address them. A resolvable failure gets resolved on the next
pass, rather than the loop blindly re-implementing from scratch (and discarding the prior
attempt's progress along with the diagnosis).

## Entry point

`sworn run` retry path — `internal/run/slice.go` `RunSlice` loop and `internal/implement`
`Run`. Reached whenever a verifier returns FAIL/Inconclusive and another attempt remains.

## Background

On FAIL, `RunSlice` resets `st.Verification = state.Verification{}` (`slice.go:123`) and calls
`implement.Run(ctx, worktreeRoot, specPath, implAgent)` for the next `escalationModels[attempt]`.
`implement.Run` only reads `spec.md` + `status.json` — it has no parameter for prior-attempt
feedback, and the verdict rationale is cleared before the retry. So the implementer re-derives
the work from the spec with zero knowledge of what the verifier objected to. A human or capable
agent handed "you failed gate 3 because the test doesn't exercise the integration point" would
fix that; sworn throws the feedback away.

## In scope

- In `RunSlice`, capture the prior attempt's `lastVerdict` (rationale + any structured
  violations) and pass it into the next `implement.Run` call as a new optional
  `priorFeedback` argument (string or small struct). Do this **before** clearing/resetting
  verification, or preserve it across the reset.
- Extend `implement.Run` to accept the optional `priorFeedback`; when non-empty, inject a
  clearly delimited "Previous attempt failed verification — address these specifically:" block
  into the implementer's user prompt (ahead of the spec), so the agent prioritises the named
  failures.
- First attempt (attempt 0) passes empty feedback — no behavioural change to the happy path.

## Out of scope

- Changing *which* model retries (escalation order is unchanged — S44 is orthogonal: feedback
  is passed to whatever model the escalation picks).
- Persisting prior attempts' diffs for the implementer to diff against — feedback is the
  verifier's prose + violations, not the prior code.
- An intelligent recovery/triage layer for BLOCKED verdicts — that is the larger
  design-capture item, tracked separately.

## Planned touchpoints

- `internal/run/slice.go` (preserve verdict, pass feedback into the retry implement call)
- `internal/implement/implement.go` (accept `priorFeedback`, inject into the prompt)
- `internal/run/slice_test.go` and/or `internal/implement/implement_test.go` (feedback is
  passed and reaches the prompt)

## Acceptance checks

- [ ] After a FAIL, the next `implement.Run` receives the prior verdict's rationale (assert the
  feedback string is non-empty and contains the verifier rationale on attempt ≥ 1)
- [ ] The injected feedback appears in the implementer's user prompt ahead of the spec (assert
  via a fake agent that records the prompt it was given)
- [ ] Attempt 0 receives empty feedback — happy-path prompt unchanged (regression guard)
- [ ] A FAIL→PASS scenario works end to end: fake agent that only succeeds when the feedback
  block is present reaches `verified` on attempt 2
- [ ] `go test -race ./internal/run/... ./internal/implement/...` passes

## Required tests

- **Unit**: `internal/implement/implement_test.go` — `TestRunInjectsPriorFeedback` (fake agent
  records prompt; assert feedback block present/absent by attempt). `internal/run/slice_test.go`
  — `TestRetryPassesVerifierRationale` (model[0] FAILs with a known rationale; assert model[1]'s
  implement call received that rationale).
- **Reachability artefact**: paste test output in `proof.md` plus a captured implementer prompt
  showing the injected "Previous attempt failed verification" block.

## Risks

- The rationale must be preserved across the `st.Verification` reset in `slice.go:123` — capture
  it into a local before the reset, or the feedback will be empty. Call this out in the design.
- Keep the injected block clearly delimited and capped in length so a verbose verifier rationale
  doesn't crowd out the spec in the prompt.

## Deferrals allowed?

No deferrals expected — bounded plumbing of existing data (the verdict) into an existing prompt.
