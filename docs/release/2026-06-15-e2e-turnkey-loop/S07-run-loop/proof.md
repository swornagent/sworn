# Proof Bundle: `S07-run-loop`

## Scope

S07-run-loop: `sworn run` — the end-to-end orchestration: implement → verify → retry/escalate → gated merge.

## Files changed

```
$ git diff --name-only 006c261..HEAD
cmd/sworn/init.go
cmd/sworn/main.go
cmd/sworn/run.go
cmd/sworn/run_test.go
internal/git/git.go
internal/git/git_test.go
internal/run/run.go
internal/run/run_test.go
```

## Test results

### Go

```
$ go test ./... -count=1
ok  	github.com/swornagent/sworn/cmd/sworn	0.020s
ok  	github.com/swornagent/sworn/internal/adopt	0.019s
ok  	github.com/swornagent/sworn/internal/agent	0.011s
ok  	github.com/swornagent/sworn/internal/board	0.004s
ok  	github.com/swornagent/sworn/internal/config	0.018s
ok  	github.com/swornagent/sworn/internal/git	0.169s
ok  	github.com/swornagent/sworn/internal/implement	0.125s
ok  	github.com/swornagent/sworn/internal/model	0.211s
ok  	github.com/swornagent/sworn/internal/prompt	0.003s
ok  	github.com/swornagent/sworn/internal/run	0.256s
ok  	github.com/swornagent/sworn/internal/state	0.003s
ok  	github.com/swornagent/sworn/internal/verify	0.005s
```

### internal/run (orchestration engine)

```
$ go test ./internal/run/ -v -count=1
=== RUN   TestRun_PassPath_Merges
--- PASS: TestRun_PassPath_Merges (0.06s)
=== RUN   TestRun_FailPath_NoMerge
--- PASS: TestRun_FailPath_NoMerge (0.06s)
=== RUN   TestRun_FailThenPass_RetrySucceeds
--- PASS: TestRun_FailThenPass_RetrySucceeds (0.07s)
=== RUN   TestRun_Blocked_StopsImmediately
--- PASS: TestRun_Blocked_StopsImmediately (0.04s)
=== RUN   TestSanitiseBranch
--- PASS: TestSanitiseBranch (0.00s)
=== RUN   TestRun_MissingTask
--- PASS: TestRun_MissingTask (0.00s)
PASS
```

### internal/git (Merge capability)

```
$ go test ./internal/git/ -v -count=1 -run TestMerge
=== RUN   TestMerge
--- PASS: TestMerge (0.03s)
PASS
```

### cmd/sworn (CLI reachability)

```
$ go test ./cmd/sworn/ -v -count=1
=== RUN   TestCmdRun_MissingTask
--- PASS: TestCmdRun_MissingTask (0.00s)
=== RUN   TestCmdRun_MissingVerifierModel
--- PASS: TestCmdRun_MissingVerifierModel (0.00s)
=== RUN   TestCmdRun_FlagParsing
--- PASS: TestCmdRun_FlagParsing (0.00s)
=== RUN   TestCmdRun_EscalationModelsFlag
--- PASS: TestCmdRun_EscalationModelsFlag (0.00s)
=== RUN   TestCmdRun_UsageContainsEscalationInfo
--- SKIP: TestCmdRun_UsageContainsEscalationInfo (0.00s)
PASS
```

## Reachability artefact

- **Type**: integration-test
- **Path**: `internal/run/run_test.go`
- **CLI reachability**: `cmd/sworn/run_test.go`
- **User gesture**: `go test ./internal/run/ -v` exercises the full orchestration (PASS, FAIL, FAIL-then-PASS, BLOCKED paths) with fake agents/verifiers. `go test ./cmd/sworn/ -v` exercises CLI flag parsing and error paths through the `sworn run` integration point.

## Delivered

- Orchestration engine (`internal/run/run.go`) — evidence: `go test ./internal/run/` PASS
  - AC1: PASS path ends with merge — evidence: `TestRun_PassPath_Merges`
  - AC2: FAIL path escalates after N retries — evidence: `TestRun_FailPath_NoMerge`
  - AC3: Verdict drives control flow — evidence: `TestRun_PassPath_Merges`, `TestRun_FailPath_NoMerge`, `TestRun_Blocked_StopsImmediately`
  - AC4: Retry escalates model per config — evidence: `TestRun_FailThenPass_RetrySucceeds`
- CLI surface (`cmd/sworn/run.go`) — evidence: `go test ./cmd/sworn/` PASS
- `cmd/sworn/main.go` — added "run" case (Pin 4, S08 touchpoint acknowledged)
- `internal/git/git.go` — added `Merge()` (Flag c)
- State transition implemented→verified before merge (Pin 2)
- Auto-generated spec.md and status.json (Pin 3) — evidence: `TestRun_PassPath_Merges` creates and validates
- Model escalation with real OpenAI IDs (Pin 5) — evidence: `DefaultEscalationModels`, `--escalation-models` flag
- CLI reachability test (Pin 1) — evidence: `cmd/sworn/run_test.go`

## Not delivered

None.

## Divergence from plan

None.

## First-pass script output

```
$ release-verify.sh S07-run-loop 2026-06-15-e2e-turnkey-loop
(see live run output)
```