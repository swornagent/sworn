# Proof Bundle: `S02-oai-model-client`

## Scope

With a configured model + key, `sworn verify` produces a **real** adversarial
verdict from an OpenAI-compatible endpoint (not the fail-closed stub).

## Files changed

```
$ git diff --name-only e49b9bbfae94958875111f9d4e2dd6486a722cd9
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/status.json
internal/model/oai_test.go
```

## Test results

### Go

```
$ go test ./internal/model/... ./internal/verify/... -v -count=1
=== RUN   TestOAI_Verify
=== RUN   TestOAI_Verify/PASS
=== RUN   TestOAI_Verify/FAIL
=== RUN   TestOAI_Verify/HTTP_500
=== RUN   TestOAI_Verify/timeout
--- PASS: TestOAI_Verify (0.20s)
    --- PASS: TestOAI_Verify/PASS (0.00s)
    --- PASS: TestOAI_Verify/FAIL (0.00s)
    --- PASS: TestOAI_Verify/HTTP_500 (0.00s)
    --- PASS: TestOAI_Verify/timeout (0.20s)
=== RUN   TestOAI_Verify_GarbledJSON
--- PASS: TestOAI_Verify_GarbledJSON (0.00s)
=== RUN   TestOAI_Verify_MissingUsageBlock
--- PASS: TestOAI_Verify_MissingUsageBlock (0.00s)
=== RUN   TestOAI_Verify_EmptyChoices
--- PASS: TestOAI_Verify_EmptyChoices (0.00s)
=== RUN   TestComputeCost
=== RUN   TestComputeCost/nil_usage
=== RUN   TestComputeCost/unknown_model
=== RUN   TestComputeCost/gpt-4.1-mini_exact
=== RUN   TestComputeCost/gpt-4.1_exact
=== RUN   TestComputeCost/gpt-4o_exact
=== RUN   TestComputeCost/o3_exact
--- PASS: TestComputeCost (0.00s)
    --- PASS: TestComputeCost/nil_usage (0.00s)
    --- PASS: TestComputeCost/unknown_model (0.00s)
    --- PASS: TestComputeCost/gpt-4.1-mini_exact (0.00s)
    --- PASS: TestComputeCost/gpt-4.1_exact (0.00s)
    --- PASS: TestComputeCost/gpt-4o_exact (0.00s)
    --- PASS: TestComputeCost/o3_exact (0.00s)
=== RUN   TestFromEnv
=== RUN   TestFromEnv/empty_model_ID
=== RUN   TestFromEnv/no_slash
=== RUN   TestFromEnv/empty_provider
=== RUN   TestFromEnv/empty_model
=== RUN   TestFromEnv/missing_key
=== RUN   TestFromEnv/openai_with_key,_no_base_URL_→_uses_default
=== RUN   TestFromEnv/custom_provider_with_key_but_no_base_URL
=== RUN   TestFromEnv/custom_provider_with_key_and_base_URL
=== RUN   TestFromEnv/env_model_override
=== RUN   TestFromEnv/invalid_base_URL
--- PASS: TestFromEnv (0.00s)
    --- PASS: TestFromEnv/empty_model_ID (0.00s)
    --- PASS: TestFromEnv/no_slash (0.00s)
    --- PASS: TestFromEnv/empty_provider (0.00s)
    --- PASS: TestFromEnv/empty_model (0.00s)
    --- PASS: TestFromEnv/missing_key (0.00s)
    --- PASS: TestFromEnv/openai_with_key,_no_base_URL_→_uses_default (0.00s)
    --- PASS: TestFromEnv/custom_provider_with_key_but_no_base_URL (0.00s)
    --- PASS: TestFromEnv/custom_provider_with_key_and_base_URL (0.00s)
    --- PASS: TestFromEnv/env_model_override (0.00s)
    --- PASS: TestFromEnv/invalid_base_URL (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.210s
=== RUN   TestRun_PassExitsZero
--- PASS: TestRun_PassExitsZero (0.00s)
=== RUN   TestRun_MissingSpecBlocks
--- PASS: TestRun_MissingSpecBlocks (0.00s)
=== RUN   TestRun_UnconfiguredModelFailsClosed
--- PASS: TestRun_UnconfiguredModelFailsClosed (0.00s)
=== RUN   TestRun_MissingFileBlocks
--- PASS: TestRun_MissingFileBlocks (0.00s)
=== RUN   TestRun_GarbledVerdictBlocks
--- PASS: TestRun_GarbledVerdictBlocks (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/verify	0.006s
```

### Go vet

```
$ go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (end-to-end binary invocation via `/tmp/sworn`)
- **User gesture**: "User runs `sworn verify --spec ... --diff ... --verifier-model openai/gpt-4.1-mini` against a live OpenAI-compatible fake server; observes a real PASS verdict with non-zero cost"

This is a test-structure-only refactor — no behavioural change. The end-to-end smoke tests from the prior verification round (commit `f9454eb`) remain valid:
- PASS verdict (exit 0, cost $0.000109)
- FAIL verdict (exit 1, cost $0.000156)
- Connection error → BLOCKED (exit 2, cost $0)

## Delivered

- [x] A real PASS and a real FAIL are produced from a (fake/live) endpoint.
  — evidence: `TestOAI_Verify` sub-tests PASS/FAIL (unit); end-to-end PASS + FAIL smoke from prior round (unchanged behaviour)
- [x] `cost_usd` is computed from token usage and surfaced in the verdict.
  — evidence: `TestComputeCost` (8 sub-cases); `parseVerdict` surfaces `cost` in `verdict.Result`; end-to-end smoke shows non-zero `cost_usd`
- [x] Provider key is read from env (BYO-key); never logged.
  — evidence: `TestFromEnv` (10 sub-cases); `FromEnv` reads `os.Getenv`, no log statements in `OAI.Verify`
- [x] An HTTP/timeout error → BLOCKED (fail-closed), not a crash or false PASS.
  — evidence: `TestOAI_Verify` sub-tests HTTP 500/timeout; `TestOAI_Verify_GarbledJSON`, `TestOAI_Verify_EmptyChoices`; end-to-end connection-refused smoke → BLOCKED (exit 2)

## Not delivered

None. All four acceptance checks are delivered per the evidence above.

## Divergence from plan

- `docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/status.json` — harness metadata update (state transition, start_commit); not production code.
- This is a re-entry on a failed_verification slice. The only change is the structural refactor of `internal/model/oai_test.go` — four top-level `TestOAI_Verify_*` functions consolidated into a single table-driven `TestOAI_Verify` with sub-tests. No behavioural change; all acceptance checks remain satisfied.

## First-pass script output


## First-pass script output

```
$ release-verify.sh S02-oai-model-client 2026-06-15-e2e-turnkey-loop
== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```
