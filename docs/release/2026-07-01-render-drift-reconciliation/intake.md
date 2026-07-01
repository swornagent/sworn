---
title: Release intake — 2026-07-01-render-drift-reconciliation
description: Close the ADR-0009 gap - a producer (sworn render) changed its output format and ~15 independent consumers, plus the CI drift guard the ADR itself mandated, never got updated. Includes the TUI's functional fix and its requested visual rework.
---

# Release Intake: `2026-07-01-render-drift-reconciliation`

## Release goal

Every consumer that reads release-board state from `index.md` (frontmatter
YAML for tracks/worktree paths, or scraped `.md` sections for
violations/EARS/AC-counts) gets migrated to read the canonical JSON records
(`board.json` via the existing `internal/board` oracle, `proof.json`,
`spec.json`) instead — closing a systemic drift introduced when
`sworn render`'s output format changed and never propagated. Alongside the
fix, implement the CI drift guard ADR-0009 already mandated
("`committed.md == render(json)`, failing on divergence") so a future format
change fails closed instead of silently breaking N call sites again. The
TUI's board view — the originally reported bug — gets both its functional
fix and the visual rework requested in this session (header/version/active
release, viewport-fit, help-bar styling). "Shipped" = `go test ./...` green,
the drift guard passes on every current board.json-backed release, and the
TUI board actually renders + looks right.

## Source of truth

