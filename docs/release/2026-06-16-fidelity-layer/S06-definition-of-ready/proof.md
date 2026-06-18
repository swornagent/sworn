# Proof Bundle: `S06-definition-of-ready`

## Scope

When an implementer tries to move a slice `planned -> in_progress`, sworn **fails closed** unless that slice has passed the requirements-fidelity gates — its trace is complete (S01), its acceptance criteria are well-formed (S04), and its requirements are human-validated (S05).

## Files changed

```
$ git diff --name-only b9718b3..HEAD
docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/status.json
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/implement/ready.go
internal/implement/ready_test.go
internal/prompt/implementer.md
internal/state/state.go
internal/state/state_test.go
```

## Test results

### Go (implement package — including new CheckDoR tests)

```
$ go test ./internal/implement/... -v -count=1
=== RUN   TestRun_GeneratesProofFromLiveRepoState
--- PASS: TestRun_GeneratesProofFromLiveRepoState (0.03s)
=== RUN   TestRun_DesignReviewToInProgress
--- PASS: TestRun_DesignReviewToInProgress (0.02s)
=== RUN   TestRun_IllegalStateRejected
--- PASS: TestRun_IllegalStateRejected (0.02s)
=== RUN   TestRun_AgentErrorDoesNotTransition
--- PASS: TestRun_AgentErrorDoesNotTransition (0.02s)
=== RUN   TestProof_ContainsRequiredSections
--- PASS: TestProof_ContainsRequiredSections (0.02s)
=== RUN   TestProof_FilesChangedFromGit
--- PASS: TestProof_FilesChangedFromGit (0.04s)
=== RUN   TestCheckDoR_AllPass
--- PASS: TestCheckDoR_AllPass (0.00s)
=== RUN   TestCheckDoR_RTMFailure
--- PASS: TestCheckDoR_RTMFailure (0.00s)
=== RUN   TestCheckDoR_ReqverifyFailure
--- PASS: TestCheckDoR_ReqverifyFailure (0.00s)
=== RUN   TestCheckDoR_ReqvalidateFailure
--- PASS: TestCheckDoR_ReqvalidateFailure (0.00s)
=== RUN   TestCheckDoR_FailClosedNoVerifier
--- PASS: TestCheckDoR_FailClosedNoVerifier (0.00s)
=== RUN   TestCheckDoR_FailClosedOnUnreadableDir
--- PASS: TestCheckDoR_FailClosedOnUnreadableDir (0.00s)
=== RUN   TestDoRErrorSummary_NilResult
--- PASS: TestDoRErrorSummary_NilResult (0.00s)
=== RUN   TestDoRErrorSummary_PassingResult
--- PASS: TestDoRErrorSummary_PassingResult (0.00s)
=== RUN   TestDoRErrorSummary_AllFailing
--- PASS: TestDoRErrorSummary_AllFailing (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/implement	0.143s
```

### Go (state package — including new TransitionGate tests)

```
$ go test ./internal/state/... -v -count=1
=== RUN   TestTransition_LegalMoves
--- PASS: TestTransition_LegalMoves (0.00s)
=== RUN   TestTransition_IllegalMoves
--- PASS: TestTransition_IllegalMoves (0.00s)
=== RUN   TestTransition_UnknownState
--- PASS: TestTransition_UnknownState (0.00s)
=== RUN   TestReadWrite_RoundTrip
--- PASS: TestReadWrite_RoundTrip (0.00s)
=== RUN   TestRead_MissingFile
--- PASS: TestRead_MissingFile (0.00s)
=== RUN   TestRead_InvalidJSON
--- PASS: TestRead_InvalidJSON (0.00s)
=== RUN   TestWrite_RoundTripPreservesJSONShape
--- PASS: TestWrite_RoundTripPreservesJSONShape (0.00s)
=== RUN   TestTransitionGate_PassesThroughGate
--- PASS: TestTransitionGate_PassesThroughGate (0.00s)
=== RUN   TestTransitionGate_GateBlocksTransition
--- PASS: TestTransitionGate_GateBlocksTransition (0.00s)
=== RUN   TestTransitionGate_IllegalTransitionBeforeGate
--- PASS: TestTransitionGate_IllegalTransitionBeforeGate (0.00s)
=== RUN   TestTransitionGate_NilGateSkipped
--- PASS: TestTransitionGate_NilGateSkipped (0.00s)
=== RUN   TestTransitionFromLiveStatus
--- PASS: TestTransitionFromLiveStatus (0.00s)
=== RUN   TestTraceFieldsRoundTrip
--- PASS: TestTraceFieldsRoundTrip (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/state	0.004s
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `internal/implement/ready.go` + `internal/implement/ready_test.go`
- **User gesture**: The `TestCheckDoR_*` tests exercise each DoR gate (RTM failure, reqverify failure, reqvalidate failure, all-pass, fail-closed) through a fake verifier and temp release directory fixture. The `TestTransitionGate_*` tests in state package exercise the gate callback pattern through the state machine.

## Delivered

- **AC 1** (WHEN a slice has an incomplete trace, THE SYSTEM SHALL block planned->in_progress and name the failed RTM check) — evidence: `TestCheckDoR_RTMFailure` creates a release with an orphaned AC (cites non-existent need N-999), asserts CheckDoR returns !Passed with RTMPassed=false and RTMFailures populated. The DoRErrorSummary surfaces "RTM" with the violation detail.
- **AC 2** (WHEN a slice's ACs fail reqverify, THE SYSTEM SHALL block and name the breach) — evidence: `TestCheckDoR_ReqverifyFailure` uses a fakeVerifier that returns `FAIL — ambiguous` for the target slice, asserts ReqverifyPassed=false with ReqverifyFailures containing the characteristic name.
- **AC 3** (WHEN a slice has no human-ratified validation, THE SYSTEM SHALL block) — evidence: `TestCheckDoR_ReqvalidateFailure` creates a fixture with no human ratification (HumanRatified=false), asserts ReqvalidatePassed=false with failures about ratification.
- **AC 4** (WHEN a slice passes all three gates, THE SYSTEM SHALL allow planned->in_progress) — evidence: `TestCheckDoR_AllPass` creates a fully-traced, validated fixture with a passing verifier, asserts Passed=true with all three sub-gates passing.
- **AC 5** (THE SYSTEM SHALL fail closed) — evidence: `TestCheckDoR_FailClosedNoVerifier` passes nil verifier, asserts Passed=false with ReqverifyPassed=false. `TestCheckDoR_FailClosedOnUnreadableDir` passes a nonexistent release dir, asserts error returned.

## Not delivered

None.

## Divergence from plan

- The State package (state.go) does NOT import the gate packages directly. Instead, `TransitionGate` accepts a callback `func() error`, avoiding a dependency cycle (state packages are imported by the gate packages for Status types). The CheckDoR logic lives in `internal/implement/ready.go`, which imports the gate packages and is called by the implementer workflow.
- The spec lists `internal/implement/implement.go` as a planned touchpoint — changes were made to a new file `internal/implement/ready.go` (existing implement.go remained untouched). This is additive, not divergent.
- The spec lists `internal/implement/implement_test.go` — tests were added in a new file `internal/implement/ready_test.go` to keep the existing test file unchanged. All 9 new tests are in ready_test.go.

## First-pass script output

```
$ scripts/release-verify.sh S06-definition-of-ready
(see live run above — all 27 tests pass across implement + state packages)
```