---
title: S54-ledger-routing proof bundle
description: Rule 6 proof bundle, generated from live repo state.
---

# Proof Bundle: `S54-ledger-routing`

## Scope

Turns the verdict ledger into a routing signal: a recommendation engine over
`docs/ledger/verdicts.jsonl` exposed as `sworn ledger recommend`, and a wire
into S09's `ResolveImplementerModel` so the resolved default is the model with
the best measured pass-rate for the slice kind, falling back to current
behaviour when the corpus is thin.

## Files changed

```
$ git diff --name-only a3170c0
cmd/sworn/ledger.go
cmd/sworn/run.go
docs/release/2026-06-19-safe-parallelism/S54-ledger-routing/status.json
internal/config/config.go
internal/config/config_test.go
internal/ledger/routing.go
internal/ledger/routing_test.go
```

## Test results

### Go — ledger package

```
$ go test -v -count=1 ./internal/ledger/...
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
=== RUN   TestPassRateByModelKind
--- PASS: TestPassRateByModelKind (0.00s)
=== RUN   TestPassRateByModelKind_Empty
--- PASS: TestPassRateByModelKind_Empty (0.00s)
=== RUN   TestPassRateByModelKind_Sorting
--- PASS: TestPassRateByModelKind_Sorting (0.00s)
=== RUN   TestAttemptsToPass
--- PASS: TestAttemptsToPass (0.00s)
=== RUN   TestAttemptsToPass_Empty
--- PASS: TestAttemptsToPass_Empty (0.00s)
=== RUN   TestAttemptsToPass_SkipsZeroAttempt
--- PASS: TestAttemptsToPass_SkipsZeroAttempt (0.00s)
=== RUN   TestGateFailureHistogram
--- PASS: TestGateFailureHistogram (0.00s)
=== RUN   TestGateFailureHistogram_Empty
--- PASS: TestGateFailureHistogram_Empty (0.00s)
=== RUN   TestGateFailureHistogram_OnlyPasses
--- PASS: TestGateFailureHistogram_OnlyPasses (0.00s)
=== RUN   TestLoad_EmptyFile
--- PASS: TestLoad_EmptyFile (0.00s)
=== RUN   TestLoad_MissingFile
--- PASS: TestLoad_MissingFile (0.00s)
=== RUN   TestLoad_RoundTrip
--- PASS: TestLoad_RoundTrip (0.00s)
=== RUN   TestLoad_SkipsMalformed
--- PASS: TestLoad_SkipsMalformed (0.00s)
=== RUN   TestReport_Render
--- PASS: TestReport_Render (0.00s)
=== RUN   TestReport_RenderEmpty
--- PASS: TestReport_RenderEmpty (0.00s)
=== RUN   TestRecommendModel_RanksByPassRate
--- PASS: TestRecommendModel_RanksByPassRate (0.00s)
=== RUN   TestRecommendModel_TieBreakByAttempts
--- PASS: TestRecommendModel_TieBreakByAttempts (0.00s)
=== RUN   TestRecommendModel_BelowMinSample
--- PASS: TestRecommendModel_BelowMinSample (0.00s)
=== RUN   TestRecommendModel_NoRecordsForKind
--- PASS: TestRecommendModel_NoRecordsForKind (0.00s)
=== RUN   TestRecommendModel_EmptyRecords
--- PASS: TestRecommendModel_EmptyRecords (0.00s)
=== RUN   TestRecommendModel_SkipsNonTerminalVerdicts
--- PASS: TestRecommendModel_SkipsNonTerminalVerdicts (0.00s)
=== RUN   TestRecommendation_FieldsRoundTrip
--- PASS: TestRecommendation_FieldsRoundTrip (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/ledger	0.008s
```

### Go — config package

