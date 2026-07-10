# Journal — S07-scheduler-failfast

## 2026-07-10 — Implementer session, design_review → implemented

**Entry state:** `design_review`, Coach acknowledgement already committed
(`captain-proceed.md` @1cd6601 on this branch, forward-synced to the track
via @27459cc). Verified the ack exists and read all four pin dispositions
before writing code.

**Pin 1 (escalate, AC-01 captain policy).** spec.json AC-01 on this branch
was already amended (@e8e8858, forward-merged from release-wt) to fail-open
the captain leg — matching S06's ratified per-attempt policy exactly. No
further spec edit needed; implemented the startup sweep in
`cmd/sworn/run.go` to match: an implementer/verifier/escalation-entry
resolution failure exits non-zero before any worker spawns; a captain-leg
failure is a stderr warning that does not block startup. Recorded as D4
(Type-1) in `status.json.design_decisions`.

**Pin 2 (memory-cited, design_decisions).** Populated `design_decisions`
(D1-D4) in `status.json` before the `design_review → in_progress`
transition, following the S04/S05/S06 record shape. Committed separately
(`c06c13d`) so the transition commit carries only the decision record, not
code.

**Pin 3 (mechanical, TestRunParallel_FailureCascade characterisation).**
Confirmed by reading the live test body: it asserted only `err != nil` and
`strings.Contains(err.Error(), "T1")` — nothing about T2's or T3's actual
outcome, contradicting design.md's original claim that it already asserted
"the dependent T3 is skipped". Corrected design.md's prose and extended the
test to read the durable per-run loop log
(`.sworn/logs/<release>/loop.log`) for both `"[T2] result: PASS"` (sibling
survives — no phase-wide cascade cancel) and `"[T3] result: SKIPPED"`
(actual dependent is gated). Ran the extended test and confirmed both lines
are emitted exactly as expected.

**Pin 4 (mechanical, touchpoint expansion).** `internal/run/resolve.go`,
`internal/run/resolve_test.go`, `internal/run/slice.go`, and
`internal/run/imports_test.go` are outside spec.json's `touchpoints` array.
Per the Coach's pre-authorisation (`captain-proceed.md` pin 4), recorded
each as a Coach-acknowledged divergence in `proof.json` rather than routing
through `/replan-release` — the design's own justification (pure
extraction / regression guard, no behavioural change) held up: `RunSlice`'s
error text and behaviour are byte-identical pre/post-refactor
(`TestRunSliceResolutionFailure`,
`TestRunSliceCaptainResolutionFailureDefersAndProceeds` pass unedited).

## Implementation approach

Extracted `ComposeEscalationModels` and `ResolveDispatch` into a new
`internal/run/resolve.go`, called from both `RunSlice` (pure refactor) and
a new pre-flight sweep in `cmd/sworn/run.go`'s `--parallel` branch — one
composition/resolution path for both "resolvable at startup" and
"resolvable per-attempt", per design D1.

AC-01's reachability test (`TestParallelStartupFailFast`,
`cmd/sworn/run_test.go`) drives `cmdRun` end-to-end (Rule 1): an
unregistered escalation-model prefix is rejected with non-zero exit before
`openDefaultDB`/`RunParallel` run, proven both by the exit code and by zero
rows in the `tracks` table (no worker's `supervisor.Acquire` ever ran).

AC-02's coverage clause closed by adding `../../cmd/sworn` to
`internal/run/imports_test.go`'s `scannedPackages` — `TestNoWireImports`
now scans `cmd/sworn` too; it was empty of wire-type references already
(no fix, pure regression guard), confirmed by the test passing unchanged.

AC-03 proven at two levels: (1) `internal/scheduler/worker_test.go`'s new
`TestRunTrack_TerminalDriverErrorHaltsTrack` — a fake `RunSliceFn`
dispatches twice successfully then returns a `run.IsBlocked`-shaped
(`"RunSlice: verification blocked:"` prefix) terminal error; `RunTrack`
returns `TrackFail` immediately, and the 4th slice in the track is never
attempted. (2) `internal/run/parallel_test.go`'s extended
`TestRunParallel_FailureCascade` — T1 fails, its independent same-phase
sibling T2 still reaches `TrackPass` in the same `RunParallel` call
(read from the loop log, since `RunParallel`'s return value only reports
`failedTracks`), while the actual dependent T3 reaches `TrackSkipped`.

## Divergences from spec.json touchpoints (Coach-acknowledged, pin 4)

- `internal/run/resolve.go` (new) — the shared helper extraction (D1).
- `internal/run/resolve_test.go` (new) — unit coverage for the helper in isolation.
- `internal/run/slice.go` — replaced the inline resolution block with calls to the new helper; no behavioural change (verified: existing capabilities_test.go assertions pass unedited).
- `internal/run/imports_test.go` — added `cmd/sworn` to the AC-02 coverage scan.

## Test results

`go test -count=1 -timeout 300s ./cmd/sworn/... ./internal/scheduler/... ./internal/run/...` — PASS.
`go test -count=1 -timeout 300s ./...` (full suite) — PASS, all 45 packages.
`go vet` + `gofmt -l` over every changed file — clean.
Newline-eating-corruption grep over every changed `.go` file — clean.

## State transition

`design_review` → `in_progress` (commit `c06c13d`, start_commit) →
`implemented` (this session's final commit). Verification left `pending` —
this implementer session never marks a slice verified (Rule 7).
