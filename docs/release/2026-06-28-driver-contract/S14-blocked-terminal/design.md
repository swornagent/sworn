# Design TL;DR — S14-blocked-terminal

**Slice:** S14-blocked-terminal / release 2026-06-28-driver-contract / track T4-resolution-loop
**State at design time:** planned. Designed against landed T4 code on this branch (S05 registry, S06 registry-dispatch rewire, S07 ComposeEscalationModels/ResolveDispatch + startup sweep, S08 Result cost fields).

## User outcome (restated)

BLOCKED is terminal-for-the-lane: an implementer returning an explicit blocked signal, or a
verifier BLOCKED verdict, ends all dispatches for that slice in the run, halts the owning
sequential track, consumes no retry budget, and the run exits non-zero with a report that
separates BLOCKED lanes (blocker text verbatim, routed to /replan-release) from FAIL lanes
(retries exhausted). FAIL retry semantics are untouched (AC-06 regression gate).

## What already exists (landed on this branch) vs what is missing

| Concern | Today | Gap |
| --- | --- | --- |
| Blocked vocabulary in the contract | `driver.Status` already has `StatusBlocked` (S01: "distinct from StatusError") | Declared, never emitted, never consumed; no field carries the blocker reason |
| Verify-leg BLOCKED | `orchestrator.Decide(Blocked) -> Halt` immediately, regardless of resolve budget (`TestBlockedIgnoresResolveBudget`); RunSlice writes `verification.result=blocked` + violations + routing, returns `"RunSlice: verification blocked:"`-prefixed error | Semantics correct; needs the AC-02 anchor test, nothing structural |
| Implement-leg blocked | Nothing. `implement.Run` ignores `Result.Status`; a `StatusBlocked` dispatch with nil error would fall through to proof generation and a **spurious `implemented` transition** | The core of the slice: consume `StatusBlocked` at the implement leg, terminal, zero further dispatches |
| Track halt | Worker returns on any RunSlice error, so subsequent slices never start — but the blocked error class is fingerprinted for the circuit breaker and reported as generic slice-failed/`track_failed` | Classify blocked-sentinel errors distinctly: no breaker fingerprint, no duplicate notify, blocked lane recorded |
| Exit report | `RunParallel` returns `"N track(s) failed"` — BLOCKED lanes are indistinguishable from FAIL lanes | New report section: BLOCKED lanes with verbatim blocker + `/replan-release` directive vs FAIL lanes |
| Resume behaviour | Router already routes a `verification.result=blocked` slice to `replan-release` (pause) | Unchanged — this slice covers the in-run path |

This is exactly the fired S05 failure: the agents correctly said BLOCKED, but only in prose;
the harness had no typed signal and re-dispatched. The fix is consumption of the typed signal
at every leg, never prose inference.

## Approach — five surgical changes

### 1. `internal/driver/driver.go` — blocked reason field (contract, additive)

The blocked *signal* already exists: `Status == StatusBlocked` (an enum member, per the spec's
"blocked boolean **or verdict enum member**"; no new boolean). Add one additive field:

```go
// BlockedReason is the blocker text, set when Status == StatusBlocked.
// The engine emits it VERBATIM (status.json violations, exit report) —
// never summarised, never truncated (R-03). The runner keys ONLY off
// Status; it never infers blockedness from ResultText prose.
BlockedReason string
```

Also tighten the `StatusBlocked` doc comment with the semantics binding:
`completed`-but-retryable = `StatusError` with non-terminal `ErrKind` (budget/env);
`StatusBlocked` = not clearable by re-dispatch (spec defect, out-of-authority change, missing
dependency) — terminal for the lane. Zero-value default (`""`, not blocked) keeps all four
drivers' behaviour byte-identical (R-02); no driver emits `StatusBlocked` this slice
(out-of-scope item 4 — subprocess CLIs map only what their harnesses expose).

### 2. `internal/implement/implement.go` — stop certifying blocked dispatches

Immediately after `d.Dispatch` returns (nil error), add:

```go
if res.Status == driver.StatusBlocked {
    return res, nil // skip spec record / proof / implemented transition
}
```

The slice's status stays `in_progress`; `RunSlice` owns the terminal handling (next item).
This also fixes the latent bug where a blocked dispatch would have been transitioned to
`implemented` and sent to the verifier.

