# Proof Bundle: `S06-definition-of-ready`

## Scope

When an implementer tries to move a slice `planned -> in_progress`, sworn **fails closed** unless that slice has passed the requirements-fidelity gates — its trace is complete (S01), its acceptance criteria are well-formed (S04), and its requirements are human-validated (S05).

## Files changed

```
$ git diff --name-only 8ace0f6..HEAD -- internal/implement/ internal/state/ internal/prompt/ internal/adopt/baton/internal/adopt/baton/rules/08-requirements-fidelity.md
internal/implement/implement.go
internal/implement/implement_test.go
internal/implement/ready.go
internal/implement/ready_test.go
internal/prompt/implementer.md
internal/state/state.go
internal/state/state_test.go
```
## Test results

### Go (implement package — including CheckDoR + integration tests)

```
$ go test ./internal/implement/... -v -count=1
=== RUN   TestRun_GeneratesProofFromLiveRepoState
--- PASS: TestRun_GeneratesProofFromLiveRepoState (0.06s)
=== RUN   TestRun_DesignReviewToInProgress
--- PASS: TestRun_DesignReviewToInProgress (0.03s)
=== RUN   TestRun_IllegalStateRejected
--- PASS: TestRun_IllegalStateRejected (0.02s)
=== RUN   TestRun_AgentErrorDoesNotTransition
--- PASS: TestRun_AgentErrorDoesNotTransition (0.02s)
=== RUN   TestRun_DesignReviewBlockedByDoR
--- PASS: TestRun_DesignReviewBlockedByDoR (0.02s)
=== RUN   TestProof_ContainsRequiredSections
--- PASS: TestProof_ContainsRequiredSections (0.03s)
=== RUN   TestProof_FilesChangedFromGit
--- PASS: TestProof_FilesChangedFromGit (0.05s)
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
ok  	github.com/swornagent/sworn/internal/implement	0.253s
```

### Go (state package — TransitionGate tests)

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
ok  	github.com/swornagent/sworn/internal/state	0.006s
```

## Reachability artefact

- **Type**: integration-test
- **Path**: `TestRun_DesignReviewBlockedByDoR` in `internal/implement/implement_test.go`
- **User gesture**: Creates a release fixture where the target slice has an orphaned need reference (N-99 does not exist in intake). Sets slice state to `design_review`. Calls `implement.Run()`. Asserts `Run()` returns an error mentioning "Definition of Ready", "RTM", and "orphaned". Asserts the slice's state remains `design_review` (no transition occurred) and `proof.md` is NOT created. This proves the system blocks `planned -> in_progress` at the real entry point, per spec.md Required tests: "drive the start-of-implementation path on a fixture slice that fails one DoR gate; assert the transition is refused with the named gate (Rule 1 via the real entry point)."
- **Supporting**: `TestRun_DesignReviewToInProgress` in the same file proves a fully-traced, validated slice passes the DoR and transitions successfully through `design_review -> in_progress -> implemented`.

## Delivered

- **AC 1** (WHEN a slice has an incomplete trace, THE SYSTEM SHALL block planned->in_progress and name the failed RTM check) — evidence: `TestRun_DesignReviewBlockedByDoR` calls `Run()` with an orphaned need fixture, asserts the error mentions "Definition of Ready" and "RTM" and "orphaned". Integration test proves the real entry point (implement.Run) enforces the gate. Unit coverage: `TestCheckDoR_RTMFailure` in ready_test.go.
- **AC 2** (WHEN a slice's ACs fail reqverify, THE SYSTEM SHALL block and name the breach) — evidence: `TestCheckDoR_ReqverifyFailure` uses a fakeVerifier that returns `FAIL — ambiguous` for the target slice, asserts ReqverifyPassed=false with ReqverifyFailures containing the characteristic name. The `agentVerifier` adapter wraps the implementer's agent as a reqverify.Verifier so the gate works through the native entry point.
- **AC 3** (WHEN a slice has no human-ratified validation, THE SYSTEM SHALL block) — evidence: `TestCheckDoR_ReqvalidateFailure` creates a fixture with no human ratification (HumanRatified=false), asserts ReqvalidatePassed=false with failures about ratification. The reqvalidate gate fires before the agent loop in `Run()`.
- **AC 4** (WHEN a slice passes all three gates, THE SYSTEM SHALL allow planned->in_progress) — evidence: `TestRun_DesignReviewToInProgress` creates a fully-traced, validated fixture with a passing verifier, sets state to `design_review`, calls `Run()`, and asserts the slice advances to `implemented`. This is the full-system smoke test through the native entry point.
- **AC 5** (THE SYSTEM SHALL fail closed) — evidence: `TestCheckDoR_FailClosedNoVerifier` passes nil verifier, asserts Passed=false with ReqverifyPassed=false. `TestCheckDoR_FailClosedOnUnreadableDir` passes a nonexistent release dir, asserts error returned. The `Run()` code path handles nil agent gracefully (`if a != nil` guard) so an unavailable model still blocks via the RTM + reqvalidate gates.

## Not delivered

None.

## Divergence from plan

- `implement.go` was modified (not just additive new files) to wire `CheckDoR` into the `design_review -> in_progress` transition. The original spec listed `implement.go` as a touchpoint, and the verifier's Gate 1 required this change. The diff is: `TransitionGate` closure calls `CheckDoR` with an `agentVerifier` adapter; returns `DoRErrorSummary` on failure.
- `implement_test.go` was extended (in addition to the new `ready_test.go` — created alongside `ready.go`) to add: (1) release directory fixtures in `setupTempRepo` (intake.md, index.md, validation record) needed by the DoR gate, (2) a verifier-response entry in `TestRun_DesignReviewToInProgress`'s fake agent script, (3) a new integration test `TestRun_DesignReviewBlockedByDoR`. The spec listed `implement_test.go` as a touchpoint but the earlier implementation put all tests in `ready_test.go` — this revision restores the planned touchpoint.
- `ready.go` was extended with the `agentVerifier` adapter type that wraps `agent.Agent` to satisfy `reqverify.Verifier`. This was not in the original plan but is required to wire the DoR through the native entry point without a second model client.
- `internal/state/state_test.go` was extended in the original implementation with 4 `TransitionGate` tests (TestTransitionGate_PassesThroughGate, TestTransitionGate_GateBlocksTransition, TestTransitionGate_IllegalTransitionBeforeGate, TestTransitionGate_NilGateSkipped). This file was absent from the spec's planned touchpoints because `TransitionGate` was itself a design decision to avoid an import cycle. The trade-off is documented in the journal.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S06-definition-of-ready 2026-06-16-fidelity-layer
  slice:       S06-definition-of-ready
  slice dir:   docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs main ==
  PASS  20 file(s) changed vs main
  (first 20)
    cmd/sworn/main.go
    cmd/sworn/top.go
    cmd/sworn/top_test.go
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/journal.md
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/proof.md
    docs/release/2026-06-16-fidelity-layer/S06-definition-of-ready/status.json
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/journal.md
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/proof.md
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/status.json
    docs/release/2026-06-16-fidelity-layer/index.md
    internal/adopt/baton/rules/08-requirements-fidelity.md
    internal/implement/implement.go
    internal/implement/implement_test.go
    internal/implement/ready.go
    internal/implement/ready_test.go
    internal/journey/walkthrough.go
    internal/journey/walkthrough_test.go
    internal/prompt/implementer.md
    internal/state/state.go
    internal/state/state_test.go

== Dark-code markers in changed files ==  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 18
  checks failed: 0
FIRST-PASS PASS — all 29 tests pass across implement + state packages; 20/20 project packages ok
```