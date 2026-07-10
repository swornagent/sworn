# Coach acknowledgement — S10-conformance-sit

Date: 2026-07-11
Decided by: Brad (Coach) — the escalate pin resolved by PROPAGATING the
release's original fail-fast-at-resolution Type-1 decision (S01,
planning intake 2026-07-02), not a new ruling; mechanical pins applied
per the pre-authorised batch protocol.
Review: review.md (Captain, verdict NEEDS_COACH)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate] AC-01 undeclared-role clause — RESOLVED: AC-01 AMENDED**
   on release-wt (@32328e5, forward-merged to this track). The
   undeclared-role failure is asserted at the RESOLUTION boundary
   (Registry.Resolve's role-arm error), per the ratified contract:
   capability IS the declared role set, fail-fast at resolution;
   ADR-0012 deliberately disclaims Dispatch-level role re-checks. The
   conformance suite does NOT harden the four drivers' Dispatch.
   Record as a design_decision citing the S01 Type-1 decision.

2. **(critical) SIT fixture DoR-completeness — ACCEPTED.** The
   cold-board fixture release must carry intake.md, covers_needs,
   reqvalidate records; the stub driver scripts the captain leg
   (design TL;DR + zero-escalate review) and reqverify PASS outputs;
   proof.md is written by implement.Run itself, not pre-baked.

3. **(critical) in-process httptest wiring — ACCEPTED, use the proxy
   route.** The cited wiring does not exist (ProviderConfig has no
   base-URL field; InProcess.newClient unexported). Wire the SIT's
   in-process leg via fake credentials + SWORN_PROXY_URL (the landed
   S06 ProxyRoute predicate makes this the honest, already-tested
   seam). If that proves insufficient, an exported seam is a declared
   touchpoint divergence — surface it in proof.json, don't hide it.

4. **Registry enrolment — ACCEPTED.** Fail-closed name->factory map
   detection (a registered driver with no conformance enrolment fails
   the suite), not zero-edit auto-enrolment over []Info.

5. **API consistency — ACCEPTED.** Drop the dead RequiresWorktree knob
   (all four drivers AssertWorktree unconditionally); fix
   DurationMS-vs-Duration and the Run(t, newDriver[, opts]) signature
   inconsistency before implementation.

6. **D7 / AC-03 'verified' not 'merged' — CONFIRMED.** MergeTrackFn
   stays nil; the answer is in the spec.

7. Any remaining pin in review.md not restated above is accepted as
   written in the Captain's suggested acknowledgement reply.

Proceed to implementation.
