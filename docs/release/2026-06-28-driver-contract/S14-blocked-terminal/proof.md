# Proof Bundle: `S14-blocked-terminal`

Generated from live repo state, 2026-07-11. Machine-readable twin: `proof.json`
(proof-v1).

## Scope

The runner treats BLOCKED as terminal-for-the-lane: a dispatched implementer
returning an explicit blocked signal, or a verifier BLOCKED verdict, ends all
dispatches for that slice in the run, halts the owning sequential track,
consumes no retry budget, and exits non-zero with a report distinguishing
BLOCKED lanes (blocker verbatim, routed to /replan-release) from FAIL lanes
(retries exhausted). FAIL keeps its existing retryable semantics unchanged.

## Files changed

```
$ git diff --name-only 79fce54..HEAD
docs/release/2026-06-28-driver-contract/S14-blocked-terminal/status.json
internal/driver/driver.go
internal/implement/implement.go
internal/orchestrator/blocked.go
internal/run/blocked_report_test.go
internal/run/blocked_terminal_test.go
internal/run/parallel.go
internal/run/slice.go
internal/scheduler/blocked_lane_test.go
internal/scheduler/worker.go
```

(plus this slice's journal.md / proof.md / proof.json / status.json
completion updates, committed with this bundle)

## Test results

```
$ go build ./...                                                    # exit 0
$ go vet ./internal/driver/ ./internal/implement/ ./internal/run/ \
        ./internal/scheduler/ ./internal/orchestrator/              # exit 0
$ gofmt -l <all 9 changed/new .go files>                            # empty
$ grep -nE '//[^/]*\t+(return|if |for |func |[a-z_]+\()' <changed .go files>
                                          # no fused comment/code lines
$ go test -count=1 -run 'TestLoopBlocked|TestLoopFail|TestLoopExitReport' \
        -v ./internal/run/ ./internal/scheduler/
--- PASS: TestLoopBlockedImplementerTerminal
--- PASS: TestLoopBlockedVerifierTerminal
--- PASS: TestLoopFailRetrySemanticsUnchanged
--- PASS: TestLoopExitReportBlockedVsFail
--- PASS: TestLoopBlockedSliceHaltsTrack
ok      github.com/swornagent/sworn/internal/run
ok      github.com/swornagent/sworn/internal/scheduler

$ go test -count=1 -timeout 300s ./...        # AC-06 regression gate
every package ok (45 ok, 2 no-test-files), 0 FAIL — including all
pre-existing retry tests, unmodified: TestRun_FailThenPass_RetrySucceeds,
TestRunSliceFail, TestRunSlice_FailNotifiesOnce, TestRunSlice_BlockedNotifies,
TestRun_Blocked_StopsImmediately, all internal/orchestrator triage tests,
TestRunTrack_SliceFail, TestRunTrack_TerminalDriverErrorHaltsTrack,
TestRunParallel_FailureCascade.
```

## Reachability artefact

