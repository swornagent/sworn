# Proof Bundle: `S03-agentic-tool-loop`

## Scope

The engine can drive a model through a tool loop (read/write/edit files, run commands, grep/glob) to perform multi-step work within a workspace.

## Files changed

```
$ git diff --name-only ae8d37959c199efdb08230e272ee7e8ae2605c0d..HEAD
docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/status.json
internal/agent/agent.go
internal/agent/agent_test.go
internal/agent/tools.go
internal/model/oai.go
internal/model/oai_test.go
```

## Test results

### Go

```
$ go test ./internal/agent/ ./internal/model/ -v
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
ok  	github.com/swornagent/sworn/internal/agent	0.010s
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
... (all pass)
=== RUN   TestFromEnv
... (all pass)
PASS
ok  	github.com/swornagent/sworn/internal/model	0.213s
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

- `docs/release/2026-06-15-e2e-turnkey-loop/S03-agentic-tool-loop/status.json` — metadata update (start_commit, state transition). Not production code.
- `internal/model/oai_test.go` — updated test helper to match extended ChatResponse struct (FinishReason field). Required by Pin 6 regression verification.