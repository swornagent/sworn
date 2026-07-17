# Captain review — S19-s02-v015-rollback

Date: 2026-07-16T23:58:24+10:00
Design commit: e0caad520c0df7c3aa4de53523f729ddf9cad237

## Pins

1. [mechanical] §5 / AC-05 — Bind the final Implementer maintainability PASS and proof to one exact implementation head.
   What I observed: §5 refers to S19's pinned `maintainability.implementation_head` and then moves directly from the proof suite to `implemented`, but it does not explicitly require the final Implementer maintainability PASS that AC-05 names before that transition.
   What to ask the implementer: Before moving S19 to `implemented`, record the final `maintainability.state=passed` and its non-null `implementation_head`; run the envelope/equality checker and hand the fresh verifier that exact head, then state the head equality in the proof and S20 gate evidence.

2. [memory-cited] §2.1 — Preserve the byte-exact and fresh-verification precedent for the Type-1 rollback.
   What I observed: The decision restores exact modes, blob OIDs, and absences, and requires an independent fresh verifier. That matches the project's prior Baton vendoring lesson that embedded schemas are normative bytes and a plausible first proof may still hide parity defects.
   What to ask the implementer: Acknowledge the cited precedent and retain mode/blob/absence equality plus the independent fresh verification in the final proof bundle.
   Citation: [[Baton v0.13.1 prerequisite upgrade and parity verification]]

Pins: 2 total — 1 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): 1

## Summary

The design is concrete and fail-closed: its immutable start tree, 45-path review control, release-record boundary, and S20 ordering agree with all five ACs and stated risks once pin 1 makes AC-05's authority ordering explicit.

## Smaller flags (not pins, worth one-line acknowledgement)

- Live history confirms `e61cb190736ee7483fb4ed1a993442b26ce3574c` resolves to tree `c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`; the current non-release diff contains exactly 45 paths, exactly matching S19's `planned_files` control list.
- S02 is `deferred` with `rollback_slice_id: S19-s02-v015-rollback`; S20 is still `planned`, and no sibling sharing these touchpoints is `in_progress` or `implemented`.
- The required LLM check did not pass and is not represented as a pass. The skill-era invocation `sworn llm-check --check design-review` exited 2 with `flag provided but not defined: -check`. The installed compatible invocation, `sworn llm-check -type design-review -release 2026-07-15-baton-v0.16-conformance -slice S19-s02-v015-rollback`, exited 1 with two findings. Its default `release-wt/2026-07-15-baton-v0.16-conformance` diff contains prior S01/S02 track history (74 changed paths), so those findings assess historical forward changes and historical release records rather than an S19 semantic change; S19 has made no semantic rollback change at this design gate. The non-pass is recorded, not relied upon as a successful design check.

## Suggested acknowledgement reply

TL;DR The rollback design is sound, with one required evidence-ordering clarification and one cited historical constraint. 2 pins + 1 flag:

1. **Bind AC-05's maintainability gate to the proof head.** Before transitioning S19 to `implemented`, obtain and record the final Implementer maintainability PASS and its `implementation_head`; run the envelope/equality checker and fresh verification against exactly that head, then cite the equality in the proof and S20 gate evidence.
2. **Preserve the byte-exact vendor precedent.** Keep the exact mode/blob/absence checks and independent fresh verification; this acknowledges the prior Sworn Baton v0.13.1 parity-verification lesson.

Flags (not pins): the attempted design LLM check is a non-pass caused by its stale `release-wt` diff base including prior S01/S02 track history; it is not a successful check and is not evidence against the S19 design.

§2 decisions are acknowledged, including the Type-1 rollback/replacement decision and its cited byte-exact/fresh-verifier precedent. §6 has no open decision question; its review controls remain covered by the proof plan.

Address pins 1–2 inline during implementation, then proceed to `in_progress`.

## LLM check

Result: NOT PASSED. See the exact commands, exit codes, and scope assessment in Smaller flags; no unavailable-credentials result occurred, and no pass is claimed.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline evidence and memory acknowledgements; no Coach authority or design re-review is required.
-->
