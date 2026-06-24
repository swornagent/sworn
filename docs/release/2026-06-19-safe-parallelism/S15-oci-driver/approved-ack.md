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