### 3. `internal/run/slice.go` — implement-leg blocked consumption

After the existing `implErr` handling and directly after the implementer dispatch is recorded
into the cost ledger (economics survive a blocked dispatch, matching the S08 posture), insert
the blocked-terminal branch, mirroring the existing verifier-BLOCKED Halt branch:

- Fires only on `implRes.Status == driver.StatusBlocked`.
- `repo.Stage(".") + Commit("chore(run): implementer blocked — terminal for lane (replan required)")`
  so partial agent edits plus the status write leave a clean tree (same reason the normal path
  commits before verify).
- Writes `verification.result = "blocked"`, `verification.routing = "needs_planner"` (the
  machine-readable replan directive the S58 router consumes), `violations = [BlockedReason]`
  **verbatim** (fallbacks: `ResultText`, then `"(no blocker reason provided)"` — same pattern
  as the verifier-blocked fallback), `attempt`, `dispatches`.
- Notifies `state: "blocked"` via `opts.Notifier` (same shape as the existing blocked paths).
- Returns `errVerdictBlockedPrefix + " " + reason + " — route: /replan-release (BLOCKED is terminal for this lane)"`.
  The reason substring stays verbatim; the directive rides in the same error so the AC-01 unit
  test is self-contained.
- Returns **before** any triage call: `resolveCount`/`modelIdx` untouched, verifier never
  dispatched — exactly one dispatch total.

The existing verifier-BLOCKED branch, the proof-absent branch, and the first-pass branch are
left byte-identical (their error strings are asserted by existing tests); the only change in
that area is `errVerdictBlockedPrefix` becoming an alias of the shared sentinel constant (next
item) with the identical string value, so `IsBlocked` and every existing assertion hold.

### 4. Shared sentinel + `internal/scheduler/worker.go` — lane classification

`scheduler` cannot import `run` (run imports scheduler), so the sentinel moves to the package
both already import: `internal/orchestrator` gains

```go
// BlockedLaneSentinel — substring of every RunSlice blocked-terminal error.
const BlockedLaneSentinel = "RunSlice: verification blocked:"
```

(precedent: `orchestrator.InterpreterInconclusiveSentinel`, consumed the same way in
worker.go). `run.errVerdictBlockedPrefix = orchestrator.BlockedLaneSentinel` — value unchanged.

`WorkerOptions` gains a nil-safe side-channel:

```go
// RecordBlocked, when non-nil, is invoked once when a slice returns a
// blocked-terminal error: (trackID, sliceID, blocker text after the sentinel).
RecordBlocked func(trackID, sliceID, reason string)
```

In the worker's RunSliceFn-error handling (three sites: router implement/verify case, redesign
case, legacy loop — factored into one small helper to avoid triplicating), a new branch checked
**after** the pause sentinels and **before** circuit-breaker fingerprinting:

- `strings.Contains(err.Error(), orchestrator.BlockedLaneSentinel)` →
  log `[T] slice S BLOCKED — terminal for lane (replan required)`, invoke `RecordBlocked`,
  `releaseTrack("blocked")`, skip the breaker fingerprint (a blocked lane must not accrue
  breaker pages) and skip the worker's `track_failed` notify (RunSlice already sent the
  `blocked` notification — removes today's double-notify), then `return TrackFail`.

**Deliberate carrier choice (D3, see decisions):** the scheduler keeps returning `TrackFail`
for blocked lanes — no new `TrackResult` value. The blocked/failed distinction lives in the
`RecordBlocked` record, which is what the report consumes. Returning ends the track loop before
any subsequent slice starts (AC-04), and `TrackFail` still triggers `failCancel()` so dependent
tracks skip (they could never proceed past an unmergeable track).

### 5. `internal/run/parallel.go` — exit report

`RunParallel` builds a mutex-guarded `[]blockedLane{Track, Slice, Reason}` collector and wires
it as `WorkerOptions.RecordBlocked` (both the phase launch and the invariant-2 retry launch).
At outcome collection, failed tracks that have a blocked record render as BLOCKED lanes; the
rest stay FAIL lanes. When any blocked lane exists, the returned error (printed by `cmdRun`,
already exit 1) carries the report:

