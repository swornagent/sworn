# Proof Bundle — S06-invariant2-enforcement

## Scope

When `sworn run` dispatches two tracks concurrently, check planned_files disjointness; block overlapping tracks with named INVARIANT-2 report; retry after conflicting track merges.

## Files changed

- `internal/run/parallel.go` — PlannedFilesFn, parseDocumentedSharedFiles, checkDisjointness, makePlannedFilesReader, invariant-2 check in phase loop, follow-up phase
- `internal/run/parallel_test.go` — 4 TestInvariant2_* tests
- `internal/scheduler/worker.go` — TrackBlocked constant

## Test results

```
=== RUN   TestInvariant2_OverlapBlocksSecondTrack
--- PASS: TestInvariant2_OverlapBlocksSecondTrack (0.01s)
=== RUN   TestInvariant2_NoOverlapBothRun
--- PASS: TestInvariant2_NoOverlapBothRun (0.01s)
=== RUN   TestInvariant2_DocumentedSharedExempt
--- PASS: TestInvariant2_DocumentedSharedExempt (0.01s)
=== RUN   TestInvariant2_OracleReadFailureFailsOpen
--- PASS: TestInvariant2_OracleReadFailureFailsOpen (0.01s)
PASS
ok  	github.com/swornagent/sworn/internal/run	0.050s
```

## Reachability artefact

`go test ./internal/run/... -v -run TestInvariant2` exits 0

## Delivered

- **AC-1**: Overlap blocks second track with INVARIANT-2 message — `checkDisjointness` in phase loop; message logged to stderr; `TestInvariant2_OverlapBlocksSecondTrack` verifies
- **AC-2**: Retry after T_a merges — Follow-up phase after wg.Wait(); `TestInvariant2_OverlapBlocksSecondTrack` verifies T2 retried and passed
- **AC-3**: Documented-shared files exempt — `parseDocumentedSharedFiles` extracts from markdown body; `TestInvariant2_DocumentedSharedExempt` verifies
- **AC-4**: Oracle failure → fail open — Error path returns empty set; `TestInvariant2_OracleReadFailureFailsOpen` verifies
- **AC-5**: Unit test with mock oracle — `PlannedFilesFn` injection seam on ParallelOptions; 4 tests inject fake

## Not delivered

None. All 5 acceptance checks delivered.

## Divergence from plan

None. All 4 design-review pins applied inline. Design TL;DR approach followed exactly.