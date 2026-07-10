# Design TL;DR — S07-scheduler-failfast

## User outcome (from spec.json)

The parallel loop resolves every role-and-model combination to a driver at
startup and exits with a named, actionable error before any worker spawns —
a missing driver or capability can never crash or strand a run mid-flight;
workers dispatch only through the driver-backed RunSlice.

## Grounding: what S06 already landed (verified, this branch)

S06-loop-dispatch-rewire (state `verified`, `internal/run/slice.go`) already
performs a full role-resolution sweep **inside `RunSlice`**, immediately
after option validation (slice.go:259-284):

- `opts.Registry.Resolve(opts.VerifierModel, driver.RoleVerifier)` — hard
  error on failure (slice.go:269-272).
- Every entry of the composed escalation list resolved for
  `driver.RoleImplementer` — hard error on failure, naming the model, role,
  and registered alternatives (slice.go:273-280).
- `escalationModels[0]` resolved for `driver.RoleCaptain` — **failure is
  captured, not fatal** (slice.go:281-284); it is recorded as a Rule 2
  deferral via `recordDesignGateDeferral` and the run proceeds. This is a
  ratified Coach decision, not an oversight: S06's own AC-02 text was
  amended live (`docs/release/2026-06-28-driver-contract/S06-loop-dispatch-rewire/captain-proceed.md`
  pin 1, 2026-07-10) to read "If Resolve fails for the captain leg, RunSlice
  SHALL record that same descriptive role error as a durable Rule 2
  deferral" — specifically because **no subprocess driver declares
  `RoleCaptain` yet** (only the in-process drivers do); a hard captain-leg
  failure would make every subprocess-only configuration unrunnable.
  Tracked: sworn#86 (restore role-universality).

The terminal-error fail-fast this slice's AC-03 depends on is also landed:
`driver.TerminalErrKind(kind string) bool { return kind == ErrKindAuth ||
kind == ErrKindCredits }` (`internal/driver/subprocess.go:54-56`), consumed
at the implement leg (slice.go:525-529, returns an `errVerdictBlockedPrefix`-
tagged error — `run.IsBlocked`) and at the verify leg per S06 spec R-03. Both
consumers share the one predicate by design (subprocess.go:51-53 doc
comment) — this slice adds no new terminal-classification logic, it only
needs the mid-run halt to reach the scheduler cleanly (AC-03) and to prove
it stays track-scoped.

**What is missing, and is this slice's actual delta:**

1. **AC-01 — a TRUE pre-spawn sweep.** S06's sweep runs *inside* `RunSlice`,
   which is itself invoked from `opts.RunSliceFn(...)` at
   `internal/scheduler/worker.go:320,377,463` — i.e. from **inside an
   already-spawned worker goroutine** (`internal/run/parallel.go:339,409`
   launch `go func(){ … scheduler.RunTrack(…) … }()` before any
   `RunSliceFn` call happens). So a bad model resolves *after* every worker
   has started, spent a supervisor-acquire round-trip, etc. — not "before
   spawning any worker" as AC-01 requires literally. `cmd/sworn/run.go`'s
   `--parallel` branch (lines 118-168) resolves `impl`/`verifier`/
   `escalationModels` as **strings only** (config-level, lines 88-105) and
   never touches the registry before calling `run.RunParallel`.
2. **AC-02 — the factory helpers are already gone.** `grep -rn
   "newAgentFromModel\|newVerifierFromModel"` across `cmd/sworn/` and
   `internal/` returns only two doc-comment mentions (capabilities_test.go:12,
   run.go:340) — the functions themselves were deleted by S06. This AC is
   satisfied except for its test-coverage clause: "asserted by the S06
   import-boundary test extended to cmd/sworn's loop wiring." Today
   `TestNoWireImports` (`internal/run/imports_test.go`) scans
   `scannedPackages = []string{".", "../verify", "../scheduler"}` —
   `cmd/sworn` is not in that list.
