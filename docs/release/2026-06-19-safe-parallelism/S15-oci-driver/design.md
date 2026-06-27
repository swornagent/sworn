# Design TL;DR: `S15-oci-driver`

## §1. User-visible change

A user with OCI credentials in `~/.oci/config` (or standard OCI environment
variables) sets `verifier.model = "oci/cohere.command-r-plus"` in config.json.
`sworn run` dispatches to OCI Generative AI via the `oci-go-sdk/v65`
`generativeaiinference` client, calls the Chat endpoint, and returns a
PASS/FAIL verdict. The compartment ID comes from `$OCI_COMPARTMENT_ID`.

## §2. Design decisions not in spec (max 5)

1. **Standalone struct (not OAI-embed).** OCI's Generative AI Inference API
   uses its own request/response shapes (`ChatRequest`/`ChatResult`),
   different from OpenAI Chat Completions. Embedding `*OAI` would create a
   misleading type relationship — `BaseURL` and `Authorization` are
   meaningless for OCI. The OCI driver is a standalone `OCI` struct like
   `AzureOAI`. Rationale: same design reasoning as S14-azure-driver.

2. **OCI SDK config from environment only.** `NewOCI` calls
   `common.NewConfigProvider("DEFAULT")` which reads `~/.oci/config` and
   `OCI_*` env vars from the standard OCI SDK. No OCI config fields are
   stored in `ProviderConfig` — the SDK handles credential discovery
   entirely on its own. Only `OCICompartmentID` (a SwornAgent-specific env
   var) is added to `ProviderConfig`. Rationale: matches the spec's deferral
   of instance principal auth; keeps the config surface minimal.

3. **Mock OCI client via interface, not `httptest`.** The OCI SDK
   `generativeaiinference` client is a concrete struct, not an interface.
   Tests will define a local `generativeAIInferenceClient` interface matching
   the `Chat` method signature and store it on the `OCI` struct so the mock
   can be substituted. Rationale: same approach used by Bedrock tests (mock
   the client, not the HTTP layer). This keeps tests fast and offline.

4. **Cost: optional token counts, return 0.0.** The OCI ChatResult
   `Usage` field is a pointer — nil when the model doesn't return counts.
   Driver checks `if cr.Usage != nil` and extracts token counts; returns
   `0.0` when absent, no error. Rationale: per spec "use 0.0 when absent
   rather than an error"; matches Azure's pattern of returning 0 cost.

5. **Region from OCI SDK, not `$OCI_REGION` override.** The region comes
   from the OCI config file's `[DEFAULT]` profile (the SDK reads it
   automatically). There is no separate `OCI_REGION` env var parsing in the
   driver — the SDK does this as part of `common.NewConfigProvider()`.
   Rationale: the OCI SDK honours `OCI_CLI_REGION`; no need to duplicate.

## §3. Files I'll touch grouped by purpose

- **OCI driver + tests** (`internal/model/oci.go`, `internal/model/oci_test.go`):
  new files — the OCI struct, NewOCI constructor, Verify method, and unit
  tests with a mock generative AI inference client.

- **Provider routing** (`internal/model/provider.go`): replace the
  placeholder `ErrDriverNotRegistered` return for the `"oci"` case with a
  `NewOCI(model, pcfg.OCICompartmentID)` call. Add `OCICompartmentID` to
  `ProviderConfig` and populate from `$OCI_COMPARTMENT_ID` in
  `ProviderConfigFromEnv()` and `swornProviderConfig()`.

- **Dependencies** (`go.mod`, `go.sum`): add `github.com/oracle/oci-go-sdk/v65`
  (`generativeaiinference` + transitive auth/common packages only via
  `go mod tidy`).

- **Config** (`internal/model/config.go`): update `FromEnv` to handle the
  `"oci"` provider case (key check for backward compat). Update
  `swornProviderConfig()` to pass OCI compartment ID.

## §4. Things I'm NOT doing

- Instance principal / resource principal auth (deferred per spec — config
  file auth only in this slice).
- OCI Generative AI streaming, embeddings, or custom model deployments.
- OCI region override beyond what the OCI SDK provides natively.
- `config.go` `FromEnv` `"oci"` case with SWORN_* backward compat — OCI
  doesn't use an API key in the OpenAI sense; the SDK handles auth. The
  `"oci"` case in `FromEnv` will check for compartment ID availability
  via the canonical `$OCI_COMPARTMENT_ID` env var, consistent with how
  `"bedrock"` and `"vertex"` use sentinel values.

## §5. Reachability plan

- **Unit tests (offline):** `go test ./internal/model/... -run OCI` — four
  tests covering mock Chat response, missing compartment ID, nil usage,
  and `NewClient` routing to `*OCI`.
- **Live integration test (skippable):** `TestOCIVerify_Live` gated on
  `OCI_COMPARTMENT_ID` and `SWORN_LIVE_TESTS=1` and valid `~/.oci/config`
  — sends "Reply with PASS." and asserts PASS returned.
- **Smoke step:** `sworn run --model oci/cohere.command-r-plus` with real
  OCI credentials.

## §6. Open questions for the Coach

None.