# Proof Bundle: `S24-dispatch-enrich`

## Scope

After a `sworn run` session, each `state.Dispatch` entry includes `duration_ms`, `input_tokens`, `output_tokens`, real `cost_usd` computed from tokens × pricing map, and `model_id_confirmed`.

## Files changed

```
cmd/sworn/reqverify_test.go
docs/release/2026-06-27-conformance-foundation/S24-dispatch-enrich/journal.md
docs/release/2026-06-27-conformance-foundation/S24-dispatch-enrich/status.json
docs/release/2026-06-27-conformance-foundation/index.md
internal/bench/runner_test.go
internal/gate/llmcheck.go
internal/gate/llmcheck_test.go
internal/implement/ready.go
internal/implement/ready_test.go
internal/model/anthropic.go
internal/model/anthropic_test.go
internal/model/azure.go
internal/model/azure_test.go
internal/model/bedrock.go
internal/model/bedrock_test.go
internal/model/cli.go
internal/model/cli_test.go
internal/model/client.go
internal/model/google.go
internal/model/google_test.go
internal/model/oai.go
internal/model/oai_test.go
internal/model/oci.go
internal/model/oci_test.go
internal/model/ollama.go
internal/model/ollama_test.go
internal/model/openai_responses.go
internal/model/openai_responses_test.go
internal/reqverify/reqverify.go
internal/reqverify/reqverify_test.go
internal/run/run_test.go
internal/run/slice.go
internal/run/slice_test.go
internal/state/state.go
internal/state/state_test.go
internal/verdict/verdict.go
internal/verify/verify.go
internal/verify/verify_test.go
```

## Test results

### Go

