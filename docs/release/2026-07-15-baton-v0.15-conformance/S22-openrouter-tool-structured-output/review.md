# Captain review — S22-openrouter-tool-structured-output
Date: 2026-07-18T07:00:01+10:00
Design commit: fcd17df5b4ce365d2e077af3f4a3d63a03390b9b

## Pins
1. [memory-cited] §Boundary / Decision 1 — Keep `openrouter/z-ai/glm-5.2` proof-only, never a model default.
   What I observed: The design calls it the “Coach-selected GLM-5.2 proof model”; the spec and status keep model defaults, catalogue, pricing, and hosted-service behavior out of scope.
   What to ask the implementer: Keep the selected model confined to the S22 receipt binding and deterministic proof seam; do not change any default, catalogue, or provider-routing policy.
   Citation: [[2026-07-14T03-04-28-9XdZ-baton_v0_15_conformance_s20_vendoring_and_model_policy]]

2. [mechanical] §Required deterministic evidence — Retain AC-05 direct-base and proxy-isolation regression evidence.
   What I observed: The design names the fake direct endpoint and route boundary, while `internal/model/config.go` and `internal/model/oai.go` are planned touchpoints; the status test command already names the direct-base override and proxy-ignore tests.
   What to ask the implementer: Run and retain the two named AC-05 regressions (`TestFromEnvOpenRouterDirectBaseURLOverride` and `TestFromEnvOpenRouterProxyIgnoresDirectBaseURLOverride`) in the S22 proof evidence, alongside the receipt tests.

## Summary
Pins: 2 total — 1 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)
The on-disk rescope already matches its strict policy: metadata-only atomic receipt, one retry only for enumerated environmental or receipt classes, no retry for parse/schema/identity/malformed/substantive outcomes, and no third dispatch after attempt 2. S21 remains verified/PASS; no active sibling collision exists.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The receipt-recovery rescope is implementable without changing OpenRouter, proxy, or model-default authority. 2 pins + 0 flags:

1. **Proof-only model selection.** Keep `openrouter/z-ai/glm-5.2` confined to the S22 proof receipt binding and deterministic proof seam; do not change defaults, catalogue, or provider-routing policy.
2. **Direct-base regression coverage.** Include `TestFromEnvOpenRouterDirectBaseURLOverride` and `TestFromEnvOpenRouterProxyIgnoresDirectBaseURLOverride` in the S22 proof evidence with the receipt tests.

Flags (not pins): the atomic metadata-only receipt and exact two-attempt policy already match the re-scoped contract; S21 verified/PASS remains the serial upstream gate.

§2 decisions Decision 1 [memory-cited] and Decision 2 acknowledged. §6 questions none.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The re-scoped design preserves the direct-only canonical gate, exact receipt binding, and bounded retry policy; remaining pins are inline confirmations.
-->
