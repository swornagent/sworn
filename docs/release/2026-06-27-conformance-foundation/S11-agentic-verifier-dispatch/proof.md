# Proof Bundle — S11-agentic-verifier-dispatch

## Scope

Agentic verifier dispatch from engine: `sworn run` dispatches the real `verifier.md` role via `model.Chat()` instead of the stateless judge, proof mandatory, `verifier_was_fresh_context` true, model fix.

## Files changed

```
cmd/sworn/verify.go
docs/release/2026-06-27-conformance-foundation/S11-agentic-verifier-dispatch/status.json
internal/gate/mock.go
internal/run/slice.go
internal/verify/verify.go
internal/verify/verify_agentic_test.go
internal/verify/verify_test.go
```

## Test results

### verify tests (agentic + boundary mock — full suite)

```
=== RUN   TestConcurrentVerifySameInput
--- PASS: TestConcurrentVerifySameInput (0.00s)
=== RUN   TestConcurrentVerifyIndependentInputs
--- PASS: TestConcurrentVerifyIndependentInputs (0.00s)
=== RUN   TestRunAgenticPass
--- PASS: TestRunAgenticPass (0.00s)
=== RUN   TestRunAgenticFail
--- PASS: TestRunAgenticFail (0.00s)
=== RUN   TestRunAgenticBlocked
--- PASS: TestRunAgenticBlocked (0.00s)
=== RUN   TestRunAgenticUnparseableBlocks
--- PASS: TestRunAgenticUnparseableBlocks (0.00s)
=== RUN   TestRunAgenticEmptyChoicesBlocks
--- PASS: TestRunAgenticEmptyChoicesBlocks (0.00s)
=== RUN   TestVerifyRun_Pass
--- PASS: TestVerifyRun_Pass (0.00s)
=== RUN   TestVerifyRun_Fail
--- PASS: TestVerifyRun_Fail (0.00s)
=== RUN   TestVerifyRun_Blocked_EmptySpec
--- PASS: TestVerifyRun_Blocked_EmptySpec (0.00s)
=== RUN   TestVerifyRun_Blocked_EmptyDiff
--- PASS: TestVerifyRun_Blocked_EmptyDiff (0.00s)
=== RUN   TestVerifyRun_Blocked_MissingFile
--- PASS: TestVerifyRun_Blocked_MissingFile (0.00s)
=== RUN   TestParseVerdictPass
--- PASS: TestParseVerdictPass (0.00s)
=== RUN   TestParseVerdictFail
--- PASS: TestParseVerdictFail (0.00s)
=== RUN   TestParseVerdictBlocked
--- PASS: TestParseVerdictBlocked (0.00s)
=== RUN   TestParseVerdictInconclusive
--- PASS: TestParseVerdictInconclusive (0.00s)
=== RUN   TestParseVerdictUnparseableBlocks
--- PASS: TestParseVerdictUnparseableBlocks (0.00s)
=== RUN   TestSystemPromptIsStatelessJudge
--- PASS: TestSystemPromptIsStatelessJudge (0.00s)
=== RUN   TestBuildPayload
--- PASS: TestBuildPayload (0.00s)
=== RUN   TestVerifyRun_OpenDeferrals
--- PASS: TestVerifyRun_OpenDeferrals (0.00s)
=== RUN   TestVerifyRun_UndeclaredMockBlocks
--- PASS: TestVerifyRun_UndeclaredMockBlocks (0.00s)
=== RUN   TestCheckBoundaryMocks_UndeclaredDbMockFails
--- PASS: TestCheckBoundaryMocks_UndeclaredDbMockFails (0.00s)
=== RUN   TestCheckBoundaryMocks_DeclaredDbMockPasses
--- PASS: TestCheckBoundaryMocks_DeclaredDbMockPasses (0.00s)
=== RUN   TestCheckBoundaryMocks_NonBoundaryMockNotFlagged
--- PASS: TestCheckBoundaryMocks_NonBoundaryMockNotFlagged (0.00s)
=== RUN   TestCheckBoundaryMocks_AuthMockUndeclaredFails
--- PASS: TestCheckBoundaryMocks_AuthMockUndeclaredFails (0.00s)
=== RUN   TestCheckBoundaryMocks_EntitlementMockUndeclaredFails
--- PASS: TestCheckBoundaryMocks_EntitlementMockUndeclaredFails (0.00s)
=== RUN   TestCheckBoundaryMocks_FakeDbDetected
--- PASS: TestCheckBoundaryMocks_FakeDbDetected (0.00s)
=== RUN   TestCheckBoundaryMocks_EmptyDiffReturnsEmpty
--- PASS: TestCheckBoundaryMocks_EmptyDiffReturnsEmpty (0.00s)
=== RUN   TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged
--- PASS: TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged (0.00s)
=== RUN   TestCheckBoundaryMocks_StubAuthDetected
--- PASS: TestCheckBoundaryMocks_StubAuthDetected (0.00s)
=== RUN   TestCheckBoundaryMocks_StubDbDetected
--- PASS: TestCheckBoundaryMocks_StubDbDetected (0.00s)
=== RUN   TestCheckBoundaryMocks_CreditsEntitlementBoundary
--- PASS: TestCheckBoundaryMocks_CreditsEntitlementBoundary (0.00s)
=== RUN   TestCheckBoundaryMocks_KeylessEntitlementBoundary
--- PASS: TestCheckBoundaryMocks_KeylessEntitlementBoundary (0.00s)
=== RUN   TestCheckBoundaryMocks_ClaudePBillingBoundary
--- PASS: TestCheckBoundaryMocks_ClaudePBillingBoundary (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

### gate mock tests

```
=== RUN   TestMockPatternRe
--- PASS: TestMockPatternRe (0.00s)
=== RUN   TestMockDeferralsHasMockBoundary
=== RUN   TestMockDeferralsHasMockBoundary/mock_in_what_field
=== RUN   TestMockDeferralsHasMockBoundary/boundary_in_why_field
=== RUN   TestMockDeferralsHasMockBoundary/stub_in_what
=== RUN   TestMockDeferralsHasMockBoundary/fixture_in_why
=== RUN   TestMockDeferralsHasMockBoundary/seed_in_what
=== RUN   TestMockDeferralsHasMockBoundary/no_mock_mention
=== RUN   TestMockDeferralsHasMockBoundary/empty_deferrals
--- PASS: TestMockDeferralsHasMockBoundary (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/mock_in_what_field (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/boundary_in_why_field (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/stub_in_what (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/fixture_in_why (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/seed_in_what (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/no_mock_mention (0.00s)
    --- PASS: TestMockDeferralsHasMockBoundary/empty_deferrals (0.00s)
=== RUN   TestMockReportJSON
--- PASS: TestMockReportJSON (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/gate	(cached)
```

### Build + vet

```
build exit: 0
vet exit: 0
```

## Reachability artefact

`sworn verify --agentic --help` displays the --agentic flag:

```
Usage of verify:
  -agentic
    	use agentic verifier (full verifier.md role via Chat) instead of stateless judge
  -deferral value
    	declared Rule-2 deferral (repeatable: 'why - tracking - ack')
  -diff string
    	path to the unified diff, or - for stdin (required)
  -proof string
    	path to the proof bundle (optional in this build)
  -spec string
    	path to the spec / acceptance criteria (required)
  -verifier-model string
    	verifier model id (provider/model)
```

## Delivered

- [x] AC1: Proof mandatory gate — RunSlice returns BLOCKED when proof.md absent/empty (`internal/run/slice.go` lines ~380-424)
- [x] AC2: Agentic dispatch — RunSlice calls verify.RunAgentic() not verify.Run() (`internal/run/slice.go` line ~440)
- [x] AC3: VerifierWasFreshContext = true on PASS (`internal/run/slice.go` line ~488)
- [x] AC4: Verification.Model = opts.VerifierModel not implModelID (`internal/run/slice.go` line ~487)
- [x] AC5: gate.RunMock called before dispatch, violations logged (`internal/run/slice.go` lines ~429-445)
- [x] AC6: entitlement/credits/subscription/keyless/claude-p keywords added to gate/mock.go and verify/verify.go
- [x] AC7: `sworn verify --agentic` compiles and routes to RunAgentic() (`cmd/sworn/verify.go`)

## Not delivered

- True test re-running via tool calls (deferred per spec: agentic tool-call infrastructure not in scope for this slice; tracking: future agentic-tool-calls slice)

## Divergence from plan

None. Implementation matches spec acceptance checks exactly.
