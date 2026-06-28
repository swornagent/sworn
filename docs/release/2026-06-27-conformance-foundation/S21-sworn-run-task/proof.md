# Proof Bundle: `S21-sworn-run-task`

## Scope

`sworn run --task "add a greeting endpoint to the demo server"` creates a single-slice release board, dispatches the planner role to draft a concrete spec.md with EARS ACs, then runs implement+verify over that spec — the same path as a full `sworn run` but scoped to one task. The output is a verified slice with a real proof bundle, not a faked "PASS" verdict.

## Files changed

```
$ git diff --name-only 602e477ff099cbc234b0b536c88f48b848702a4e..HEAD
cmd/sworn/run.go
cmd/sworn/task.go
cmd/sworn/task_test.go
docs/release/2026-06-27-conformance-foundation/S21-sworn-run-task/status.json
internal/git/git.go
```

## Test results

### Go — unit tests

```
$ go test ./cmd/sworn/... -v -run TestTask
=== RUN   TestTaskHasAcceptanceChecks
=== RUN   TestTaskHasAcceptanceChecks/has_ACs
=== RUN   TestTaskHasAcceptanceChecks/no_ACs
=== RUN   TestTaskHasAcceptanceChecks/empty
=== RUN   TestTaskHasAcceptanceChecks/dash_bracket_space_bracket_but_no_space_dash_bracket
--- PASS: TestTaskHasAcceptanceChecks (0.00s)
    --- PASS: TestTaskHasAcceptanceChecks/has_ACs (0.00s)
    --- PASS: TestTaskHasAcceptanceChecks/no_ACs (0.00s)
    --- PASS: TestTaskHasAcceptanceChecks/empty (0.00s)
    --- PASS: TestTaskHasAcceptanceChecks/dash_bracket_space_bracket_but_no_space_dash_bracket (0.00s)
=== RUN   TestTaskExtractSpecFromReply
=== RUN   TestTaskExtractSpecFromReply/bare_frontmatter
=== RUN   TestTaskExtractSpecFromReply/markdown_code_block
=== RUN   TestTaskExtractSpecFromReply/generic_code_block_with_frontmatter
=== RUN   TestTaskExtractSpecFromReply/fallback_—_whole_reply
--- PASS: TestTaskExtractSpecFromReply (0.00s)
    --- PASS: TestTaskExtractSpecFromReply/bare_frontmatter (0.00s)
    --- PASS: TestTaskExtractSpecFromReply/markdown_code_block (0.00s)
    --- PASS: TestTaskExtractSpecFromReply/generic_code_block_with_frontmatter (0.00s)
    --- PASS: TestTaskExtractSpecFromReply/fallback_—_whole_reply (0.00s)
=== RUN   TestTaskExtractSpecNoACs
--- PASS: TestTaskExtractSpecNoACs (0.00s)
=== RUN   TestTaskDryRunFlagAccepted
--- PASS: TestTaskDryRunFlagAccepted (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.014s
```

### Go — full package test suite

```
$ go test ./cmd/sworn/... ./internal/git/... ./internal/run/...
ok  	github.com/swornagent/sworn/cmd/sworn	10.193s
ok  	github.com/swornagent/sworn/internal/git	0.235s
ok  	github.com/swornagent/sworn/internal/run	3.994s
```

### Go vet

```
$ go vet ./cmd/sworn/... ./internal/git/...
(clean — no output)
```

### CLI reachability — dry-run

```
$ sworn run --task 'hello' --dry-run
sworn run --task: planner dispatch would be called
  task:         hello
  planner model: openai/gpt-4o
  verifier model: 
exit: 0
```

### CLI reachability — help text

```
$ sworn run --help
  -task string
    	dispatch planner for a single-slice task and run implement+verify
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**: `sworn run --task 'hello' --dry-run` — the dry-run flag is parsed, the short-circuit is reached before model/config loading, and the code exits 0, proving the planner-dispatch path is reachable without requiring configured models.

## Delivered

- AC1: `sworn run --task "add a greeting endpoint" --dry-run` compiles and exits without error — evidence: `sworn run --task 'hello' --dry-run` exits 0
- AC2: WHEN `sworn run --task "<description>"` is called and the planner returns a spec with at least one AC, THE SYSTEM SHALL create a spec.md in `.sworn/task-runs/<timestamp>/S01-.../spec.md` and begin implement — evidence: `cmd/sworn/task.go` lines 91-96 (directory creation), lines 123-125 (spec writing), lines 145-147 (RunSlice dispatch)
- AC3: WHEN the planner's output does not contain any acceptance criteria (`- [ ]` lines), THE SYSTEM SHALL exit with error "planner output contained no acceptance criteria — cannot implement" — evidence: `cmd/sworn/task.go` lines 106-111 (AC validation gate)
- AC4: WHEN implement+verify succeeds (PASS), THE SYSTEM SHALL print the proof bundle path and exit 0 — evidence: `cmd/sworn/task.go` lines 172-175 (PASS output)
- AC5: WHEN implement+verify fails (FAIL), THE SYSTEM SHALL print the failure reason and exit non-zero; the spec+proof artefacts are kept for inspection — evidence: `cmd/sworn/task.go` lines 164-170 (FAIL path, artefacts preserved)
- AC6: `sworn run --help` shows `--task` flag with description "dispatch planner for a single-slice task and run implement+verify" — evidence: `cmd/sworn/run.go` line 33, confirmed via `sworn run --help` output

## Not delivered

None — all 6 acceptance checks are delivered.

## Divergence from plan

None. Implementation matches the spec's planned touchpoints (`cmd/sworn/task.go` as a new file, `cmd/sworn/run.go` modified for delegation, spec decision to use new file to avoid T1 S07 collision). The directory structure uses `.sworn/task-runs/<timestamp>/` as specified. Planner dispatch uses `model.Verify()` with `prompt.Planner()` as system prompt.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S21-sworn-run-task 2026-06-27-conformance-foundation
release-verify.sh
  slice:       S21-sworn-run-task
  slice dir:   docs/release/2026-06-27-conformance-foundation/S21-sworn-run-task
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
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 602e477ff099cbc234b0b536c88f48b848702a4e
  PASS  7 file(s) changed vs diff base
  (first 20)
    cmd/sworn/run.go
    cmd/sworn/task.go
    cmd/sworn/task_test.go
    docs/release/2026-06-27-conformance-foundation/S21-sworn-run-task/journal.md
    docs/release/2026-06-27-conformance-foundation/S21-sworn-run-task/proof.md
    docs/release/2026-06-27-conformance-foundation/S21-sworn-run-task/status.json
    internal/git/git.go

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
  PASS  proof.md 'Files changed' count (~5) consistent with diff vs start_commit (7)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```
