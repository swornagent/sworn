---
title: Slice proof bundle
description: Rule 6 proof bundle for S13-bedrock-driver. Generated from live repo state.
---

# Proof Bundle: `S13-bedrock-driver`

## Scope

A developer with AWS credentials (env vars or `~/.aws/credentials`) sets `verifier.model = "bedrock/anthropic.claude-sonnet-4-5"` in config.json; `sworn run` dispatches to AWS Bedrock Converse API and returns a PASS/FAIL verdict. No Anthropic API key is needed — the call authenticates via AWS IAM.

## Files changed

```
$ git diff --name-only 91f7768873a5b0acdb2686e00c2eab302cec3277
docs/release/2026-06-19-safe-parallelism/S13-bedrock-driver/status.json
go.mod
go.sum
internal/model/bedrock.go
internal/model/bedrock_test.go
internal/model/config.go
internal/model/provider.go
internal/model/provider_test.go
```

## Test results

### Go — Bedrock-specific tests

```
$ go test ./internal/model/... -run Bedrock -v
=== RUN   TestBedrockVerify_ReturnsText
--- PASS: TestBedrockVerify_ReturnsText (0.00s)
=== RUN   TestBedrockVerify_APIError
--- PASS: TestBedrockVerify_APIError (0.00s)
=== RUN   TestBedrockVerify_AuthError
--- PASS: TestBedrockVerify_AuthError (0.00s)
=== RUN   TestBedrockRegionResolution_ExplicitRegion
--- PASS: TestBedrockRegionResolution_ExplicitRegion (0.00s)
=== RUN   TestBedrockRegionResolution_EnvVar
--- PASS: TestBedrockRegionResolution_EnvVar (0.00s)
=== RUN   TestBedrockRegionResolution_DefaultEnvVar
--- PASS: TestBedrockRegionResolution_DefaultEnvVar (0.00s)
=== RUN   TestBedrockRegionResolution_Fallback
--- PASS: TestBedrockRegionResolution_Fallback (0.00s)
=== RUN   TestNewClient_BedrockRouted
--- PASS: TestNewClient_BedrockRouted (0.00s)
=== RUN   TestBedrockVerify_UnknownModelCostIsZero
--- PASS: TestBedrockVerify_UnknownModelCostIsZero (0.00s)
=== RUN   TestBedrockVerify_NonHTTPErrorIsTransient
--- PASS: TestBedrockVerify_NonHTTPErrorIsTransient (0.00s)
=== RUN   TestBedrockVerify_Live
    bedrock_test.go:253: live test requires SWORN_LIVE_TESTS=1 and AWS_ACCESS_KEY_ID
--- SKIP: TestBedrockVerify_Live (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.014s
```

### Go — Full model test suite

```
$ go test ./internal/model/...
ok  	github.com/swornagent/sworn/internal/model	1.647s	coverage: 75.6% of statements
```

All prior model tests pass (Anthropic, Google, OAI, errors, env, provider — no regressions).

### Go build

```
$ go build ./...
(exit 0, no output)
```

### Go vet

```
$ go vet ./internal/model/...
(exit 0, no output)
```

### Dependency audit (spec Risk #1)

All added dependencies are `github.com/aws/aws-sdk-go-v2/*` or `github.com/aws/smithy-go` — 16 AWS-internal packages, zero unexpected packages.

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `internal/model/bedrock_test.go` — `TestNewClient_BedrockRouted`
- **User gesture**: `sworn run` dispatches `model.NewClient("bedrock/amazon.nova-pro-v1:0", cfg)` → returns `*Bedrock` — the `bedrock/*` prefix routes through the full `NewClient` dispatch chain

Additional reachability via mocked Converse API:
- `TestBedrockVerify_ReturnsText` exercises the full `Verify()` path through `httptest.Server` + `BaseEndpoint` override — real SDK serialisation path, zero external network calls
- `TestBedrockVerify_Live` (skipped by default) provides end-to-end reachability when `SWORN_LIVE_TESTS=1` and `AWS_ACCESS_KEY_ID` are set

## Delivered

- [x] `go build ./...` succeeds with aws-sdk-go-v2 packages in go.mod — evidence: `go build ./...` exit 0
- [x] `NewBedrock("anthropic.claude-sonnet-4-5", "us-east-1")` returns non-nil `*Bedrock` with no error (credential loading deferred to first API call) — evidence: `TestBedrockRegionResolution_ExplicitRegion` (PASS)
- [x] `model.NewClient("bedrock/amazon.nova-pro-v1:0", cfg)` returns non-nil Verifier — evidence: `TestNewClient_BedrockRouted` (PASS)
- [x] `Verify()` with a mock Bedrock transport returns the first text block from the Converse response output — evidence: `TestBedrockVerify_ReturnsText` (PASS)
- [x] Region falls back to `us-east-1` when no region is set in env or cfg — evidence: `TestBedrockRegionResolution_Fallback` (PASS)
- [x] `go test ./internal/model/... -run Bedrock` passes with zero failures (no AWS creds) — evidence: 10 PASS, 1 SKIP, 0 FAIL
- [x] All prior model tests still pass — evidence: full `go test ./internal/model/...` PASS

## Not delivered

- Live AWS integration test (`TestBedrockVerify_Live`) — **Why**: requires real AWS credentials (`AWS_ACCESS_KEY_ID` + `SWORN_LIVE_TESTS=1`); per spec "Deferrals allowed?" section this is acceptable in CI without AWS creds. **Tracking**: S13-bedrock-driver spec §Deferrals allowed. **Acknowledged**: Brad, design-review (2026-07-09).
- IAM permissions documentation — **Why**: documented below per spec Risk #3. **Tracking**: this proof.md §IAM permissions.

### IAM permissions (spec Risk #3)

The caller needs one of:
- `bedrock:InvokeModel` (for the Converse API)
- `bedrock:Converse`

Minimal IAM policy:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["bedrock:InvokeModel", "bedrock:Converse"],
      "Resource": "*"
    }
  ]
}
```

## Divergence from plan

- **`internal/model/config.go` added to planned_files** (Coach Pin 1): `FromEnv()` now has `case "bedrock": key = "iam"` to bypass the API key gate, and `swornProviderConfig()` / `ProviderConfigFromEnv()` include `AwsRegion` from `AWS_REGION` → `AWS_DEFAULT_REGION`.
- **`internal/model/provider.go` added `AwsRegion` to `ProviderConfig`** (Coach Pin 3): region flows through the config struct rather than only through the `NewBedrock` parameter. This is correct per design §3.
- **Pricing table keys use `anthropic.` prefix** (Coach Pin 2): model IDs arrive from `parseModelID` with the `anthropic.` prefix (e.g. `anthropic.claude-sonnet-4-6`), so pricing table keys match.
- **Added `bedrock.go` and `bedrock_test.go`** as planned.

## First-pass script output

```
$ /home/user/.claude/bin/release-verify.sh S13-bedrock-driver 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 91f7768873a5b0acdb2686e00c2eab302cec3277
  PASS  10 file(s) changed vs diff base
    docs/release/2026-06-19-safe-parallelism/S13-bedrock-driver/journal.md
    docs/release/2026-06-19-safe-parallelism/S13-bedrock-driver/proof.md
    docs/release/2026-06-19-safe-parallelism/S13-bedrock-driver/status.json
    go.mod
    go.sum
    internal/model/bedrock.go
    internal/model/bedrock_test.go
    internal/model/config.go
    internal/model/provider.go
    internal/model/provider_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~8) consistent with diff vs start_commit (10)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0
FIRST-PASS PASS
```

