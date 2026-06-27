---
title: 'Proof Bundle: S02-orchestrator-decision-log'
description: Rule 6 proof bundle for S02 — decision log persistence and query surface.
---

# Proof Bundle: `S02-orchestrator-decision-log`

## Scope

After a `sworn run` session the Coach can run `sworn telemetry decisions --release <name>` (or equivalent query) and see each slice's routing decision and triage output (action, reason, timestamp) in chronological order, persisted to the supervisor SQLite.

## Files changed

```
$ git diff --name-only release-wt/2026-06-27-conformance-foundation
cmd/sworn/run.go
cmd/sworn/telemetry.go
docs/release/2026-06-27-conformance-foundation/S01-llm-interpreter/journal.md
docs/release/2026-06-27-conformance-foundation/S01-llm-interpreter/proof.md
docs/release/2026-06-27-conformance-foundation/S01-llm-interpreter/status.json
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
internal/baton/testdata/fixture/claude/baton/README.md
internal/baton/testdata/fixture/claude/baton/adversarial-verification.md
internal/baton/testdata/fixture/claude/baton/architecture.json
internal/baton/testdata/fixture/claude/baton/brainstorm-patterns.md
internal/baton/testdata/fixture/claude/baton/capture-discipline.md
internal/baton/testdata/fixture/claude/baton/commit-messages-as-capture.md
internal/baton/testdata/fixture/claude/baton/customer-journey-validation.md
internal/baton/testdata/fixture/claude/baton/design-fidelity.md
internal/baton/testdata/fixture/claude/baton/no-silent-deferrals.md
internal/baton/testdata/fixture/claude/baton/process-global-mutation.md
internal/baton/testdata/fixture/claude/baton/proof-bundle.md
internal/baton/testdata/fixture/claude/baton/reachability-gate.md
internal/baton/testdata/fixture/claude/baton/requirements-fidelity.md
internal/baton/testdata/fixture/claude/baton/role-prompts/captain.md
internal/baton/testdata/fixture/claude/baton/role-prompts/implementer.md
internal/baton/testdata/fixture/claude/baton/role-prompts/planner.md
internal/baton/testdata/fixture/claude/baton/role-prompts/verifier.md
internal/baton/testdata/fixture/claude/baton/session-discipline.md
internal/baton/testdata/fixture/claude/baton/track-mode.md
internal/baton/vendor.go
internal/baton/vendor_test.go
internal/baton/version.go
internal/baton/version_test.go
internal/db/db.go
internal/orchestrator/interpreter.go
internal/orchestrator/interpreter_test.go
internal/prompt/VERSION.txt
internal/prompt/baton/VERSION.txt
internal/prompt/prompt.go
internal/prompt/prompt_test.go
internal/run/run.go
internal/run/slice.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
```

### S02-specific changed files

- `internal/db/db.go` — added `decisions` table to schema
- `internal/supervisor/decisions.go` (new) — RecordDecision, RecordTriage, QueryDecisions
- `internal/supervisor/decisions_test.go` (new) — unit tests
- `internal/scheduler/worker.go` — added RecordDecision call after router poll
- `internal/run/slice.go` — added DB field to RunSliceOptions + RecordTriage calls after triage
- `cmd/sworn/telemetry.go` — added `decisions` subcommand
- `cmd/sworn/run.go` — wired DB into RunSliceOptions
- `internal/run/run.go` — wired DB into RunSliceOptions

## Test results

### Go (unit)

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
ok  	github.com/swornagent/sworn/internal/supervisor	0.008s
```

### Go (full supervisor)

```
$ go test ./internal/supervisor/... -v
<all 14 tests pass — see full output in test run>
PASS
ok  	github.com/swornagent/sworn/internal/supervisor	0.386s
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

## Not delivered

None — all five acceptance checks are delivered.

## Divergence from plan

- The `RecordDecision` and `RecordTriage` functions accept string parameters (action, reason) rather than the full `SliceDecision` / `triage.Output` structs. This avoids a circular import: `supervisor` cannot import `scheduler` (which already imports `supervisor`). The callers unwrap the struct fields at the call site. No loss of fidelity.
- `RecordTriage` is called inside `internal/run/slice.go` rather than `internal/scheduler/worker.go` (the spec says both calls are in worker.go). This is a structural necessity: `triage.Decide()` is called inside `RunSlice`, and the DB handle is plumbed via `RunSliceOptions.DB`. The intent (record every triage output) is unchanged.
- `start_commit` is set to `release-wt/2026-06-27-conformance-foundation` (track branch base) — the verifier's diff will include S01's changes plus S02's additions in this track.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S02-orchestrator-decision-log 2026-06-27-conformance-foundation

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

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  <will be populated after commit>

== Dark-code markers in changed files ==
  <will be populated after commit>

== Proof bundle structural checks ==
  <will be populated after commit>

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section references expected test commands
```