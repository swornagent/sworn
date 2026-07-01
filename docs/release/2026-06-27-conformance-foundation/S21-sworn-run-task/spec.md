---
title: 'S21 — sworn run --task: real single-slice planner-assist quickstart (direction C)'
description: 'sworn run --task "<description>" dispatches the planner role to draft a concrete-AC spec.md, then runs implement+verify over that spec in a new single-slice release/worktree — replacing the current faked/Rule-8-violating stub. This is the honest demo/on-ramp path.'
---

# Slice: `S21-sworn-run-task`

## User outcome

`sworn run --task "add a greeting endpoint to the demo server"` creates a single-slice release board, dispatches the planner role to draft a concrete spec.md with EARS ACs, then runs implement+verify over that spec — the same path as a full `sworn run` but scoped to one task. The output is a verified slice with a real proof bundle, not a faked "PASS" verdict.

## Entry point

`sworn run --task "<description>"` CLI flag — new flag on `cmd/sworn/run.go` or a new `cmd/sworn/task.go` subcommand. Spec decision: new file `cmd/sworn/task.go` to avoid `cmd/sworn/run.go` collision with T1 S07.

## In scope

- `cmd/sworn/task.go` (new): `sworn run --task` subcommand
  1. Accept `--task "<description>"` and optional `--model <model>` (defaults to configured implementer model)
  2. Create a single-slice release in a temp directory (`.sworn/task-runs/<timestamp>/`) or using the existing release infra with release name `task-<timestamp>`
  3. Dispatch the planner role via `model.Verify()` (or `model.Chat()` if available) with the planner.md system prompt and the task description as the user message; expect back a spec.md content block
  4. Parse the planner's output to extract the spec.md content; validate it has at least one AC (`- [ ]`) and a covers_needs reference (or allow N/A for task-mode slices)
  5. Write spec.md + status.json for the single slice
  6. Run `sworn run` (internal call) for that single slice — implement then verify
  7. Report PASS/FAIL with the proof bundle path
- Uses existing `model.Verify()` for the planner dispatch (no dependency on T2 agentic Chat — OAI-compatible drivers support Verify() for the planner call)
- Task-mode releases are ephemeral by default (cleaned up after success; kept on failure for inspection)
- On FAIL, print the proof bundle path and exit non-zero

## Out of scope

- Full multi-slice planning (that is `/plan-release`)
- Tool-use or agent-mode for the planner dispatch (single-shot Verify() call is sufficient)
- Persistent task history (not stored in board.json)
- The `--task` flag modifying or colliding with the existing `sworn run --release` path

## Planned touchpoints

- `cmd/sworn/task.go` (new — task subcommand, independent of run.go)
- `cmd/sworn/task_test.go` (new — unit and integration tests for task subcommand)
- `cmd/sworn/run.go` (modify — delegate `--task` flag to cmdRunTask; gofmt clean)
- `internal/git/git.go` (modify — add Config() method for ephemeral git repo setup)
## Acceptance checks

- [ ] `sworn run --task "add a greeting endpoint" --dry-run` compiles and exits without error (dry-run verifies the planner dispatch would be called without actually running)
- [ ] WHEN `sworn run --task "<description>"` is called and the planner returns a spec with at least one AC, THE SYSTEM SHALL create a spec.md in `.sworn/task-runs/<timestamp>/S01-.../spec.md` and begin implement
- [ ] WHEN the planner's output does not contain any acceptance criteria (`- [ ]` lines), THE SYSTEM SHALL exit with error "planner output contained no acceptance criteria — cannot implement"
- [ ] WHEN implement+verify succeeds (PASS), THE SYSTEM SHALL print the proof bundle path and exit 0
- [ ] WHEN implement+verify fails (FAIL), THE SYSTEM SHALL print the failure reason and exit non-zero; the spec+proof artefacts are kept for inspection
- [ ] `sworn run --help` shows `--task` flag with description "dispatch planner for a single-slice task and run implement+verify"

## Required tests

- **Unit**: `cmd/sworn/task_test.go` (new) — mock planner dispatch, verify spec.md written with ACs; test the "no ACs" error path
- **Reachability artefact**: `sworn run --task "add a greeting" --dry-run` exits 0; the help text includes --task

## Risks

- The single-shot planner dispatch may not produce a EARS-compliant spec in one shot; the implementer should accept any spec with at least one AC and one in-scope bullet; quality is the planner's responsibility, not this slice's gate

## Deferrals allowed?

No.
