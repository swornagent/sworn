# Coach acknowledgement — S09-model-catalog

Date: 2026-07-11
Decided by: Brad (Coach) — the single escalate pin (Rule 2 tracking
decision) resolved by filing the issue; remaining pins mechanical,
applied per the pre-authorised batch protocol.
Review: review.md (Captain, verdict NEEDS_COACH)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **design.md anthropic.go citation — ACCEPTED.** Correct the claim:
   anthropic.go uses the anthropic-sdk-go SDK (the ADR-0007 exception),
   not stdlib net/http. Fix the design text; no code impact.

2. **D1 uniform no-dispatch credential check — CONFIRMED as designed.**
   The catalog runs its own credential check across all 7 providers
   rather than reading the S05 registry enumeration (which deliberately
   excludes Google/Ollama). D2-D4 Type-2 defaults acknowledged.

3. **[escalate] Pricing-column follow-on — RESOLVED: tracking issue
   FILED as sworn#92.** The Rule 2 deferral now has all three legs
   (why in the design, tracking sworn#92, acknowledgement here). Cite
   sworn#92 in proof.json not_delivered.

4. **cmd/sworn/main.go touchpoint — ACCEPTED, no action.** The design
   correctly declines to touch it (capabilities.go self-registration
   precedent); record as a touchpoint divergence in proof.json.

5. **Reachability artefact naming — ACCEPTED.** Name TestModelsCommand's
   fixture-driven end-to-end run through cmdModels explicitly in the
   proof bundle's reachability_artifacts.

Proceed to implementation.
