# Captain review — S22-openrouter-tool-structured-output
Date: 2026-07-18T20:03:06+10:00
Design commit: d02899f69905cd875eb123c3b9c3fd677f6927b4

## Pins

1. [mechanical] §Acceptance trace — Replace the superseded attempt-2 design with the ratified configured-attempt-3 contract.
   What I observed: `design.md` says attempt 2 is the final allowed dispatch, its retry table says "Any attempt-2 outcome" permits no third request, and its release gate authorizes only attempt 2. Current AC-06/09/10/12 and `status.json` instead preserve immutable attempts 1–2 and authorize exactly one config-resolved, terminal attempt 3 under `llm-check-proof-receipt-v2` with no `--model` override or attempt 4.
   What to ask the implementer: Rewrite the design around the exact configured-recovery-v2 state machine: immutable v1 attempts 1–2, `config.Load` plus `ResolveVerifierModel("", cfg)`, v2 attempt 3, zero-dispatch rejection for overrides/config/history/S21 mismatches, and terminal exhaustion after every attempt-3 outcome. Re-submit the corrected design for fresh Captain review before any provider action.

2. [escalate] §Retry decision table — Reconcile spec Risk R-05 with the acceptance contract for the administrative third attempt.
   What I observed: Risk R-05's binding mitigation still says to "stop after any attempt-2 non-final outcome without a third call," while AC-09/10/12 and the ratified recovery record explicitly allow one separately governed attempt 3 after the opaque attempt-2 outcome. The design repeats the risk mitigation rather than acknowledging the newer administrative exception.
   What to ask the implementer: Return this contradiction through `/replan-release` so the Coach can confirm and the Planner can state one coherent rule: opaque remains non-retryable under the typed classifier, but the separately ratified configured-recovery-v2 authority may permit exactly terminal attempt 3 after all gates. Do not make implementation infer precedence between contradictory spec clauses.

3. [mechanical] §Design-fit gate — Restore the required design structure and carry the new Type-1 decision into it.
   What I observed: The current document has no numbered §1–§6, no §2 decision list, no §3 file plan, and no §6 questions. Consequently the new Type-1 status decision, "After immutable attempt 2 ended opaque, authorize one config-only administrative recovery outside the typed retry classifier," is absent from the design and its planned touchpoints cannot be checked through the normal ancestry/collision gate.
   What to ask the implementer: Produce a fresh Design TL;DR with §1 user-visible outcome, all three §2 decisions and their recorded Coach authority, an explicit §3 file plan, §4 exclusions, §5 reachability/evidence, and §6 open questions or an explicit none. Include the configured-attempt-3 options, trade-offs, and prior authority verbatim enough for Rule 9 to be reviewed.

4. [mechanical] §Release gate — Restore the design-before-code ordering before any further source or provider action.
   What I observed: `design.md` says the material rescope "requires a fresh Captain design review before any source or provider action," and AC-12 requires Captain-reviewed design acknowledgement before configured recovery. Nevertheless current HEAD `d02899f6` is `feat(llm-check): add bounded configured recovery for S22`, changing four planned source/test files after the replan and before this review.
   What to ask the implementer: Freeze provider action and further source changes, treat `d02899f6` only as an unapproved implementation candidate, revise the design against the current configured-recovery contract and live tree, and obtain a fresh Captain verdict/Coach acknowledgement before resuming. The later Verifier must independently assess the already-written code; this review does not retroactively certify it.

5. [memory-cited] §Boundary — Preserve customer-configured model choice without creating a default or fallback.
   What I observed: Status Decision 3 resolves the operator's existing `verifier.model` through the standard config authority, forbids a CLI override/fallback, and records only the resolved model ID. That aligns with the project's ratified no-model-defaults policy: role requirements are capability floors and the customer explicitly chooses the model/provider. The stale design does not yet cite or express this alignment.
   What to ask the implementer: Confirm `[[no-model-defaults-policy]]` applies in the revised design; keep config consumption read-only, require structured-output capability, and prohibit any hard-coded substitute, fallback, config mutation, or inferred provider/model default.
   Citation (if [memory-cited]): [[no-model-defaults-policy]]

## Summary

Pins: 5 total — 3 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): 1, 2, 4

## Smaller flags (not pins, worth one-line acknowledgement)

No active sibling is `in_progress` or `implemented` on the colliding files: the relevant siblings are verified, deferred, or blocked. S21 remains verified at its declared authoritative status commit, so the upstream dependency is mechanically present; the blocker is contract/design coherence and lifecycle ordering, not sibling sequencing.

## Suggested acknowledgement reply

TL;DR The config-only recovery direction is sound, but its governing spec/design must be made coherent and the design-before-code gate restored. 5 pins + 1 flag:

1. **Replace the stale attempt-2 design.** Rewrite the design for immutable v1 attempts 1–2, config-only v2 attempt 3, exact zero-dispatch gates, and terminal exhaustion with no attempt 4; submit it for fresh Captain review before provider action.
2. **Reconcile Risk R-05 with AC-09/10/12.** Carry the contradiction through `/replan-release` and state explicitly that opaque remains non-retryable while the separately ratified administrative authority permits only terminal attempt 3 after all gates.
3. **Restore the Design TL;DR contract.** Add numbered §1–§6, all three §2 decisions with Coach authority, an explicit §3 file plan, reachability/evidence, exclusions, and explicit open questions or none.
4. **Restore design-before-code ordering.** Freeze further source/provider action, treat `d02899f6` as an unapproved candidate, and obtain fresh Captain/Coach acknowledgement before resuming; leave certification to the fresh Verifier.
5. **Preserve no-model-defaults policy.** Apply `[[no-model-defaults-policy]]`; consume the configured verifier read-only, require capability, and allow no override, fallback, mutation, or inferred default.

Flags (not pins): (a) no active sibling is concurrently implementing a colliding file, and the declared S21 verified/PASS upstream evidence is present.

Status design decisions 1–3 acknowledged subject to Decision 3 being carried into the revised `design.md`. §6 is absent and must be restored as explicit questions or explicit none.

Address pins 1–5 through the replan and revised design, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Risk R-05 contradicts the ratified attempt-3 acceptance contract, design.md still describes attempt 2, and configured-recovery code landed before the required fresh review.
-->