```
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
=== RUN   TestVerification_ModelAttemptRoundTrip
--- PASS: TestVerification_ModelAttemptRoundTrip (0.00s)
=== RUN   TestVerification_ModelAttemptOmitEmpty
--- PASS: TestVerification_ModelAttemptOmitEmpty (0.00s)
=== RUN   TestDispatches_RoundTrip
--- PASS: TestDispatches_RoundTrip (0.00s)
=== RUN   TestDispatches_OmitEmpty
--- PASS: TestDispatches_OmitEmpty (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/state	(cached)
=== RUN   TestRunSliceDefaultsNilFactories
--- PASS: TestRunSliceDefaultsNilFactories (0.04s)
=== RUN   TestExtractFrontmatter
=== RUN   TestExtractFrontmatter/simple_frontmatter
=== RUN   TestExtractFrontmatter/no_frontmatter
=== RUN   TestExtractFrontmatter/empty_frontmatter
=== RUN   TestExtractFrontmatter/trailing_whitespace_on_---_lines
=== RUN   TestExtractFrontmatter/single_line_(too_short)
--- PASS: TestExtractFrontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/simple_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/no_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/empty_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/trailing_whitespace_on_---_lines (0.00s)
    --- PASS: TestExtractFrontmatter/single_line_(too_short) (0.00s)
=== RUN   TestExtractReleaseWorktreePath
=== RUN   TestExtractReleaseWorktreePath/simple_path
=== RUN   TestExtractReleaseWorktreePath/no_path
=== RUN   TestExtractReleaseWorktreePath/quoted_path
--- PASS: TestExtractReleaseWorktreePath (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/simple_path (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/no_path (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/quoted_path (0.00s)
=== RUN   TestDirExists
--- PASS: TestDirExists (0.00s)
=== RUN   TestRunParallel_Basic
sworn run --parallel: loaded 2 tracks in 1 phases
[T1] starting
[T2] starting
[T1] done
[T2] done
[T1] result: PASS
[T2] result: PASS
RunParallel: all 2 tracks PASS (skipped: 0)
--- PASS: TestRunParallel_Basic (0.01s)
=== RUN   TestRunParallel_ReleaseWorktreePathMissing
--- PASS: TestRunParallel_ReleaseWorktreePathMissing (0.00s)
=== RUN   TestRunParallel_NoTracks
--- PASS: TestRunParallel_NoTracks (0.00s)
=== RUN   TestRunParallel_MissingIndex
--- PASS: TestRunParallel_MissingIndex (0.00s)
=== RUN   TestRunParallel_FailureCascade
sworn run --parallel: loaded 3 tracks in 2 phases
[T2] starting
[T1] starting
[T2] running slice S02-t2-slice (legacy)
[T1] running slice S01-t1-slice (legacy)
[T1] slice S01-t1-slice failed: simulated T1 failure
[T2] done
[T3] skipped: depends_on failed (phase barrier)
[T1] result: FAIL
[T2] result: PASS
[T3] result: SKIPPED
--- PASS: TestRunParallel_FailureCascade (0.01s)
=== RUN   TestRunParallel_TimingConcurrency
sworn run --parallel: loaded 2 tracks in 1 phases
[T2] starting
[T1] starting
[T2] running slice S02-t2 (legacy)
[T1] running slice S01-t1 (legacy)
[T2] done
[T1] done
[T1] result: PASS
[T2] result: PASS
RunParallel: all 2 tracks PASS (skipped: 0)
--- PASS: TestRunParallel_TimingConcurrency (0.01s)
=== RUN   TestRunParallel_DependentTrackRunsAfterSuccess
sworn run --parallel: loaded 2 tracks in 2 phases
[T1] starting
[T1] running slice S01-t1-slice (legacy)
[T1] done
[T2] starting
[T2] running slice S02-t2-slice (legacy)
[T2] done
[T1] result: PASS
[T2] result: PASS
RunParallel: all 2 tracks PASS (skipped: 0)
--- PASS: TestRunParallel_DependentTrackRunsAfterSuccess (0.01s)
=== RUN   TestRunParallel_TrackPaused
sworn run --parallel: loaded 1 tracks in 1 phases
[T1] starting
[T1] router: S01-pause → coach_decision (needs Coach approval)
[T1] paused: coach_decision — needs Coach approval
[T1] result: PAUSED
--- PASS: TestRunParallel_TrackPaused (0.00s)
=== RUN   TestRun_PassPath_Merges
sworn run: generating design TL;DR with openai/gpt-4o-mini
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/4, resolve 0/1) — implementing with openai/gpt-4o-mini
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: all good
sworn run: merged sworn/write-a-hello-file into main (PASS)
--- PASS: TestRun_PassPath_Merges (0.11s)
=== RUN   TestRun_FailPath_NoMerge
sworn run: generating design TL;DR with fake/impl1
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: missing test
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/1, resolve 1/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: still missing
sworn run: triage: halt — FAIL/Inconclusive: escalation list exhausted (model 0 of 1) — halting
--- PASS: TestRun_FailPath_NoMerge (0.11s)
=== RUN   TestRun_FailThenPass_RetrySucceeds
sworn run: generating design TL;DR with fake/impl1
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/2, resolve 0/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: first try fail
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/2, resolve 1/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: second try ok
sworn run: merged sworn/write-retry-file into main (PASS)
--- PASS: TestRun_FailThenPass_RetrySucceeds (0.12s)
=== RUN   TestRun_Blocked_StopsImmediately
sworn run: generating design TL;DR with fake/impl1
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/4, resolve 0/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict BLOCKED (cost $0.0000)
sworn run: rationale: BLOCKED: spec missing required section
sworn run: triage: halt — BLOCKED: halting immediately — violations will be routed to replan-release by the router (S58)
--- PASS: TestRun_Blocked_StopsImmediately (0.09s)
=== RUN   TestSanitiseBranch
--- PASS: TestSanitiseBranch (0.00s)
=== RUN   TestRun_MissingTask
--- PASS: TestRun_MissingTask (0.00s)
=== RUN   TestRun_VerifyMarkdownPass
sworn run: generating design TL;DR with openai/gpt-4o-mini
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/4, resolve 0/1) — implementing with openai/gpt-4o-mini
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: **PASS** — verification successful
sworn run: merged sworn/write-a-markdown-pass-file into main (PASS)
--- PASS: TestRun_VerifyMarkdownPass (0.11s)
=== RUN   TestRun_VerifyStatelessPromptWired
sworn run: generating design TL;DR with openai/gpt-4o-mini
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/4, resolve 0/1) — implementing with openai/gpt-4o-mini
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS — looks good
sworn run: merged sworn/stateless-prompt-check into main (PASS)
--- PASS: TestRun_VerifyStatelessPromptWired (0.10s)
=== RUN   TestRun_VerifyToolCallLeakBlocks
sworn run: generating design TL;DR with openai/gpt-4o-mini
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/4, resolve 0/1) — implementing with openai/gpt-4o-mini
sworn run: verifying with fake/verifier
sworn run: verdict BLOCKED (cost $0.0000)
sworn run: rationale: verifier reply did not start with PASS/FAIL/BLOCKED/INCONCLUSIVE
sworn run: triage: halt — BLOCKED: halting immediately — violations will be routed to replan-release by the router (S58)
--- PASS: TestRun_VerifyToolCallLeakBlocks (0.10s)
=== RUN   TestRunSlice
sworn run: generating design TL;DR with fake/impl
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: all good
--- PASS: TestRunSlice (0.05s)
=== RUN   TestRunSliceFail
sworn run: generating design TL;DR with fake/impl1
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: missing test
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/1, resolve 1/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: still missing
sworn run: triage: halt — FAIL/Inconclusive: escalation list exhausted (model 0 of 1) — halting
--- PASS: TestRunSliceFail (0.07s)
=== RUN   TestRunSlice_MissingVerifierModel
--- PASS: TestRunSlice_MissingVerifierModel (0.03s)
=== RUN   TestRunSlice_FailNotifiesOnce
sworn run: generating design TL;DR with fake/impl1
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: missing test
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/1, resolve 1/1) — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: still missing
sworn run: triage: halt — FAIL/Inconclusive: escalation list exhausted (model 0 of 1) — halting
--- PASS: TestRunSlice_FailNotifiesOnce (0.07s)
=== RUN   TestRunSlice_BlockedNotifies
sworn run: generating design TL;DR with fake/impl
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl
sworn run: verifying with fake/verifier
sworn run: verdict BLOCKED (cost $0.0000)
sworn run: rationale: BLOCKED: spec missing required section
sworn run: triage: halt — BLOCKED: halting immediately — violations will be routed to replan-release by the router (S58)
--- PASS: TestRunSlice_BlockedNotifies (0.06s)
=== RUN   TestRunSlice_NilNotifierNoOp
sworn run: generating design TL;DR with fake/impl
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with fake/impl
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: ok
--- PASS: TestRunSlice_NilNotifierNoOp (0.05s)
=== RUN   TestImplementTimeoutEscalates
sworn run: generating design TL;DR with blocking
sworn run: design TL;DR timed out after 500ms — proceeding without design.md
sworn run: attempt 1 (model 1/2, resolve 0/1) — implementing with blocking
sworn run: implement attempt 1 timed out after 500ms
sworn run: triage (implementer error): resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/2, resolve 1/1) — implementing with blocking
sworn run: implement attempt 2 timed out after 500ms
sworn run: triage (implementer error): escalate_model — FAIL/Inconclusive: resolve budget (1) exhausted for model 0 — escalating to model 1
sworn run: attempt 3 (model 2/2, resolve 0/1) — implementing with working
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestImplementTimeoutEscalates (1.55s)
=== RUN   TestImplementTimeoutExhaustsToHuman
sworn run: generating design TL;DR with blocking1
sworn run: design TL;DR timed out after 100ms — proceeding without design.md
sworn run: attempt 1 (model 1/2, resolve 0/1) — implementing with blocking1
sworn run: implement attempt 1 timed out after 100ms
sworn run: triage (implementer error): resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/2, resolve 1/1) — implementing with blocking1
sworn run: implement attempt 2 timed out after 100ms
sworn run: triage (implementer error): escalate_model — FAIL/Inconclusive: resolve budget (1) exhausted for model 0 — escalating to model 1
sworn run: attempt 3 (model 2/2, resolve 0/1) — implementing with blocking2
sworn run: implement attempt 3 timed out after 100ms
sworn run: triage (implementer error): resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 1 — retrying same model with S44 feedback
sworn run: attempt 4 (model 2/2, resolve 1/1) — implementing with blocking2
sworn run: implement attempt 4 timed out after 100ms
sworn run: triage (implementer error): halt — FAIL/Inconclusive: escalation list exhausted (model 1 of 2) — halting
--- PASS: TestImplementTimeoutExhaustsToHuman (0.53s)
=== RUN   TestImplementTimeoutHappyPath
sworn run: generating design TL;DR with quick
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestImplementTimeoutHappyPath (0.04s)
=== RUN   TestImplementTimeoutZeroUsesDefault
sworn run: generating design TL;DR with quick
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestImplementTimeoutZeroUsesDefault (0.04s)
=== RUN   TestImplementTimeoutNegativeNoTimeout
sworn run: generating design TL;DR with quick
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with quick
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestImplementTimeoutNegativeNoTimeout (0.04s)
=== RUN   TestRetryPassesVerifierRationale
sworn run: generating design TL;DR with model-a
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: gate 1 — no feedback block in implementer prompt
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/1, resolve 1/1) — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestRetryPassesVerifierRationale (0.05s)
=== RUN   TestAttempt0EmptyFeedback
sworn run: generating design TL;DR with model-a
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestAttempt0EmptyFeedback (0.04s)
=== RUN   TestRetryFeedbackResolvesToPass
sworn run: generating design TL;DR with model-a
sworn run: design TL;DR: design: model response missing required sections (need §1–§6 headers) — proceeding without design.md
sworn run: attempt 1 (model 1/1, resolve 0/1) — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: implementer prompt missing feedback block
sworn run: triage: resolve_in_place — FAIL/Inconclusive: resolve_in_place attempt 1/1 on model 0 — retrying same model with S44 feedback
sworn run: attempt 2 (model 1/1, resolve 1/1) — implementing with model-a
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS
--- PASS: TestRetryFeedbackResolvesToPass (0.06s)
PASS
ok  	github.com/swornagent/sworn/internal/run	(cached)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (backend-only slice — no UI surface)
- **User gesture**: Run `go test ./internal/state/... ./internal/run/... -v` — all tests pass; Dispatch fields marshal/unmarshal correctly with new S24 fields.

## Delivered

- [x] `state.Dispatch` has `DurationMS`, `InputTokens`, `OutputTokens`, `ModelIDConfirmed` fields — evidence: `internal/state/state.go:80-90`
- [x] Verifier dispatch has `duration_ms > 0` — evidence: `internal/verify/verify.go:86-89` wraps `v.Verify()` with `time.Since()`
- [x] OAI dispatch populates `input_tokens` and `output_tokens` from usage — evidence: `internal/model/oai.go:205-209`
- [x] Pricing map populates `cost_usd` from tokens — evidence: `internal/model/client.go:56-62` (`ComputeCostFromTokens`) + `internal/model/oai.go:272-280` (`computeCost`)
- [x] `state_test.go` extended with round-trip test for new Dispatch fields — evidence: `TestDispatches_RoundTrip` in `internal/state/state_test.go:317-358`

## Not delivered

- **Response-confirmed model ID in Verify()**: `ModelIDConfirmed` populated from configured model, not response body. — **Why**: scope ceiling — capturing `cr.Model` would require extending the Verifier interface beyond 5 return values; spec says "or add a new VerifyWithUsage() variant" acknowledging this as optional. **Tracking**: S24-dispatch-enrich journal, open_deferrals. **Acknowledged**: implementer, 2026-07-12.

## Divergence from plan

- `model.PriceForModel()` and `ComputeCostFromTokens()` added to `internal/model/client.go` (not in `planned_files`) — exceeds planned touchpoints but is required for AC "pricing map populates cost_usd". No conflict with other tracks.
- Verifier interface changed to 5-return signature (`text, costUSD, inputTokens, outputTokens, error`) per spec's "backward-compatible approach" — this is a breaking interface change affecting ~30 implementations across 10+ files. All updated.

## First-pass script output

```
release-verify.sh
[90m  slice:       S24-dispatch-enrich[0m
[90m  slice dir:   docs/release/2026-06-27-conformance-foundation/S24-dispatch-enrich[0m
[90m  base branch: main[0m

== Slice artefacts ==
[32m  PASS  slice folder exists[0m
[32m  PASS  spec.md present[0m
[31m  FAIL  proof.md missing[0m
[32m  PASS  status.json present[0m
[32m  PASS  journal.md present[0m
[32m  PASS  spec.md has Required tests section[0m

== Status ==
[32m  PASS  status.json is valid JSON[0m
[90m  state: in_progress[0m
[31m  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first[0m

== Integration branch drift ==
[90m  integration branch: release/v0.1.0[0m
[32m  PASS  worktree branch is current with release/v0.1.0 (no drift)[0m

== Diff vs start_commit (verifier base) ==
[90m  diff base: start_commit e74b5ddd7df29b89e13c30b0dc49e1c6334cff34[0m
[32m  PASS  37 file(s) changed vs diff base[0m
[90m  (first 20)[0m
    cmd/sworn/reqverify_test.go
    docs/release/2026-06-27-conformance-foundation/S24-dispatch-enrich/journal.md
    docs/release/2026-06-27-conformance-foundation/S24-dispatch-enrich/status.json
    internal/bench/runner_test.go
    internal/gate/llmcheck.go
    internal/gate/llmcheck_test.go
    internal/implement/ready.go
    internal/implement/ready_test.go
    internal/model/anthropic.go
    internal/model/anthropic_test.go
    internal/model/azure.go
    internal/model/azure_test.go
    internal/model/bedrock.go
    internal/model/bedrock_test.go
    internal/model/cli.go
    internal/model/cli_test.go
    internal/model/client.go
    internal/model/google.go
    internal/model/google_test.go
    internal/model/oai.go

== Dark-code markers in changed files ==
[32m  PASS  no dark-code markers in changed source files[0m

== Proof bundle structural checks ==

== Frontmatter YAML safety ==
[32m  PASS  spec.md frontmatter is strict-YAML safe[0m

== Test results section scope ==
[90m  proof.md not found; skipping Test results scope check[0m
/home/brad/.claude/bin/release-verify.sh: line 532: PLAYWRIGHT_OPTIN: unbound variable
EXIT:1
```
