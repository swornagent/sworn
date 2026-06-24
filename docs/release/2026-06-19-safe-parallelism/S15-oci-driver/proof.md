---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by the implementer after implementation.
---

# Proof Bundle: `S15-oci-driver`

## Scope

Implement the OCI Generative AI driver (`internal/model/oci.go`) using the
`oci-go-sdk/v65` `generativeaiinference` client. Register `oci/*` prefix in
provider routing so `sworn run --model oci/cohere.command-r-plus` dispatches
to OCI Generative AI Chat endpoint.

## Files changed

```
go.mod
go.sum
internal/model/config.go
internal/model/oci.go
internal/model/oci_test.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### `go test ./internal/model/... -run OCI -v`

```
=== RUN   TestOCIVerify_ReturnsText
--- PASS: TestOCIVerify_ReturnsText (0.00s)
=== RUN   TestOCIVerify_MissingCompartment
--- PASS: TestOCIVerify_MissingCompartment (0.00s)
=== RUN   TestOCIVerify_MissingTokenCount
--- PASS: TestOCIVerify_MissingTokenCount (0.00s)
=== RUN   TestNewClient_OCIRouted
--- PASS: TestNewClient_OCIRouted (0.00s)
=== RUN   TestOCIVerify_MissingModelID
--- PASS: TestOCIVerify_MissingModelID (0.00s)
=== RUN   TestOCINew_DeferredCredentialLoading
    oci_test.go:152: OCI deferred-loading contract: NewOCI succeeded regardless of config state
--- PASS: TestOCINew_DeferredCredentialLoading (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	(cached)
```

### `go test ./internal/model/...`

All 100+ tests pass — no regressions in existing drivers (Anthropic, Azure,
Bedrock, Google, OAI, ProviderConfig).

### `go build ./...` and `go vet ./...`

Clean — no warnings, no errors.

## Reachability artefact

- **Unit tests (offline):** `go test ./internal/model/... -run OCI` — 6 tests
  covering mock Chat response, missing compartment ID, nil usage, NewClient
  routing to `*OCI`, missing model ID, and deferred credential loading. All
  PASS.
- **Live integration test:** Skipped — requires `OCI_COMPARTMENT_ID` +
  `SWORN_LIVE_TESTS=1` + valid `~/.oci/config`.
- **Smoke step:** `sworn run --model oci/cohere.command-r-plus` with real OCI
  credentials (requires `OCI_COMPARTMENT_ID` and valid `~/.oci/config`).

## Delivered

- [x] `go build ./...` succeeds with `github.com/oracle/oci-go-sdk/v65` in
  go.mod
- [x] `NewOCI("cohere.command-r-plus", compartmentID)` returns non-nil `*OCI`
  with no error (credential loading deferred to first API call) —
  `TestOCINew_DeferredCredentialLoading`
- [x] `model.NewClient("oci/cohere.command-r-plus", cfg)` returns non-nil
  Verifier — `TestNewClient_OCIRouted`
- [x] `Verify()` with a mock OCI transport returns the first text content from
  the ChatResult — `TestOCIVerify_ReturnsText`
- [x] `cfg.OCICompartmentID` empty and `$OCI_COMPARTMENT_ID` absent → Verify
  returns a non-nil error naming the missing compartment ID —
  `TestOCIVerify_MissingCompartment`
- [x] `go test ./internal/model/... -run OCI` passes with zero failures (no
  live OCI key) — 6/6 PASS
- [x] All prior model tests still pass — full `go test ./internal/model/...`
  PASS

## Not delivered

- Instance principal / resource principal auth: deferred post-R3 (Why: requires
  OCI SDK config provider switching; adds complexity without covering the
  primary enterprise use case of CLI tool users with `~/.oci/config`. Tracking:
  post-R3 issue. Acknowledged: 2026-06-20 planning session.)
- OCI Generative AI streaming, embeddings, custom model deployments: out of
  scope per spec.

## Divergence from plan

- **$OCI_REGION → OCI_CLI_REGION (Coach-acked).** Spec In Scope line 34 names
  `$OCI_REGION` as the env var for OCI region. The OCI SDK natively honours
  `OCI_CLI_REGION` and config-file region. Per Captain pin 3 escalated to Coach
  and Coach ack (decision D5: "region from SDK"), the driver defers entirely to
  the OCI SDK's region discovery (`DefaultConfigProvider()` → config file /
  `OCI_CLI_REGION`). No separate `$OCI_REGION` parsing is done. The spec will be
  amended via `/replan-release` to reflect the SDK-native mechanism.

## First-pass script output

*(Populated after final first-pass run — see below.)*

## OCI config prerequisites

Per spec Risks #2 and #3:

- **OCI config file format:** `~/.oci/config` requires a `[DEFAULT]` section
  with `user`, `fingerprint`, `key_file`, `tenancy`, `region`. Missing any field
  causes auth errors at Verify time.
- **Region availability:** OCI Generative AI is not available in all OCI
  regions. As of mid-2026, supported regions include `us-chicago-1`,
  `eu-frankfurt-1`, and others. The user must specify a region (via
  `~/.oci/config` or `OCI_CLI_REGION` env var) that has the Generative AI
  service enabled.