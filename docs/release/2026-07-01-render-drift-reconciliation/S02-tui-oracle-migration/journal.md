# Journal — S02-tui-oracle-migration

## 2026-07-02 — Implementer session start

Design review (`review.md`, commit `132a43a`) returned `DECISION: PROCEED`,
`CONSTITUTIONAL: no`, 3 pins (1 mechanical, 2 memory-cited, 0 escalate). No
`approved-ack.md` marker convention exists in this repo, so per Rule 9
(design stays human-owned) and S01's precedent, recording the acknowledgement
here as the durable artefact: the Coach (Brad) dispatched this implementer
session directly against the reviewed design.md, which is itself the
acknowledgement that PROCEED stands (no escalations were raised requiring a
separate reply).

Applying the 3 pins inline during implementation:
1. **Proof-visibility theme scope check (pin 1).** DC-1's local `struct{
   NotDelivered []string }` read of `proof.json` in `blocked.go` is a narrow,
   spec-scoped fix for AC-03 only — not a stand-in for the future
   proof-panel theme (`project_proof_visibility_theme`). Confirmed
   understood as such; noting it explicitly in `proof.json`'s `divergence`
   section so the later proof-visibility release isn't surprised.
2. **Board-v1 shape cross-check (pin 2).** Already independently verified by
   the Captain against live code (`StringRelease` emits the canonical
   object form) — no action needed here beyond citing the confirmation,
   which I'm doing in this note per the review's suggested acknowledgement.
3. **tui_test.go touchpoint overlap with S03-tui-chrome-rework (pin 3).**
   No design change — S03 is still `planned`, single serial implementer per
   track worktree means S02 lands first. Flagging here so S03's implementer
   inherits the regenerated `writeBoardFixture` helper without surprise.

Plan: migrate `board.go` `LoadBoard` and `blocked.go` `LoadBlockedView` /
`ExtractViolations` onto `internal/board.ReadBoard`, following the same
pattern already landed in `internal/mcp/context.go` (S04, same release).
Regenerate the hand-authored `tracks:` frontmatter test fixtures in
`tui_test.go` via a new `writeBoardFixture` helper that calls the real
`board.WriteBoard` path, keeping exactly one dedicated test on the legacy
frontmatter-only path to prove AC-06.

## 2026-07-02 — Implementation complete

Migrated both loaders to `board.ReadBoard`:

- `board.go` `LoadBoard`: replaced the `yaml.Unmarshal` frontmatter parse
  with `board.ReadBoard(repoRoot, releaseName)`, mapping `board.BoardTrack`
  to the existing `tui.TrackInfo` (joining `DependsOn` `StringList` into the
  single `Depends` display string, as design.md specified). Dropped the
  `gopkg.in/yaml.v3` import.
- `blocked.go` `LoadBlockedView`: replaced the frontmatter `worktree_path`
  parse with `board.ReadBoard`, matching the track whose `Slices` contains
  the target slice ID (matching S04's `AssembleSliceContext` pattern —
  membership in `Slices`, not a `status.json.track` field match, since that
  field is a hint not an authoritative key). Preserved the existing
  `worktreePath == "" -> repoRoot` fallback. Dropped `gopkg.in/yaml.v3`.
- `ExtractViolations` deleted; `blocked.go` now reads `proof.json`'s
  `not_delivered` array directly via a local minimal struct (DC-1), each
  entry surfaced as one violation line. `proofContent` (raw `proof.md`) is
  kept only for the `[4] view full proof bundle` display — unchanged UX.

Test fixtures in `tui_test.go`: added `writeBoardFixture(t, root, release,
tracks []board.BoardTrack)`, which constructs a `board.BoardRecord` and
calls `board.WriteBoard` (the real write path, validated against
`board-v1`). Converted every test that hand-authored `tracks:` frontmatter
and drove `LoadBoard`/`LoadBlockedView` through it — `TestBoardViewShowsSlices`,
`TestAutoTransitionNoTracks`, `TestDeferWritesRuleTwo`,
`TestBoardEnterTransitionsToBlocked`,
`TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict`,
`TestBlockedPanelViewProof`, `TestBoardViewShowsMergeBadge`,
`TestBoardViewNoMergeBadge` — to `writeBoardFixture`. Added a new dedicated
`TestBoardViewLegacyIndexFallback` that writes ONLY a legacy `tracks:`
frontmatter `index.md` (no `board.json`) and asserts `LoadBoard` still
populates tracks via `board.ReadBoard`'s `migrateFromIndex` lazy path —
proving AC-06. `TestBlockedPanelExtractsViolations` was replaced with a
`proof.json`-fixture-driven test asserting `LoadBlockedView`'s violations
come from `not_delivered`.

Added an AC-05 reachability test (`TestBoardViewLoadsRealOperationalReadinessRelease`)
that drives the integration point (`BoardView.LoadBoard`, called from
`Model.handleReleasesKey` at `model.go:174`) rooted at this worktree's own
repo root, loading the real, committed
`2026-06-30-sworn-operational-readiness` release (`board.json`, 5 tracks)
and asserting all 5 track IDs are present with non-empty slice states.

`go build ./...` and `go test ./internal/tui/...` both green. `go vet
./internal/tui/...` clean. `gofmt -l` empty on all three touched files.

State -> `implemented`. Stopping here per role boundaries — no verifier
prompt in this session.
