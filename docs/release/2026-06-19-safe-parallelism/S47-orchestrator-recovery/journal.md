# S47-orchestrator-recovery — Session Log

## 2026-07-21: Implementation session

### State transition: planned → in_progress → implemented

### Decisions

- **K=1 resolve-in-place budget**: Implemented the spec's proposed K=1 default (one same-model retry with S44 feedback before escalating). No configuration surface exposed yet — K is hardcoded; can be made configurable in a follow-up if the policy shape proves correct.
- **Implementer error triage**: Extended the triage to also handle implementer errors (timeout, model failure) by treating them as `verdict.Fail` and running through the same `Decide()` policy. This ensures implementer timeouts don't bypass the escalation budget and loop infinitely.
- **`RetryCap` backward compat**: Kept `RetryCap` in `RunSliceOptions` but it is no longer used (`_ = opts.RetryCap`). The triage policy's natural exhaustion (K × escalation list) replaces it. No consumer code was found that depended on `RetryCap` beyond the test suite, which was updated.
- **No `Blocked` slice state**: The `blocked` state exists as `verification.result = "blocked"` in status.json, not as a `state.State`. The BLOCKED halt path writes `verification.result`, `verification.violations`, commits, and returns an error — the router (S58) then routes `blocked → replan-release`. No new `state.State` value was added.
- **Violation extraction**: Since `verdict.Result` doesn't carry structured violations, the BLOCKED halt path uses `extractViolations()` to parse numbered/bulleted items from the rationale. If no structured items found, the full rationale is used as a single violation. This satisfies S38's `ValidateBlockedViolations` guard.

### Trade-offs

- **Same-model retry changes test semantics**: Existing tests assumed immediate escalation (attempt N → model N). With K=1, the first FAIL retries the same model before escalating. This is the intended behavior but required updating 7 tests. All tests now pass.
- **No LLM triage**: The spec explicitly defers a general LLM-orchestrator. The deterministic policy is simpler, faster, and auditable. The test `TestTriageReasonAuditability` verifies every decision carries an explainable rationale.

### Test changes required

1. `TestRun_FailPath_NoMerge`: Reduced from 3 models/3 FAILs to 1 model/2 FAILs (K=1 exhausts properly)
2. `TestRunSliceFail`: Reduced from 2 models to 1 model (same reason)
3. `TestRunSlice_FailNotifiesOnce`: Reduced from 2 models to 1 model (same reason)
4. `TestImplementTimeoutExhaustsToHuman`: Updated error message check from "implementer failed after" to "verification failed after" (triage halt path)
5. `TestRetryPassesVerifierRationale`: Changed from 2-model escalation to 1-model resolve_in_place
6. `TestRetryFeedbackResolvesToPass`: Changed from 2-model escalation to 1-model resolve_in_place

### Out of scope (Rule 2 deferrals)

- **Full LLM-orchestrator**: Deferred. Why: prove deterministic policy shape first. Tracking: S47 spec "Out of scope". Ack: Coach 2026-06-21.
- **Interactive human-halt UX**: Deferred. Tracking: S47 spec "Out of scope". Ack: Coach 2026-06-21.
- **Lifecycle routing / BLOCKED-resolvability**: Not deferred — reassigned to S58 (T17-orchestration-core).