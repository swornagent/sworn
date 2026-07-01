---
title: 'S01 — LLM interpreter for non-typed outcomes'
description: 'Add a bounded cheap-model decision step between dispatch and the router poll so non-typed implementer/verifier outputs do not stall the loop.'
---

# Slice: `S01-llm-interpreter`

## User outcome

When `sworn run` receives an implementer or verifier output that does not parse to a clean state transition, the loop routes through a bounded cheap-model interpreter step that classifies the outcome and advances the state machine, rather than pausing to page the Coach for routine ambiguity.

## Entry point

`sworn run --release <name>` (autonomous loop) — specifically the `internal/scheduler/worker.go` worker goroutine path that currently calls `triage.Decide()` after a model dispatch returns a non-verdict result, stalling when the output is not a clean PASS/FAIL/BLOCKED string.

## In scope

- New `internal/orchestrator/interpreter.go`: `Interpret(ctx, rawOutput, sliceID, role) (verdict.Verdict, reason, error)` — calls a bounded cheap model (max_tokens ~200, temperature 0) to classify the raw output as PASS, FAIL, BLOCKED, or INCONCLUSIVE
- Worker integration: `internal/scheduler/worker.go` calls `Interpret()` before `triage.Decide()` when the raw model output does not parse to a known verdict
- Interpreter uses the existing `model.Verifier` interface (stateless LLM call, no tools)
- Interpreter prompt instructs: "classify this output into PASS, FAIL, BLOCKED, or INCONCLUSIVE; if uncertain, return INCONCLUSIVE"; response is capped at 200 tokens
- Interpreter is bounded: if it returns INCONCLUSIVE after one attempt, the worker pages the Coach (same as current pause behaviour) — the interpreter does not retry itself
- Test: `internal/orchestrator/interpreter_test.go`

## Out of scope

- The interpreter does not propose redesigns or edit specs (that stays with the router → replan-release)
- LLM-based routing decisions (which slice to run next) — the deterministic router (S58/T1) owns that
- Multi-turn interpreter conversations — single bounded call only
- Errors classified as terminal (KindAuth, KindCredits) — these are handled by S09 before the interpreter step

## Planned touchpoints

- `internal/orchestrator/interpreter.go` (new)
- `internal/orchestrator/interpreter_test.go` (new)
- `internal/scheduler/worker.go` (add Interpret() call in the non-typed outcome path)

## Acceptance checks

- [ ] WHEN a worker goroutine receives a model response that does not begin with PASS/FAIL/BLOCKED/INCONCLUSIVE (case-insensitive), THE SYSTEM SHALL call `Interpret()` with the raw output before calling `triage.Decide()`
- [ ] WHEN `Interpret()` returns PASS/FAIL/BLOCKED, THE SYSTEM SHALL feed that verdict to `triage.Decide()` as if it were a parsed model response
- [ ] WHEN `Interpret()` returns INCONCLUSIVE or errors, THE SYSTEM SHALL emit a PAGE event (same path as current max_turns pause) and stop the worker for that track
- [ ] IF the model client for the interpreter is nil or unconfigured, THE SYSTEM SHALL return INCONCLUSIVE immediately (fail closed, not panic)
- [ ] `interpreter_test.go` has a table test covering: clean PASS response, clean FAIL response, ambiguous prose (expects INCONCLUSIVE or classified result), empty response (expects INCONCLUSIVE), nil model (expects INCONCLUSIVE)
- [ ] `worker_test.go` (or existing integration test) covers the non-typed-output → interpreter → triage path and asserts the state machine advances correctly when interpreter returns PASS

## Required tests

- **Unit**: `internal/orchestrator/interpreter_test.go` — table-driven, covers all five input scenarios above
- **Integration**: existing `internal/scheduler/` worker tests must pass after the new call path is added; add one integration scenario for the non-typed-output path
- **Reachability artefact**: `go test ./internal/orchestrator/... ./internal/scheduler/... -v -run TestInterpreter` exits 0

## Risks

- Cheap-model availability: if the Interpret() model is the same as the implementer model, a KindCredits error on the main model also blocks the interpreter — S09 terminal-kind halt fires first, which is correct
- Token cost: bounded at 200 tokens; at ~$0.001/call this is negligible vs a full implementer run

## Deferrals allowed?

No. If the interpreter cannot be wired into worker.go without touching triage.go (which is owned by T2), that surfaces as a spec defect in the implementer's Step 0 and this spec must be updated.
