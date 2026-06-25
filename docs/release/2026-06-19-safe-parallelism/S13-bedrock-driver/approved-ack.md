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
