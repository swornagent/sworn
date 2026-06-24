# Design TL;DR — S13-bedrock-driver

## §1. User-visible change

A developer sets `verifier.model = "bedrock/anthropic.claude-sonnet-4-5"` in config.json; `sworn run` dispatches to AWS Bedrock Converse API using IAM credentials (env vars, `~/.aws/credentials`, or IAM role) — no separate Anthropic API key needed. The driver returns a PASS/FAIL verdict with per-model cost from Bedrock's pricing page.

## §2. Design decisions not in spec (max 5)

1. **Mock strategy: `httptest.Server` + endpoint override.** The `bedrockruntime.Client` is a concrete struct, not an interface. Rather than wrapping it in an abstraction, tests point the client at an `httptest.Server` via `aws.EndpointResolverV2`. This exercises the real SDK serialisation path — the same shape as the Anthropic driver's mock pattern.
2. **Region resolution: `NewBedrock(modelID, region string)` with explicit parameter.** The spec says `cfg.BedrockRegion → env → default`. Since `NewBedrock` is called from `NewClient` (provider.go switch), the region routing lives in `NewBedrock` itself: if the `region` arg is empty, fall through `AWS_REGION` → `AWS_DEFAULT_REGION` → `"us-east-1"`. No `config.json` struct change needed — Bedrock is the only AWS-native driver currently; the region flows through the function parameter, not a global config field.
3. **Pricing table: 6 models (Claude on Bedrock + Nova).** Bedrock pricing differs from Anthropic direct API pricing because AWS marks up the infrastructure. Table sourced from [AWS Bedrock pricing page](https://aws.amazon.com/bedrock/pricing/): `claude-opus-4-8` ($15/$75), `claude-sonnet-4-6` ($3/$15), `claude-haiku-4-5` ($1/$5), `claude-sonnet-4` ($3/$15), `amazon.nova-pro-v1:0` ($0.80/$3.20), `amazon.nova-lite-v1:0` ($0.06/$0.24). Unknown models get zero cost (same posture as OAI/Anthropic/Google).
4. **Converse API types: direct use of bedrockruntime types.** The spec mandates using `types.SystemContentBlockMemberText{Value: systemPrompt}` and `types.ContentBlockMemberText{Value: text}` — no Anthropic SDK types in this file. The import segregation is clean: `bedrock.go` imports only `aws-sdk-go-v2/*` packages and `internal/model` for `Error`/`Verifier` taxonomy.
5. **Error classification: parse `smithy.APIError` for status code.** The aws-sdk-go-v2 wraps HTTP errors in `smithy.APIError` (interface with `StatusCode()`). We extract the HTTP status and route through `NewProviderError` — same taxonomy as Anthropic/Google drivers, consistent `IsTerminal`/`IsTransient` behaviour.

## §3. Files I'll touch grouped by purpose

- **New driver:** `internal/model/bedrock.go` — `Bedrock` struct, `NewBedrock()`, `Verify()`, cost computation, region resolution
- **Unit tests:** `internal/model/bedrock_test.go` — 4 test cases (returns text, API error, region resolution, routing)
- **Provider registration:** `internal/model/provider.go` — replace `bedrock` case from `ErrDriverNotRegistered` to `NewBedrock(model, region)` call; add `AwsRegion` to `ProviderConfig` (used by `swornProviderConfig` too)
- **Dependencies:** `go.mod`, `go.sum` — add `aws-sdk-go-v2/config` + `aws-sdk-go-v2/service/bedrockruntime`

## §4. Things I'm NOT doing

- **No `internal/model/config.go` struct change** for a `BedrockRegion` field — region flows through the `NewBedrock` parameter. If a future slice needs a config-level default, it can add the field then.
- **No live AWS integration test in CI** — the reachability artefact test is skipped unless `AWS_ACCESS_KEY_ID` + `SWORN_LIVE_TESTS=1` are set (spec-allowed deferral).
- **No `bedrockruntime` interface abstraction layer** — the mock strategy (httptest server) doesn't need one. Adding a `BedrockConverseAPI` interface would be premature abstraction.

## §5. Reachability plan

1. **Unit test reachability:** `go test ./internal/model/... -run Bedrock` — exercises all 4 test cases against mocked Bedrock Converse endpoint.
2. **Provider routing reachability:** `TestNewClient_BedrockRouted` — `model.NewClient("bedrock/amazon.nova-pro-v1:0", cfg)` returns `*Bedrock`, proving the `bedrock/*` prefix routes through the full `NewClient` dispatch.
3. **Live integration test (skipped gate):** `TestBedrockVerify_Live` — skipped by default; when run with real AWS creds, calls Bedrock Converse API with "Reply with PASS." and asserts PASS returned.

## §6. Open questions for the Coach

None.