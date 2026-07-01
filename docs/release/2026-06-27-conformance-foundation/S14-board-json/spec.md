---
title: 'S14 — board.json as oracle source of truth'
description: 'Build board.json as the oracle''s data source; the oracle reads board.json and renders/drifts index.md from it; existing releases auto-migrate on first read. Eliminates the index.md YAML frontmatter corruption surface (ADR-0009).'
---

# Slice: `S14-board-json`

## User outcome

When the board oracle reads a release, it reads `board.json` (not index.md YAML frontmatter). Existing releases that have no `board.json` automatically generate one from the current index.md frontmatter on the first oracle read (lazy migration). Writing to the board (e.g. updating a track state) updates `board.json` and re-renders index.md from it, so index.md stays in sync but is no longer the authoritative source.

## Entry point

`board.Oracle` — all callers of `oracle.ReadSliceStatus()`, `oracle.ReadTracks()`, and the board parse path (`ParseTracks`) in `internal/board/oracle.go` and `internal/board/index.go`.

## In scope

- New `internal/board/board.go`: `BoardRecord` type (board-v1 JSON schema); `ReadBoard(repoRoot, release string) (*BoardRecord, error)` and `WriteBoard(repoRoot, release string, b *BoardRecord) error` — reads/writes `docs/release/<release>/board.json`
- `BoardRecord` shape: mirrors the current `index.md` frontmatter but as a typed Go struct with JSON tags; fields: `schema_version`, `release`, `release_worktree_path`, `release_worktree_branch`, `tracks` (array of Track), `slices` (derived — populated from the per-slice status.json, not stored redundantly in board.json)
- Lazy migration: `ReadBoard()` returns a generated `BoardRecord` from index.md frontmatter if `board.json` does not exist, then writes it (creates board.json on first read)
- `internal/board/oracle.go`: update `ReadSliceStatus()`, `ReadTracks()`, and the release-ref read path to call `ReadBoard()` instead of extracting YAML frontmatter from index.md; maintain a rendered index.md drift-check (read board.json, render expected frontmatter, compare with actual index.md; warn on drift)
- `internal/board/oracle.go` schema validation: add `validator.Validate("board-v1", data)` call after writing board.json (using the embedded schema from S13)
- Add `internal/baton/schemas/board-v1.json` to the embedded schemas (prerequisite: S13 must ship or be in same track — it is, T4)

## Out of scope

- Changing the slice-status-v1 schema (S13)
- spec.json or proof.json (S15)
- The merge gate reading from oracle (S05) — S05 calls the existing oracle API; this slice changes what the oracle reads internally

## Planned touchpoints

- `internal/board/board.go` (new — BoardRecord type + read/write)
- `internal/board/oracle.go` (update to read board.json via ReadBoard())
- `internal/board/index.go` (update render path from board.json + drift guard)
- `internal/baton/schemas/board-v1.json` (new embedded schema — add to S13's embedding directory)

## Acceptance checks

- [ ] WHEN a release has no `board.json`, `ReadBoard()` generates one from index.md frontmatter AND writes it to `docs/release/<release>/board.json` (lazy migration)
- [ ] WHEN `board.json` exists, `ReadBoard()` reads it (does NOT read index.md frontmatter for track data)
- [ ] WHEN `board.Oracle.ReadSliceStatus()` is called, it internally calls `ReadBoard()` and derives the track ownership from `board.json` tracks, not from index.md YAML parsing
- [ ] WHEN `board.json` is written, the drift guard reads the current index.md and logs a warning if the frontmatter does not match the board.json tracks (does not BLOCK on drift — drift guard is advisory for this slice)
- [ ] `board_test.go`: round-trip test — create index.md with frontmatter, call ReadBoard (triggers migration), verify board.json written with correct track data; modify board.json directly, call oracle.ReadSliceStatus, verify it reads from board.json
- [ ] `grep -rn "extractFrontmatterBody\|ParseTracks" internal/board/oracle.go` — after this slice, these functions are called only during migration, not as the primary data path

## Required tests

- **Unit**: `internal/board/board_test.go` (new or extend) — lazy migration scenario + board.json read scenario
- **Integration**: update `internal/board/oracle_test.go` to use a board.json as the source (not index.md frontmatter)
- **Reachability artefact**: `go test ./internal/board/... -v` exits 0

## Risks

- `board.json` lazy migration writes to the repo; in a test environment this may pollute the test repo — use t.TempDir() for all oracle tests
- The drift-guard comparison between board.json and index.md frontmatter requires a YAML serialiser; prefer a simple string comparison of the `tracks:` section to avoid adding a yaml dep

## Deferrals allowed?

Yes — the drift guard can be advisory (warning only) for this slice; promoting it to BLOCK can be a follow-up. Rule 2: Why = BLOCK would require all existing releases to be migrated first; warning is safe for initial rollout. Tracking = this spec's open_deferrals. Acknowledged = Brad, 2026-06-27.
