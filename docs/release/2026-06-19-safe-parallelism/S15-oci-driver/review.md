# Captain review — S15-oci-driver
Date: 2026-07-09
Design commit: f70196795c060f0f3ccb7646c2da7438e3a7feb7

## Pins

1. [mechanical] §3/config.go — `config.go` missing from `planned_files` in status.json (5th recurrence of S12/S13/S14 pattern)
   What I observed: Design §3 lists `internal/model/config.go` as a file to touch ("update FromEnv to handle the oci provider case"). The spec's In Scope also requires `cfg.OCICompartmentID` resolution from `ProviderConfig`, which lives in `provider.go` but the `FromEnv` key-gate in `config.go` must add a `case "oci"` sentinel (like `case "bedrock": key = "iam"`) or `oci/*` models are blocked before `NewClient()` is ever reached. `status.json` `planned_files` is `["internal/model/oci.go", "internal/model/oci_test.go", "internal/model/provider.go", "go.mod", "go.sum"]` — `config.go` is absent. Gate 2 (planned_files vs actual_files) will FAIL.
   What to ask the implementer: Add `internal/model/config.go` to `planned_files` in `status.json` before writing code. The `FromEnv` switch needs a `case "oci": key = "compartment"` (or similar sentinel) so the key-gate passes, matching the `bedrock`/`vertex` pattern.

2. [mechanical] §2 Decision 3 — rationale factually wrong about Bedrock test approach
   What I observed: Design Decision 3 says "same approach used by Bedrock tests (mock the client, not the HTTP layer)." I verified `internal/model/bedrock_test.go`: Bedrock tests use `httptest.NewServer` + `o.BaseEndpoint = aws.String(baseURL)` on `bedrockruntime.NewFromConfig` — they mock the HTTP transport layer, not a client interface. The OCI SDK's `generativeaiinference` client is a concrete struct (like Bedrock's), so the interface-extraction approach is valid, but the cited precedent is incorrect.
   What to ask the implementer: Correct the rationale in design.md to say "OCI SDK client is a concrete struct like Bedrock's, but unlike Bedrock the OCI SDK does not expose a BaseEndpoint override, so we extract a local interface for the Chat method." The approach is sound; the citation is wrong.

3. [escalate] §2 Decision 5 — spec says `$OCI_REGION` env var; design says SDK uses `OCI_CLI_REGION` and driver skips `$OCI_REGION` parsing
   What I observed: Spec In Scope line 34 says "OCI region: read from OCI config file or `$OCI_REGION` env var (standard OCI SDK)". Design Decision 5 says "The region comes from the OCI config file's [DEFAULT] profile (the SDK reads it automatically). There is no separate OCI_REGION env var parsing in the driver — the SDK does this as part of common.NewConfigProvider(). Rationale: the OCI SDK honours OCI_CLI_REGION; no need to duplicate." This is a spec deviation with explicit acknowledgement and rationale. The spec names `$OCI_REGION`; the design says the SDK uses `OCI_CLI_REGION` and the driver won't parse `$OCI_REGION` separately. The deviation is reasonable (the SDK handles region discovery), but the spec's stated env var name differs from what the SDK actually honours.
   What to ask the implementer: Coach must either ack the deviation (the SDK's `OCI_CLI_REGION` / config-file region is the correct mechanism, and the spec's `$OCI_REGION` was a planning imprecision) or require explicit `$OCI_REGION` parsing as a fallback. If acking, `/replan-release` should amend the spec to say "OCI region: read from OCI config file or `OCI_CLI_REGION` env var (standard OCI SDK)".

4. [mechanical] status.json — `design_decisions` field absent (6th recurrence)
   What I observed: `status.json` has no `design_decisions` array. The design has 5 decisions, all Type-2 (standalone struct, SDK config from env, mock via interface, cost 0.0, region from SDK). The designfit gate (`sworn designfit`) would fail closed on a missing array. S13-bedrock-driver has `design_decisions` with 5 Type-2 entries as precedent.
   What to ask the implementer: Add a `design_decisions` array to `status.json` with entries D1-D5 matching the 5 design decisions, all `type_2: true`. Follow the S13-bedrock-driver status.json format.

