---
title: S03-agentic-tool-loop proof
description: Rule 6 proof bundle, generated from live repo state. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S03-agentic-tool-loop`

## Scope

The engine can drive a model through a tool loop (read/write/edit files, run commands, grep/glob) to perform multi-step work within a workspace.

## Files changed

```
$ git diff --name-only ae8d37959c199efdb08230e272ee7e8ae2605c0d..HEAD
docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/approved-ack.md
docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/journal.md
docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/proof.md
docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/status.json
docs/release/2026-06-15-e2e-turnkey-loop/activity.md
internal/agent/agent.go
internal/agent/agent_test.go
internal/agent/tools.go
internal/model/oai.go
internal/model/oai_test.go
```

## Test results

### Go

```
$ go test ./internal/agent/ -v
=== RUN   TestRun_SuccessPath
--- PASS: TestRun_SuccessPath (0.00s)
=== RUN   TestRun_ToolError_ModelAdapts
--- PASS: TestRun_ToolError_ModelAdapts (0.00s)
=== RUN   TestRun_TurnCap
--- PASS: TestRun_TurnCap (0.01s)
=== RUN   TestRun_WorkspaceConfinement
--- PASS: TestRun_WorkspaceConfinement (0.00s)
=== RUN   TestRun_PathTraversalRejected
--- PASS: TestRun_PathTraversalRejected (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/agent	0.015s
```

```
$ go test ./internal/model/ -v
=== RUN   TestOAI_Verify
=== RUN   TestOAI_Verify/PASS
=== RUN   TestOAI_Verify/FAIL
=== RUN   TestOAI_Verify/HTTP_500
=== RUN   TestOAI_Verify/timeout
--- PASS: TestOAI_Verify (0.20s)
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
PASS
ok  	github.com/swornagent/sworn/internal/model	0.210s
```

```
$ go vet ./...
(no output — clean)
```

## Reachability artefact

- **Type**: `manual-smoke-step` (no UI — internal Go package; tested via integration test at the agent boundary)
- **Path**: `internal/agent/agent_test.go`
- **User gesture**: `go test ./internal/agent/ -run TestRun_SuccessPath -v` — exercises `agent.Run()` with a FakeAgent scripting Write → Bash → text termination. Asserts file written, tool output in history, loop terminates.
- **Error-path gesture**: `go test ./internal/agent/ -run TestRun_ToolError_ModelAdapts -v` — exercises tool error → model receives error → model adapts → loop terminates.

## Delivered

- **AC1: Given a task, the loop performs ≥1 file edit and ≥1 command, then terminates.** — evidence: `TestRun_SuccessPath` in `internal/agent/agent_test.go` — Write tool creates hello.txt, Bash tool reads it, loop terminates with text response.
- **AC2: Tool errors are returned to the model (not fatal); the loop continues.** — evidence: `TestRun_ToolError_ModelAdapts` in `internal/agent/agent_test.go` — Read tool on missing file returns error, model adapts by writing file, loop terminates successfully.
- **AC3: All file/command operations are confined to the workspace root.** — evidence: `TestRun_WorkspaceConfinement` and `TestRun_PathTraversalRejected` in `internal/agent/agent_test.go` — absolute paths and `../` traversal are rejected with descriptive errors.
- **AC4: The turn cap halts a non-terminating loop deterministically.** — evidence: `TestRun_TurnCap` in `internal/agent/agent_test.go` — MaxTurns=3 with non-terminating script returns turn-cap error at exactly 3 turns.

## Not delivered

N/A — all four acceptance checks are delivered.

## Divergence from plan

- `internal/model/oai.go` — 115+ lines of new production code (exported types: ChatMessage, ChatResponse, ToolCall, FunctionCall, UsageBlock, ToolDef; `Chat()` method). Required by the agent package to perform multi-turn tool-loop conversations. The OAI client in S02 was single-shot (verifier); the agent needs the Chat endpoint and structured tool-call responses. Extending the existing client rather than creating a separate one keeps one HTTP client and one pricing table.
- `internal/model/oai_test.go` — updated test helper to match extended ChatResponse struct (FinishReason field). Required by Pin 6 regression verification.
- `docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/status.json` — metadata update (start_commit, state transitions, verification verdict records). Not production code.
- `docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/approved-ack.md` — design-review token from Captain (transient). Not production code.
- `docs/release/2026-06-15-e2e-turnkey-loop/activity.md` — board-level activity log. Not S03 production scope.

## First-pass script output

```
$ release-verify.sh S03-agentic-tool-loop 2026-06-15-e2e-turnkey-loop
  slice:       S03-agentic-tool-loop
  slice dir:   docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  10 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~6) consistent with diff vs start_commit (10)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 21
  checks failed: 1
FIRST-PASS FAIL (state in_progress — expected; transitions to implemented next)
```