# Approved Acknowledgement — S44-feedback-driven-retry

## Design TL;DR reviewed

The design TL;DR at `docs/release/2026-06-19-safe-parallelism/S44-feedback-driven-retry/design.md`
was reviewed and the following deferral is acknowledged.

## Deferred scope: provider-error retry policy (ACs 5–6)

**Why:** S44's provider-error classification depends on S10-provider-foundation's
`model.Error{Kind}`, `IsTerminal`, and `IsTransient` helpers. These types/functions do
not exist on the T12-harness-hardening track branch because S10 is on T5-providers and
has not yet merged into `release-wt/2026-06-19-safe-parallelism`.

**Tracking:** S10-provider-foundation is currently `implemented` on T5-providers and
must be verified/merged into `release-wt` before S44 can wire ACs 5 (terminal dispatch
errors fail fast) and 6 (transient dispatch errors retry on same model). Once S10 is
available, re-enter S44 to implement the remaining acceptance checks.

**Acknowledgement:** Coach approves implementing S44 with ACs 1–4 only in this pass;
ACs 5–6 are deferred to a follow-up pass after S10 lands. This is a Rule 2 compliant
deferral.

## Approved decisions

1. Capture `lastVerdict.Rationale` into a local `priorFeedback` string **before**
   `st.Verification = state.Verification{}` in `RunSlice`.
2. Extend `implement.Run` signature to accept `priorFeedback string` as a new
   parameter ahead of the agent: `Run(ctx, workspaceRoot, specPath, priorFeedback, implAgent)`.
3. Inject a delimited feedback block ahead of the spec in the implementer user prompt
   only when `priorFeedback` is non-empty.
4. Attempt 0 passes empty feedback — happy path unchanged.

## In-scope for this implementation pass

- AC1: next `implement.Run` receives prior verdict rationale on attempt ≥ 1.
- AC2: injected feedback appears in the implementer user prompt ahead of the spec.
- AC3: attempt 0 receives empty feedback.
- AC4: FAIL → PASS end-to-end when feedback block is present.

## Out-of-scope for this implementation pass

- AC5: terminal dispatch errors fail fast without escalating.
- AC6: transient dispatch errors retry on the same model.

Both will be added once S10-provider-foundation is merged into `release-wt`.