```
RunParallel: 1 lane(s) BLOCKED (replan required), 1 track(s) failed
BLOCKED lanes — terminal for this run; route to /replan-release:
  [T4-resolution-loop] S05-section-owned-saves: <blocker text verbatim>
      -> /replan-release 2026-06-28-driver-contract
FAIL lanes — retries exhausted:
  [T2-other]
```

The same text is written to the loop log (`lw`). When **no** blocked lane exists the error
format stays byte-identical to today's `"RunParallel: %d track(s) failed: %s"` (protects
`TestRunParallel_FailureCascade` and any caller matching on it). `cmd/sworn/run.go` needs no
change — it already prints the returned error and exits non-zero; I do not plan to touch it
(declared touchpoint left unused unless the implementation finds the report needs a direct
print site).

## Key design decisions

- **D1 (contract surface — flag for Captain, otherwise Type-2 additive).** Blocked signal =
  the existing `Status` enum member + new `Result.BlockedReason` string. Rejected: a separate
  `Blocked bool` (two sources of truth for one fact on the same struct). Touches the S01
  contract file all four drivers implement, but zero-value semantics are unchanged (R-02
  mitigation) and the ErrKindAuth-style vocab-binding precedent applies.
- **D2 (Type-2).** Sentinel constant home = `internal/orchestrator`, value identical to the
  current private string. Rejected: duplicating the literal in scheduler (drift risk), moving
  it to `internal/verdict` (it is a RunSlice error-shape fact, and orchestrator already hosts
  the analogous interpreter sentinel).
- **D3 (Type-2, the load-bearing one).** No new `TrackResult` value; `TrackFail` remains the
  scheduler carrier and the `RecordBlocked` side-channel carries the distinction to the
  report. Rationale: `TestRunTrack_TerminalDriverErrorHaltsTrack` (S07 AC-03) asserts
  `TrackFail` on exactly this error shape, and `TrackBlocked` is already taken by invariant-2
  — a new enum value forces edits to existing scheduler tests and supervisor-state plumbing
  for zero AC value (no AC requires a distinct scheduler enum; AC-05 requires a distinct
  *report*). Dependent-phase skip via `failCancel` is inherited for free. Rejected
  alternative recorded here for the reviewer: `TrackBlockedLane TrackResult = "blocked_lane"`.
- **D4 (Type-2).** `implement.Run` early-returns on `StatusBlocked` with nil error and no
  state transition (slice stays `in_progress`); RunSlice owns terminal status/commit/notify.
  Keeps implement.Run's "never certifies" contract and one blocked-handling site.
- **D5 (Type-2).** Report is embedded in RunParallel's returned error + loop log; no new
  printing surface. TUI/board surfacing is explicitly out of scope.
- **D6 (Type-2).** Blocked lanes skip circuit-breaker fingerprinting and the worker-level
  `track_failed` notification (RunSlice already emitted the `blocked` notification; removes
  an existing double-notify on verifier-blocked lanes).

## Consequence worth the reviewer's eye (pin)