- **Human stakeholder**: Brad (project owner)
- **Tracking issue / epic**: none yet
- **Related captures**: this session's conversation is the primary record (no capture doc written yet — will add one alongside the drift-guard slice per Rule 3)
- **Related memory entries**: `feedback_releaseverify_specmd_false_fail` (same failure family: "a tightened reader/contract can regress test fixtures in OTHER packages"), `project_sworn_home_surface` (TUI header direction), `project_parallel_cold_start_broken` (2026-06-28 eval found real `--parallel` bugs — `internal/run/parallel.go`'s broken track-parsing found here may be a contributing root cause, not just a coincidence)

## Users and their gestures

- **Operator running `sworn` bare / `sworn top`**: sees the TUI board render correctly for any release, with a header showing sworn's version and the current active release.
- **Operator running `sworn merge-release` / `sworn regress`**: these CLI commands succeed against a canonical-shape (board.json) release instead of hard-erroring on a missing `release_worktree_path` frontmatter key.
- **Operator running `sworn run --parallel`**: the loop actually finds its tracks (today: hard error "no tracks found in release board" against any current-format release) and its shared-file/cold-start detection works correctly instead of silently misfiring.
- **Any MCP-connected agent** (Claude Code driving sworn via MCP tools): `get_blocked`, `get_slice_context`, board reads, and `approve_merge` all return real data instead of silently-empty results.
- **Future implementer changing `sworn render`'s output format**: gets a fail-closed CI error naming exactly which committed `.md` file diverged from its `.json` source, instead of silent, undetected breakage across N unrelated call sites.

## What's currently broken or missing

Confirmed via direct code reads and two parallel investigation agents, not guessed:

- `internal/tui/board.go` `LoadBoard` — hand-parses `index.md` frontmatter for a `tracks:` YAML key that `sworn render`'s current output never emits (tracks moved to a Markdown table). Silently renders zero tracks, no error. **The originally reported bug.**
- `internal/tui/blocked.go` `LoadBlockedView` — same frontmatter parse for `worktree_path`; silently falls back to `repoRoot` instead of the real per-track worktree path. Also has its own `ExtractViolations` that regex-scrapes `proof.md` sections instead of reading `proof.json.not_delivered` (already a clean string array).
- `internal/mcp/tools_ops.go` (4 call sites), `internal/mcp/context.go`, `internal/mcp/tools_plan.go`, `internal/mcp/catalog.go` — all call `board.ParseTracks(extractFrontmatterBody(...))` directly on raw `index.md`, bypassing the already-correct `internal/board.ReadBoard`/Oracle. `tools_plan.go` is worse than read-only: it *mutates* tracks parsed from the stale frontmatter and writes them back.
- `cmd/sworn/merge.go` (`resolveReleaseWorktree`) and `cmd/sworn/regress.go` (`extractReleaseWorktreePath`) — hard error `"release_worktree_path not found in index.md frontmatter"` against any current-format release.
- `internal/run/parallel.go` — `RunParallel` hard-errors `"no tracks found in release board"`; `extractReleaseWorktreePath` always falls into the cold-start branch (silently overriding an already-set worktree path); `parseDocumentedSharedFiles` always returns empty (silently disables invariant-2 shared-file detection).
- `internal/rtm/rtm.go` — `parseReleaseBenefit`/`parseOrgObjective` silently return empty strings for Rule 8's own golden-thread trace.
- `internal/mcp/context.go` `extractViolations`, `internal/account/notify.go` `ViolationsSummary` — two MORE independent regex scrapers of `proof.md` for violations, each with a different heuristic than `blocked.go`'s and than each other, all redundant with `proof.json.not_delivered`.
- `internal/ears/ears.go` — re-classifies EARS keywords from `spec.md` text on every lint run, redundant with `spec.json.acceptance_criteria[].ears_keyword` (already computed and stored at write time).
- `cmd/sworn/ledger.go` — counts acceptance checks via `- [ ]` lines in `spec.md` instead of `len(spec.json.acceptance_criteria)`.
- **The harness gap, refined**: `docs/adr/0009-records-json-prose-markdown.md`'s own Consequences section mandates a drift guard ("fail closed: treat rendered `.md` as build artifacts and add a CI check asserting `committed.md == render(json)`"). A guard exists — `internal/board/board.go`'s `driftGuard` function — but it is (a) advisory-only (`log.Printf`, explicitly documented "does not block the write" — not fail-closed as ADR-0009 requires), (b) only runs inside `WriteBoard` at planning/implementation time, not as a standalone CI/repo-wide check, and (c) itself calls `board.ParseTracks(extractFrontmatterBody(...))` on raw `index.md` — the exact same broken pattern as every other consumer in this intake, so it wouldn't even correctly detect the drift class this release exists to fix. A third instance of the bug, inside the package that's supposed to be the canonical fix.
- **Why nothing caught any of this**: every test for every listed consumer hand-writes its own `index.md` fixture in the pre-migration shape, directly in the test source (confirmed for `internal/tui/tui_test.go`, `internal/run/parallel_test.go`, `internal/run/cold_start_test.go`, `internal/mcp/tools_test.go`, `internal/mcp/lint_test.go`, `internal/rtm/rtm_test.go`, `internal/mcp/catalog_test.go`, `cmd/sworn/merge_test.go`). None generate the fixture via `sworn render`'s actual code. Same Rule-1-shaped gap as the earlier board-oracle bug, ~15 times over.
- **TUI visual issues** (screenshot: `docs/release/2026-07-01-render-drift-reconciliation/screenshots/2026-07-01-tui-current-state.png`): left pane too narrow, release names wrap across 2-3 lines making the list hard to scan; no header at all (jumps straight to the two-pane layout, no branding/version/active-release context); the bottom help line floats on the bare terminal background with no bar/background behind it, leaving black empty space at both edges instead of spanning full width; and a reported (not yet screenshotted) viewport issue in VS Code's integrated terminal where the first row or two of characters render above the visible viewport top on startup/resize.

## What the human wants

- **N-01**: every `index.md`/`proof.md`/`spec.md` consumer listed above reads its data from the canonical JSON record (`board.json` via the oracle, `proof.json`, `spec.json`) instead of hand-parsing the rendered `.md`.
- **N-02**: a fail-closed CI/lint check exists asserting every committed release's `index.md` matches `render(board.json + slice records)` — the ADR-0009-mandated drift guard.
- **N-03**: the TUI board actually renders tracks/slices for a current-format release (the reported bug, now root-caused as an instance of N-01).
- **N-04**: TUI visual rework — a header announcing sworn's version and the current active release; the release list pane sized/wrapped so entries are readable; the bottom help bar styled as a full-width bar, not floating text; the viewport-fit issue (content rendering above the terminal's visible top row in some terminals) fixed.
- Brad's words: "bloody hell, this is an absolute mess" — the priority is breadth (find and close every instance of this pattern), not just the one reported symptom.

