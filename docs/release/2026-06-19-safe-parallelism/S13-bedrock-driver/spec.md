---
title: 'S13-bedrock-driver — AWS Bedrock Converse API driver'
description: 'Implements model.Verifier for AWS Bedrock models via the aws-sdk-go-v2 bedrockruntime package using the Converse API. Registers bedrock/* prefix in the provider router.'
---

# Slice: `S13-bedrock-driver`

## User outcome

A developer with AWS credentials (env vars or `~/.aws/credentials`) sets
`verifier.model = "bedrock/anthropic.claude-sonnet-4-5"` in config.json; `sworn run`
dispatches to AWS Bedrock Converse API and returns a PASS/FAIL verdict. No Anthropic API
key is needed — the call authenticates via AWS IAM.

## Entry point

`sworn run` → `model.NewClient("bedrock/anthropic.claude-sonnet-4-5", cfg)` →
`*Bedrock` driver → `Verify()` call to Bedrock Converse API.

## In scope

- `internal/model/bedrock.go`:
  - `type Bedrock struct` with fields: `Client *bedrockruntime.Client`, `ModelID string`,
    `Region string`
  - `NewBedrock(modelID, region string) (*Bedrock, error)` — loads AWS config via
    `aws/config.LoadDefaultConfig(ctx, config.WithRegion(region))`; constructs
    `bedrockruntime.NewFromConfig(cfg)`; uses standard AWS credential chain
    (env vars → `~/.aws/credentials` → IAM role)
  - `Verify(ctx, systemPrompt, userPayload string) (string, float64, error)` — calls
    `client.Converse(ctx, &bedrockruntime.ConverseInput{ModelId: modelID, System:
    []types.SystemContentBlock{...}, Messages: []types.Message{...}})`;
    extracts text from the response's `Output.Message.Content` first text block
  - Region resolution: `cfg.BedrockRegion` → `$AWS_REGION` → `$AWS_DEFAULT_REGION` →
    `"us-east-1"` (Bedrock is currently US-only for most model families)
  - Cost: Bedrock reports `InputTokens`/`OutputTokens` in `ConverseOutput.Usage`;
    use per-model pricing from the Bedrock pricing page; add to pricing table
- `internal/model/bedrock_test.go`
- `internal/model/provider.go` update: register `bedrock/*` → `NewBedrock()`
- `go.mod`: add `github.com/aws/aws-sdk-go-v2/config`,
  `github.com/aws/aws-sdk-go-v2/service/bedrockruntime`

## Out of scope

- Bedrock streaming (InvokeModelWithResponseStream) — sworn uses single-shot calls
- Bedrock Agents (separate API surface)
- Bedrock model inference profiles
- Cross-region inference endpoints
- ProvisionedThroughput ARNs (use model IDs directly)

## Planned touchpoints

- `internal/model/bedrock.go` (new)
- `internal/model/bedrock_test.go` (new)
- `internal/model/provider.go` (modify — register bedrock/* prefix)
- `go.mod`, `go.sum` (modify — add aws-sdk-go-v2 packages)

## Acceptance checks

- [ ] `go build ./...` succeeds with aws-sdk-go-v2 packages in go.mod
- [ ] `NewBedrock("anthropic.claude-sonnet-4-5", "us-east-1")` returns non-nil `*Bedrock`
  with no error (credential loading deferred to first API call)
- [ ] `model.NewClient("bedrock/anthropic.claude-sonnet-4-5", cfg)` returns non-nil Verifier
- [ ] `Verify()` with a mock Bedrock transport returns the first text block from the
  Converse response output
- [ ] Region falls back to `us-east-1` when no region is set in env or cfg
- [ ] `go test ./internal/model/... -run Bedrock` passes with zero failures (no AWS creds)
- [ ] All prior model tests still pass

## Required tests

- **Unit** `internal/model/bedrock_test.go`:
  - `TestBedrockVerify_ReturnsText`: mock Converse response; assert Verify returns text
    block and non-negative cost
  - `TestBedrockVerify_APIError`: mock error; assert non-nil error returned
  - `TestBedrockRegionResolution`: cfg.BedrockRegion, env var, and default fallback cases
  - `TestNewClient_BedrockRouted`: `model.NewClient("bedrock/amazon.nova-pro-v1:0", cfg)`
    returns `*Bedrock`
- **Reachability artefact**: live integration test (skipped unless `AWS_ACCESS_KEY_ID`
  and `SWORN_LIVE_TESTS=1`): call Verify with "Reply with PASS."; assert "PASS" returned.

## Risks

- AWS SDK v2 brings in a non-trivial transitive dep tree. Run `go mod tidy` and check
  for unexpected packages. Accept if all are AWS-internal.
- The Bedrock Converse API uses `types.SystemContentBlock` (a union type) which differs
  from the Anthropic SDK's `ContentBlock`. The implementer must use the bedrockruntime
  types directly, not the Anthropic SDK types.
- IAM permissions: the caller needs `bedrock:InvokeModel` or `bedrock:Converse`. This is
  an operational concern, not a code concern, but must be documented in `proof.md`.

## Deferrals allowed?

Live AWS integration test may be skipped in CI without `AWS_ACCESS_KEY_ID`. Acceptable.