3. **AC-03 — the mechanics are already correct; only the proof is missing.**
   `RunTrack` per track runs on the **parent** `ctx`, not `phaseCtx`
   (`internal/run/parallel.go:358-362,427-431`, tagged `#33` in the
   comments): a sibling's `failCancel()` cannot cancel an already-launched
   track mid-run. A `TrackFail` from one track's goroutine only gates
   *dependent* tracks in a later phase via the `phaseCtx.Err()` check at
   launch (parallel.go:301). Combined with S06's TerminalErrKind halt, a
   terminal driver error already surfaces as `Status=error`/`ErrKind` through
   `RunSlice`'s existing triage path for *that slice's track only*
   (`worker.go:320-365`: log, notify, `releaseTrack(StateFailed)`,
   `return TrackFail` — scoped to the one goroutine). There is currently no
   test proving this end-to-end with a fake driver; `AC-03` names one
   explicitly ("proven by a scheduler test with a fake driver that fails
   after N dispatches").

## Approach

**Extract, don't reimplement, the resolution logic S06 already wrote** — one
shared helper in `internal/run`, called both by `RunSlice` (replacing its
current inline block, pure refactor) and by a new pre-flight sweep in
`cmd/sworn/run.go`'s `--parallel` branch, so "resolvable at startup" and
"resolvable per-attempt" are structurally the same code path and can never
diverge (the same principle the codebase already applies to
`TerminalErrKind`: "one contract predicate at both consumption points").

New file `internal/run/resolve.go`:

```go
// ComposeEscalationModels builds the final ordered model list RunSlice
// resolves and dispatches against: implementerModel prepended (if set) to
// escalationModels, defaulting to DefaultEscalationModels when the result
// would otherwise be empty. Extracted from RunSlice (slice.go:250-257) so
// the startup sweep (S07 AC-01) composes the IDENTICAL list RunSlice itself
// will resolve per-attempt — a list built two different ways is exactly the
// kind of drift that would make "resolved at startup" a false promise.
func ComposeEscalationModels(implementerModel string, escalationModels []string) []string

// DispatchResolution is the outcome of resolving every role leg a slice
// dispatch touches, in one place.
type DispatchResolution struct {
    Verifier     driver.Driver
    Implementers []driver.Driver // parallel to the input escalationModels
    Captain      driver.Driver
    // CaptainErr is non-nil when the captain leg failed to resolve. Per the
    // S06 Coach ruling (captain-proceed.md pin 1, 2026-07-10) this is NEVER
    // fatal — callers log/record it as a Rule 2 deferral and proceed.
    CaptainErr error
}

// ResolveDispatch resolves the verifier, every entry of escalationModels
// (RoleImplementer), and escalationModels[0] (RoleCaptain) through reg.
// Verifier/implementer resolution failure returns err (fatal — S06 AC-02);
// captain resolution failure is returned via DispatchResolution.CaptainErr
// (non-fatal). errPrefix names the caller ("RunSlice" or "sworn run") so
// the wrapped error reads naturally at either call site; the wrapped text
// shape (%q model, %q role) is unchanged from RunSlice's existing wrap.
func ResolveDispatch(reg *registry.Registry, errPrefix, verifierModel string, escalationModels []string) (DispatchResolution, error)
```

`RunSlice` (slice.go:250-284) is rewritten to:

```go
escalationModels := ComposeEscalationModels(opts.ImplementerModel, opts.EscalationModels)
resolution, err := ResolveDispatch(opts.Registry, "RunSlice", opts.VerifierModel, escalationModels)
if err != nil {
    return err
}
verifierDriver := resolution.Verifier
implDrivers := resolution.Implementers
captainDriver, captainResolveErr := resolution.Captain, resolution.CaptainErr
```

— byte-identical error text to today (same prefix, same `%q`/`%q` shape),
so `TestRunSliceResolutionFailure` and
`TestRunSliceCaptainResolutionFailureDefersAndProceeds`
(`internal/run/capabilities_test.go`) assert unchanged and require no edits.

`cmd/sworn/run.go`'s `--parallel` branch gets a new block **before
`openDefaultDB()`** (before line 119 today) — failing before any DB/event
store opens, let alone any worker spawns:

```go
// ── Startup resolution sweep (S07 AC-01) ────────────────────────────
// Resolve every role RunSlice will need — implementer escalation chain,
// verifier, captain — through the registry BEFORE any worker spawns.
// Reuses run.ComposeEscalationModels/run.ResolveDispatch, the exact
// composition+resolution RunSlice performs per-attempt (S06 AC-02), so a
// model that resolves here is guaranteed to resolve identically inside
// every worker's RunSlice call.
startupModels := run.ComposeEscalationModels(impl, escalationModels)
startupReg := registry.Default(model.ProviderConfigFromEnv())
resolution, rerr := run.ResolveDispatch(startupReg, "sworn run", verifier, startupModels)
if rerr != nil {
    fmt.Fprintf(os.Stderr, "sworn run: %v\n", rerr)
    return 1
}
if resolution.CaptainErr != nil {
    fmt.Fprintf(os.Stderr,
        "sworn run: warning: %v — captain leg proceeds (S06 D2: no subprocess "+
            "driver declares RoleCaptain yet; sworn#86); recorded per-slice as "+
            "a design-gate deferral when the affected slice runs\n",
        resolution.CaptainErr)
}
```

AC-02's coverage clause is satisfied by adding `"../../cmd/sworn"` to
`internal/run/imports_test.go`'s `scannedPackages` — verified empty today
(no `model.ChatMessage`/`ToolDef`/`ChatResponse`/`ToolCall` selector in
`cmd/sworn/*.go`), so this is a pure regression guard, not a fix.

AC-03 gets a new test in `internal/scheduler/worker_test.go` — a fake
`RunSliceFn` standing in for a driver that dispatches successfully N times
then returns a `run.IsBlocked`-shaped (terminal `ErrKind`) error, proving
`RunTrack` halts that track immediately (`TrackFail`, no further retry
inside the worker loop — retry/escalation is `RunSlice`'s concern per
spec's explicit out-of-scope, this test only proves the scheduler *surfaces*
the terminal error and stops) — and an extension of the existing
`TestRunParallel_FailureCascade` (`internal/run/parallel_test.go:197`),
which already fixtures T2 as an independent same-phase sibling of the
failing T1 and T3 as the dependent (`depends_on: [T1]`), but — per Captain
review pin 3 (2026-07-10) — its ONLY existing assertions are `err != nil`
and `strings.Contains(err.Error(), "T1")`; it asserts **nothing** about
either T2's or T3's actual per-track outcome today. (Design's original
draft claimed the base test "asserts the dependent T3 is skipped" — that
claim did not hold against the live test body and is corrected here.) The
extension reads the durable per-run loop log
(`.sworn/logs/<release>/loop.log`, `"[<track>] result: <OUTCOME>"` lines —
`RunParallel`'s own outcome-reporting sink; its return value only reports
`failedTracks`, not skip/pass detail) and asserts BOTH `"[T2] result: PASS"`
(no phase-wide cascade cancel — the sibling completes in the same
`RunParallel` call where T1 fails) AND `"[T3] result: SKIPPED"` (the actual
dependent is gated) are present.

## Key design decisions

**D1 — Shared resolution helper (`internal/run/resolve.go`), not a
duplicated sweep.** Two independent implementations of "resolve verifier +
escalation list + captain" would let the startup sweep and RunSlice's
per-attempt resolution silently disagree (e.g. a future edit to one and not
the other). Extracting once and calling from both closes that class of bug
structurally, mirroring the existing `TerminalErrKind` "one predicate, two
consumers" pattern in this same track.

**D2 — Registry re-constructed, not threaded through `ParallelOptions`.**
`registry.Default(cfg)` is a cheap, stateless build (struct literals +
`exec.LookPath`/env reads inside probes, no dispatch) — `RunSlice` already
calls it fresh per invocation when `opts.Registry` is nil (slice.go:201-203).
The startup sweep builds its own `registry.Default(model.ProviderConfigFromEnv())`
in `cmd/sworn/run.go`, same construction RunSlice's own default falls back
to. Threading a shared `*registry.Registry` through `ParallelOptions` →
`WorkerOptions` → `RunSliceOptions` would touch far more surface for a
build that costs nothing to repeat, and `ParallelOptions.RunSliceFn` is
already the seam that owns per-worker registry choice (test fakes inject
their own).

**D3 — Startup sweep placed in `cmd/sworn/run.go`, not inside
`run.RunParallel`.** `ParallelOptions` carries no model identifiers today —
they live only inside the `RunSliceFn` closure `cmd/sworn/run.go` builds
(lines 134-144). Pushing model-aware resolution into `RunParallel` would
mean adding `VerifierModel`/`ImplementerModel`/`EscalationModels` fields to
`ParallelOptions` purely to re-derive what the closure already captured —
duplicate state that can drift from the closure's own composition. Sweeping
in `cmd/sworn/run.go`, immediately before the `RunParallel` call (and before
`openDefaultDB`/`supervisor.Open`), satisfies AC-01's "before spawning any
worker" literally (workers spawn only inside `RunParallel`'s phase loop) and
keeps `internal/run/parallel.go` free of a second, parallel model-resolution
path. This also matches Rule 1 (Reachability Gate): the affordance is the
`sworn run --parallel` CLI flag, so the first failing test
(`TestParallelStartupFailFast`, `cmd/sworn/run_test.go`) drives `cmdRun`
end-to-end — following the existing `TestCmdRun_Parallel` fixture pattern
(cmd/sworn/run_test.go:72) with one escalation/verifier model swapped for an
unregistered prefix (e.g. `"nope/model-x"`) and asserting (a) non-zero exit,
(b) the DB's `tracks` table has **zero** acquired rows (proving no worker
ever started), matching the AC-01 text precisely.

