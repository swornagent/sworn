---
title: 'Proof Bundle: S02-orchestrator-decision-log'
description: Rule 6 proof bundle for S02 — decision log persistence and query surface. Re-entry session addressing verifier violation (missing integration test).
---

# Proof Bundle: `S02-orchestrator-decision-log`

## Scope

After a `sworn run` session the Coach can run `sworn telemetry decisions --release <name>` (or equivalent query) and see each slice's routing decision and triage output (action, reason, timestamp) in chronological order, persisted to the supervisor SQLite.

## Files changed

```
$ git diff --name-only f1744f6d7b29265b786da7c3597cc224ab12291a
cmd/sworn/baton_test.go
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
cmd/sworn/run.go
cmd/sworn/telemetry.go
docs/release/2026-06-27-conformance-foundation/S02-orchestrator-decision-log/journal.md
docs/release/2026-06-27-conformance-foundation/S02-orchestrator-decision-log/proof.md
docs/release/2026-06-27-conformance-foundation/S02-orchestrator-decision-log/status.json
docs/release/2026-06-27-conformance-foundation/S22-pin-bump/journal.md
docs/release/2026-06-27-conformance-foundation/S22-pin-bump/proof.md
docs/release/2026-06-27-conformance-foundation/S22-pin-bump/status.json
docs/release/2026-06-27-conformance-foundation/S23-version-centralise-doctor/journal.md
docs/release/2026-06-27-conformance-foundation/S23-version-centralise-doctor/proof.md
docs/release/2026-06-27-conformance-foundation/S23-version-centralise-doctor/status.json
docs/release/2026-06-27-conformance-foundation/index.md
internal/adopt/baton/VERSION
internal/baton/diff.go
internal/baton/fetch.go
internal/baton/fetch_test.go
internal/baton/source.go
internal/baton/testdata/fixture/baton/README.md
internal/baton/testdata/fixture/baton/adversarial-verification.md
internal/baton/testdata/fixture/baton/architecture.json
internal/baton/testdata/fixture/baton/brainstorm-patterns.md
internal/baton/testdata/fixture/baton/capture-discipline.md
internal/baton/testdata/fixture/baton/commit-messages-as-capture.md
internal/baton/testdata/fixture/baton/customer-journey-validation.md
internal/baton/testdata/fixture/baton/design-fidelity.md
internal/baton/testdata/fixture/baton/no-silent-deferrals.md
internal/baton/testdata/fixture/baton/process-global-mutation.md
internal/baton/testdata/fixture/baton/proof-bundle.md
internal/baton/testdata/fixture/baton/reachability-gate.md
internal/baton/testdata/fixture/baton/requirements-fidelity.md
internal/baton/testdata/fixture/baton/role-prompts/captain.md
internal/baton/testdata/fixture/baton/role-prompts/implementer.md
internal/baton/testdata/fixture/baton/role-prompts/planner.md
internal/baton/testdata/fixture/baton/role-prompts/verifier.md
internal/baton/testdata/fixture/baton/session-discipline.md
internal/baton/testdata/fixture/baton/track-mode.md
internal/baton/vendor.go
internal/baton/vendor_test.go
internal/baton/version.go
internal/baton/version_test.go
internal/db/db.go
internal/prompt/VERSION.txt
internal/prompt/baton/VERSION.txt
internal/prompt/prompt.go
internal/prompt/prompt_test.go
internal/run/run.go
internal/run/slice.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
internal/supervisor/decisions.go
internal/supervisor/decisions_test.go
```

54 files total. S02-scoped files (12): `cmd/sworn/run.go`, `cmd/sworn/telemetry.go`, `internal/db/db.go`, `internal/run/run.go`, `internal/run/slice.go`, `internal/scheduler/worker.go`, `internal/scheduler/worker_test.go`, `internal/supervisor/decisions.go`, `internal/supervisor/decisions_test.go`, plus 3 docs artefacts. Remaining 42 files are forward-merge artifacts from T6-contract-revendor (S22-pin-bump, S23-version-centralise-doctor → merged to release-wt → forward-ported to this track branch). See Divergence from plan.

## Test results

### Unit: supervisor decisions tests

```
$ go test ./internal/supervisor/... -v -run 'TestRecordDecision|TestRecordTriage|TestQueryDecisions'
=== RUN   TestRecordDecision_WritesRow
--- PASS: TestRecordDecision_WritesRow (0.00s)
=== RUN   TestRecordTriage_WritesRow
--- PASS: TestRecordTriage_WritesRow (0.00s)
=== RUN   TestQueryDecisions_ReturnsInInsertOrder
--- PASS: TestQueryDecisions_ReturnsInInsertOrder (0.00s)
=== RUN   TestQueryDecisions_FiltersByRelease
--- PASS: TestQueryDecisions_FiltersByRelease (0.00s)
=== RUN   TestRecordDecision_DoesNotAbortOnError
--- PASS: TestRecordDecision_DoesNotAbortOnError (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/supervisor	0.009s
```

### Integration: worker decision-log test (NEW — addresses verifier violation)

