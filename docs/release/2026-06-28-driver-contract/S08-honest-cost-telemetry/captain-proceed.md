# Coach acknowledgement — S08-honest-cost-telemetry

Date: 2026-07-10
Decided by: Brad (Coach) — both escalate pins ratified live; mechanical
and memory-cited pins applied per the pre-authorised batch protocol.
Review: review.md @b10972d (Captain, 2026-07-10; verdict NEEDS_COACH)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate] D1/D2 subscription CostSource inference — RATIFIED:
   fail-closed classification.** Subprocess drivers (claude-cli, codex)
   emit CostSource "subscription" ONLY on a positively identified,
   testable marker in real CLI output; anything ambiguous or inferred
   from undocumented behaviour is "unknown". A fabricated dollar figure
   never appears on either path; token counts are recorded whenever the
   CLI exposes them. An honest "unknown" is worth more to the telemetry
   data-moat than a plausible guess that poisons it. Record D1/D2 in
   status.json.design_decisions as Type-1, decided by the Coach in this
   acknowledgement. If no positively identifiable subscription marker
   exists in the current CLI output, ship everything as "unknown" and
   record the absent marker as a Rule 2 note — do NOT implement the
   unverified inference.

2. **[escalate] AC-02 "provider" branch — RATIFIED: AC-02 AMENDED**
   on release-wt (@aaa2861, forward-merged to this track). The
   "provider" vocabulary + Result-envelope plumbing exist as a named
   constant with a contract test, reserved for a future driver whose
   wired client genuinely returns billing; no live dispatch path claims
   it this slice, and emitting it from a computed or inferred figure is
   a spec violation. Record as a design_decision citing this
   acknowledgement.

3. **[mechanical] sworn#89 — ACKNOWLEDGED.** The Google/Bedrock
   pricing-lookup duplication deferral is correctly filed with all
   three Rule 2 legs; cite it in proof.json's not_delivered.

4. **[mechanical] agent.Run signature change — ACKNOWLEDGED.** Blast
   radius independently confirmed contained to the two listed call
   sites; proceed with the float64 cost-return removal.

5. **[memory-cited] D3 named CostSource constants — CONFIRMED.**
   Mirror the ErrKind* named-constant pattern; proceed as designed.

6. **[memory-cited] AC-05 state.Dispatch cost_source enrichment —
   CONFIRMED.** On-thesis for the Day-1 telemetry foundation; proceed
   as designed.

Proceed to implementation.
