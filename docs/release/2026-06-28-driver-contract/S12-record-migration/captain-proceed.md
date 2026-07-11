# Coach acknowledgement — S12-record-migration

Date: 2026-07-11
Decided by: Brad (Coach) — both critical escalate pins ratified live;
mechanical pins applied per the pre-authorised batch protocol.
Review: review.md (Captain, verdict NEEDS_COACH; 8 pins)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate/CRITICAL] ears_pattern reshape — RATIFIED: FOLD INTO
   S12.** v0.10.0 spec-v1's AC item allows only id/text/ears_pattern/
   test_refs; S11 strips the retired type/ears_keyword. The migration
   MUST MAP the old `type` -> the canonical `ears_pattern`
   (unwanted->unwanted-behaviour etc.), NOT just drop ears_keyword, and
   repoint ears.go/classifySpecJSON to read `ears_pattern` — so EARS
   lint classification is PRESERVED, no all-Ubiquitous degradation
   across the five releases. Homed here (not re-opening verified S11)
   because the code change must land atomically with the record
   reshape. Spec amended on release-wt (@4c01dff): AC-07 added,
   out_of_scope[0] widened for the ears.go read-field, in_scope +
   touchpoints updated. Tracked sworn#95. Record as a Type-1
   design_decision citing this ack.

2. **[escalate/CRITICAL] AC-02 vacuous — RATIFIED: zero-match GUARD.**
   Zero feature-quadrant records exist; AC-02 amended (@4c01dff) to a
   `grep quadrant:feature -> zero` guard across the five releases (same
   shape as AC-01), satisfiable and non-vacuous.

3. **[mechanical] records-conformance test — ACCEPTED.** The Go test
   globbing the five releases + asserting baton.ValidateSchema per
   record is the AC-03 mechanism; approved-inline touchpoint expansion.

4. **[mechanical] whitelist board projection — ACCEPTED.** Project each
   board.json to exactly {$schema, release, tracks:{id,slices,
   depends_on}}, dropping stray activity-log + worktree/state + the
   top-level release worktree fields (matching S11's Pin-1 release-level
   pure-plan).

5. **[mechanical] S15->S12 ordering — ACKNOWLEDGED, operationally
   enforced.** The T7 order (S11->S15->S12) is enforced by the
   orchestrator's serial implementation, not a board depends_on. S12 is
   implemented LAST and migrates S15's chore records (authored in
   current vocab) to quick with the rest. S12 removes the normalise shim
   ONLY after all data (incl. S15) is migrated.

6. **[mechanical] design_decisions — ACCEPTED.** Populate Type-2
   classifications (whitelist projection, conformance test) plus the
   Type-1 pin-1 ears_pattern decision, before in_progress.

7. Remaining pins accepted as written in the Captain's suggested reply.

Proceed to implementation.
