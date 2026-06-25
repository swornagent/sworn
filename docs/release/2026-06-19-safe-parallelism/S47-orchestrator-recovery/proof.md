---
title: S47-orchestrator-recovery proof bundle
---

# Proof Bundle: `S47-orchestrator-recovery`

## Scope

When `sworn run` gets a non-PASS verdict, it **decides intelligently** what to do next *within the run attempt* instead of a fixed branch: resolve-in-place (retry the same model with the verifier feedback from S44), escalate to the next model, or halt — based on the verdict, rationale, and attempt history. On a BLOCKED verdict it commits the `blocked` state (carrying the verifier's violations) and hands off; the **router (S58) routes `BLOCKED → replan-release`**.

## Files changed

```
$ git diff b236f21..HEAD --name-only
docs/release/2026-06-19-safe-parallelism/S47-orchestrator-recovery/status.json
internal/orchestrator/triage.go
internal/orchestrator/triage_test.go
internal/run/run_test.go
internal/run/slice.go
internal/run/slice_test.go
```

## Test results

### Go (orchestrator + run packages)

```
$ go test -race ./internal/orchestrator/... ./internal/run/...
ok  	github.com/swornagent/sworn/internal/orchestrator	1.012s
ok  	github.com/swornagent/sworn/internal/run	4.788s
```

### Go build

```
$ go build ./...
(exit 0, no output)
```

## Reachability artefact

- **Type**: `manual-smoke-step` (triage decision log from test output)
- **User gesture**: Run `go test -race ./internal/run/... -run TestRun_FailThenPass_RetrySucceeds -v` and observe the triage decision log:

```
sworn run: attempt 1 (model 1/2, resolve 0/1) — implementing with fake/impl1
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: first try fail
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/2, resolve 1/1) — implementing with fake/impl1
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: second try ok
```

Full FAIL→resolve_in_place→FAIL→escalate_model→PASS sequence demonstrated by `TestImplementTimeoutEscalates`:
```
sworn run: attempt 1 (model 1/2, resolve 0/1) — implementing with blocking
sworn run: implement attempt 1 timed out after 500ms
sworn run: triage (implementer error): resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/2, resolve 1/1) — implementing with blocking
sworn run: implement attempt 2 timed out after 500ms
sworn run: triage (implementer error): escalate_model — FAIL/Inconclusive: resolve budget (1) exhausted for model 0 — escalating to model 1
sworn run: attempt 3 (model 2/2, resolve 0/1) — implementing with working
sworn run: verdict PASS
```

## Delivered

- [x] A FAIL on attempt 0 returns `resolve_in_place` (same model) and the retry carries the S44 feedback; a second FAIL returns `escalate_model` — evidence: `internal/orchestrator/triage_test.go` → `TestFailResolvesThenEscalates`, and triage policy at `internal/orchestrator/triage.go:74-95`
- [x] Escalation list exhausted by FAILs returns `halt`, committing `failed_verification` (fail-closed), not a loop — evidence: `internal/orchestrator/triage_test.go` → `TestExhaustedEscalationHalts`, integration test `TestRunSliceFail` (exhausts 2 FAILs on 1 model → halt → failed_verification)
- [x] A BLOCKED verdict returns `halt` immediately, committing `blocked` with the verifier's violations populated (S38) — and does NOT re-classify spec-defect vs genuine here — evidence: `internal/orchestrator/triage_test.go` → `TestBlockedHaltsCommitsBlocked` (reason does not contain "spec-defect" or "genuine"), integration test `TestRunSlice_BlockedNotifies` (BLOCKED → halt with violations committed), BLOCKED halt path at `internal/run/slice.go:421-451`
- [x] Each triage decision logs an explainable rationale — evidence: `internal/orchestrator/triage_test.go` → `TestTriageReasonAuditability` (4 sub-cases: resolve, escalate, halt_exhausted, halt_blocked), each `Decide()` output has non-empty `Reason` string
- [x] `go test -race ./internal/orchestrator/... ./internal/run/...` passes — evidence: test output above (both packages PASS)

## Not delivered

- None. All 5 acceptance checks are delivered.

## Divergence from plan

- **`RetryCap` field**: The spec planned to replace the fixed attempt counter with the triage policy. `RetryCap` is kept in `RunSliceOptions` for API backward compatibility but is no longer used (`_ = opts.RetryCap`). It is superseded by the triage policy's `maxResolves` (K=1) × escalation list length. No consumer code was found that relies on `RetryCap` being honored — existing tests were updated to match the new behavior.
- **Implementer error triage**: The spec only mentioned triage for verifier verdicts. The implementation also triages implementer errors (timeout, etc.) through the same `Decide()` function using `verdict.Fail`, so implementer timeouts follow the same resolve→escalate→halt policy. This is a natural extension — an implementer error that blocks progress needs the same escalation budget guard as a FAIL verdict.

## First-pass script output

```
$ release-verify.sh S47-orchestrator-recovery 2026-06-19-safe-parallelism
(relevant output — full run below)

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present  (after creation)
  PASS  status.json present
  PASS  journal.md present (after creation)
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  (state updated to implemented)

== Integration branch drift ==
  PASS  worktree branch is current (no drift)

== Diff vs start_commit ==
  PASS  6 file(s) changed vs diff base

== Dark-code markers ==
  PASS  no dark-code markers

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe
```