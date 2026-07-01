# Design TL;DR — S04-mcp-oracle-migration

**Slice:** migrate every MCP tool that reads or acts on release state off the
broken `board.ParseTracks(extractFrontmatterBody(index.md))` scrape and onto the
already-correct `internal/board` oracle (board.json), so an agent gets real data
for a current-format release instead of silently-empty results, a wrong-track
error, or (in the plan-mutation path) a silent board wipe on write.

**Track:** T3-mcp · **State target of this doc:** `design_review` (Rule 9 gate — no
code until PROCEED).

---

## Root cause (one sentence)

Six MCP call sites parse tracks out of `index.md` YAML frontmatter, but a
current-format `index.md` renders tracks as a **markdown table** and carries no
`tracks:` frontmatter — so `ParseTracks` returns **zero tracks**: reads go
silently empty and `tools_plan.go` writes that empty parse back, wiping the board.

## Approach

The correct engine already exists in `internal/board` and is what the other
consumers in this release migrate to:

- `board.ReadBoard(repoRoot, release) (*BoardRecord, error)` — reads `board.json`
  (the oracle's source of truth; typed `.Tracks []BoardTrack` with `ID`, `State`,
  `Slices`, `WorktreePath`, `WorktreeBranch`, and `.ReleaseWorktreePath`).
- `board.WriteBoard(repoRoot, release, *BoardRecord) error` — writes `board.json`.
- `board.RenderToFile(projectRoot, release) error` — deterministically renders
  `index.md` from `board.json` + slice records.

The migration is therefore mostly mechanical: replace each
`extractFrontmatterBody(...)` + `ParseTracks(...)` pair with `ReadBoard`, iterate
`.Tracks`. The one non-mechanical site is `tools_plan.go`, whose write path is
rebuilt on `ReadBoard → mutate .Tracks → WriteBoard → RenderToFile` (deleting the
~120 lines of frontmatter/body string-munging that produce a format the renderer
no longer emits).

The read paths that already branch to a git-ref oracle when `ot.repo != nil`
(`readReleaseBoardOracle`, `checkTrackVerifiedOracle`) are **already correct** and
are left as-is; this slice only fixes the `repo == nil` filesystem/fallback paths
and the sites that have no oracle path at all.

## AC → change map (traceability)

| AC | Site(s) | Change |
|----|---------|--------|
| **AC-01** | `tools_ops.go` `readReleaseBoard` (222-223), `findBlockedInRelease` (320-321), `handleApproveMerge` (445-446 tracks + 481/569 `extractReleaseWorktreePath`), `handleListReleases` (694-695) | Swap frontmatter scrape → `board.ReadBoard`; read release worktree path from `.ReleaseWorktreePath` not the frontmatter scrape. |
| **AC-02** | `context.go` `AssembleSliceContext` (61-62 worktree, 43-46 violations) | Worktree path via `ReadBoard`; violations via new `readNotDelivered(sliceDir)` reading `proof.json.not_delivered` instead of the `extractViolations` proof.md regex. |
| **AC-03** | `tools_plan.go` (121-252) | Rebuild write path: `ReadBoard` → find/append track in `.Tracks` → `WriteBoard` → `RenderToFile`. No read from stale frontmatter; no hand-built `tracks:` YAML. |
| **AC-04** | `catalog.go` `releaseStateSummary`/`countSliceTableRows` (355-397) | Derive counts from `ReadBoard().Tracks` + each slice's `status.json` state (via `state.Read`), not `state:`-line/table-header greps that never match the renderer output. |
| **AC-05** | `tools_test.go`, `lint_test.go`, `catalog_test.go` | Replace hand-written legacy-`tracks:`-YAML `index.md` fixtures with a helper that writes a `board.json` (`WriteBoard`) then `RenderToFile` — fixtures now exercise real renderer output. |
| **AC-06** | whole slice | `go build ./...` and `go test ./internal/mcp/...` green. |

## Key design choices + rationale

1. **Oracle entry point = `board.ReadBoard` (working-tree `board.json`), not the
   git-ref `Oracle`, for the filesystem/fallback paths.** Stakes: **Type-2**
   (local, reversible, within an established pattern). The MCP tools operate on a
   single working tree; the git-ref `Oracle` is already wired on the `repo != nil`
   branches and stays. `ReadBoard` is board.go's documented "oracle's source of
   truth" and is exactly what the sibling migrations consume. *Reviewer eye: is
   `ReadBoard` (working-tree) the intended "oracle" for these fallback paths, or
   should the fallback be removed entirely in favour of requiring `repo != nil`?*

2. **`tools_plan.go` write-back becomes `board.json` + `RenderToFile`.** Stakes:
   **architecturally-significant / Type-1**, but **already ratified by the spec**:
   AC-03 mandates "read the existing tracks from board.json ... before mutating
   and writing back." The design implements the planner's decision; it does not
   originate it. *Reviewer: confirm the write path should mutate `board.json` and
   re-render, and that dropping the bespoke body-table rewrite is acceptable.*

3. **`not_delivered` reader tolerates both string and object items.** Stakes:
   **Type-2** (defensive). Live drift: proof-v1 schema declares string items, but
   real proof.json files (e.g. `S02-board-render`) store objects
   `{item, why, tracking, acknowledgement}`. The reader uses `.item` for objects,
   the raw string otherwise, so violations render for both shapes.

4. **Kill the redundant `extractViolations` proof.md scrape at both use sites**
   (`context.go` and `findBlockedInRelease`), replacing with the `proof.json`
   reader. Stakes: **Type-2** (consistency). Leaving one site on the regex while
   removing it elsewhere would be incoherent; the rationale explicitly folds this
   scraper into the slice.

5. **AC-05 fixture helper renders via the real path.** Stakes: **Type-2** (test
   infra). A `writeBoardFixture(t, tracks…)` helper writes `board.json` then calls
   `RenderToFile`, so every migrated test asserts against renderer output rather
   than a legacy shape the renderer never produces.

## Risks / pins for the reviewer

- **`BoardTrack` vs `TrackInfo` type seam.** `checkTrackVerified*` and the
  git-ref oracle's `trackMap` expect `board.TrackInfo`; `ReadBoard` yields
  `board.BoardTrack`. Plan: a tiny local adapter (shared fields: `ID`, `Slices`,
  `WorktreeBranch`). No behaviour change; just a type bridge.
- **`lint_test.go` per-test care.** Some of its `tracks:` heredocs may exercise
  the index.md *structural* validator rather than the oracle path. Each will be
  migrated to the render-based fixture individually; if any genuinely tests
  legacy-frontmatter validation being removed, that surfaces as a journal note,
  not a silent fixture deletion.
- **Dead code.** `extractViolations`, and possibly `extractFrontmatterBody` /
  `extractReleaseWorktreePath`, may become unused after the swap — removed if so,
  to avoid leaving the broken scrapers as attractive nuisances.

## Out of scope (Rule 2 observations, not fixed here)

- **`context.go` reads `spec.md` (line 37) but current slices carry `spec.json`.**
  A related drift, outside AC-02 (which is worktree-path + violations only).
  Tracked as an observation for a follow-up; will be journaled, not silently
  changed. *(No owning slice yet — flagged for the planner.)*
- **T2-tui recorded its worktree as a nested `worktree:{path,branch}` object,
  which the flat-key oracle parser (`board.go:88-89`) silently ignores.** Found
  while materialising T3-mcp (recorded in the flat form the parser reads). This is
  a sibling-track record-format drift; not this slice's code. Flagged for the
  planner/merge owner.
