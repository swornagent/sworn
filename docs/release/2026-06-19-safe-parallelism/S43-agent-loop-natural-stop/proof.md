# Proof Bundle: `S43-agent-loop-natural-stop`

## Scope

Terminate the agent loop on the model's natural stop (empty content + no tool calls) instead of spinning to `MaxTurns`, so completed work flows to verification instead of being discarded by a turn-cap error.

## Files changed

```
$ git diff --name-only d4f6729
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/journal.md
docs/release/2026-06-19-safe-parallelism/S43-agent-loop-natural-stop/status.json
internal/agent/agent.go
internal/agent/agent_test.go
internal/implement/implement.go
```

## Test results

### Agent + implement tests

```
$ go test -race -count=1 -v ./internal/agent/... -run 'TestRunReturnsOnEmptyStopAfterToolCalls|TestRunStillCapsOnEndlessToolCalls|TestRun_SuccessPath|TestRun_TurnCap'
=== RUN   TestRunReturnsOnEmptyStopAfterToolCalls
--- PASS: TestRunReturnsOnEmptyStopAfterToolCalls (0.00s)
=== RUN   TestRunStillCapsOnEndlessToolCalls
--- PASS: TestRunStillCapsOnEndlessToolCalls (0.01s)
=== RUN   TestRun_SuccessPath
--- PASS: TestRun_SuccessPath (0.00s)
=== RUN   TestRun_TurnCap
--- PASS: TestRun_TurnCap (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/agent	1.025s

$ go test -race -count=1 ./internal/agent/... ./internal/implement/...
ok  	github.com/swornagent/sworn/internal/agent	1.033s
ok  	github.com/swornagent/sworn/internal/implement	1.270s

$ go vet ./...
$ go build ./...
```

### Full package test runs

```
$ go test -race -count=1 ./internal/agent/...
ok  	github.com/swornagent/sworn/internal/agent	1.032s

$ go test -race -count=1 ./internal/implement/...
ok  	github.com/swornagent/sworn/internal/implement	1.208s
```

## Reachability artefact

- **Type**: integration-test-run
- **Path**: `internal/agent/agent_test.go`
- **User gesture**: `go test -v ./internal/agent/...` drives the real `agent.Run` loop with a fake `Agent` implementation and asserts that the empty-content natural stop returns cleanly while the endless-tool-calls case still hits `MaxTurns`.

## Delivered

- `agent.Run` now returns when `len(msg.ToolCalls) == 0` regardless of `msg.Content`, preserving completed tool work rather than spinning to `MaxTurns` — evidence: `internal/agent/agent.go` lines 110–125, `TestRunReturnsOnEmptyStopAfterToolCalls`
- `MaxTurns` remains the upper bound for non-terminating tool-call loops — evidence: `TestRunStillCapsOnEndlessToolCalls` and existing `TestRun_TurnCap` still pass
- Happy path (text + no tool calls) unchanged — evidence: `TestRun_SuccessPath` still passes
- `implement.Run` documented as tolerating empty agent prose because proof.md is built from `git diff` + test output — evidence: `internal/implement/implement.go` lines 84–87
- Tests added at the integration point (through `agent.Run`) — evidence: `internal/agent/agent_test.go` `TestRunReturnsOnEmptyStopAfterToolCalls`, `TestRunStillCapsOnEndlessToolCalls`

## Not delivered

None.

## Divergence from plan

None.

## Design trade-off captured

A model that returns empty content *before* doing useful work will now terminate early with a thin or empty diff. This is acceptable because downstream `verify.Run` evaluates the actual diff and tests; an empty diff will FAIL and the escalation loop advances. The prior behavior discarded potentially good work and forced a blind escalation, so the new behavior is strictly better for the common case where the model did the work and then stopped silently.
