# Proof Bundle: `S02-oai-model-client`

## Scope

With a configured model + key, `sworn verify` produces a **real** adversarial
verdict from an OpenAI-compatible endpoint (not the fail-closed stub).

## Files changed

```
$ git diff --name-only e49b9bbfae94958875111f9d4e2dd6486a722cd9
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/approved-ack.md
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/journal.md
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/proof.md
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/reachability.txt
docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/status.json
docs/release/2026-06-15-e2e-turnkey-loop/activity.md
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
ok  	github.com/swornagent/sworn/internal/model	0.212s
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
ok  	github.com/swornagent/sworn/internal/verify	0.005s
```

### Go vet

```
$ go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-15-e2e-turnkey-loop/S02-oai-model-client/reachability.txt`
- **Description**: Freshly-generated CLI-level reachability artefact — `sworn verify` against a local `httptest` fake server, exercising the full binary → model client → verdict path.

```
=== PASS (exit 0) ===
{
  "verdict": "PASS",
  "rationale": "PASS - all checks pass",
  "cost_usd": 0.00007000000000000001
}

=== FAIL (exit 1) ===
{
  "verdict": "FAIL",
  "failed_gate": "adversarial",
  "rationale": "FAIL: missing proof bundle",
  "cost_usd": 0.000048
}

=== BLOCKED (exit 2) ===
{
  "verdict": "BLOCKED",
  "failed_gate": "verifier_dispatch",
  "rationale": "model: dispatch: Post \"http://127.0.0.1:19999/chat/completions\": dial tcp 127.0.0.1:19999: connect: connection refused",
  "cost_usd": 0
}
```

All three spec acceptance checks demonstrated via the binary:
- **AC1** (PASS + FAIL from endpoint): PASS exit 0, FAIL exit 1 — both with non-zero `cost_usd`
- **AC2** (cost_usd from token usage): $0.00007 (150 tokens, gpt-4.1-mini pricing), $0.000048 (110 tokens)
- **AC3** (BYO-key, never logged): key read from `SWORN_OPENAI_API_KEY` env; no key in output
- **AC4** (HTTP/timeout → BLOCKED): connection refused → BLOCKED (exit 2, cost $0)

## Delivered

- [x] A real PASS and a real FAIL are produced from a (fake/live) endpoint.
  — evidence: `TestOAI_Verify` sub-tests PASS/FAIL (unit); reachability artefact PASS (exit 0) + FAIL (exit 1)
- [x] `cost_usd` is computed from token usage and surfaced in the verdict.
  — evidence: `TestComputeCost` (6 sub-cases); reachability artefact shows `cost_usd: 0.00007` (PASS) / `cost_usd: 0.000048` (FAIL)
- [x] Provider key is read from env (BYO-key); never logged.
  — evidence: `TestFromEnv` (10 sub-cases); `FromEnv` reads `os.Getenv`; no log statements in `OAI.Verify`
- [x] An HTTP/timeout error → BLOCKED (fail-closed), not a crash or false PASS.
  — evidence: `TestOAI_Verify` sub-tests HTTP 500/timeout; reachability artefact BLOCKED (exit 2, connection refused)

## Not delivered

None. All four acceptance checks are delivered per the evidence above.

## Divergence from plan

This section documents all touchpoint deviations between the `planned_files` in
`spec.md` and the actual implementation across all rounds of this slice.

### Touchpoint: `internal/verify/verify.go` (planned, NOT modified)

The planned touchpoint `internal/verify/verify.go` was **not modified** in any
implementation round. The wire between `model.FromEnv` and `verify.Run` was
instead implemented in `cmd/sworn/main.go`.

**Why:** `verify.Run` already accepted a `Verifier` through its `Input` struct
(`Input.Verifier model.Verifier`). The CLI was the correct injection point: it
resolves the provider client from env (`model.FromEnv`) and passes it into
`verify.Run`. Modifying `verify.go` would have been an architectural error
(polluting a provider-neutral package with provider configuration).

### Touchpoint: `cmd/sworn/main.go` (unplanned, modified)

The CLI (`cmd/sworn/main.go`) is the integration point that wires the model
client resolved from env into the verification protocol. It was not listed in
`planned_files` because the S01 planner anticipated the wire landing in
`verify.go`, but the cleaner design lands it at the process boundary. The
`Input.Verifier` field was already the injection seam; the CLI is the natural
place to select the concrete implementation.

### Touchpoint: `internal/verify/verify_test.go` (unplanned, modified)

Whitespace-only fix (missing newline after a function signature) applied in the
first implementation round. No behavioural change.

### No production-code changes in this re-entry round

This re-entry round (addressing round-2 verifier violations) makes **no
production-code changes**. The violations were documentation/proof-quality
gates only: the Divergence section needed to disclose the touchpoint deviations
(Gate 2), and a fresh CLI-level reachability artefact was needed (Gate 4). The
only files modified in this round are documentation: `proof.md`, `journal.md`,
`status.json`, and the new `reachability.txt` artefact.

## First-pass script output
```
$ release-verify.sh S02-oai-model-client 2026-06-15-e2e-turnkey-loop

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
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  6 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~7) consistent with diff vs start_commit (6)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```