5. [mechanical] §2 Decision 2 — `ProviderConfig` already has a comment saying OCI vars are NOT stored there, but design says to add `OCICompartmentID`
   What I observed: `provider.go` line 30 has `// OCI SDK env vars are read directly by the OCI driver (S15); not stored here.` The design (§2 Decision 2 and §3) says to add `OCICompartmentID` to `ProviderConfig` and populate from `$OCI_COMPARTMENT_ID`. This is not a contradiction — the comment refers to OCI SDK auth vars (key_file, fingerprint, etc.), while `OCICompartmentID` is a SwornAgent-specific routing param, not an SDK auth var. But the existing comment will become misleading once `OCICompartmentID` is added.
   What to ask the implementer: Update the comment on `ProviderConfig` to clarify: "OCI SDK auth vars are read directly by the OCI driver; OCICompartmentID is a SwornAgent-specific routing param stored here."

## Summary

Pins: 5 total — 4 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins: 1 (pin 1 — config.go missing from planned_files will cause Gate 2 FAIL)

## Smaller flags (not pins, worth one-line ack)

- (a) Spec Risks #2 and #3 require documentation in proof.md (config file format prerequisites, region availability). Design §5 mentions `~/.oci/config` as a live-test prerequisite but doesn't explicitly commit to proof.md documentation. The implementer should include this in the proof bundle.
- (b) Design §3 says `ProviderConfigFromEnv()` and `swornProviderConfig()` both need `OCICompartmentID` population. Both functions exist in different files (`provider.go` and `config.go` respectively). The implementer should update both consistently.
- (c) No `NewProviderError` usage mentioned in the design. All other native drivers (Bedrock, Azure, Anthropic, Google) route HTTP errors through `NewProviderError` for the typed `model.Error` taxonomy. The OCI driver should follow the same pattern for consistency.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is sound and follows the established native-driver pattern; 5 pins + 3 flags:

1. **config.go missing from planned_files.** Add `internal/model/config.go` to `planned_files` in status.json. The `FromEnv` key-gate switch in `config.go` needs a `case "oci"` sentinel (like `case "bedrock": key = "iam"`) or `oci/*` models are blocked before `NewClient()` is reached. This is the same S12/S13/S14 recurrence — fix before code.

2. **Bedrock test rationale is factually wrong.** Design Decision 3 cites "same approach used by Bedrock tests (mock the client, not the HTTP layer)" but Bedrock tests actually use `httptest.Server + BaseEndpoint override`. Correct the rationale: the OCI SDK client is a concrete struct without a BaseEndpoint override, so a local interface extraction is needed instead. The approach is valid; the citation is not.

3. **Spec says `$OCI_REGION`, design says `OCI_CLI_REGION`.** Spec In Scope line 34 names `$OCI_REGION`; Design Decision 5 says the SDK honours `OCI_CLI_REGION` and the driver won't parse `$OCI_REGION` separately. Coach ack required — either accept the SDK-native mechanism (and amend spec via `/replan-release`) or require explicit `$OCI_REGION` fallback parsing.

4. **design_decisions absent from status.json.** Add a `design_decisions` array with D1-D5 (all `type_2: true`), following the S13-bedrock-driver status.json format. The designfit gate fails closed without it.

5. **ProviderConfig comment will become misleading.** Update the `// OCI SDK env vars are read directly by the OCI driver (S15); not stored here.` comment to clarify that OCICompartmentID IS stored there (it's a routing param, not an SDK auth var).

Flags (not pins): (a) include OCI config file format + region availability docs in proof.md per spec Risks #2/#3; (b) update both `ProviderConfigFromEnv()` and `swornProviderConfig()` with OCICompartmentID consistently; (c) route OCI HTTP errors through `NewProviderError` for the typed model.Error taxonomy, matching all other native drivers.

§2 decisions 1, 4, 5 ack (standalone struct, cost 0.0, region from SDK). §6 questions: none. Address pins 1-5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are apply-inline corrections (planned_files fix, comment fix, rationale fix, design_decisions addition) except pin 3 which is a spec-vs-design naming deviation with a determinable answer — the OCI SDK's region mechanism is a technical fact, not a judgement call; Coach ack is a formality to amend the spec text, and the design's approach (defer to SDK) is the correct one.
-->