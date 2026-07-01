# Implementation journal — S06-core-loop-and-rtm-oracle-migration

## 2026-07-02 (UTC) — implementation (first pass)

- **State transition**: `design_review` → `in_progress` → `implemented`.
  `start_commit` set to `d427d181` (current HEAD at transition, first-set,
  never overwritten).
- **Pins addressed** (from `review.md`):
  - Pin 1 (effort_complexity "medium" enum blocking `state.Read`): already
    resolved release-wide by the earlier replan (`cfc0a2c`) — S06 already carries
    `low`/`low`/`chore`; `sworn designfit` and `state.Read` pass on all 7 slices.
    Confirmed live before writing code.
  - Pin 2 (design_decisions absent): backfilled the 5 Type-2 choices from
    design.md into `status.json.design_decisions` (all non-architecturally-
    significant).
  - Pin 3 (rtm.go's new board.json reader duplicates internal/board's unmarshal):
    added a cross-reference comment to `board.Release`'s JSON tags on the
    anonymous struct, plus `TestBuild_VerticalTraceFromBoardJSON` which
    round-trips a real board-shaped document through `readBoardVerticalTrace`.
  - Pin 4 (board.json `release` OBJECT shape): confirmed against the live
    board.json — `release.vertical_trace.benefit` present, no `org_objective`.
  - Pin 5 (extractReleaseWorktreePath duplicates in tools_ops.go/regress.go owned
    by S04/S05): informational, no action — deleted only `internal/run`'s copy.
  - Pin 6 (touchpoint-matrix collision avoidance): confirmed — zero
    `internal/board` edits; change stays inside T5's six declared touchpoints.
  - Pin 7 (AC-06 reachability substitute): implemented per the recorded Coach
    decision — option (a), Go-level `run.RunParallel` against the live board.json
    with a pausing router and throwaway in-memory DB.
- **Implementation**:
  - `internal/run/parallel.go` — `RunParallel` reads tracks + release worktree
    path from `board.ReadBoard`; documented-shared detection delegates to
    `router.ParseDocumentedShared` (fail-open). Added local
    `trackInfosFromBoardTracks`. Deleted dead `extractFrontmatter`,
    `extractReleaseWorktreePath`, `stripInlineComment`,
    `parseDocumentedSharedFiles`.
  - `internal/rtm/rtm.go` — added `readBoardVerticalTrace`; `Build` prefers
    board.json's vertical trace, falls back to the markdown parse when no
    board.json exists. `parseReleaseBenefit`/`parseOrgObjective` retained as the
    fallback (still have a live call path).
  - Tests: `parallel_test.go` — new `TestInvariant2_DocumentedSharedFromRenderedBoard`
    (AC-03), new `TestRunParallel_AC06_RealReleaseBoardResolvesTracks` (AC-06),
    added `## Touchpoint matrix` heading to the `TestInvariant2_DocumentedSharedExempt`
    fixture (the router parser requires the literal heading), removed
    `TestExtractFrontmatter`/`TestExtractReleaseWorktreePath`. `cold_start_test.go`
    — removed `TestStripInlineComment`/`TestExtractReleaseWorktreePath_CommentPlaceholder`.
    `rtm_test.go` — new `TestBuild_VerticalTraceFromBoardJSON`/`_LegacyFallback`.
    `router_test.go` — new `TestParseDocumentedSharedFromRenderedBoard` (AC-05).
- **TDD note (Rule 1)**: the AC-03 test was proven a real red — forcing
  `docShared = nil` in `RunParallel` makes it FAIL (invariant-2 wrongly blocks a
  track), and the delegation makes it PASS. The failing test renders through the
  `RunParallel` integration point that owns the affordance.
- **Test results**: `go build ./...` exit 0;
  `go test ./internal/run/... ./internal/rtm/... ./internal/router/...` — all
  pass (`internal/run` 5.100s, `internal/rtm` 0.023s, `internal/router` 0.053s);
  `go vet` clean; `gofmt -l` empty on all touched files.
- **Out-of-scope discoveries**: none beyond design.md's already-recorded Rule 2
  deferrals (`migrateFromIndex` discards vertical_trace when lazily migrating a
  legacy release — inside `internal/board/board.go`, T1's exclusive touchpoint,
  flagged to the Coach for T1 or a follow-up).
- **Rule 2 deferral (llm-check)**: `sworn llm-check --type ac-satisfaction`
  cannot run in this session — no `SWORN_ANTHROPIC_API_KEY` credential available
  (why). Tracking: the fresh-context `/verify-slice` dispatch is the model-backed
  check for this slice. Acknowledgement: surfaced in the implementer's
  session-end output.
- **Next step**: `/verify-slice S06-core-loop-and-rtm-oracle-migration
  2026-07-01-render-drift-reconciliation` in a fresh session.
