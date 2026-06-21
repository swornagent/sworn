---
title: 'S42-implement-step-timeout — bound each implement attempt so a hung implementer escalates instead of hanging forever'
description: 'sworn run wraps each implement attempt in a context deadline. A wedged/hung implementer (model API hang, agent infinite loop) is cancelled at the timeout, implement.Run returns a deadline error, and the existing escalation loop advances to the next model — closing the gap where an unbounded context.Background() lets a stuck implementer block the run indefinitely.'
---

# Slice: `S42-implement-step-timeout`

## User outcome

A developer running `sworn run` no longer has the loop hang forever when an implementer
wedges (model API stalls, or the agent loops without returning). Each implement attempt is
bounded by a timeout; on deadline the attempt is cancelled and the run **escalates to the
next model** (the existing behaviour for an implementer error), or fails closed to the human
when the escalation list is exhausted.

## Entry point

`sworn run --task ... [--implement-timeout <duration>]` (also `SWORN_IMPLEMENT_TIMEOUT` env
and an `implementer.timeout` config field). The deadline is enforced per attempt inside the
`RunSlice` escalation loop.

## Background

The escalation loop in `internal/run/slice.go` already advances `escalationModels[attempt]`
on an `implement.Run` error — but nothing bounds the implement step. `cmd/sworn/run.go`
passes `context.Background()` (no deadline), `internal/model/oai.go` defaults to
`http.DefaultClient` (no timeout), and the model call honours the context
(`http.NewRequestWithContext`). So a hung implementer blocks `implement.Run` forever and the
escalation never fires. Setting a per-attempt deadline is the missing piece; the model call
already respects context cancellation, so the deadline propagates end-to-end.

## In scope

- In `internal/run/slice.go`, wrap each implement attempt in
  `ctx, cancel := context.WithTimeout(parentCtx, timeout)` (cancel deferred per iteration),
  passing that ctx to `implement.Run`. A deadline-exceeded return is treated exactly like the
  existing implementer-error path: log, `continue` to escalate (or fail closed on the last
  attempt).
- Add a configurable per-attempt timeout: `Options.ImplementTimeout time.Duration`
  (`internal/run`), a `--implement-timeout` flag + `SWORN_IMPLEMENT_TIMEOUT` env
  (`cmd/sworn/run.go`), with a sensible default (e.g. 15m) when unset/zero.
- Surface a clear stderr line on timeout: `implement attempt N timed out after <d> —
  escalating`.

## Out of scope

- A default `http.Client.Timeout` on the model client (`internal/model/oai.go`) — the
  per-attempt context deadline already bounds the HTTP call end-to-end; a client-level
  timeout is redundant here and `oai.go` is a future S39/T5 touchpoint. **Deferred** (Rule 2;
  why: ctx deadline suffices; tracking: revisit with S39 if a non-ctx hang path appears; ack:
  Coach 2026-06-21).
- Killing OS subprocesses the agent spawned — the supervisor's stale-PID reaping covers
  cross-session orphans; in-process cancellation is what this slice adds.
- Per-step timeouts for the verify step (verifier runs on a bounded Claude model already).

## Planned touchpoints

- `internal/run/slice.go` (wrap attempt in WithTimeout; treat deadline as escalate)
- `internal/run/run.go` (thread `ImplementTimeout` through `Options` / `SliceOptions`)
- `cmd/sworn/run.go` (`--implement-timeout` flag + `SWORN_IMPLEMENT_TIMEOUT` env + default)
- `internal/run/slice_test.go` (blocking-fake-agent timeout → escalation test)

## Acceptance checks

- [ ] A fake implementer that blocks past a short `ImplementTimeout` causes that attempt to be
  cancelled and the loop to advance to the next escalation model (assert via a 2-model
  escalation list: model[0] blocks → times out → model[1] runs)
- [ ] When the escalation list is exhausted by timeouts, `RunSlice` returns the
  fail-closed "escalate to human" error (not a hang, not a panic)
- [ ] An implementer that completes within the timeout is unaffected (no behavioural change to
  the happy path)
- [ ] Default timeout is applied when `--implement-timeout`/env/config are all unset
- [ ] `--implement-timeout` flag and `SWORN_IMPLEMENT_TIMEOUT` env are honoured with correct
  precedence (flag > env > config > default)

## Required tests

- **Unit**: `internal/run/slice_test.go` — `TestImplementTimeoutEscalates` (blocking fake
  agent on slot 0, fast agent on slot 1, short timeout → asserts slot 1 ran and slice
  reached verify/PASS); `TestImplementTimeoutExhaustsToHuman` (all slots block → escalate
  error). Use a fake `agent.Agent` whose `Run` blocks on `<-ctx.Done()` to simulate a hang.
- **Reachability artefact**: paste the test run output in `proof.md`; plus an explicit
  stderr-line example showing the timeout → escalation message from a scripted short-timeout
  run.

## Risks

- The fake agent must block on `ctx.Done()` (not `time.Sleep`) so the test actually exercises
  context cancellation rather than wall-clock waiting; otherwise the test is slow/flaky.
- Default must be generous enough not to cut off legitimately long implement steps — 15m is a
  starting point; make it a single named constant for easy tuning.

## Deferrals allowed?

Yes, with Rule 2 compliance — the two Out-of-scope items (model-client timeout, subprocess
kill) carry why / tracking / ack.