## Constraints and non-negotiables

- Public-safe repo (project CLAUDE.md): no business/pricing/competitive content.
- Single Go binary, minimal justified deps — this release adds no new dependency (the fix is "read the JSON that's already there," not new infrastructure).
- `internal/board`'s `Oracle`/`ReadBoard`/`BoardRecord` machinery is already correct (it's what `cmd/sworn board` and this session's own `/merge-release` use) — the fix pattern everywhere is "point the consumer at the oracle," not "invent a new reader."
- Backward compatibility for genuinely pre-migration (no-`board.json`) releases must be preserved wherever a consumer currently supports them (the oracle already has this fallback; consumers doing their own ad hoc parsing generally do not need to preserve anything beyond matching the oracle's existing contract).

## Adjacent / out of scope

- **Item**: `touchpoints` field for spec.json / touchpoint-matrix consumers (`internal/lint/touchpoints.go`, `internal/reqverify/reqverify.go`, `internal/gate/coverage.go`, `internal/specquality/specquality.go`, `internal/rtm/rtm.go`'s touchpoint reads) still parse `spec.md` only. **Why deferred**: this is a schema/implementation gap, not stale-reader drift — `touchpoints` isn't a field in `spec-v1.json` or the Go `specRecord` struct at all yet, so there is no JSON source to migrate to. A real feature (add the field, wire the writers), not a bug fix. **Tracking**: none yet — flag for a future release. **Acknowledged**: 2026-07-01 (Brad, this session).
- **Item**: `internal/router/router.go`'s `ParseDocumentedShared`/`parseTouchpointMatrix` is structurally compatible with current `render.go` output (verified live against a real rendered fixture) but has no test exercising the multi-track/shared-file branch against a real rendered file — unverified, not confirmed broken. **Why deferred from the "fix" set**: nothing to fix, only to test. **Tracking**: folded into S06 as a regression test addition, not a separate slice. **Acknowledged**: 2026-07-01 (Brad, this session).
- **Item**: the `2026-07-01-loop-cli-ux` release (`sworn use`/bare `sworn loop`) and `S01-embedded-version` (release-hygiene, parked at `design_review`). **Why deferred**: lower severity than this release — cosmetic/ergonomic vs. this release's "the autonomous loop and merge/regress CLI are silently broken." **Tracking**: `docs/release/2026-07-01-loop-cli-ux/`, `docs/release/2026-07-01-release-hygiene/`. **Acknowledged**: 2026-07-01 (Brad, prior session turns — sequencing decision already made).

## Decisions made during planning

### 2026-07-01 — drift guard folds into `sworn doctor`

- **Context**: where should the ADR-0009-mandated `committed.md == render(json)` check hook in?
- **Options considered**: a new `sworn lint render-drift` subcommand (matches the `sworn lint ac`/`sworn lint trace` release-scoped pattern); fold into `sworn doctor` (repo-wide health-check pattern).
- **Decision**: fold into `sworn doctor`.
- **Why**: `doctor` already runs exactly this class of check (legacy `docs/baton/` detection, embedded-prompt integrity, timestamp sanity) and is already wired into whatever CI/pre-commit hook runs it — no new command surface to remember or wire up separately.

### 2026-07-01 — TUI header sources "active release" from its own selection state

- **Context**: S03's header announces the current active release — from the TUI's own navigation state, or from the separate `2026-07-01-loop-cli-ux` release's (not-yet-built) persisted active-release store?
- **Options considered**: TUI's own selection state (self-contained); read from `loop-cli-ux`'s `.sworn/sworn.db` store once it exists.
- **Decision**: TUI's own selection state — whichever release is currently navigated to inside the TUI, or "none selected" on the initial releases-list screen.
- **Why**: keeps this release fully independent of `loop-cli-ux`'s landing order; the TUI's own navigation state is a real, always-available answer to "what's active" that needs no cross-release coupling. (If `loop-cli-ux` lands later, the TUI could additionally default its initial cursor to that stored release — a small, separate follow-up, not blocking this slice.)

### 2026-07-01 — 5 independent tracks, 7 slices, confirmed as drafted

- **Context**: confirm the drafted track/slice decomposition (drift-guard, TUI×2, MCP, CLI, core-loop+RTM) before writing specs.
- **Options considered**: 5 tracks as drafted (max parallelism, each touchpoint-disjoint); fewer/bigger tracks (less worktree overhead, less parallelism).
- **Decision**: 5 tracks as drafted — `T1-drift-guard`, `T2-tui` (S02, S03), `T3-mcp`, `T4-cli-merge-regress`, `T5-core-loop-rtm-rescrape` (S06, S07 folded together — see touchpoint matrix below).
- **Why**: confirmed touchpoint-disjoint across all 5; each subsystem's fix is independently mergeable and independently valuable (e.g. the MCP fix doesn't need to wait on the TUI fix), so there's no reason to force serialization.

## Schema-vs-spec audit notes

`proof-v1.json`'s `not_delivered` field is already a clean string array (confirmed by direct schema read) — the three independent `proof.md`-scraping call sites are pure redundancy, not filling a schema gap. `spec-v1.json`'s `acceptance_criteria[].ears_keyword` is already computed and stored at write time (`internal/implement/spec_record.go`) — `internal/ears/ears.go`'s re-classification from `spec.md` text is redundant, not a schema gap. By contrast, `touchpoints` genuinely has no JSON home yet (see Adjacent/out of scope) — confirmed by reading the current Go `specRecord` struct and `spec-v1.json`, not assumed.

## Proposed slice decomposition (final)

- `T1-drift-guard` / `S01-render-drift-guard` — implement the ADR-0009-mandated CI/lint check, folded into `sworn doctor`: fail closed on any committed `index.md` that doesn't match `render(board.json + slice records)` for a board.json-backed release.
- `T2-tui` / `S02-tui-oracle-migration` — migrate `internal/tui/board.go` and `internal/tui/blocked.go` (tracks/worktree-path parsing AND `blocked.go`'s violations-from-`proof.md` scraping, same file) to read `board.json`/`proof.json` via the oracle; regenerate stale test fixtures from real `sworn render` output.
- `T2-tui` / `S03-tui-chrome-rework` (depends on S02 within the track — same package, sequential) — header (version + TUI's own active-release selection state), release-list pane sizing/wrapping, full-width help bar, viewport-fit fix.
- `T3-mcp` / `S04-mcp-oracle-migration` — migrate `internal/mcp/tools_ops.go`, `tools_plan.go`, `catalog.go`, `context.go` (tracks-parsing AND violations-parsing, same file) to the oracle/`proof.json`.
- `T4-cli-merge-regress` / `S05-cli-merge-regress-oracle-migration` — migrate `cmd/sworn/merge.go`, `cmd/sworn/regress.go`.
- `T5-core-loop-rtm-rescrape` / `S06-core-loop-and-rtm-oracle-migration` — migrate `internal/run/parallel.go`, `internal/rtm/rtm.go`; add the `internal/router` multi-track regression test noted above.
- `T5-core-loop-rtm-rescrape` / `S07-remaining-rescrape-cleanup` (sequential after S06 — same track) — `internal/account/notify.go` (violations from `proof.json`), `internal/ears/ears.go` (EARS from `spec.json`), `cmd/sworn/ledger.go` (AC count from `spec.json`).

All 5 tracks are independent (`depends_on: []`) and touchpoint-disjoint — verified no file appears under two different tracks (see touchpoint matrix, rendered in `index.md`).

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Where does the drift guard hook in? | S01 scope | **Resolved 2026-07-01**: fold into `sworn doctor` as a new check. |
| A-02 | S03's header "current active release" source? | S03 AC | **Resolved 2026-07-01**: TUI's own selection/navigation state, not the loop-cli-ux store. |
| A-03 | Track grouping and count? | all slices | **Resolved 2026-07-01**: 5 tracks as drafted, confirmed touchpoint-disjoint. |

## Screenshots / references

- `docs/release/2026-07-01-render-drift-reconciliation/screenshots/2026-07-01-tui-current-state.png` — current TUI releases-list + board pane, showing the narrow-pane text wrapping and the placeholder "Select a release from the left pane" state (pre-selection; the reported blank-board-after-selection bug isn't visible in this particular frame, but the layout issues are).