```
$ go test -v -count=1 ./internal/config/...
...
=== RUN   TestResolveImplementerModel_FlagWins
--- PASS: TestResolveImplementerModel_FlagWins (0.00s)
=== RUN   TestResolveImplementerModel_EnvFallback
--- PASS: TestResolveImplementerModel_EnvFallback (0.00s)
=== RUN   TestResolveImplementerModel_ConfigFallback
--- PASS: TestResolveImplementerModel_ConfigFallback (0.00s)
=== RUN   TestResolveImplementerModel_EscalationFallback
--- PASS: TestResolveImplementerModel_EscalationFallback (0.00s)
=== RUN   TestResolveImplementerModel_Error
--- PASS: TestResolveImplementerModel_Error (0.00s)
=== RUN   TestResolveImplementerModel_LedgerDefault
--- PASS: TestResolveImplementerModel_LedgerDefault (0.00s)
=== RUN   TestResolveImplementerModel_LedgerFlagWins
--- PASS: TestResolveImplementerModel_LedgerFlagWins (0.00s)
=== RUN   TestResolveImplementerModel_LedgerThinCorpusFallback
--- PASS: TestResolveImplementerModel_LedgerThinCorpusFallback (0.00s)
=== RUN   TestResolveImplementerModel_LedgerAbsentCorpusFallback
--- PASS: TestResolveImplementerModel_LedgerAbsentCorpusFallback (0.00s)
=== RUN   TestResolveImplementerModel_LedgerEmptySliceKind
--- PASS: TestResolveImplementerModel_LedgerEmptySliceKind (0.00s)
...
PASS
ok  	github.com/swornagent/sworn/internal/config	0.023s
```

### Go — cmd/sworn (ledger/recommend tests)

```
$ go test -count=1 -run 'TestLedger|TestRecommend|TestResolveImplementer' ./cmd/sworn/...
ok  	github.com/swornagent/sworn/cmd/sworn	0.022s
```

### Build

```
$ go build ./...
(exit 0, no output)
```

### go vet

```
$ go vet ./internal/ledger/... ./internal/config/... ./cmd/sworn/...
(exit 0, no output)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: `sworn ledger recommend <kind>` CLI output against the real corpus
- **User gesture**: "User runs `sworn ledger recommend harness`. CLI prints the
  ranked model recommendation with its pass-rate and sample size, or a message
  that the corpus is too thin."

This is a backend-only slice (no UI screenshots). The Rule 1 reachability gate
is satisfied at the `cmd/sworn` integration point — `TestResolveImplementerModel_LedgerDefault`
proves the resolver returns the ledger-recommended model end-to-end (loading
from a real temp JSONL file, calling through the actual `ResolveImplementerModel`
function the harness uses), and the `sworn ledger recommend` subcommand proves
the CLI affordance.

## Delivered

- [x] `RecommendModel` over a corpus where model A passes 9/10 harness slices and
  model B passes 3/10 returns A with its pass-rate and sample size, `ok==true`
  — evidence: `TestRecommendModel_RanksByPassRate` in `internal/ledger/routing_test.go`
- [x] `RecommendModel` below the minimum-sample threshold returns `ok==false`
  — evidence: `TestRecommendModel_BelowMinSample` in `internal/ledger/routing_test.go`
- [x] `sworn ledger recommend harness` prints the ranked model with pass-rate +
  sample; with no kind argument it prints usage and exits non-zero
  — evidence: `cmdLedgerRecommend` in `cmd/sworn/ledger.go`, dispatch wired in `runLedger`
- [x] `ResolveImplementerModel` returns the ledger-recommended model as the
  default when the corpus is confident AND no flag/env override is set
  — evidence: `TestResolveImplementerModel_LedgerDefault` in `internal/config/config_test.go`
- [x] With an explicit `--model` flag or `$SWORN_IMPLEMENTER_MODEL`,
  `ResolveImplementerModel` returns the override unchanged
  — evidence: `TestResolveImplementerModel_LedgerFlagWins` in `internal/config/config_test.go`
- [x] With an absent or thin `docs/ledger/verdicts.jsonl`, `ResolveImplementerModel`
  returns exactly what S09 returned before this slice
  — evidence: `TestResolveImplementerModel_LedgerThinCorpusFallback` + `TestResolveImplementerModel_LedgerAbsentCorpusFallback` in `internal/config/config_test.go`
- [x] `go test ./internal/ledger/... ./internal/config/... ./cmd/sworn/...`
  passes; `go build ./...` succeeds with no new `go.mod` deps
  — evidence: all test suites pass (see Test results above), no new imports beyond stdlib + existing `internal/ledger`

## Not delivered

None — all 7 acceptance checks are delivered.

## Divergence from plan

None — implementation matches spec exactly. Every planned touchpoint was touched;
no unplanned files were modified.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S54-ledger-routing 2026-06-19-safe-parallelism
FIRST-PASS PASS — 23 checks passed, 0 failed
```