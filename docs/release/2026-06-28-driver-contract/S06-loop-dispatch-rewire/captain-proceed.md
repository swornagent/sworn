# Coach acknowledgement — S06-loop-dispatch-rewire

Date: 2026-07-10
Decided by: Brad (Coach) — escalate pin ratified live; mechanical and
memory-cited pins applied per the pre-authorised batch protocol.
Review: review.md (Captain, 2026-07-10; verdict NEEDS_COACH,
constitutional flag acknowledged — D6/D7 change credential-based
routing, covered by dispositions 4 and 5 below).
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate] D2 captain-leg AC-02 reading — RATIFIED: deferral, with
   AC-02 amended to match.** The fail-open deferral stands: AC-02's
   hard-error contract applies to the implement and verify legs; a
   captain-leg Resolve failure records the registry's descriptive role
   error inside a Rule 2 deferral via recordDesignGateDeferral
   (sworn#51) and proceeds. AC-02's text is AMENDED on release-wt
   (@8ff95cc, forward-merged to this track) so spec and code agree —
   the verifier grades the amended AC, not a privately narrowed one.
   Record as a design_decision citing this acknowledgement as the human
   decision. Follow-up restoring role-universality (RoleCaptain on
   subprocess drivers) filed as sworn#86.

2. **Resolve errors must name the model — ACCEPTED.** Wrap every
   upfront Resolve failure at the RunSlice call site
   (`fmt.Errorf("RunSlice: resolve %q for role %q: %w", modelID, role,
   err)`); TestRunSliceResolutionFailure asserts model ID, role, and
   registered alternatives all appear.

3. **Terminal-set citation — ACCEPTED.** D3 closes the S04 tracked
   obligation ({auth, credits}, T3 captain-proceed.md 2026-07-10). Cite
   that record in the TerminalErrKind doc comment and in the
   design_decisions entry.

4. **Proxy predicate unification — CONFIRMED, proceed as designed.**
   The three-part reachability test (advertise / server-side-observed
   dispatch / SWORN_DIRECT flips both surfaces) is the binding R-04
   artefact. SWORN_PROXY_URL stays test-only per the credential-trust
   boundary.

5. **D7 canonical-wins test — ACCEPTED.** The SWORN_* widening proceeds;
   add a precedence test for every widened key (both CANONICAL and
   SWORN_CANONICAL set → canonical wins) so "strictly additive" is
   proven, not asserted.

6. **Record design_decisions — ACCEPTED.** Populate before in_progress:
   D1 (planning intake 2026-07-02), D2 (this acknowledgement, pin 1),
   D3 (S04 captain-proceed.md), D6+D7 (R-04 binding + this
   acknowledgement), per the S04/S05 record shape.

## Flags (a)–(e): acknowledged as recorded in review.md — delete
InterpretVerifier without a shim; repair the pre-existing fused comment
at slice.go:694 during the verify-leg rewrite and re-run the
newline-corruption sweep; keep acceptStructuredVerdict assertions
minimally diffed (R-01); signature breaks are module-internal with
callers enumerated; S07/S08 sequencing resolves by serial track order.

Proceed to implementation.
