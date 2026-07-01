# Design TL;DR — S02-tui-oracle-migration

**Slice state at authoring:** `planned` → this doc gates entry to `design_review` (Rule 9: design review before code).

## User outcome (from spec.json)

The sworn TUI board actually renders tracks and slices for any current-format
(`board.json`-backed) release — the originally reported "no tracks show" bug —
and the blocked-slice view resolves the real per-track worktree path and reads
violations from `proof.json` instead of a stale, silently-wrong fallback.

## Root cause (why the bug shipped)

`sworn render` moved the tracks list from index.md YAML frontmatter (`tracks:`)
into a Markdown **table**. Two TUI loaders still hand-parse the old frontmatter
with `yaml.Unmarshal`:

- `internal/tui/board.go` `LoadBoard` — parses `tracks:` frontmatter that render
  no longer emits → **silently sets zero tracks, returns no error**.
- `internal/tui/blocked.go` `LoadBlockedView` — same frontmatter parse for
  `worktree_path` (→ silently falls back to `repoRoot`), plus `ExtractViolations`
  regex-scrapes `proof.md` section bullets instead of reading the clean
  `proof.json.not_delivered` string array.

The existing `tui_test.go` tests hand-author the legacy `tracks:` frontmatter as
a Go string literal, so the fixture *encodes the pre-migration shape as the
contract* — the exact Reachability-Gate gap (Rule 1) that let the bug ship.

## Approach

Migrate both loaders onto `internal/board`'s already-correct, typed reader
`board.ReadBoard(repoRoot, release) (*BoardRecord, error)`. That reader:
returns typed `BoardTrack{ID, Slices, DependsOn, WorktreePath, WorktreeBranch,
State}` from `board.json` when present, and **lazily migrates** from index.md
frontmatter when `board.json` is absent (via `migrateFromIndex` → `ParseTracks`).
That lazy path **is** the AC-06 legacy fallback — I adopt it rather than
re-implement one.

### AC-by-AC design

- **AC-01 (`LoadBoard` reads board.json):** replace the frontmatter
  `yaml.Unmarshal` block with `br, err := board.ReadBoard(repoRoot, releaseName)`.
  Map each `board.BoardTrack` → the existing `tui.TrackInfo` (join `DependsOn`
  `StringList` into the single `Depends` display string). Slice-state loading
  from each `status.json` is unchanged. Removes the `gopkg.in/yaml.v3` import
  from board.go.
- **AC-02 (`LoadBlockedView` worktree from board.json):** replace its
  frontmatter parse with `board.ReadBoard`, find the track whose `ID == st.Track`,
  take `BoardTrack.WorktreePath`. Preserve the existing `worktreePath == "" →
  repoRoot` fallback. Removes the `yaml.v3` import from blocked.go.
- **AC-03 (violations from proof.json.not_delivered):** replace the `proof.md`
  scraper. `LoadBlockedView` reads `proof.json` and takes its `not_delivered`
  array as the violations. The `proof.md` read is **kept only** for the `[4]
  "view full proof bundle"` display (unchanged UX). `ExtractViolations` is
  refactored from `(proofContent string)` to a small typed JSON read of
  `proof.json` (see design choice DC-1).
- **AC-04 (regenerate fixtures via real render path):** add a test helper
  `writeBoardFixture(t, root, release, []board.BoardTrack)` that constructs a
  valid `BoardRecord` and calls `board.WriteBoard` — the **real internal/board
  write path**, which validates against `board-v1` (required track fields: `id`,
  `state`, `worktree_branch`). Convert every test that currently hand-authors
  `tracks:` frontmatter *and* drives `LoadBoard`/`LoadBlockedView` through it:
  `TestBoardViewShowsSlices`, `TestDeferWritesRuleTwo` (worktree_path →
  `WorktreePath`), `TestBoardEnterTransitionsToBlocked`,
  `TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict`,
  `TestBlockedPanelViewProof`. Update `TestBlockedPanelExtractsViolations` to
  feed a `proof.json` fixture. **Keep one dedicated test** exercising the
  no-`board.json` legacy fallback (index.md frontmatter only) to prove AC-06.
- **AC-05 (reachability artefact):** add an integration test that drives the
  **integration point** (`tui.Model`, which owns the affordance via
  `model.go:174` `m.Board.LoadBoard`), rooted at the live repo, selecting the
  real `2026-06-30-sworn-operational-readiness` release (committed `board.json`,
  5 tracks) and asserting `BoardView.Tracks` is non-empty with the expected
  track IDs. Plus an explicit manual smoke step in proof.json
  (`sworn` → select that release → Enter → observe real tracks).
- **AC-06 (legacy fallback preserved):** satisfied for free by `board.ReadBoard`'s
  `migrateFromIndex` path; proven by the dedicated legacy test above.
- **AC-07 (build + tests green):** `go build ./...` and `go test ./internal/tui/...`.

## Files to touch (matches spec touchpoints exactly)

- `internal/tui/board.go` — `LoadBoard` rewired to `board.ReadBoard`; drop yaml import.
- `internal/tui/blocked.go` — `LoadBlockedView` worktree + violations rewired; drop yaml import.
- `internal/tui/tui_test.go` — `writeBoardFixture` helper; convert legacy-shape
  fixtures; add AC-05 reachability test + AC-06 legacy-fallback test.

No production files outside the three spec touchpoints. No new dependencies
(net removal of `yaml.v3` from two files).

## Design choices for reviewer

- **DC-1 (Type-2, local/reversible) — proof.json reader location.** The TUI needs
  only `not_delivered` (a `proof-v1`-stable string array). I plan a **local
  minimal struct** (`struct{ NotDelivered []string \`json:"not_delivered"\` }`) in
  blocked.go rather than importing/exporting a reader from `internal/implement`
  (a heavy package: exec, baton, git). Rationale: narrow one-shot read of a
  contract-stable field; importing the implement package into the presentation
  layer is heavier coupling than a 3-line struct. **Alternative** if the reviewer
  prefers DRY: export `implement.ReadNotDelivered(path) ([]string, error)` and
  call it. Flagging for a Captain call.
- **DC-2 (Type-2) — lazy-migration write side effect.** `board.ReadBoard` writes
  `board.json` when it lazily migrates a legacy release. So a TUI *browse* of a
  genuinely-pre-migration release will materialise `board.json` on disk. This is
  the oracle's existing designed behaviour (every other consumer already triggers
  it); adopting it keeps one source of truth. Called out for awareness — not a
  new mutation this slice invents.

## Design-level risks

- `tui.TrackInfo.Depends` is a single string; `BoardTrack.DependsOn` is a
  `StringList`. Join on display; the TUI only shows it, so lossless-enough. No
  consumer computes on `Depends`.
- `board.WriteBoard` in tests requires `baton` schema validation to pass — the
  helper must set `worktree_branch`/`state`/`id` on every fixture track.

## Traceability

| AC | Change | Test |
|----|--------|------|
| AC-01 | `LoadBoard` → `board.ReadBoard` | `TestBoardViewShowsSlices` (regenerated) |
| AC-02 | `LoadBlockedView` worktree from board.json | `TestDeferWritesRuleTwo` (regenerated) |
| AC-03 | violations from `proof.json.not_delivered` | `TestBlockedPanelExtractsViolations` (proof.json) |
| AC-04 | `writeBoardFixture` real render path | all regenerated tests |
| AC-05 | Model-driven integration render | new reachability test + manual smoke |
| AC-06 | legacy fallback via `ReadBoard` | new legacy-fallback test |
| AC-07 | build + tui tests | `go build ./...`, `go test ./internal/tui/...` |
