---
title: 'S12 — Demote stateless judge to deterministic first-pass; re-vendor verifier.md'
description: 'The stateless LLM judge is demoted to a labelled deterministic first-pass (structure/mock/dark-code checks only) that never drives the slice to verified state; verifier.md is re-vendored from canonical post-records-as-JSON.'
---

# Slice: `S12-first-pass-demote`

## User outcome

The stateless judge (`verify.Run()`) is clearly labelled as a first-pass gate — it catches structural issues (empty spec, no proof, dark-code patterns) before the expensive agentic verifier runs, but a first-pass PASS result does NOT transition the slice to `implemented` or `verified`. Only the agentic verifier (S11 `RunAgentic()`) drives state transitions. `internal/prompt/verifier.md` is re-vendored from the canonical post-records-as-JSON version.

## Entry point

`sworn run` engine path: `internal/run/slice.go` — the first-pass call happens before the agentic verify dispatch introduced in S11; the stateless judge is now called `runFirstPass()` or equivalent.

## In scope

- `internal/verify/verify.go`: rename/refactor `Run()` to `RunFirstPass()`; clearly document in the function comment that it is a structural pre-flight gate and must NOT be used to drive state transitions to `verified`
- `internal/run/slice.go` (T3 section §412): replace any remaining `Run()` call (now `RunFirstPass()`) so it runs as a cheap pre-flight; on first-pass FAIL/BLOCKED, return early with an informative reason ("first-pass: empty spec" etc.) and BLOCK the agentic verifier from running until the issue is resolved
- `internal/prompt/verifier.md`: re-vendor from canonical (copy from `$HOME/.claude/baton/role-prompts/verifier.md`) — the current embedded version is `v0.4.2` stale
- `internal/prompt/VERSION.txt`: bump or note the re-vendor commit reference (not the full pin bump, which is S22/T6)
- Ensure the first-pass gate includes: (a) non-empty spec, (b) non-empty diff, (c) proof path present (if required — optional for first-pass since proof requirement is enforced by S11 before agentic dispatch), (d) no dark-code patterns (existing `validate_blocked.go` check)

## Out of scope

- Adding new first-pass check logic beyond the existing structural checks
- Changes to `internal/orchestrator/triage.go`
- The full pin bump and VERSION centralisation (T6 S22/S23)
- Re-vendoring planner.md, implementer.md, captain.md (T5 S20)

## Planned touchpoints

- `internal/verify/verify.go` (rename Run → RunFirstPass, add clear non-state-machine comment)
- `internal/run/slice.go` (T3 section §412 — update call site from Run to RunFirstPass)
- `internal/prompt/verifier.md` (re-vendor)

## Acceptance checks

- [ ] `verify.Run()` is renamed to `verify.RunFirstPass()` (or clearly annotated as first-pass-only); `grep -rn '"verify.Run\b"'` in the non-test codebase returns zero results; all callers use `RunFirstPass`
- [ ] WHEN `RunFirstPass()` returns PASS, THE SYSTEM SHALL NOT write `state.Verified` to status.json — the state machine is not advanced by the first-pass result
- [ ] WHEN `RunFirstPass()` returns FAIL or BLOCKED, THE SYSTEM SHALL short-circuit and NOT call `RunAgentic()` for that attempt, returning the first-pass failure reason to the caller
- [ ] `internal/prompt/verifier.md` content matches canonical `$HOME/.claude/baton/role-prompts/verifier.md` (verified by `diff` in the proof bundle)
- [ ] `internal/prompt/verifier.md` does NOT contain references to `v0.4.2` or stale section headings

## Required tests

- **Unit**: `internal/verify/verify_test.go` — update existing tests to call `RunFirstPass()` (rename); add one test asserting that a `RunFirstPass()` PASS result does not trigger a state write
- **Reachability artefact**: `go test ./internal/verify/... -v` exits 0; `diff <(cat internal/prompt/verifier.md) <(cat ~/.claude/baton/role-prompts/verifier.md)` exits 0

## Risks

- Renaming `Run()` to `RunFirstPass()` may break callers in `cmd/sworn/verify.go` and any MCP tool that calls it — the implementer must audit all call sites before renaming

## Deferrals allowed?

No.
