# Design TL;DR: S44-feedback-driven-retry

## §1. User-visible change

When `sworn run` verifies an implementation and gets a FAIL, the retry loop now
passes the verifier's rationale and specific gate violations to the next
implementer attempt. The implementer is told exactly why the previous attempt
failed (e.g. "gate 3: the test doesn't exercise the integration point") and is
instructed to address those failures. A resolvable FAIL is resolved on the next
pass rather than the loop discarding the diagnosis and re-implementing from
scratch. Attempt 0 (the first attempt) is unchanged — no feedback is injected
when there is no prior failure. Provider-error classification (S10's
`model.Error{Kind}`) is deferred until S10 merges into `release-wt` — see §4.

## §2. Design decisions not in spec

1. **Capture rationale before the verification reset.** In `RunSlice`, the
   `st.Verification` is cleared for retry at line 159 (`st.Verification = state.Verification{}`).
   The `lastVerdict` variable already holds the prior verdict's `Rationale` and
   `FailedGate` — capture it into a local `priorFeedback` string BEFORE the reset
   and pass it to `implement.Run`. No new state field needed; the feedback is
   transient call-parameter data, not persisted status.

2. **`implement.Run` gets a new `priorFeedback string` parameter** rather than a
   struct. The feedback is a single prose block to inject into the user prompt.
   A string is the simplest thing that works — the verifier's rationale is prose,
   and the `FailedGate` field is additive context. If future slices need structured
   feedback (e.g. per-violation directives), the parameter can become a struct
   without breaking existing callers (a string is a subset of a future struct).

3. **Injection point: ahead of the spec in the user prompt, with a clear delimiter.**
   The user prompt currently reads `"Implement the following spec in workspace …"`.
   When `priorFeedback` is non-empty, the prompt becomes:
   ```
   Previous attempt failed verification — address these specifically:
   
   [rationale]
   
   ---
   
   Implement the following spec in workspace …
   ```
   The delimiter is a markdown horizontal rule (`---`), which most models
   recognise as a section break.

4. **`RunSlice`'s `implement.Run` call site changes from 2 args to 3.** The
   signature becomes `Run(ctx, workspaceRoot, specPath, priorFeedback, implAgent)`.
   Happy-path callers (`sworn run` attempt 0) pass `""`. The `RunSlice` retry path
   passes the captured rationale. Backward-compatible with the existing
   `internal/run/run.go` call (which always passes `""` for attempt 0).

## §3. Files I'll touch grouped by purpose

- **`internal/implement/implement.go`**: Add `priorFeedback string` parameter to
  `Run`. When non-empty, inject a "Previous attempt failed verification" block
  into the user prompt ahead of the spec. This is the core injection point.

- **`internal/run/slice.go`**: In `RunSlice`, capture `lastVerdict.Rationale` (and
  optionally `lastVerdict.FailedGate`) into a local before the verification reset.
  Pass it as `priorFeedback` to `implement.Run` on retry (attempt ≥ 1). Attempt 0
  passes `""`.

- **`internal/implement/implement_test.go`**: New test `TestRunInjectsPriorFeedback`
  — a recording fake agent that captures the user prompt; assert that when
  `priorFeedback` is non-empty, the prompt contains the feedback block ahead of
  the spec, and when empty, it does not.

- **`internal/run/slice_test.go`**: New test `TestRetryPassesVerifierRationale` —
  a multi-model test: model[0] FAILs with a known rationale; assert model[1]'s
  implement call received that rationale in its prompt. New test
  `TestAttempt0EmptyFeedback` — assert no feedback on first attempt.

## §4. Things I'm NOT doing

1. **Provider-error retry policy (spec ACs 5-6).** S44 depends on S10
   (`model.Error{Kind}`, `IsTerminal`, `IsTransient`). S10 is on T5-providers
   which is `in_progress` and not yet merged into `release-wt`. The error
   taxonomy does not exist on this track branch. This is a declared Rule 2
   deferral: **why** — S10 not merged; **tracking** — S10 in T5-providers
   (`implemented` state, awaiting verification/merge); **acknowledgement** —
   Coach to ack in `approved-ack.md`. When S10 merges, re-enter this slice to
   wire ACs 5-6.

2. **Changing escalation order.** The spec is explicit: feedback is passed to
   whatever model the escalation picks. No reordering logic.

3. **Persisting prior diffs.** Feedback is the verifier's prose + violations,
   not the prior code diff. The implementer gets the diagnosis, not the
   implementation that produced it.

## §5. Reachability plan

- **Type**: test output (manual-smoke-step).
- **Artefact**: `go test -race -run 'TestRetryPassesVerifierRationale|TestAttempt0EmptyFeedback|TestRunInjectsPriorFeedback' ./internal/run/... ./internal/implement/...`
- **User gesture**: The test `TestRetryPassesVerifierRationale` exercises the full
  retry path end-to-end: implementer[0] produces code that gets a FAIL with a
  specific rationale → implementer[1] receives that rationale in its prompt →
  implementer[1] addresses the failure → PASS. The fake agents and verifiers
  capture and assert the prompt content at each stage.

## §6. Open questions for the Coach

None.