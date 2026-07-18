# Captain review — S22-openrouter-tool-structured-output
Date: 2026-07-18T13:50:40+10:00
Design commit: 19d2ab1b66afedee3b50a5ac29761da89f038366

## Pins

1. [escalate] §Release gate — The pinned S21 authority and the current release identity cannot both satisfy the stated equality gate.
   What I observed: `status.json.upstream_gate.authoritative_track_status_commit` pins `240a2ede...`; the S21 status at that commit identifies release `2026-07-15-baton-v0.15-conformance`, while S22 and the copied current S21 status identify `2026-07-15-baton-v0.16-conformance`. The design says the authoritative reference and release identity must agree but does not define which identity is authoritative after the rename.
   What to ask the implementer: Do not infer the answer. The Coach must choose whether the original verifier commit remains authoritative across the release rename or whether the gate is repinned to a v0.16 status commit; then `/replan-release` must make `spec.json`, `status.json`, and the exact lookup path agree before implementation.

2. [escalate] §Native receipt lifecycle.3 — The double-fault branch has no durable trust protocol.
   What I observed: The design says that after finalization rename succeeds, directory sync fails, and reservation restoration also fails, the implementation may “overwrite or surface” a receipt failure. Surfacing a process-local failure does not stop a later process from reading the renamed final verdict that remains on disk; the live implementation currently ignores the restoration error, so this is the exact load-bearing gap introduced by the replan.
   What to ask the implementer: Present at least two durable authority designs with trade-offs (for example, a separately committed finalization marker versus a durable invalidation/quarantine protocol). The Coach selects the trust protocol; record it as the Type-1 receipt decision before code. The chosen design must make an unacknowledged renamed verdict mechanically untrustworthy on every later read.

3. [mechanical] §Required deterministic evidence — The design has no explicit file plan, and the status plan omits the MCP boundary required by AC-11.
   What I observed: `spec.json.touchpoints` includes `internal/mcp/lint.go` and `internal/mcp/lint_test.go`, and AC-11 requires registered-tool reachability, but both paths are absent from `status.json.planned_files`; design.md has no §3 file plan at all.
   What to ask the implementer: Add the complete file plan to design.md and add both MCP paths to the tracked planned files before source work. Keep the MCP change limited to the exact stable provider-error text and its reachability/leak canaries.

4. [mechanical] §Boundary — The existing proof command is still bound to the old release name.
   What I observed: `cmd/sworn/llmcheck.go` currently sets `s22ProofReceiptRelease` to `2026-07-15-baton-v0.15-conformance`, while the revised design and receipt binding require `2026-07-15-baton-v0.16-conformance`. The design acknowledges the material rescope but does not explicitly name this live cross-release anchor.
   What to ask the implementer: Re-anchor the command, status lookup, historical receipt binding, and deterministic built-command tests to the v0.16 release without changing the immutable S22 start, model, check, or two-attempt budget. Prove the old/mismatched release makes zero provider dispatches.

5. [mechanical] §Native receipt lifecycle — The enlarged receipt module needs the required cohesion review.
   What I observed: The current design gate flags `internal/gate/llmcheck_receipt.go` at 610 added lines against the 250-line growth threshold, and the revised design adds another durability protocol without explaining why one file remains cohesive.
   What to ask the implementer: Before implementation, record a short cohesion audit naming the state-machine, persistence, rendering, and runner seams. Split only if those responsibilities are independently testable; otherwise state why keeping them together preserves the atomic invariant.

6. [memory-cited] §Retry decision table — The narrow proof classifier correctly remains separate from legacy broad transient handling.
   What I observed: The design allows retry only for explicit typed environmental/receipt classes and prohibits error-text classification or reuse of `IsTransient`; this aligns with the ratified typed provider-error taxonomy and its policy-consumer boundary.
   What to ask the implementer: Confirm `[[project_provider_error_taxonomy]]` applies, keep existing callers unchanged, and prove unknown/auth/credits/client failures cannot become retry authority.
   Citation (if [memory-cited]): [[project_provider_error_taxonomy]]

7. [memory-cited] §Boundary — The fixed GLM selection is a proof-scoped override, not a model eligibility rule or default.
   What I observed: The design keeps `openrouter/z-ai/glm-5.2` limited to the Coach-selected S22 proof and explicitly forbids model-default or generic-provider changes. That matches the ratified capability-first policy, where explicit model pins are overrides and capability remains the general eligibility authority.
   What to ask the implementer: Confirm `[[capability-based-model-selection-ratified]]` applies and keep every direct-GLM constant and test scoped to this proof path only.
   Citation (if [memory-cited]): [[capability-based-model-selection-ratified]]

## Summary

Pins: 7 total — 3 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins (if any): 1, 2, 3, 4

## Smaller flags (not pins, worth one-line acknowledgement)

None.

## Suggested acknowledgement reply

TL;DR The safety direction is coherent once the two release-authority decisions are made. 7 pins + 0 flags:

1. **Resolve the S21 authority identity.** Apply the Coach's decision on old-verifier-commit authority versus a v0.16 repin through `/replan-release`, so the pinned commit, lookup path, release identity, and gate checks agree exactly.
2. **Choose a durable double-fault trust protocol.** Record the Coach-selected finalization-marker or invalidation/quarantine design and ensure a renamed but unacknowledged verdict is mechanically untrusted by every later reader.
3. **Declare the complete file plan.** Add `internal/mcp/lint.go` and `internal/mcp/lint_test.go` to planned files and keep the MCP change to the stable public diagnostic plus reachability/leak canaries.
4. **Re-anchor the proof binding to v0.16.** Update the command, status lookup, historical receipt binding, and built-command tests while preserving the immutable start, model, check, and two-attempt budget; mismatched v0.15 input must dispatch zero calls.
5. **Record the receipt-module cohesion audit.** Name the state-machine, persistence, rendering, and runner seams and either split them or justify the single atomic module.
6. **Preserve the typed classifier boundary.** Apply `[[project_provider_error_taxonomy]]`; leave legacy callers unchanged and keep unknown/auth/credits/client failures non-retryable.
7. **Keep the GLM pin proof-only.** Apply `[[capability-based-model-selection-ratified]]`; do not turn the selected proof model into a default or general capability rule.

Flags (not pins): none.

§2 decisions 1 and 2 acknowledged after the Coach decisions above are durably recorded. §6 has no open questions and is acknowledged.

Address pins 1–7 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: The renamed release has an unresolved S21 authority identity and the receipt double-fault path needs a Coach-selected durable trust protocol.
-->
