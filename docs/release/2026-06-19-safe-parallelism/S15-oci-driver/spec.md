---
title: 'S15-oci-driver — OCI Generative AI driver'
description: 'Implements model.Verifier for Oracle Cloud Infrastructure Generative AI via the official oci-go-sdk. Registers oci/* prefix in the provider router.'
---

# Slice: `S15-oci-driver`

## User outcome

A developer with OCI credentials configured (`~/.oci/config` or OCI env vars) sets
`verifier.model = "oci/cohere.command-r-plus"` in config.json; `sworn run` dispatches to
the OCI Generative AI service and returns a PASS/FAIL verdict.

## Entry point

`sworn run` → `model.NewClient("oci/cohere.command-r-plus", cfg)` → `*OCI` driver →
`Verify()` call to OCI Generative AI ChatResult endpoint.

## In scope

- `internal/model/oci.go`:
  - `type OCI struct` with fields: `Client generativeaiinference.GenerativeAiInferenceClient`,
    `ModelID string`, `CompartmentID string`
  - `NewOCI(modelID, compartmentID string) (*OCI, error)` — creates OCI config from
    environment (OCI SDK reads `~/.oci/config` by default, or from `OCI_*` env vars);
    constructs `generativeaiinference.NewGenerativeAiInferenceClientWithConfigurationProvider()`
  - `Verify(ctx, systemPrompt, userPayload string) (string, float64, error)` — calls
    `client.Chat(ctx, generativeaiinference.ChatRequest{CompartmentId: compartmentID,
    ChatDetails: generativeaiinference.GenericChatDetails{Messages: [...],
    ServingMode: generativeaiinference.OnDemandServingMode{ModelId: modelID}}})`;
    extracts text from `ChatResult.ChatResponse.Choices[0].Message.Content[0].Text`
  - CompartmentID resolution: `cfg.OCICompartmentID` → `$OCI_COMPARTMENT_ID` → error
    (compartment ID is mandatory for OCI API calls)
  - OCI region: read from OCI config file or `$OCI_REGION` env var (standard OCI SDK)
  - Cost: OCI does not always return token counts in the response; use 0.0 when absent
    rather than an error
- `internal/model/oci_test.go`
- `internal/model/provider.go` update: register `oci/*` → `NewOCI()` using
  `cfg.OCICompartmentID`
- `internal/model/provider.go`: add `OCICompartmentID` field to `ProviderConfig`
  (read from `$OCI_COMPARTMENT_ID`)
- `go.mod`: add `github.com/oracle/oci-go-sdk/v65` (generativeaiinference sub-package)

## Out of scope

- OCI Generative AI streaming
- OCI model deployment (custom fine-tuned models)
- OCI Generative AI embeddings
- Instance principal / resource principal auth (config file auth only in this slice)

## Planned touchpoints

- `internal/model/oci.go` (new)
- `internal/model/oci_test.go` (new)
- `internal/model/provider.go` (modify — register oci/* prefix, add OCI fields to
  ProviderConfig)
- `go.mod`, `go.sum` (modify — add oci-go-sdk)

## Acceptance checks

- [ ] `go build ./...` succeeds with `github.com/oracle/oci-go-sdk/v65` in go.mod
- [ ] `NewOCI("cohere.command-r-plus", compartmentID)` returns non-nil `*OCI` with no
  error (credential loading deferred to first API call)
- [ ] `model.NewClient("oci/cohere.command-r-plus", cfg)` returns non-nil Verifier
- [ ] `Verify()` with a mock OCI transport returns the first text content from the
  ChatResult
- [ ] `cfg.OCICompartmentID` empty and `$OCI_COMPARTMENT_ID` absent → Verify returns
  a non-nil error naming the missing compartment ID
- [ ] `go test ./internal/model/... -run OCI` passes with zero failures (no live OCI key)
- [ ] All prior model tests still pass

## Required tests

- **Unit** `internal/model/oci_test.go`:
  - `TestOCIVerify_ReturnsText`: mock Chat response with one Choice; assert Verify returns
    text content
  - `TestOCIVerify_MissingCompartment`: call with empty compartmentID; assert error
  - `TestOCIVerify_MissingTokenCount`: response with nil usage; assert cost = 0.0, no error
  - `TestNewClient_OCIRouted`: `model.NewClient("oci/cohere.command-r-plus", cfg)` returns
    `*OCI`
- **Reachability artefact**: live integration test (skipped unless `OCI_COMPARTMENT_ID`
  and `SWORN_LIVE_TESTS=1` and OCI config present): call Verify with "Reply with PASS.";
  assert "PASS" returned.

## Risks

- `oci-go-sdk` v65 is a large package (OCI has many services). Only the
  `generativeaiinference` sub-package is needed; import only that. Check `go mod tidy`
  output to ensure no unexpected transitive OCI packages are pulled in beyond auth and
  common.
- OCI config file format (`~/.oci/config`) requires `[DEFAULT]` section with `user`,
  `fingerprint`, `key_file`, `tenancy`, `region`. Missing any field causes auth errors.
  Document in proof.md as a setup prerequisite.
- OCI Generative AI is not available in all OCI regions. Document that the user must
  specify a region that has the service enabled (as of mid-2026: us-chicago-1,
  eu-frankfurt-1, and others).

## Deferrals allowed?

Instance principal / resource principal auth: deferred post-R3 (Why: requires OCI SDK
config provider switching; adds complexity without covering the primary enterprise use
case of CLI tool users with `~/.oci/config`. Tracking: post-R3 issue. Acknowledged:
2026-06-20 planning session).