- **Type**: cli-run (loop-state-machine tests driving the real engine entry
  points, per the spec's own validation-fixture in_scope item)
- **Evidence**:
  - `go test -count=1 -run TestLoopBlockedImplementerTerminal -v ./internal/run/`
    — PASS. The fired S05 replay with synthetic verdict objects: drives the
    REAL `run.RunSlice` (the function `sworn loop` dispatches) against a real
    temp git repo; a fake driver's implementer leg returns
    `{StatusBlocked, BlockedReason}`; asserts exactly one implementer dispatch,
    zero verifier dispatches, lane halted with `IsBlocked` error carrying the
    blocker VERBATIM + the `/replan-release` directive, and the WRITTEN
    status.json read back off disk with `verification.result=blocked`,
    `routing=needs_planner`, state still `in_progress`.
  - `go test -count=1 -run TestLoopExitReportBlockedVsFail -v ./internal/run/`
    — PASS. Drives the REAL `run.RunParallel` (the `sworn loop --parallel`
    engine) with two tracks — one blocked-terminal, one plain-fail — and
    asserts the non-nil (exit-1) error carries the BLOCKED section (verbatim
    blocker, `-> /replan-release test-report`) distinct from the FAIL section,
    and that the same report lands in the durable loop log.
- **Dark-code declaration** (Captain review flag (b), spec out_of_scope 4): no
  production driver emits implement-leg `StatusBlocked` yet — the subprocess
  CLIs map only what their harnesses expose, and absent an explicit signal
  they return retryable shapes, never a fabricated blocked=true. The LIVE
  reachability of the new terminal path today is (1) the verify-leg BLOCKED
  verdict (vocabulary already emitted by real verifiers) and (2) the S07/S09
  terminal auth/credits driver errors, which now render as BLOCKED lanes in
  the exit report as designed (Coach ack pin 3, no reason-string sniffing).
  This is spec-sanctioned, not dark code.

## First-pass verdict (`sworn verify`, deterministic)

See proof.json `first_pass` for the captured output of:

```
$ go run ./cmd/sworn verify -spec .../S14-blocked-terminal/spec.json \
    -diff <git diff 79fce54..HEAD> -proof .../S14-blocked-terminal/proof.md
```

## Delivered

- **AC-01** implementer blocked ⇒ exactly one dispatch, lane halted, blocker
  verbatim + replan directive — `internal/run/blocked_terminal_test.go:
  TestLoopBlockedImplementerTerminal`; code: driver.go `BlockedReason`,
  implement.go early return, slice.go blocked-terminal branch.
- **AC-02** verifier BLOCKED ⇒ identical terminal, no retry budget consumed,
  never mapped onto FAIL — `TestLoopBlockedVerifierTerminal` (2-model
  escalation list with retries remaining; 1 implementer + 1 verifier dispatch;
  state ≠ failed_verification; `IsFailed(err)=false`).
- **AC-03** FAIL with retries ⇒ semantics unchanged — 
  `TestLoopFailRetrySemanticsUnchanged` (FAIL→PASS: 2 implementer dispatches,
  violations text forwarded verbatim in dispatch-2 payload, retry consumed,
  verified).
- **AC-04** blocked slice halts the sequential track —
  `internal/scheduler/blocked_lane_test.go:TestLoopBlockedSliceHaltsTrack`
  (2-slice track, slice 2 never dispatched; `RecordBlocked` invoked exactly
  once with the verbatim reason, route suffix trimmed; supervisor row
  persisted `failed`, never `done` — Captain pin 1 anchor).
- **AC-05** exit report distinguishes BLOCKED vs FAIL, non-zero exit —
  `internal/run/blocked_report_test.go:TestLoopExitReportBlockedVsFail`; code:
  parallel.go collector + `renderBlockedVsFailReport`; legacy error format
  byte-identical when no lane is blocked.
- **AC-06** existing retry tests pass green UNMODIFIED — full
  `go test -count=1 -timeout 300s ./...` green; `git diff 79fce54..HEAD`
  contains NO existing test file (the only `_test.go` files in the diff are
  the three new ones).

## Not delivered

- **Baton role-prompt return-contract documentation for the blocked field** —
  out of scope by spec (out_of_scope 2); why: explicitly non-blocking per the
  inbound packet's ordering ruling; tracking: owned by the later Baton
  contract-edge work (fired proposal capture
  docs/captures/2026-07-10-baton-sworn-edge-contracts-proposal.md, Rec 4);
  acknowledgement: spec.json itself (planner/Coach-authored, second-pass
  replan 2026-07-10).
- **Production driver emission of implement-leg `StatusBlocked`** — out of
  scope by spec (out_of_scope 4); why: the drivers map what their harnesses
  expose and no upstream CLI exposes an explicit blocked signal today;
  tracking: spec out_of_scope 4 + this declaration (dark-code section above);
  acknowledgement: Captain review flag (b) + Coach ack pin 4 catch-all.
- **`sworn coverage` + `sworn llm-check --check ac-satisfaction` gates not
  run** — why: neither command exists in this branch's binary and no provider
  credentials are present in this environment; tracking: AC↔test coverage
  traced manually above (one named test per AC) and the fresh-context
  Verifier (Rule 7) backstops, same posture as review.md flag (e);
  acknowledgement: Coach ack pin 4 catch-all accepting review.md's flags.
- **Release-wide `sworn designfit` currently exits 2 on S11-baton-revendor's
  pre-existing invalid quadrant "beast"** — not this slice's defect (T7
  planner artefact, out of S14's touchpoints — track collision rule); why:
  fixing another track's status.json from this session would be a silent
  cross-track write; tracking: **sworn#90** (filed this session);
  acknowledgement: this proof + journal entry.

## Divergence from plan

- Blocked branch writes the status record BEFORE `Stage(".")+Commit` so the
  commit is never empty when the blocked dispatch made no file edits
  (design.md's phrasing assumed edits; same clean-tree intent).
- `verification.routing` uses the literal `"needs_planner"` with a
  vocab-binding comment instead of importing internal/board into slice.go for
  `board.BlockedNeedsPlanner` (no new import coupling; value identical at
  both bound ends).
- Captain flag (a): the trim option was chosen — `blockedLaneReason` strips
  the route-directive suffix before recording, so the report renders the
  directive once per lane (asserted in the AC-05 test).
- Everything else matches design.md exactly, including the declared
  touchpoints deliberately left untouched (`internal/run/resolve.go`,
  `internal/run/parallel_test.go`, `internal/verify/`, `cmd/sworn/run.go`,
  `cmd/sworn/run_test.go`).