## Design risk requiring explicit ratification (escalate)

**AC-01's literal text includes "captain" in the fail-fast set; the
just-ratified S06 precedent (this same track, captain-proceed.md pin 1,
2026-07-10) makes captain-leg resolution failure explicitly non-fatal.**
S07's spec.json AC-01 reads: "...resolve the implementer, verifier, AND
CAPTAIN models AND every escalation-list entry ... BEFORE spawning any
worker; **on any failure it SHALL exit non-zero**..." — read literally, a
captain-only resolution failure at startup would need to hard-fail the run.
That directly contradicts S06's ratified AC-02 amendment ("If Resolve fails
for the captain leg, RunSlice SHALL record that same descriptive role error
as a durable Rule 2 deferral ... and proceeds"), landed on this exact branch
minutes before this slice's design. Enforcing AC-01 literally would mean the
startup sweep and every worker's in-flight `RunSlice` call enforce
**opposite** policies for the identical role/model pair — a coherence break
a user would experience as "the run refused to start over an error that, had
it started, would not have stopped it."

**Design taken above (D3's sweep code): captain-leg failure at startup is
surfaced loudly (named model/role/alternatives, to stderr) but does NOT
exit non-zero** — matching S06's ratified fail-open policy exactly, on the
theory that a startup sweep and the per-attempt sweep enforcing the same
policy is a stronger invariant than either spec's literal wording. This is
a Type-2-shaded call by RunSlice's own established precedent, but AC-01's
text was written before that precedent existed on this branch and was not
re-reconciled — recommend the same treatment S06 got: **AC-01 amended
in-place** ("on any failure of the implementer, verifier, or escalation-list
entries it SHALL exit non-zero; a captain-leg resolution failure SHALL be
surfaced as a warning and SHALL NOT block startup, matching RunSlice's
per-attempt policy") so the verifier grades the reconciled AC, not a
narrowed private reading. Flagging for Captain/Coach ratification before
implementation proceeds — do not treat this design.md as having silently
resolved it.

## Files to touch

| File | Change |
|---|---|
| `internal/run/resolve.go` (new) | `ComposeEscalationModels`, `DispatchResolution`, `ResolveDispatch` — extracted from slice.go:250-284 |
| `internal/run/slice.go` | Replace inline composition+resolution block (lines 250-284) with calls to the new helpers; no behavioural change |
| `internal/run/imports_test.go` | Add `"../../cmd/sworn"` to `scannedPackages` (AC-02 coverage) |
| `cmd/sworn/run.go` | New startup-sweep block in the `--parallel` branch, before `openDefaultDB()` |
| `cmd/sworn/run_test.go` | `TestParallelStartupFailFast` — unregistered-prefix model, asserts non-zero exit + zero DB track rows, before `TestCmdRun_Parallel`'s existing happy path |
| `internal/run/resolve_test.go` (new) | Unit coverage for `ComposeEscalationModels`/`ResolveDispatch` in isolation (fake registry) |
| `internal/run/parallel_test.go` | Extend the `TestRunParallel_FailureCascade` pattern with an independent same-phase sibling track proving `TrackPass` survives a sibling's terminal-shaped `TrackFail` (AC-03) |
| `internal/scheduler/worker_test.go` | New test: fake `RunSliceFn` failing with a terminal (`run.IsBlocked`-shaped) error after N successful slice dispatches within one track — `RunTrack` returns `TrackFail` promptly, no further slices attempted in that track (AC-03) |

## Acceptance-criteria traceability

- **AC-01** — `cmd/sworn/run.go` startup-sweep block + `TestParallelStartupFailFast` (`cmd/sworn/run_test.go`).
- **AC-02** — factory helpers already deleted (confirmed by grep); coverage closed by `internal/run/imports_test.go` scanning `cmd/sworn`.
- **AC-03** — landed terminal-halt (S06) + landed no-cascade wiring (`#33`, parallel.go:358-362) + new proof tests in `internal/scheduler/worker_test.go` and `internal/run/parallel_test.go`.
- **AC-04** — `go test ./cmd/sworn/... ./internal/scheduler/... ./internal/run/...` — no new import edges introduced; run as part of proof.

## Out of scope (per spec.json, unchanged)

RunSlice internals (S06, done); any new retry/escalation policy design;
board-oracle reading in parallel.go (render-drift release).
