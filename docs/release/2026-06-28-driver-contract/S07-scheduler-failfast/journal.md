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

## Verifier verdicts received

### 2026-07-10 — PASS (fresh-context verifier)

```
PASS

Slice: `S07-scheduler-failfast`
Verified against: `3aac7aee4f302850920ce1708406800c2325491a`
Verifier session: `fresh, artefact-only`
```

Gate walk (fresh session, artefact-only inputs, all tests re-run live):

1. Gate 1 (user-reachable outcome) — PASS. The startup resolution sweep is
   wired into the real `sworn run --parallel` entry point (cmd/sworn/run.go,
   --parallel branch, before openDefaultDB/supervisor.Open/RunParallel).
   TestParallelStartupFailFast drives cmdRun end-to-end: unregistered
   escalation prefix "nope/model-x" → non-zero exit naming the model, role
   "implementer", and every registered prefix; zero rows in the tracks table
   proves no worker ever spawned. Re-run live: PASS.
2. Gate 2 (touchpoints) — PASS with explanations. Four files beyond
   spec.json touchpoints (internal/run/resolve.go, resolve_test.go,
   slice.go, imports_test.go) were pre-authorised by the Coach
   (captain-proceed.md pin 4) and recorded in proof.json divergence.
   internal/run/parallel.go (planned) is unchanged per ratified design
   decision D3 (sweep lives in cmd/sworn/run.go; parallel.go kept free of a
   second resolution path) — no AC required a parallel.go change and its
   planned test file parallel_test.go WAS extended. No unrelated churn.
3. Gate 3 (tests) — PASS. Re-run live in the track worktree:
   go build ./... clean; go test -count=1 -timeout 300s ./cmd/sworn/...
   ./internal/scheduler/... ./internal/run/... all ok (AC-04); full
   go test -count=1 -timeout 300s ./... = 45 packages ok, exit 0.
   Named tests re-run verbose, all PASS: TestParallelStartupFailFast,
   TestRunTrack_TerminalDriverErrorHaltsTrack,
   TestRunParallel_FailureCascade (T2 PASS sibling survives / T3 SKIPPED
   dependent, asserted from loop.log), TestNoWireImports (now scanning
   cmd/sworn — AC-02), TestComposeEscalationModels, TestResolveDispatch_*
   (verifier/implementer fatal, captain non-fatal), and the pre-existing
   TestRunSliceResolutionFailure +
   TestRunSliceCaptainResolutionFailureDefersAndProceeds pass unedited
   (extraction is behaviour-preserving).
   Gate 3b/4b (LLM checks) skipped non-blocking: no $SWORN_MODEL configured.
   Manual AC walk: AC-01 graded against the AMENDED spec text (Coach
   2026-07-10, S07 captain-proceed.md pin 1 propagating the S06
   ratification): fatal legs exit non-zero pre-spawn; captain-leg failure
   warns and proceeds, durable Rule 2 recording owned by RunSlice's
   recordDesignGateDeferral per-slice — exactly the S06-ratified mechanism
   the amended AC cites. AC-02: newAgentFromModel/newVerifierFromModel exist
   only in comments; import-boundary net extended to cmd/sworn. AC-03:
   fake-driver-after-N-dispatches scheduler test halts only its own track;
   no phase-wide cascade. AC-04: green.
4. Gate 4 (reachability artefact) — PASS. cli-run artefact re-executed
   live; names the user gesture (`sworn run --parallel` with a bad
   escalation model).
5. Gate 5 (silent deferrals) — PASS. TODO/FIXME/placeholder grep clean on
   all changed files; newline-corruption grep clean; gofmt -l clean; go vet
   clean. sworn#86 confirmed to exist (OPEN) as the tracking leg for the
   captain-leg deferral policy. proof.json not_delivered empty;
   status.json has no open_deferrals; spec out_of_scope items all name
   owners.
6. Gate 6 (design conformance) — EXEMPT: sworn designaudit reports project
   not ui_bearing.
7. Gate 7 (claimed scope) — PASS. Every delivered item's evidence reference
   verified against live repo state, including the corrected design.md
   AC-03 coverage claim (Captain review pin 3) and the D1 shared-helper
   extraction.

Deterministic first-pass (~/.claude/bin/release-verify.sh): only FAILs are
the known spec.md/proof.md false negatives on spec-v1 slices (declared
project hazard); dark-code scan and structural checks pass.
