# Proof bundle — fix router-review-unhandled (2026-07-02)

## Scope

Handle the router's "review" decision (slice at design_review awaiting the
Captain, Rule 9) in `runTrackRouter` as a human-gated pause instead of letting
it fall to the default case and fail the whole track (open issue sworn#46).

## Files changed

`git diff --name-only 632d4f3` (cumulative on the audit branch; this fix's
own diff is the two scheduler files + this proof):

```
cmd/sworn/run_test.go
docs/captures/2026-07-02-fix-prodmerge-gitfile-noop-proof.md
internal/run/parallel.go
internal/run/parallel_test.go
internal/scheduler/worker.go
internal/scheduler/worker_test.go
```

## Test results

RED first (defect reproduced through RunTrack, the integration point):

```
$ go test -timeout 120s -run 'TestReviewDecisionPausesTrack' ./internal/scheduler/ -v
[T1] router: S01-design → review (design.md awaits Captain review)
[T1] unrecognised router decision "review" for S01-design: design.md awaits Captain review
    worker_test.go:1697: expected TrackPaused for review decision, got fail
--- FAIL: TestReviewDecisionPausesTrack (0.00s)
```

GREEN after the fix:

```
$ go test -timeout 120s -run 'TestReviewDecisionPausesTrack' ./internal/scheduler/ -v
[T1] router: S01-design → review (design.md awaits Captain review)
[T1] paused: review — design.md awaits Captain review
--- PASS: TestReviewDecisionPausesTrack (0.00s)
ok  	github.com/swornagent/sworn/internal/scheduler	0.016s
```

Slice-relevant package suites (not full repo — merge gate owns that):

```
$ go test -timeout 300s ./internal/scheduler/... ./internal/router/... ./internal/run/...
ok  	github.com/swornagent/sworn/internal/scheduler	0.186s
ok  	github.com/swornagent/sworn/internal/router	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
```

`gofmt -l internal/scheduler` and `go vet ./internal/scheduler/...` clean.

## Reachability artefact

`TestReviewDecisionPausesTrack` drives the scripted `{Type: "review"}`
decision through `RunTrack` → `runTrackRouter` — the exact production
dispatch path (`productionSliceRouter` in internal/run/parallel.go passes
router NextType through verbatim). Before the fix the live output was
`unrecognised router decision "review" ... result fail`; after, the track
pauses (`[T1] paused: review — design.md awaits Captain review`) and the
supervisor row is not StateFailed.

## Delivered

- "review" added to the human-gated pause case in `runTrackRouter`
  (internal/scheduler/worker.go), mirroring coach_decision/replan-release/
  merge-release: surface to stderr, release track non-failed, return
  TrackPaused (evidence: TestReviewDecisionPausesTrack).
- "review" added to `pauseSet` for consistency with the S59 pause-set
  declaration (evidence: internal/scheduler/worker.go).
- Regression test through the RunTrack integration point (evidence:
  internal/scheduler/worker_test.go TestReviewDecisionPausesTrack).

## Not delivered

- `pauseSet` itself is referenced nowhere outside its definition (dead map;
  behaviour lives inline in the switch) — noted by the adversarial verifier.
  Wiring it in or deleting it is a refactor beyond this defect's scope; left
  as-is and surfaced here (Rule 2: why = out of defect scope, tracking =
  this bundle + audit punch list, acknowledgement = this section).
- `supervisor.Release` coerces any non-done/non-failed label to "done", so a
  paused track's DB row reads "done" — pre-existing behaviour shared by ALL
  pause paths (coach_decision included), not introduced or worsened here.
  Same Rule 2 surfacing as above; candidates for the audit punch list.

## Divergence from plan

- None functionally. The regression test asserts "not StateFailed" rather
  than a literal "paused" DB label because of the pre-existing
  supervisor.Release coercion described above.
