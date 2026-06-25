# Captain review — S13-bedrock-driver
Date: 2026-07-09
Design commit: b0101f5422d96e92d21ced0da4329b00996cf0ad

## Pins

1. [mechanical] §3/§4 — CRITICAL: internal/model/config.go missing from file plan
   What I observed: §4 says "No internal/model/config.go struct change" but config.go's
   FromEnv() has a key gate (lines 71-79) that checks SWORN_BEDROCK_API_KEY for bedrock/*
   models. Bedrock uses IAM credentials, not an API key — this gate returns
   "model: SWORN_BEDROCK_API_KEY not set" before NewClient() is ever reached. The same
   critical pin was caught in S12-google-driver R1. Additionally, swornProviderConfig()
   in config.go (line 139) needs AwsRegion: os.Getenv("AWS_REGION") to populate the new
   ProviderConfig field the design adds in §3.
   What to ask the implementer: Add internal/model/config.go to planned_files. In FromEnv(),
   add `case "bedrock": key = "iam"` (same pattern as `case "vertex": key = "adc"`) to
   bypass the API key gate. In swornProviderConfig(), add AwsRegion from AWS_REGION or
   AWS_DEFAULT_REGION env var. Remove the §4 "No config.go change" item.

2. [mechanical] §2.3 — Pricing table model ID format mismatch
   What I observed: The pricing table uses keys like "claude-opus-4-8", "claude-sonnet-4-6"
   but Bedrock model IDs arrive from NewClient as "anthropic.claude-sonnet-4-5" (with the
   anthropic. prefix, since parseModelID strips only the "bedrock/" prefix). The pricing
   lookup bedrockPricing[model] will never match because the keys lack the "anthropic."
   prefix. Nova models ("amazon.nova-pro-v1:0") are correct.
   What to ask the implementer: Update pricing table keys to match actual Bedrock model IDs
   as they arrive from the dispatch (e.g. "anthropic.claude-opus-4-8", not "claude-opus-4-8").
   Alternatively, strip the "anthropic." prefix before lookup — but document that choice.

3. [mechanical] §2.2 vs §3 vs §4 — Internal contradiction about config struct changes
   What I observed: §2 Decision 2 says "No config.json struct change needed — region
   flows through the function parameter, not a global config field." §3 says "add AwsRegion
   to ProviderConfig (used by swornProviderConfig too)." §4 says "No internal/model/config.go
   struct change for a BedrockRegion field." Adding AwsRegion to ProviderConfig IS a config
   struct change, and config.go's swornProviderConfig() must populate it. The three sections
   contradict each other on whether a config change is needed.
   What to ask the implementer: Reconcile — acknowledge that ProviderConfig gets a new AwsRegion
   field (§3 is correct), that config.go's swornProviderConfig() must populate it, and that
   §2/§4's "no config change" claims are wrong. Remove the §4 "No config.go change" item.

4. [mechanical] status.json — design_decisions absent (recurring)
   What I observed: status.json has no design_decisions field. This is the 6th+ recurrence
   in the trial log. The designfit gate trivially passes on empty. Design decisions D1-D5
   include architecturally-significant choices (new SDK dependency, mock strategy, error
   classification approach) that should be recorded.
   What to ask the implementer: Add design_decisions to status.json with at least D1 (mock
   strategy — Type-2, reversible test pattern) and D5 (error classification via smithy.APIError
   — Type-2, consistent with existing drivers). The AWS SDK dependency addition is covered
   by ADR 0007 so doesn't need a separate Type-1 entry.

## Summary

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: #1 (config.go missing from file plan — bedrock/* models will fail at FromEnv key gate before NewClient is reached)

## Smaller flags (not pins, worth one-line ack)

- Spec Risk #1 says "run go mod tidy and check for unexpected packages" — the implementer should do this during implementation and note results in proof.md.
- Spec Risk #3 says IAM permissions must be documented in proof.md — ensure this lands there.
- The existing provider_test.go has a test asserting bedrock/* returns ErrDriverNotRegistered — that test will need updating to expect a *Bedrock instead.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is sound and follows the established driver pattern well. 4 pins + 3 flags:

1. **CRITICAL: config.go missing from file plan.** Add `internal/model/config.go` to planned_files. In `FromEnv()`, add `case "bedrock": key = "iam"` to bypass the API key gate (same pattern as `case "vertex": key = "adc"`). In `swornProviderConfig()`, add `AwsRegion: os.Getenv("AWS_REGION")` (fallback to `AWS_DEFAULT_REGION`). Remove the §4 "No config.go change" item — it's wrong.

2. **Pricing table model ID format mismatch.** Bedrock model IDs arrive as `"anthropic.claude-sonnet-4-5"` (with the `anthropic.` prefix) because `parseModelID` strips only the `bedrock/` prefix. Update pricing table keys to match (e.g. `"anthropic.claude-opus-4-8"`, not `"claude-opus-4-8"`), or strip the prefix before lookup — document whichever you pick.

3. **Internal contradiction on config changes.** §2 D2 says "no config struct change", §3 says "add AwsRegion to ProviderConfig", §4 says "no config.go change". §3 is correct — reconcile §2 and §4 to match.

4. **design_decisions absent from status.json.** Add design_decisions with D1 (mock strategy — Type-2) and D5 (error classification — Type-2). AWS SDK dep is covered by ADR 0007.

Flags (not pins): (a) run `go mod tidy` and audit transitive deps per spec Risk #1, note in proof.md; (b) document IAM permissions in proof.md per spec Risk #3; (c) update the existing `provider_test.go` bedrock assertion from `ErrDriverNotRegistered` to expect `*Bedrock`.

§2 decisions D1-D5 ack (mock strategy, region resolution, pricing table, Converse types, error classification all follow established patterns). §6 questions: none, ack.

Address pins 1-4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline corrections (config.go touchpoint, pricing key format, doc contradiction, status.json field) — no design re-check needed before code.
-->