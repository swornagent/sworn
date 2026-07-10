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

---

## Supplementary Coach decision — 2026-07-11 (sworn#93 fold)

**Context:** the S10 cold-board SIT (AC-03), while being implemented, surfaced a
real parallel-loop bug (sworn#93): `RunSlice`'s verified/PASS path wrote
`status.json` but never committed it — alone among all terminal paths (the four
blocked variants and `failed_verification` all `repo.Stage`+`repo.Commit`). The
parallel router reads committed track-ref state, so a verified slice was re-read
as `implemented` and re-dispatched `verify` endlessly. Single-slice `Run()`
masked it by committing the whole tree afterward. Strong candidate root cause of
the historically-unreliable autonomous parallel loop.

**Decision (Brad, Coach): FOLD the fix into S10.** Not routed to the owning
slice S06 (merged/immutable), and not a separate slice — because AC-03's SIT
cannot reach a stable verified state without the fix, so it blocks this slice's
own acceptance. Spec amended on release-wt @b80c7ad (forward-merged here):
`internal/run/slice.go` added to in_scope + touchpoints; AC-06 added as the
non-tautological regression assertion (reverting the fix must make the SIT
fail); out_of_scope carve-out + a one-line-fix ceiling. Traces to N-07 (dead
loop wiring caught in CI, not shipped DOA). Tracked: sworn#93.

**Implementer instructions (this is a RESUMED dispatch — continuation handshake
required):**
- Already committed on this branch: the drivertest conformance suite (AC-01/02,
  @be5b9a2) and the verified-path commit fix in `internal/run/slice.go` (WIP
  checkpoint @108c945 — do NOT rewrite it; verify it is present and correct).
- REMAINING: write the cold-board SIT (`internal/run/loop_sit_test.go` +
  `testdata/sit-fixture/`) satisfying AC-03/AC-04/AC-05, INCLUDING the AC-06
  regression assertion that the verified transition is committed to the ref and
  the router does not re-dispatch; record the sworn#93 fold as a **Type-1**
  design_decision citing this acknowledgement as the human decision; write the
  proof bundle; transition to `implemented`.
- Prove AC-06 has teeth: demonstrate (in the journal/proof) that with the
  verified-path commit reverted, the SIT fails or stalls to its bounded deadline.

Proceed to implementation (resume).
