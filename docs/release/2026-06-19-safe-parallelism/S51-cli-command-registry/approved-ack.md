TL;DR clean design, faithful to the approved registration plan — central registration keeps S51 touchpoint-disjoint from every in-flight track. 3 mechanical pins + 2 flags, all apply-inline:

1. **Declare verify.go.** §2.1/§3 move cmdVerify + openDeferralsFlag into a new cmd/sworn/verify.go. Add it to status.json planned_files AND the T15 rows of the index.md touchpoint matrix (it's T15-owned, collides with nothing), or document it under proof.md "Divergence from plan". Otherwise Gate 2 + the S30 lint-touchpoints gate flag an undeclared file.
2. **Assert non-empty Summary.** Spec Risk 3 mitigation: commands_test.go must assert every command.All() entry has a non-empty Summary (no blank help lines), not just resolution/handler identity. Add that assertion.
3. **Populate design_decisions.** status.json design_decisions is empty; S51 introduces an architecturally-significant registry pattern (Coach-decided this session). Record that decision so sworn designfit passes — or confirm it's benign-empty.

Flags (not pins): (a) §2.5 usage() keeps hand-written per-verb prose, only the listing is registry-generated — confirm intentional; (b) downstream, T3/S07 resolves main.go→register on its re-entry, already gated by the T3 depends_on T15 edge — no action here.

§2 decisions 1–5 ack (all mechanical relocation/struct-shape choices; no memory conflict; no new deps). §6 (none) ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline gate-hygiene fixes (declare a relocated file, add a test assertion, populate design_decisions); none changes the design or requires Coach judgement.
-->
