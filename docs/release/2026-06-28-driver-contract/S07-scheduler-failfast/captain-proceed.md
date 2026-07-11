# Coach acknowledgement — S07-scheduler-failfast

Date: 2026-07-10
Decided by: Brad (Coach). Pin 1 is the SAME decision Brad ratified live
for S06 pin 1 earlier today (captain-leg deferral policy), propagated
here rather than re-litigated; pins 2-4 applied per the pre-authorised
mechanical batch protocol.
Review: review.md (Captain, 2026-07-10)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate] AC-01 captain-fail-fast contradiction — RESOLVED by
   propagating the S06 ratification.** AC-01 is AMENDED on release-wt
   (@e8e8858, forward-merged to this track): the pre-spawn startup
   sweep exits non-zero on implementer-, verifier-, or
   escalation-entry resolution failure; a captain-leg failure records
   the descriptive error as a durable Rule 2 deferral and proceeds —
   mirroring S06 captain-proceed.md pin 1 and S06's amended AC-02.
   The design's proposed reading is exactly this; proceed with it.
   Record as a design_decision citing the S06 ratification (2026-07-10)
   as the human decision. sworn#86 remains the tracked path to making
   captain resolution succeed outright.

2. **[memory-cited] design_decisions absent — ACCEPTED.** Populate
   status.json.design_decisions before the in_progress transition per
   the S04/S05/S06 record shape, including the shared
   ComposeEscalationModels/ResolveDispatch helper extraction (Type-2
   noted default) and pin 1's captain policy (Type-1, citing the S06
   Coach ratification).

3. **[mechanical] TestRunParallel_FailureCascade characterisation —
   ACCEPTED.** The design's claim about the test's existing coverage is
   inaccurate: correct the design.md claim, and ensure the new AC-03
   test asserts the actual dependent/sibling outcomes rather than
   relying on coverage the existing test does not provide.

4. **[mechanical] Four files outside touchpoints — ACCEPTED.** The
   planned internal/run/resolve.go, resolve_test.go, slice.go, and
   imports_test.go edits are pre-declared here as Coach-acknowledged
   touchpoint additions (the shared-helper extraction is the design's
   core anti-divergence mechanism); record each in proof.json as a
   divergence with this acknowledgement cited.

Proceed to implementation.
