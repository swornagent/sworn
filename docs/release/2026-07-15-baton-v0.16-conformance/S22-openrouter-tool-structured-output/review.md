# Captain review — S22-openrouter-tool-structured-output
Date: 2026-07-18T14:17:42+10:00
Design commit: 19d2ab1b66afedee3b50a5ac29761da89f038366

## Pins

1. [mechanical] §Native receipt lifecycle.3 — Make the double-fault trust invariant explicit in code and deterministic evidence.
   What I observed: The design already requires that a final verdict renamed before a directory-sync failure must not remain trusted when reservation restoration also fails. The live implementation currently ignores the restoration error, so the missing piece is the implementation guard, not a further Coach decision.
   What to ask the implementer: Apply a durable trust marker, invalidation record, or equivalent fail-closed mechanism so every later reader rejects an unacknowledged renamed verdict. Add the named post-rename-plus-restoration-double-fault test and prove the only surfaced outcome is sanitized receipt_failure/UNPARSEABLE with unavailable exit semantics.

2. [mechanical] §Required deterministic evidence — Declare the MCP files required by AC-11.
   What I observed: `spec.json.touchpoints` includes `internal/mcp/lint.go` and `internal/mcp/lint_test.go`, and AC-11 requires registered-tool reachability, but both paths are absent from `status.json.planned_files`; design.md also has no explicit file-plan section.
   What to ask the implementer: Add both MCP paths to the tracked planned files while implementing and keep their change limited to the exact stable provider-error text plus registered-tool reachability and leak canaries.

3. [mechanical] §Boundary — Re-anchor the runtime proof binding to the active v0.16 release.
   What I observed: The active release records, current S21/S22 statuses, and attempt-2 binding correctly identify `2026-07-15-baton-v0.16-conformance`. Only the pre-replan source constant `s22ProofReceiptRelease` in `cmd/sworn/llmcheck.go` still names v0.15, which is expected because the replan made no source changes.
   What to ask the implementer: Update the command, status lookup, historical receipt binding, and built-command tests to v0.16 without changing the immutable S22 start, model, check, or two-attempt budget. Prove v0.15 or any other mismatched release makes zero provider dispatches.

4. [mechanical] §Native receipt lifecycle — Record the enlarged receipt module's cohesion audit.
   What I observed: The design gate flags `internal/gate/llmcheck_receipt.go` at 610 added lines against the 250-line growth threshold, and this implementation adds another durability guard.
   What to ask the implementer: Record a short cohesion audit naming the state-machine, persistence, rendering, and runner seams. Split only if those responsibilities are independently testable; otherwise state why keeping them together preserves the atomic invariant.

5. [memory-cited] §Retry decision table — The narrow proof classifier correctly remains separate from legacy broad transient handling.
   What I observed: The design allows retry only for explicit typed environmental/receipt classes and prohibits error-text classification or reuse of `IsTransient`; this aligns with the ratified typed provider-error taxonomy and its policy-consumer boundary.
   What to ask the implementer: Confirm `[[project_provider_error_taxonomy]]` applies, keep existing callers unchanged, and prove unknown/auth/credits/client failures cannot become retry authority.
   Citation (if [memory-cited]): [[project_provider_error_taxonomy]]

6. [memory-cited] §Boundary — The fixed GLM selection is a proof-scoped override, not a model eligibility rule or default.
   What I observed: The design keeps `openrouter/z-ai/glm-5.2` limited to the Coach-selected S22 proof and explicitly forbids model-default or generic-provider changes. That matches the ratified capability-first policy, where explicit model pins are overrides and capability remains the general eligibility authority.
   What to ask the implementer: Confirm `[[capability-based-model-selection-ratified]]` applies and keep every direct-GLM constant and test scoped to this proof path only.
   Citation (if [memory-cited]): [[capability-based-model-selection-ratified]]

## Summary

Pins: 6 total — 4 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2, 3

## Smaller flags (not pins, worth one-line acknowledgement)

The pinned S21 verifier commit predates the release rename and therefore retains the historical v0.15 path and record identity. That is immutable provenance, not the active runtime binding: the current S21 status and S22 recovery binding correctly identify v0.16.

The required `sworn llm-check --type design-review` ran on 2026-07-18 and reported only the known pre-implementation v0.15 runtime constant/test mismatch. Mechanical pin 3 captures that exact correction; the check found no additional design divergence.

## Suggested acknowledgement reply

TL;DR The v0.16 replan is coherent and the design can proceed with six apply-inline safeguards. 6 pins + 2 flags:

1. **Close the receipt double-fault guard.** Make every later reader reject an unacknowledged renamed verdict and add the deterministic post-rename-plus-restoration-failure test.
2. **Declare the MCP touchpoints.** Add `internal/mcp/lint.go` and `internal/mcp/lint_test.go` to planned files and confine the change to the stable diagnostic plus reachability/leak canaries.
3. **Re-anchor runtime binding to v0.16.** Update the command constant, lookup, receipt binding, and tests while preserving the immutable start, model, check, and two-attempt budget; mismatched releases dispatch zero calls.
4. **Record the receipt-module cohesion audit.** Name the state-machine, persistence, rendering, and runner seams and either split them or justify the single atomic module.
5. **Preserve the typed classifier boundary.** Apply `[[project_provider_error_taxonomy]]`; leave legacy callers unchanged and keep unknown/auth/credits/client failures non-retryable.
6. **Keep the GLM pin proof-only.** Apply `[[capability-based-model-selection-ratified]]`; do not turn the selected proof model into a default or general capability rule.

Flags (not pins): (a) the v0.15 identity at pinned commit `240a2ede...` is retained historical verification provenance; active S21/S22 records and runtime work belong to v0.16; (b) the required design-review LLM check reported only the known v0.15 runtime constant/test mismatch already covered by pin 3.

§2 decisions 1 and 2 acknowledged. §6 has no open questions and is acknowledged.

Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The v0.16 records are coherent; all remaining pins are deterministic apply-inline guards that the fresh Verifier can backstop.
-->
