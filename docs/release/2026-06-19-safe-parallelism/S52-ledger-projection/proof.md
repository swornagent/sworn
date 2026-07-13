---
title: Proof Bundle — S52-ledger-projection
description: Rule 6 proof bundle. Generated from live repo state — not recollection.
---

# Proof Bundle: `S52-ledger-projection`

## Scope

A maintainer runs the sync and the repo gains a durable, git-tracked `docs/ledger/verdicts.jsonl` with one line per slice verdict — every PASS / FAIL / BLOCKED the board has ever recorded, including the resolved implementer model and attempt number.

## Files changed

```
$ git diff --name-only d2ab43d..HEAD
docs/release/2026-06-19-safe-parallelism/S52-ledger-projection/status.json
internal/ledger/ledger.go
internal/ledger/ledger_test.go
internal/run/slice.go
internal/state/state.go
internal/state/state_test.go
```

## Test results

### Go

```
$ go test ./internal/ledger/... ./internal/state/... ./internal/run/... -v -count=1
=== RUN   TestProject_Pass
--- PASS: TestProject_Pass (0.00s)
=== RUN   TestProject_Fail
--- PASS: TestProject_Fail (0.00s)
=== RUN   TestProject_Blocked
--- PASS: TestProject_Blocked (0.00s)
=== RUN   TestProject_Pending_NoVerdict
--- PASS: TestProject_Pending_NoVerdict (0.00s)
=== RUN   TestProject_EmptyResult_NoVerdict
--- PASS: TestProject_EmptyResult_NoVerdict (0.00s)
=== RUN   TestSliceKind
--- PASS: TestSliceKind (0.00s)
=== RUN   TestKey
--- PASS: TestKey (0.00s)
=== RUN   TestAppend_WritesLines
--- PASS: TestAppend_WritesLines (0.00s)
=== RUN   TestAppend_Idempotent
--- PASS: TestAppend_Idempotent (0.00s)
=== RUN   TestAppend_CreatesDir
--- PASS: TestAppend_CreatesDir (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/ledger	0.005s
=== RUN   TestTransition_LegalMoves
--- PASS: TestTransition_LegalMoves (0.00s)
...
=== RUN   TestVerification_ModelAttemptRoundTrip
--- PASS: TestVerification_ModelAttemptRoundTrip (0.00s)
=== RUN   TestVerification_ModelAttemptOmitEmpty
--- PASS: TestVerification_ModelAttemptOmitEmpty (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/state	0.006s
=== RUN   TestRunSlice
--- PASS: TestRunSlice (0.05s)
...
=== RUN   TestRetryFeedbackResolvesToPass
--- PASS: TestRetryFeedbackResolvesToPass (0.06s)
PASS
ok  	github.com/swornagent/sworn/internal/run	3.523s
```

### go vet

```
$ go vet ./internal/ledger/... ./internal/state/... ./internal/run/...
(no output — clean)
```

### go build

```
$ go build ./...
(no output — clean; go.mod unchanged, no new dependencies)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A — library package only; no CLI command in this slice. Reachability is through the `go test` suite and the `go build ./...` compile check.
- **User gesture**: N/A — backend library. The CLI consumer (`sworn ledger sync`) lands in S53. The verdict-record site (run/slice.go) is exercised by `TestRunSlice`, `TestRunSliceFail`, `TestRunSlice_BlockedNotifies`, etc., which persist model + attempt through the escalation loop.

## Delivered

- `ledger.Project` on a status with `verification.result: "fail"` returns a Record with `Verdict=="fail"`, the slice's release/track/slice_id, SliceKind derived from the track, GateCount equal to the count of `- [ ]` lines passed in, and `ok==true` — evidence: `TestProject_Fail` in `internal/ledger/ledger_test.go`
- `ledger.Project` on a `planned` slice with empty `verification.result` returns `ok==false` (no record emitted for un-verified slices) — evidence: `TestProject_Pending_NoVerdict`, `TestProject_EmptyResult_NoVerdict`
- `ledger.Append` writes exactly one JSON object per line; appending N records yields N lines; the file and `docs/ledger/` are created if absent — evidence: `TestAppend_WritesLines`, `TestAppend_CreatesDir`
- Appending a record whose `Key` already exists in the file is a no-op (idempotent re-sync); a second sync of an unchanged board adds zero lines — evidence: `TestAppend_Idempotent`
- `state.Verification` round-trips `Model` and `Attempt` through `state.Write`/`state.Read`; both are omitted from JSON when zero-valued (`omitempty`) — evidence: `TestVerification_ModelAttemptRoundTrip`, `TestVerification_ModelAttemptOmitEmpty`
- After a slice's verdict is recorded through `internal/run/slice.go`, its `status.json` `verification.model` and `verification.attempt` reflect the model + attempt index the escalation loop used — evidence: existing run tests (`TestRunSlice`, `TestRunSliceFail`, `TestRunSlice_BlockedNotifies`) continue to pass; Model + Attempt set at all three verdict-record sites (PASS line ~391, BLOCKED line ~435, haltFailedVerification line ~475)
- `go test ./internal/ledger/... ./internal/state/... ./internal/run/...` passes; no new external deps in `go.mod` (`go build ./...` succeeds without `go get`) — evidence: full test output above; `go.mod` unchanged

## Not delivered

- None. All 7 acceptance checks are delivered.

## Divergence from plan

- `SliceKind("T16-verdict-ledger")` returns `"verdict"` (first-segment rule), not `"ledger"` as the spec example suggests. The spec's examples are labelled as illustrative ("e.g."). The first-segment-with-depluralisation rule is mechanically consistent across all 22 tracks and produces the spec-example values for T3, T5, T8, T12. T16 is the sole divergence. If the planner intends `"ledger"` for T16, a future slice can add a literal mapping overlay. Noted in journal.md.

## First-pass script output

```
$ PLAYWRIGHT_OPTIN=false /home/user/.claude/bin/release-verify.sh S52-ledger-projection 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  PASS  state is 'implemented' — ready for verifier

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  6 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has all 8 required sections

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 24
  checks failed: 0

FIRST-PASS PASS
```