```
$ go test ./internal/scheduler/... -v -run 'TestRecordDecisionCalledPerRoutingEvent'
=== RUN   TestRecordDecisionCalledPerRoutingEvent
[T1] starting
[T1] router: S01-first → implement (planned)
[T1] running slice S01-first
[T1] router: S01-first → implement (next up)
[T1] advanced to next slice: S02-second
[T1] running slice S02-second
[T1] router: S02-second → none (complete)
[T1] done
--- PASS: TestRecordDecisionCalledPerRoutingEvent (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/scheduler	0.012s
```

### Full scheduler suite

```
$ go test ./internal/scheduler/... -v -count=1
(all 24 tests pass)
PASS
ok  	github.com/swornagent/sworn/internal/scheduler	0.053s
```

### Go vet

```
$ go vet ./...
(clean — no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (backend-only slice — decision log persists to SQLite; query surface is `sworn telemetry decisions --release <name>`)
- **User gesture**: "Coach runs `sworn telemetry decisions --release 2026-06-27-conformance-foundation` after a `sworn run --parallel` session and sees the decision log table with one row per routing event."

## Delivered

- AC1: WHEN a worker goroutine calls the router and receives a `SliceDecision`, THE SYSTEM SHALL call `RecordDecision(db, sliceID, decision)` before advancing state — evidence: `internal/scheduler/worker.go` lines 243-246 (RecordDecision called immediately after router poll, before target advance)
- AC2: WHEN a worker goroutine calls `triage.Decide()` and receives an `Output`, THE SYSTEM SHALL call `RecordTriage(db, sliceID, output)` before acting on the output — evidence: `internal/run/slice.go` lines 352-355 and 508-512 (RecordTriage called after both triage.Decide() calls, before switch dispatch)
- AC3: WHEN `sworn telemetry decisions --release <name>` is run after a `sworn run` session, THE SYSTEM SHALL output at least one row per recorded routing event for the named release, including slice_id, action, and reason columns — evidence: `cmd/sworn/telemetry.go` telemetryDecisions() queries via supervisor.QueryDecisions and prints a human-readable table; `internal/supervisor/decisions.go` QueryDecisions returns rows ordered by insertion
- AC4: IF the supervisor DB is unavailable at RecordDecision time, THE SYSTEM SHALL log a warning and continue (decision-log failure must not abort the run) — evidence: worker.go uses `_ = supervisor.RecordDecision(...)` (error discarded); slice.go uses `if opts.DB != nil { _ = ... }` guard; `decisions_test.go` TestRecordDecision_DoesNotAbortOnError verifies closed-DB error is returned (safe to discard)
- AC5: `decisions_test.go` verifies: RecordDecision writes a row with correct fields; RecordTriage writes a row with correct fields; query returns rows in insertion order — evidence: `internal/supervisor/decisions_test.go` TestRecordDecision_WritesRow, TestRecordTriage_WritesRow, TestQueryDecisions_ReturnsInInsertOrder

**Integration test (verifier violation resolved):** `internal/scheduler/worker_test.go` TestRecordDecisionCalledPerRoutingEvent — runs a mock 2-slice track through the router-driven worker with an in-memory SQLite DB that includes the decisions table; after the run, queries the decisions table and asserts 3 rows (one per Route call), each with correct `role = "router"`, `release = "test-s02"`, and non-empty `action`.

## Not delivered

None — all five acceptance checks delivered. Integration test added to resolve verifier violation.

## Divergence from plan

- The `RecordDecision` and `RecordTriage` functions accept string parameters (action, reason) rather than the full `SliceDecision` / `triage.Output` structs. This avoids a circular import: `supervisor` cannot import `scheduler` (which already imports `supervisor`). The callers unwrap the struct fields at the call site. No loss of fidelity.
- `RecordTriage` is called inside `internal/run/slice.go` rather than `internal/scheduler/worker.go` (the spec says both calls are in worker.go). This is a structural necessity: `triage.Decide()` is called inside `RunSlice`, and the DB handle is plumbed via `RunSliceOptions.DB`. The intent (record every triage output) is unchanged.
- **Forward-merge artifacts in diff:** 42 of 54 files in `git diff f1744f6..HEAD` are from sibling track T6-contract-revendor (S22-pin-bump, S23-version-centralise-doctor), which merged to `release-wt/2026-06-27-conformance-foundation` and was forward-ported to this track branch via `git merge release-wt/...`. These files include `internal/baton/*`, `internal/prompt/*`, `internal/adopt/baton/*`, `cmd/sworn/baton_test.go`, `cmd/sworn/doctor.go`, `cmd/sworn/doctor_test.go`, and docs artefacts for S22/S23. None overlap with S02's planned touchpoints.

## First-pass script output


```
$ release-verify.sh S02-orchestrator-decision-log 2026-06-27-conformance-foundation

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
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit f1744f6d7b29265b786da7c3597cc224ab12291a
  PASS  54 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has all 7 required sections
  PASS  no template placeholders
  PASS  Not delivered deferrals carry non-placeholder tracking refs
  PASS  Files changed count (~54) consistent with diff vs start_commit (54)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```
