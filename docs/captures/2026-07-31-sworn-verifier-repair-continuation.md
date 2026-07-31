# Sworn Verifier repair continuation

Date: 2026-07-31

Status: settled and validated; pending Baton RC12 revendor

## Decision

The first Verifier for a work candidate starts with clean context, read-only
access, and no Implementer or Captain conversation.

If that Verifier returns an exact FAIL and Baton records it, Sworn may keep the
same Verifier thread for the direct repair:

```text
fresh read-only Verifier -> exact recorded FAIL
                                      |
direct repair candidate --------------+
                                      v
same Verifier thread -> new read-only invocation -> FAIL or PASS
```

The resumed invocation truthfully says it is not fresh. It still has read-only
access and contains only the Verifier's own earlier work. It never receives the
Implementer or Captain conversation.

The Verifier rechecks the whole contract against the new exact candidate. Its
earlier findings are useful context, not a reduced checklist.

Only an accepted FAIL carries this thread across candidates into a repair.
Questions may still use Sworn's ordinary within-invocation recovery path, but
they do not create repair authority. PASS and BLOCKED close the thread; an
operational failure or other no-verdict result requires a fresh thread for the
next invocation. Assembly verification always starts fresh.

## Safe fallback

Continuation is an optimization, not authority. Sworn starts a fresh read-only
Verifier when:

- the process-local handle is missing, expired, unsupported, or rejected;
- the run, slice, Verifier, model, tool access, plan, target, or input evidence
  no longer matches;
- the current candidate is not the direct repair bound to the recorded FAIL; or
- Sworn cannot prove the match from Baton's current accepted facts.

This fallback is expected after a Sworn restart because handles are opaque,
memory-only, and never written to the journal, Git, the board, telemetry, or a
notification.

For a consuming slice, its prepared base may move as the next receipt is
recorded. That movement alone does not end the Verifier thread. A changed plan,
target, or consumed evidence does.

## The exact-head case

Baton can return a slice to the Implementer when the worktree advanced after a
candidate receipt but before a Verifier verdict. The Implementer can then
record the current clean head as the next candidate without replacing the
slice.

There is no valid FAIL in that case, so there is no approved Verifier thread to
carry forward. The next candidate starts with a fresh Verifier. This is the
safe path for the Fired incident that prompted the change.

## Baton and Sworn have different jobs

Baton decides whether the candidate, FAIL, repair, and next verdict form valid
release history. It owns receipts, exact candidate bindings, and which role is
eligible next.

Sworn owns the private conversation handle. It retains, resumes, or discards
that handle only after Baton's accepted state matches. It does not invent a
transition or repeat Baton's lifecycle checks as a second scheduler.

## Lean implementation

The change extends Sworn's existing common continuation driver and process-local
registry. It adds no Baton role, stage, command, receipt schema, journal field,
provider-specific role driver, transcript store, or orchestration loop.

Existing content-free continuation events report reuse or fresh fallback
through `dispatch_completed.continuation.<mode>.<outcome>`. The existing OTel
metrics `sworn.eval.continuations` and
`sworn.eval.continuation.outcomes` consume those events. No prompt, response,
reasoning, session ID, candidate ID, or source content is added to telemetry.

Focused checks cover:

- a fresh first Verifier and exact-handle-only read-only repair resumption;
- rejection of an ordinary non-fresh Verifier, cross-role reuse, and Assembly
  reuse;
- repeated `FAIL -> repair` cycles ending in PASS;
- safe fresh fallback after handle loss or authority drift;
- consuming-slice prepared-base movement without weakening plan, target, or
  input-evidence binding;
- one OpenAI Responses replay path and the shared native-session path; and
- the built production repair journey plus existing continuation OTel mapping.

This keeps the quality boundary: independent first review, exact immutable
history, full re-verification, and fresh Assembly assurance. It removes only
the cost of teaching the same Verifier its own findings again.
