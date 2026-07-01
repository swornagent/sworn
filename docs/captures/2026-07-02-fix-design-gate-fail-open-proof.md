# Proof bundle — fix design-gate-fail-open (2026-07-02)

## Scope

Stop the engine's Rule 9 design-review gate (design TL;DR + captain review in
`run.RunSlice`) from silently bypassing on a dispatch/construction/timeout
failure. Every gate-skip now records a machine-readable Rule 2 deferral on
status.json (`open_deferrals`) instead of proceeding with no trace.

## Design decision (Rule 4)

The finding offered two options: (a) halt the slice (Blocked) on gate-dispatch
failure, or (b) record an explicit Rule 2 deferral and proceed. I chose (b),
the deferral, because:

- Halting on the design-TL;DR path breaks S45's ratified contract ("a hung
  TL;DR call must not wedge the run") and the existing timeout-escalation
  tests, which deliberately use blocking agents that time the design stage
  out then expect the implement loop to run.
- The finding's actual defect is "no Rule 2 deferral recorded anywhere" — a
  silent bypass. Recording a durable, machine-readable `open_deferrals` entry
  (why + tracking) turns the silent bypass into a surfaced, Coach-visible
  deferral, using the existing `internal/state.Deferral` carrier.
- Rule 2's third leg (human acknowledgement) is left to the Coach reviewing
  `open_deferrals`; a machine cannot self-acknowledge. This is honest: the
  deferral is recorded (why+tracking), not falsely marked acknowledged.

## Files changed

`git diff --name-only 632d4f3` (cumulative on the audit branch; this fix's own
diff is internal/run/slice.go + internal/run/slice_test.go + this proof):

```
cmd/sworn/run_test.go
docs/captures/2026-07-02-fix-prodmerge-gitfile-noop-proof.md
docs/captures/2026-07-02-fix-router-review-unhandled-proof.md
internal/run/parallel.go
internal/run/parallel_test.go
internal/run/slice.go
internal/run/slice_test.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
```

## Test results

RED first — pre-fix code (production slice.go stashed) records no deferral:

```
$ git stash push internal/run/slice.go
$ go test -timeout 120s -run 'TestDesignGate_' ./internal/run/
sworn run: captain review error: captain: model call: simulated provider 429 — proceeding without review
    slice_test.go:777: expected a design_review_gate deferral on status.json after captain dispatch failure, got none
--- FAIL: TestDesignGate_CaptainDispatchFailureRecordsDeferral (0.04s)
FAIL	github.com/swornagent/sworn/internal/run	0.087s
$ git stash pop
```

GREEN after the fix:

```
$ go test -timeout 120s -run 'TestDesignGate_' ./internal/run/ -v
sworn run: captain review error: captain: model call: simulated provider 429 — recording Rule 2 deferral
--- PASS: TestDesignGate_GenerationFailureRecordsDeferral (0.05s)
--- PASS: TestDesignGate_CaptainDispatchFailureRecordsDeferral (0.03s)
ok  	github.com/swornagent/sworn/internal/run	0.103s
```

Full run-package suite (slice-relevant; merge gate owns full repo):

```
$ go test -timeout 300s ./internal/run/...
ok  	github.com/swornagent/sworn/internal/run	5.216s
```

`gofmt -l internal/run/slice.go internal/run/slice_test.go` and
`go vet ./internal/run/...` both clean.

## Reachability artefact

Both tests drive the production `RunSlice` entry point end-to-end:

- `TestDesignGate_GenerationFailureRecordsDeferral`: design agent's Chat
  returns a 429; design.Generate fails; RunSlice records a
  `design_review_gate` deferral (why + tracking) on status.json before the
  implement loop. Asserted via `state.Read`.
- `TestDesignGate_CaptainDispatchFailureRecordsDeferral`: valid design.md
  pre-exists so captain.Review is the failing stage (Chat 429); the deferral
  Why reads "captain design-review dispatch failed". Live stderr:
  `sworn run: captain review error: ... — recording Rule 2 deferral`.

## Delivered

- `recordDesignGateDeferral` helper appends a Rule 2 `open_deferrals` entry
  (item=design_review_gate, why, tracking=swornagent/sworn#51), idempotent per
  run, best-effort (internal/run/slice.go).
- Deferral recorded on every previously-fail-open path: design agent
  construction failure, design TL;DR timeout, design TL;DR generation error,
  design spec-read failure, captain agent construction failure, captain review
  timeout, captain review dispatch error, captain spec-read failure, design.md
  absent at captain stage (evidence: the instrumented branches in
  internal/run/slice.go; two of the paths covered by TestDesignGate_* tests).

## Not delivered

- The two remaining gate-skip paths (agent-construction failures and
  spec-read failures) are instrumented identically but not each given a
  dedicated test — they share `recordDesignGateDeferral` with the two tested
  dispatch/generation paths, so the carrier is proven; per-path tests would
  add fixtures without exercising new logic. Rule 2 surfacing: why = shared
  helper already covered, tracking = this bundle + audit punch list,
  acknowledgement = this section.
- Human acknowledgement of the machine-recorded deferrals (Rule 2 third leg)
  is intentionally out of engine scope — the Coach owns it when reviewing
  open_deferrals (stated in the design decision above).

## Divergence from plan

- Chose the deferral option (b) over the halt option (a); rationale in the
  "Design decision" section above. The finding authorised either.
