# Implementation journal — S04-mcp-oracle-migration

## 2026-07-02 — initial implementation

- **State transition**: `design_review` → `in_progress` → `implemented`.
- **Touched files**:
  - `internal/mcp/tools_ops.go` — `readReleaseBoard`,
    `findBlockedInRelease`, `handleApproveMerge`, `handleListReleases`,
    `checkTrackVerifiedOracle`, `checkTrackVerifiedFS` now read
    `board.json` via `board.ReadBoard` instead of parsing
    `index.md` frontmatter with `board.ParseTracks`.
  - `internal/mcp/context.go` — `AssembleSliceContext` resolves
    the slice's worktree path via `board.ReadBoard` (not the
    `index.md` frontmatter) and reads `spec.json` (with a `spec.md`
    fallback) instead of `spec.md` only.
  - `internal/mcp/tools_plan.go` — `set_track` now reads via
    `board.ReadBoard` and writes via `board.WriteBoard`. The
    previous frontmatter write-back is gone, which removes the
    silently-wipe-track-data footgun for current-format releases.
  - `internal/mcp/catalog.go` — `releaseStateSummary` and
    `countSliceTableRows` derive counts from `board.json` +
    `status.json` (via `board.ReadBoard`) instead of grepping
    the rendered `index.md` table.
  - `internal/mcp/tools_plan_test.go` — `TestSetTrackUpdates` and
    `TestSetTrackColon` now assert against `board.json` (the
    oracle's source of truth) per the slice's AC-05.
- **Test results**: `go test ./...` — all packages PASS, no failures.
  `go vet ./...` — clean. `go build ./...` — exit 0.
- **Out-of-scope discoveries**: none. The slice's spec is
  delivered in full.
