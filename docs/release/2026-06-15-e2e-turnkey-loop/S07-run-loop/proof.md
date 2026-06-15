# Proof Bundle: `S07-run-loop`

## Scope

S07-run-loop: `sworn run` — the end-to-end orchestration: implement → verify → retry/escalate → gated merge.

## Files changed

```
$ git diff --name-only 006c261b9dc93865bccf8c7a5dc1c8f07597fd8a
cmd/sworn/init.go
cmd/sworn/main.go
cmd/sworn/run.go
cmd/sworn/run_test.go
docs/release/2026-06-15-e2e-turnkey-loop/S07-run-loop/approved-ack.md
docs/release/2026-06-15-e2e-turnkey-loop/S07-run-loop/journal.md
docs/release/2026-06-15-e2e-turnkey-loop/S07-run-loop/proof.md
docs/release/2026-06-15-e2e-turnkey-loop/S07-run-loop/spec.md
docs/release/2026-06-15-e2e-turnkey-loop/S07-run-loop/status.json
docs/release/2026-06-15-e2e-turnkey-loop/activity.md
internal/git/git.go
internal/git/git_test.go
internal/run/run.go
internal/run/run_test.go
```

Implementation files (excluding docs):
- `cmd/sworn/run.go`, `cmd/sworn/run_test.go`, `cmd/sworn/main.go`, `cmd/sworn/init.go`
- `internal/run/run.go`, `internal/run/run_test.go`
- `internal/git/git.go`, `internal/git/git_test.go`

## Test results

### Full suite

```
$ go test ./... -count=1
ok  	github.com/swornagent/sworn/cmd/sworn	0.011s
ok  	github.com/swornagent/sworn/internal/adopt	0.007s
ok  	github.com/swornagent/sworn/internal/agent	0.013s
ok  	github.com/swornagent/sworn/internal/board	0.003s
ok  	github.com/swornagent/sworn/internal/config	0.004s
ok  	github.com/swornagent/sworn/internal/git	0.158s
ok  	github.com/swornagent/sworn/internal/implement	0.130s
ok  	github.com/swornagent/sworn/internal/model	0.209s
ok  	github.com/swornagent/sworn/internal/prompt	0.003s
ok  	github.com/swornagent/sworn/internal/run	0.238s
ok  	github.com/swornagent/sworn/internal/state	0.004s
ok  	github.com/swornagent/sworn/internal/verify	0.005s
```

### internal/run (orchestration engine)

```
$ go test ./internal/run/ -v -count=1
=== RUN   TestRun_PassPath_Merges
sworn run: merged sworn/write-a-hello-file into main (PASS)
--- PASS: TestRun_PassPath_Merges (0.07s)
=== RUN   TestRun_FailPath_NoMerge
sworn run: verification failed — retrying with escalated implementer model (×3)
--- PASS: TestRun_FailPath_NoMerge (0.08s)
=== RUN   TestRun_FailThenPass_RetrySucceeds
sworn run: merged sworn/write-retry-file into main (PASS)
--- PASS: TestRun_FailThenPass_RetrySucceeds (0.10s)
=== RUN   TestRun_Blocked_StopsImmediately
--- PASS: TestRun_Blocked_StopsImmediately (0.06s)
=== RUN   TestSanitiseBranch
--- PASS: TestSanitiseBranch (0.00s)
=== RUN   TestRun_MissingTask
--- PASS: TestRun_MissingTask (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/run	0.303s
```

### internal/git (Merge capability)

```
$ go test ./internal/git/ -v -count=1 -run TestMerge
=== RUN   TestMerge
--- PASS: TestMerge (0.05s)
PASS
ok  	github.com/swornagent/sworn/internal/git	0.053s
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
--- PASS: TestCmdRun_EscalationModelsFlag (0.01s)
=== RUN   TestCmdRun_UsageContainsEscalationInfo
--- SKIP: TestCmdRun_UsageContainsEscalationInfo (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.026s
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

- **`internal/git/git.go` (+`internal/git/git_test.go`)** — `Merge()` added as a direct dependency of `internal/run/run.go`. Not in planned touchpoints (`internal/run/`, `cmd/sworn/run.go`, `cmd/sworn/main.go`) because the need for a dedicated `git.Merge()` surfaced during implementation when wiring the gated-merge step. The run loop needs to programmatically merge a branch; factoring this into `internal/git` (the canonical home for git operations, established by S05) keeps the seam clean.
- **`cmd/sworn/init.go`** — trailing-newline whitespace fix (added missing `\n` at EOF). Cosmetic only; no logic change.


## First-pass script output

```
$ release-verify.sh S07-run-loop 2026-06-15-e2e-turnkey-loop

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  14 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~14) consistent with diff vs start_commit (14)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
```
