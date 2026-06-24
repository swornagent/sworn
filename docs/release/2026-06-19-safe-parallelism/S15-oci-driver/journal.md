---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S15-oci-driver`

## Session log

### 2026-07-09 — Implementation (implementer session)

State transition: `design_review` → `implemented`.

**Coach-approved design pins applied:**
1. Added `internal/model/config.go` to `planned_files` in status.json.
2. Corrected D3 rationale: OCI SDK client is a concrete struct without a
   BaseEndpoint override (unlike Bedrock, which uses httptest), so local
   interface extraction is needed for test substitution.
3. Coach ack for pin 3: `$OCI_REGION` spec drift → SDK-native `OCI_CLI_REGION`
   / config-file region accepted. Spec amendment needed in `/replan-release`.
4. Added `design_decisions` array to status.json (D1-D5, all `type_2: true`).
5. Updated ProviderConfig comment: "OCI SDK auth env vars are read directly by
   the OCI driver; OCICompartmentID is a SwornAgent-specific routing param
   stored here."

**Implementation decisions:**
- OCI driver is a standalone `OCI` struct (not OAI-embedded), following the
  `AzureOAI` pattern. Uses `DefaultConfigProvider()` from the OCI SDK for
  credential discovery (config file + env vars).
- Credential loading deferred to first `Verify()` call — `NewOCI` returns
  non-nil `*OCI` with nil client if config is absent. `EnsureClient()` lazily
  creates the client at Verify time.
- Mock via local `generativeAIInferenceClient` interface matching `Chat()`.
  Tests use `fakeOCIClient` instead of `httptest` (no BaseEndpoint override in
  OCI SDK).
- OCI HTTP errors routed through `NewProviderError` via `common.IsServiceError()`
  for the typed `model.Error` taxonomy.
- Cost always 0.0 — `Usage` is optional (pointer, nil when absent).
- Region from OCI SDK's `DefaultConfigProvider()` → honours `OCI_CLI_REGION` /
  config-file `[DEFAULT].region`. No separate `$OCI_REGION` parsing.
- `OCICompartmentID` populated from `$OCI_COMPARTMENT_ID` in both
  `ProviderConfigFromEnv()` and `swornProviderConfig()`.
- `FromEnv` key-gate switch: added `case "oci": key = "compartment"` (sentinel
  matching `bedrock`/`vertex` pattern — no API key required).

**Files changed:**
- `internal/model/oci.go` (new)
- `internal/model/oci_test.go` (new, 6 tests)
- `internal/model/provider.go` (add OCICompartmentID, wire oci case)
- `internal/model/config.go` (oci key-gate, OCICompartmentID in swornProviderConfig)
- `internal/model/provider_test.go` (empty native stub list)
- `go.mod`, `go.sum` (add oci-go-sdk/v65)

**Test results:** 6/6 OCI tests PASS, all 100+ model tests PASS, `go vet` clean.

**Reachability artefact:** `go test ./internal/model/... -run OCI` (6/6 PASS).

**Skeptic panel:** skipped — runtime does not support subagent dispatch (single-threaded API mode).

**Divergence from plan:**
- `$OCI_REGION` env var: spec named `$OCI_REGION`; design uses SDK-native
  `OCI_CLI_REGION` / config-file region. Coach acked (pin 3). Tracked for
  `/replan-release` spec amendment.

## Open questions

None.

## Deferrals surfaced

- Instance principal / resource principal auth: deferred post-R3 (per spec).

## Verifier verdicts received

*(None yet.)*
### 2026-07-09 — Verifier (fresh context, artefact-only)

FAIL

Slice: S15-oci-driver

Violations:
1. Gate 2 — Planned touchpoints mismatch (config.go and provider_test.go modified but not listed in spec.md "Planned touchpoints"; proof.md "Divergence from plan" does not explain them — only covers $OCI_REGION drift).
   Evidence: spec.md:53-57 (Planned touchpoints), git diff --name-only 3d60456432fd6dbfcdfb6248bf084bfe3da9564a..HEAD, proof.md Divergence section, journal.md implementation notes (added to status.json planned_files but spec.md untouched).
2. Gate 3 — Compartment ID validation and test name do not match spec acceptance check (spec says "Verify returns a non-nil error" for missing compartment; implementation errors in NewOCI; test TestOCIVerify_MissingCompartment calls NewOCI directly, not Verify).
   Evidence: spec.md acceptance checks ("cfg.OCICompartmentID empty ... → Verify returns..."), oci.go:42-44 (if compartmentID == "" return error in NewOCI), oci_test.go:68-72 (TestOCIVerify_MissingCompartment), Verify path at oci.go:84.

Required to address:
1. Update spec.md "Planned touchpoints" to include config.go and provider_test.go (or document as divergence).
2. Align spec acceptance check, test name, or implementation for compartment ID validation location (NewOCI vs Verify).
3. Update proof.md "Divergence from plan" and "Delivered" evidence references to be accurate.
4. Document extra indirect deps in go.mod if they are unexpected (per spec Risk #1).

Next step for human: re-open `/implement-slice S15-oci-driver 2026-06-19-safe-parallelism` in a fresh session to address the violations.
