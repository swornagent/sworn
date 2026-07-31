# Sworn Verifier repair continuation

Date: 2026-07-31

Status: implemented and release-validated; awaiting publication

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
- the current candidate receipts do not form one bounded, valid repair chain
  back to the recorded FAIL; or
- Sworn cannot prove the match from Baton's current accepted facts.

This fallback is expected after a Sworn restart because handles are opaque,
memory-only, and never written to the journal, Git, the board, telemetry, or a
notification.

For a consuming slice, its prepared base may move as the next receipt is
recorded. That movement alone does not end the Verifier thread. A changed plan,
target, or consumed evidence does.

## The exact-head case

Baton can return a slice to the Implementer when its exact head advanced after
a candidate receipt but before a Verifier verdict. Sworn rechecks that head
without replacing the slice.

If the resumed Implementer makes no further product edit, Sworn records the
existing head exactly. It does not manufacture an empty commit. If the
Implementer makes a real edit, Sworn creates one child of that head. In both
cases, scope, changed paths, lineage, checks, and product identity cover the
whole change from the prior candidate receipt to the final candidate. A
pre-existing out-of-scope change therefore cannot hide behind the resumed
turn.

The durable recovery record keeps the two Git meanings separate:

- `before` is the physical head that a ref transaction may move from; and
- `refresh_from` is the earlier candidate receipt used for evidence and scope.

When no earlier Verifier FAIL exists, the next Verifier starts fresh. When a
fresh Verifier already recorded FAIL and its retained thread still matches,
Sworn may follow the validated candidate-receipt chain through one or more
exact-head refreshes back to that FAIL. It resumes the same read-only Verifier
thread for the whole-contract recheck. A missing or broken chain falls back
fresh.

## Baton and Sworn have different jobs

Baton decides whether the candidate, FAIL, repair, and next verdict form valid
release history. It owns receipts, exact candidate bindings, and which role is
eligible next.

Sworn owns the private conversation handle. It retains, resumes, or discards
that handle only after Baton's accepted state matches. It does not invent a
transition or repeat Baton's lifecycle checks as a second scheduler.

## Lean implementation

The change extends Sworn's existing common continuation driver, process-local
registry, and implementation seal. It adds no Baton role, stage, command,
public receipt schema, provider-specific role driver, transcript store, or
orchestration loop. Two optional fields inside Sworn's existing private seal
payload distinguish an exact-head refresh and its evidence base.

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
- `FAIL -> repair candidate -> exact-head refresh -> same Verifier` with one
  fresh thread and read-only reuse;
- clean exact-head adoption for consuming and non-consuming slices, plus a
  refresh that adds one legitimate edit;
- full-chain rejection when the pre-existing refresh changed an out-of-scope
  path, with or without a later legitimate edit;
- restart recovery that completes one clean-adoption receipt without rewinding
  or duplicating it, and preserves a foreign head while failing closed;
- safe fresh fallback after handle loss or authority drift;
- consuming-slice prepared-base movement without weakening plan, target, or
  input-evidence binding;
- one OpenAI Responses replay path and the shared native-session path; and
- the built production repair journey plus existing continuation OTel mapping.

This keeps the quality boundary: independent first review, exact immutable
history, full re-verification, and fresh Assembly assurance. It removes only
the cost of teaching the same Verifier its own findings again.

## Validation

- The exact serial RC12 real-binary E2E suite passed in 996.271s.
- Every non-E2E package passed normally and under the race detector.
- Vet, format, tidy, diff, Baton golden identity, stripped build, and executable
  version checks passed.
- Live provider certification was reused rather than repeated because no
  production driver or provider-wire behaviour changed.
