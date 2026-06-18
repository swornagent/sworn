---
title: Slice journal — S04-requirements-verify-gate
description: Implementation log for the requirements-quality verification gate.
---

# Journal: S04-requirements-verify-gate

## Session log

### 2026-06-18 12:00 — implementation start

- **State**: `planned → in_progress`
- **Notes**:
  - Track T1-fidelity-core worktree materialised at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core`.
  - S01-rtm-spine and S02-ears-ac-format already `verified` — sequential ordering satisfied.
  - Designed and implemented `internal/reqverify/` package (core logic + test).
  - Created `internal/prompt/requirements-verifier.md` — fresh-context prompt for grading ACs against ISO/IEC/IEEE 29148 quality characteristics.
  - Created `cmd/sworn/reqverify.go` — CLI handler following the same `config.ResolveVerifierModel` pattern as `cmdVerify`.
  - Modified `cmd/sworn/main.go` — added `case "reqverify"` and usage text.
  - Modified `internal/prompt/prompt.go` — added `RequirementsVerifier()` accessor and embedded the new prompt.

### 2026-06-18 12:30 — implementation complete

- **State**: `in_progress → implemented`
- **Notes**:
  - All 20 unit tests pass in `internal/reqverify/`.
  - All 4 CLI integration tests pass in `cmd/sworn/reqverify_test.go`.
  - `go vet ./...` clean.
  - First-pass script: 18/18 PASS.
  - Design decisions:
    - Batched model dispatch (all ACs in one call) rather than per-AC model calls, for efficiency.
    - Model output parsed from `## RESULTS` section with per-AC lines in format `AC <N> (<slice-id>): PASS|FAIL — <characteristic>`.
    - AC extraction uses markdown checkbox regex under `## Acceptance checks` section header.
    - Fail-closed: missing AC in model response → FAIL; missing RESULTS section → BLOCKED (via error).
    - CLI behaviour mirrors `verify` command: flag > env > config > Unconfigured for model resolution.
  - Divergence from plan:
    - `internal/prompt/prompt.go` modified (not in planned_files) to add accessor.
    - `cmd/sworn/reqverify_test.go` created (not in planned_files) for CLI integration tests.
    - `internal/adopt/baton/rules/08-requirements-fidelity.md` not modified (already authored by plan/S16).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-06-18 14:00 — FAIL (fresh-context session)

```
FAIL

Slice: `S04-requirements-verify-gate`

Violations:
1. Gate 3 — CLI integration test does not exercise the reqverify logic through the CLI boundary.
   Evidence: `cmd/sworn/reqverify_test.go` — `TestReqverifyCmd_WithFixtureRelease` creates a
   fixture release and calls `cmdReqverify([]string{"test-release"})` but stops at "sworn
   reqverify: model: SWORN_OPENAI_API_KEY not set" (exit 2) before `reqverify.Run()` is called.
   Steps 4–6 of `cmdReqverify` (run, print, return exit 1 on violations) are never tested
   through the CLI. The `internal/reqverify/reqverify_test.go` unit tests test `Run()` directly
   — that is the leaf package, not the CLI integration point. Spec states "E2E gate type: local
   (stubbed model client; no live key needed)" but the CLI is not injectable and the prescribed
   stubbed-client path is unreachable from the CLI tests.

2. Gate 4 — Reachability smoke step is unrunnable without a live model key.
   Evidence: `proof.md` "Reachability artefact" states "This requires a configured model (env:
   SWORN_OPENAI_API_KEY)" and substitutes unit-test evidence for the smoke step. This directly
   contradicts the spec's "E2E gate type: local (stubbed model client; no live key needed)."

Required to address:
1. Refactor `cmdReqverify` to accept an injectable `reqverify.Verifier` (e.g. add
   `cmdReqverifyWithVerifier(args []string, v reqverify.Verifier) int` and have `cmdReqverify`
   resolve and delegate). Update `TestReqverifyCmd_WithFixtureRelease` to pass a `fakeVerifier`
   stub so the test exercises the full path: fixture ACs → extraction → model stub → violations
   detected → exit 1. Also add a passing-path test (all-pass reply → exit 0). Pattern already
   present in `internal/reqverify/reqverify_test.go`.
2. Update the `proof.md` reachability artefact to document a smoke step that uses the injectable
   path (no live key), or reference the new passing CLI-level test.
```
### 2026-06-18 15:00 — re-implementation (address verifier violations)

- **State**: `failed_verification -> in_progress -> implemented`
- **Notes**:
  - Addressed both verifier violations from the 2026-06-18 14:00 FAIL verdict.
  - **Gate 3 fix**: Refactored `cmd/sworn/reqverify.go` — split into `cmdReqverify` (public,
    model-resolving) and `cmdReqverifyWithVerifier(releaseName string, v reqverify.Verifier) int`
    (injectable, accepts a pre-resolved verifier stub). The injectable path does the full business
    logic: resolve release dir -> extract ACs -> call `reqverify.Run()` with the stub -> print
    report -> return exit code.
  - **Gate 4 fix**: Updated proof.md reachability artefact — replaced "requires SWORN_OPENAI_API_KEY"
    with `go test` commands for `TestReqverifyCmdWithVerifier_AllPass` and
    `TestReqverifyCmdWithVerifier_Violations` that run with a stubbed model client (no live key).
  - Rewrote `cmd/sworn/reqverify_test.go`:
    - Replaced `TestReqverifyCmd_WithFixtureRelease` with injectable-path tests:
      `TestReqverifyCmdWithVerifier_AllPass` (exit 0), `TestReqverifyCmdWithVerifier_Violations`
      (exit 1), `TestReqverifyCmdWithVerifier_ModelError` (exit 2),
      `TestReqverifyCmdWithVerifier_NonexistentRelease` (exit 2).
    - Removed old test that could only test the unconfigured-model path.
  - All 20 internal/reqverify tests + 8 CLI tests pass. `go vet` clean.
  - **Key design decision**: `cmdReqverifyWithVerifier` accepts a `reqverify.Verifier` (the
    package's own local interface) rather than `model.Verifier`, keeping the reqverify package
    dependency-free. The `model.Unconfigured` / `model.FromEnv` resolution stays in `cmdReqverify`.
  - No changes to `internal/reqverify/reqverify.go` or `internal/reqverify/reqverify_test.go`
    — the existing fakeVerifier pattern was already correct from the first pass.
