# S46-captain-review — Proof Bundle

## Scope

Add a captain design-review stage to `sworn run`: after the design TL;DR (S45), a captain agent reviews the design against the spec and live code, emits classified pins (mechanical / memory-cited / escalate), writes `review.md`, and gates implementation — proceed on no escalate pins, halt otherwise.

## Files changed

```
internal/captain/review.go       — new: Review(), parsePins(), buildReviewMD(), FormatPinsAsFeedback()
internal/captain/review_test.go  — new: TestEscalatePinHalts, TestCleanDesignProceeds, TestPinsClassified, TestReviewModelError, TestFormatPinsAsFeedbackNil
internal/run/slice.go            — modified: captain review step inserted between TL;DR generation and implement loop
```

## Test results

```
=== RUN   TestEscalatePinHalts
--- PASS: TestEscalatePinHalts (0.00s)
=== RUN   TestCleanDesignProceeds
--- PASS: TestCleanDesignProceeds (0.00s)
=== RUN   TestPinsClassified
--- PASS: TestPinsClassified (0.00s)
=== RUN   TestReviewModelError
--- PASS: TestReviewModelError (0.00s)
=== RUN   TestFormatPinsAsFeedbackNil
--- PASS: TestFormatPinsAsFeedbackNil (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/captain	1.047s
```

Full run test suite also passes (`go test -race ./internal/captain/... ./internal/run/...` — all 27 run tests pass including the existing RunSlice tests).

## Reachability artefact

The captain review is integrated into `RunSlice` in `internal/run/slice.go`. The reachability is exercised by existing `TestRunSlice*` tests (which call `RunSlice` end-to-end), plus the dedicated captain unit tests which exercise the Review function directly with fake agents.

Reachability evidence:
- `TestEscalatePinHalts`: verifies that a captain response with `[escalate]` pins causes `HasEscalatePins=true` and `review.md` is written
- `TestCleanDesignProceeds`: verifies that a clean captain response has `HasEscalatePins=false` and feedback excludes escalate pins
- `TestPinsClassified`: verifies all three pin tags appear in `review.md`
- All existing `TestRunSlice*` tests pass with the captain review step integrated

## Delivered

- [x] `internal/captain/review.go` — `Review()` function that calls the captain model, parses pins, writes `review.md`
- [x] `internal/captain/review_test.go` — unit tests for escalate halt, clean proceed, pin classification
- [x] Integration in `internal/run/slice.go` — captain review runs after TL;DR, gates on escalate pins, injects mechanical/memory-cited pins via `priorFeedback`
- [x] State handling — slice transitions to `DesignReview` before captain review; stays in `DesignReview` on escalate halt
- [x] Timeout handling — captain review bounded by the same per-attempt timeout as implement steps
- [x] Error handling — model errors/timeouts skip the review and proceed to implementation
- [x] `review.md` contains pin classification tags (mechanical, memory-cited, escalate)
- [x] `go test -race ./internal/captain/... ./internal/run/...` passes

## Not delivered

- Dedicated `captain.model` config — the captain review uses the first escalation model (same as TL;DR). A separate captain model setting is a deferred design decision tracked in the spec's open design decisions.
- Interactive `--review` mode for ack/decline — out of scope per spec.

## Divergence from plan

- **State model**: The spec proposed a new `awaiting_design_decision` state for the halt. The implementation reuses the existing `DesignReview` state, which is semantically equivalent and avoids state machine bloat. The state machine already permits `DesignReview → InProgress` and `DesignReview → Deferred` — the halt-on-escalate leaves the slice in `DesignReview`, and the human (or re-run) can resolve by re-running `sworn run`.