The S09/S07 terminal driver errors (auth/credits) already return sentinel-prefixed errors, so
under this design they render as BLOCKED lanes in the exit report (with their "check provider
credentials" reason) rather than FAIL lanes. This is semantically correct — they are not
clearable by re-dispatch — and their scheduler-level result (`TrackFail`) is unchanged, so
`TestRunTrack_TerminalDriverErrorHaltsTrack` stays green unmodified. Flagging because it widens
the report's BLOCKED section beyond verifier/implementer verdicts.

## Files to touch

| File | Change |
| --- | --- |
| `internal/driver/driver.go` | `Result.BlockedReason` + `StatusBlocked` semantics doc |
| `internal/orchestrator/triage.go` (or sibling `blocked.go`) | `BlockedLaneSentinel` const |
| `internal/implement/implement.go` | early return on `StatusBlocked` |
| `internal/run/slice.go` | implement-leg blocked-terminal branch; alias prefix to shared sentinel |
| `internal/scheduler/worker.go` | blocked-error classification helper (3 sites), `WorkerOptions.RecordBlocked` |
| `internal/run/parallel.go` | blocked-lane collector, exit-report sections |
| `internal/run/blocked_terminal_test.go` (new) | AC-01/AC-02/AC-03 tests |
| `internal/run/blocked_report_test.go` (new) | AC-05 test |
| `internal/scheduler/blocked_lane_test.go` (new) | AC-04 test |

All new tests live in **new files**; zero existing test files are edited (AC-06). New tests
reuse the existing package-level fixtures (temp git repo + fake registry drivers in
`internal/run`, fake RunSliceFn + in-memory sqlite in `internal/scheduler`). Declared
touchpoints I do **not** plan to change: `internal/run/resolve.go`, `internal/run/parallel_test.go`,
`internal/verify/`, `cmd/sworn/run.go`, `cmd/sworn/run_test.go`.

## AC traceability

| AC | Planned change | Test (new) |
| --- | --- | --- |
| AC-01 implementer blocked ⇒ one dispatch, lane halt, verbatim + replan directive | items 1–3 | `TestLoopBlockedImplementerTerminal` — fake implementer driver returns `{StatusBlocked, BlockedReason}` (fired S05 replay, synthetic verdicts); assert 1 implementer dispatch, 0 verifier dispatches, `IsBlocked(err)`, reason verbatim in error and in `status.json` violations, `routing=needs_planner`, error carries `/replan-release` |
| AC-02 verifier BLOCKED ⇒ identical terminal, no budget consumed, never FAIL | existing halt path (anchored) | `TestLoopBlockedVerifierTerminal` — 2-model escalation list with retries remaining; verifier driver emits BLOCKED verdict JSON; assert 1 implementer + 1 verifier dispatch, terminal, state ≠ `failed_verification`, `verification.result=blocked` |
| AC-03 FAIL with retries ⇒ unchanged retry semantics | no code change | `TestLoopFailRetrySemanticsUnchanged` — FAIL then PASS; assert 2 implementer dispatches, violations text forwarded in dispatch-2 payload, retry consumed |
| AC-04 blocked slice halts track | item 4 | `TestLoopBlockedSliceHaltsTrack` — 2-slice track, slice 1 returns blocked-sentinel error; assert slice 2 never dispatched, `RecordBlocked` invoked once with verbatim reason |
| AC-05 exit report distinguishes BLOCKED vs FAIL, non-zero | item 5 | `TestLoopExitReportBlockedVsFail` — two tracks, one blocked one plain-fail; assert error non-nil, BLOCKED section carries verbatim reason + `/replan-release`, FAIL section names the other track |
| AC-06 existing retry tests green unmodified | design constraint throughout (D3 chiefly) | full `go test -count=1 -timeout 300s ./...`; protected set includes `TestRun_FailThenPass_RetrySucceeds`, `TestRetryPassesVerifierRationale`, `TestRetryFeedbackResolvesToPass`, `TestRunSliceFail`, `TestRunSlice_FailNotifiesOnce`, `TestImplementTimeoutEscalates`, `TestTerminalError_*`, all `internal/orchestrator` triage tests, `TestRunTrack_SliceFail`, `TestRunTrack_TerminalDriverErrorHaltsTrack`, `TestRunParallel_FailureCascade` |

## Design-level risks

- **R-01 (retry/escalation bleed):** blocked branch returns before any triage/budget mutation;
  AC-01/AC-02 assert dispatch counts, AC-03/AC-06 hold the FAIL side. `orchestrator.Decide` is
  not modified at all.
- **R-02 (S01 contract file):** additive field, zero-value = today; full suite + driver
  conformance tests gate the track merge.
- **R-03 (verbatim blocker):** one string, carried by `BlockedReason` → status violations →
  error suffix → `RecordBlocked` → report, no truncation anywhere on that path (the existing
  200-char truncation applies only to the notification summary, unchanged); tests assert
  substring presence end-to-end.
- **Sentinel breadth:** `strings.Contains` classification means any future error embedding the
  sentinel text becomes a blocked lane — same exposure the existing pause sentinels carry;
  acceptable and consistent.

## Effort/complexity

Agree with the spec's rating: low effort / high complexity ("puzzle") — small surface, but
every line threads through the just-landed S06/S07 state machine. Will set
`confirmed_by_implementer: true` at the in_progress transition.
