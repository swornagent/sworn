# Journal — S06-invariant2-enforcement

## Session 2026-07-28 — Implementation

**State transition:** design_review → in_progress → implemented

### Decisions

- Added `PlannedFilesFn func(ctx context.Context, trackID string) ([]string, error)` on `ParallelOptions` as injection seam (Captain pin 1). Default reads from `release-wt/<release>` ref via `git show`. Tests inject fakes without real git.
- Parse DOCUMENTED SHARED rows from index.md markdown **body** (after frontmatter close), not frontmatter (Captain pin 2). Uses line-oriented regex: find `(DOCUMENTED SHARED)`, extract first backtick-quoted path.
- Per-phase disjointness check before goroutine launch. If overlap: log INVARIANT-2 message, store `TrackBlocked`, don't launch.
- Follow-up phase after `wg.Wait()` retries blocked tracks. Same S04 phase-barrier pattern — conflicting track has finished + auto-merged by then, so re-check passes.
- Running union per-phase (with `fileOwner` map for T_a identification). Reset on follow-up phase.
- Added `TrackBlocked` `TrackResult` constant to scheduler.
- Error-string assertion on shared prefix "both write" (Captain pin 4). Test asserts `strings.Contains(stderr, "both write")` — both spec forms share this prefix.
- Design decisions (5) classified Type-2, populated in status.json `design_decisions` (Captain pin 3).

### Files changed

- `internal/run/parallel.go` — PlannedFilesFn, parseDocumentedSharedFiles, checkDisjointness, makePlannedFilesReader, invariant-2 check in phase loop, follow-up phase
- `internal/run/parallel_test.go` — 4 TestInvariant2_* tests (overlap, no-overlap, documented-shared, fail-open)
- `internal/scheduler/worker.go` — TrackBlocked constant

### Test coverage

- `TestInvariant2_OverlapBlocksSecondTrack` — AC-1/AC-5: overlap → INVARIANT-2 logged, T2 retries and passes
- `TestInvariant2_NoOverlapBothRun` — disjoint → both launch
- `TestInvariant2_DocumentedSharedExempt` — AC-3: doc-shared overlap → no block
- `TestInvariant2_OracleReadFailureFailsOpen` — AC-4: error → empty set, launches

### Trade-offs

- `stderr` capture for INVARIANT-2 log verification (AC-1 test). Simpler than plumbing a logger interface but couples test to `os.Stderr`.
- PlannedFilesFn default reads from release-wt ref — requires git repo at run time, which is fine for production but means the default path is not exercised in unit tests (which inject fakes). This is the established pattern in this file (Router, MergeTrackFn, etc.).

## Verifier verdicts received

### 2026-07-28T22:30:00Z — PASS

Verifier session: fresh, artefact-only. Verdict: PASS.

All 6 gates passed:
- Gate 1 (User-reachable): `RunParallel` is the core of `sworn run`; the disjointness check is wired into the dispatch path before goroutine launch.
- Gate 2 (Touchpoints): `parallel_test.go` is the test file; `worker.go` adds the single `TrackBlocked` constant — minimal necessary cross-file touch. Both documented in `actual_files`.
- Gate 3 (Tests): 4 `TestInvariant2_*` tests pass, exercising the integration point via `RunParallel` with injected `PlannedFilesFn`.
- Gate 3b (AC satisfaction LLM): Skipped — LLM provider not configured (non-blocking).
- Gate 4 (Reachability): Test output shows INVARIANT-2 message and track blocking/retry in stderr.
- Gate 4b (Semantic coverage LLM): Skipped — LLM provider not configured (non-blocking).
- Gate 5 (Deferrals): No TODO/FIXME/placeholder in production code.
- Gate 6 (Design): Non-UI project — passes automatically.
- Gate 7 (Claims): All 5 AC delivery claims verified against live code.

Next step: `/implement-slice S07-pause-resume-committed 2026-06-27-conformance-foundation`