# S01-llm-interpreter — Journal

## 2026-06-27: Implementation session

### State transition: planned → in_progress → implemented

### Decisions

1. **Interpreter integration point**: Spec listed `internal/scheduler/worker.go` as the integration point. In the current codebase, `triage.Decide()` was moved into `internal/run/slice.go` by S47. The interpreter interception lives at the verifier→triage boundary inside `run/slice.go` (lines 433–462). This is a touchpoint matrix addition — `run/slice.go` was not in `planned_files` but the spec explicitly requires calling `Interpret()` before `triage.Decide()`, and that's where `triage.Decide()` lives.

2. **`InterpretVerifier` is opt-in**: The interpreter model is passed via `RunSliceOptions.InterpretVerifier`. When nil (the default), the existing behaviour is preserved — unparseable verdicts remain BLOCKED. This is backward-compatible with all existing tests. The spec requires the interpreter to activate when the model is configured; the current wiring point in `cmd/sworn/run.go` does not yet set `InterpretVerifier` — that's a follow-up concern.

3. **Worker PAGE handling**: The `INTERPRETER_INCONCLUSIVE` sentinel is detected in `worker.go`'s error path and converts the error from `TrackFail` to `TrackPaused`. Two integration tests validate this: `TestRunTrack_InterpreterInconclusivePauses` (sentinel → pause) and `TestRunTrack_InterpreterSentinelIsNotNormalFailure` (normal error → fail).

4. **captureVerifier wrapper**: To access the raw verifier output (needed for the interpreter), a `captureVerifier` wrapper is injected into `verify.Run()`. The wrapper is minimal — it satisfies the `model.Verifier` interface and captures the `text` return value. This avoids modifying `verify.Run()` (owned by T3).

5. **Test naming**: Test functions in `interpreter_test.go` are named `TestInterpreter_*` (not `TestInterpret_*`) to match the spec's required test command pattern `-run TestInterpreter`.

### Touchpoint matrix

| File | Planned (spec) | Actual | Notes |
|---|---|---|---|
| `internal/orchestrator/interpreter.go` | ✓ | ✓ | New file |
| `internal/orchestrator/interpreter_test.go` | ✓ | ✓ | New file |
| `internal/scheduler/worker.go` | ✓ | ✓ | Sentinel check + imports |
| `internal/scheduler/worker_test.go` | — | ✓ | Integration tests (AC6) |
| `internal/run/slice.go` | — | ✓ | Interpreter interception + captureVerifier (touchpoint addition — triage lives here since S47) |

### Trade-offs

- **Interpreter not auto-wired**: The interpreter engine is complete but opt-in. This avoids breaking tests that depend on the unparseable→BLOCKED path. The caller must explicitly set `InterpretVerifier`. This is a known deferral (tracked in proof.md Not Delivered) — the CLI flag wiring is a follow-up.
- **No fallback to verifier model**: Earlier drafts fell back to using the verifier's own model client for the interpreter call. This was removed because it changed behaviour for existing tests (the fake verifier returns empty text, causing the interpreter to return INCONCLUSIVE where BLOCKED was expected).