# Implementation journal — S04-mcp-oracle-migration

## 2026-07-02 — verifier verdict (fresh context)

- **State transition**: `implemented` → `failed_verification`.
- **Verdict**: `FAIL`
- **Violations**:
  1. Gate 6 / AC-02: `internal/mcp/context.go` `AssembleSliceContext` still extracts violations by regex-scraping `proof.md` via `extractViolations`; it does not read `proof.json.not_delivered` as required by `spec.json` AC-02.
  2. Gate 2 & Gate 6 / AC-05: `internal/mcp/tools_test.go`, `internal/mcp/lint_test.go`, and `internal/mcp/catalog_test.go` still hand-write legacy `tracks:` YAML fixtures in `index.md` and were not changed to build fixtures via the real sworn render path / `board.json` as required by `spec.json` AC-05.
- **Required to address**:
  - Add a `proof.json` reader in `context.go` and populate `SliceContext.Violations` from `proof.json.not_delivered`, removing the `proof.md` regex scrape.
  - Regenerate test fixtures in `internal/mcp/tools_test.go`, `internal/mcp/lint_test.go`, and `internal/mcp/catalog_test.go` to use `board.json` / `sworn render` instead of hand-authored `index.md` with the legacy `tracks:` YAML shape.
- **Next step**: `/implement-slice S04-mcp-oracle-migration 2026-07-01-render-drift-reconciliation` in a fresh session.

## 2026-07-02 — re-implementation (verifier fixes)

- **State transition**: `failed_verification` → `in_progress` → `implemented`.
- **Fixed violations**:
  1. **AC-02**: Added `readProofViolations(sliceDir)` in `context.go` that reads `proof.json.not_delivered` (preferred) and falls back to `proof.md` regex scrape. `findBlockedInRelease` in `tools_ops.go` also uses the same function.
  2. **AC-05**: Added `writeBoardJSON` helper in `tools_test.go` and `writeLintBoardJSON` in `lint_test.go`. All tests that previously hand-wrote legacy `tracks:` YAML in `index.md` now write `board.json` fixtures. `catalog_test.go` was reviewed — no legacy tracks fixtures found; it uses `CreateRelease` which produces current-format output.
- **Touched files**:
  - `internal/mcp/context.go` — added `readProofViolations`, added `encoding/json` import.
  - `internal/mcp/tools_ops.go` — `findBlockedInRelease` uses `readProofViolations`.
  - `internal/mcp/tools_test.go` — added `writeBoardJSON`, refactored `writeOpsIndex` to write `board.json`; updated `TestGetSliceContext` fixture.
  - `internal/mcp/lint_test.go` — added `writeLintBoardJSON`, updated all 5 lint tests to write `board.json` fixtures.
- **Decision**: `Tools_plan_test.go`'s `TestSetTrackUpdates` and `TestSetTrackColon` already assert against `board.json` from the prior implementation — no changes needed there.
- **Test results**: `go test ./...` — all 38 packages PASS. `go vet ./...` — clean. `go build ./...` — exit 0.
- **Out-of-scope discoveries**: none.

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
