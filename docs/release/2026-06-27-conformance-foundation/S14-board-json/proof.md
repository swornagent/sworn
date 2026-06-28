# Proof bundle — S14-board-json

## Scope

Build `board.json` as the oracle's source of truth for release board track metadata, replacing YAML frontmatter extraction from `index.md`. Existing releases auto-migrate from index.md on first `ReadBoard()` call. The oracle (`internal/board/oracle.go`) reads `board.json` first, falling back to index.md frontmatter for legacy releases.

## Files changed

```
docs/release/2026-06-27-conformance-foundation/S14-board-json/status.json
internal/baton/schemas/board-v1.json
internal/baton/schemas/embed.go
internal/baton/validator.go
internal/board/board.go
internal/board/board_test.go
internal/board/oracle.go
```

## Test results

```
$ go test ./internal/board/... -v
=== RUN   TestReadBoard_LazyMigration
--- PASS: TestReadBoard_LazyMigration (0.00s)
=== RUN   TestReadBoard_ExistingBoardJSON
--- PASS: TestReadBoard_ExistingBoardJSON (0.00s)
=== RUN   TestWriteBoard_Validation
--- PASS: TestWriteBoard_Validation (0.00s)
=== RUN   TestWriteBoard_RoundTrip
--- PASS: TestWriteBoard_RoundTrip (0.00s)
=== RUN   TestOracleReadBoard_BoardJSONFirst
--- PASS: TestOracleReadBoard_BoardJSONFirst (0.00s)
=== RUN   TestOracleReadBoard_FallbackToIndex
--- PASS: TestOracleReadBoard_FallbackToIndex (0.00s)
=== RUN   TestBoardTracksToTrackInfos_RoundTrip
--- PASS: TestBoardTracksToTrackInfos_RoundTrip (0.00s)
(all 27 board tests pass — new + existing)
PASS
ok  	github.com/swornagent/sworn/internal/board	0.111s

$ go test ./internal/baton/... -v -run "Validate"
(all 16 validator tests pass)
PASS
ok  	github.com/swornagent/sworn/internal/baton	0.004s
```

`go vet` and `go build ./...` both clean.

## Reachability artefact

`go test ./internal/board/... -v` exits 0 — the new `board.go` functions (`ReadBoard`, `WriteBoard`) are exercised through the oracle test path (`TestOracleReadBoard_BoardJSONFirst`) and filesystem tests (`TestReadBoard_LazyMigration`, `TestReadBoard_ExistingBoardJSON`, `TestWriteBoard_RoundTrip`).

## Delivered

- [x] **New `internal/board/board.go`**: `BoardRecord` and `BoardTrack` types; `ReadBoard()` (filesystem read + lazy migration); `WriteBoard()` (write + schema validation + advisory drift guard); `migrateFromIndex()` (index.md → board.json)
  - Evidence: `internal/board/board.go`
- [x] **New `internal/baton/schemas/board-v1.json`**: JSON Schema for board.json records with track-level required fields (`id`, `state`, `worktree_branch`) and board-level required fields (`schema_version`, `release`, `tracks`)
  - Evidence: `internal/baton/schemas/board-v1.json`
- [x] **Updated `internal/baton/schemas/embed.go`**: Embeds `board-v1.json`; registers in `SchemaMap` alongside `slice-status-v1`
  - Evidence: `internal/baton/schemas/embed.go`
- [x] **Updated `internal/baton/validator.go`**: Schema-dispatch `validateBoard()` for board-v1 with track-level required-field validation (distinct from slice-status-v1's slice-level checks)
  - Evidence: `internal/baton/validator.go`
- [x] **Updated `internal/board/oracle.go`**: `readTrackInfos()` tries `board.json` first (via git ref), falls back to `index.md` frontmatter; `ReadBoard()` and `NewOracleReaderAdapter()` use this path
  - Evidence: `internal/board/oracle.go`
- [x] **AC: Lazy migration test** — `TestReadBoard_LazyMigration` verifies `ReadBoard()` creates `board.json` from index.md frontmatter when none exists
  - Evidence: `internal/board/board_test.go:59`
- [x] **AC: Existing board.json read** — `TestReadBoard_ExistingBoardJSON` verifies `ReadBoard()` reads existing `board.json` instead of index.md
  - Evidence: `internal/board/board_test.go:108`
- [x] **AC: Oracle reads board.json first** — `TestOracleReadBoard_BoardJSONFirst` verifies the oracle reads from `board.json` even when `index.md` has different content
  - Evidence: `internal/board/board_test.go:210`
- [x] **AC: Oracle falls back to index.md** — `TestOracleReadBoard_FallbackToIndex` verifies legacy releases without `board.json` still work
  - Evidence: `internal/board/board_test.go:251`
- [x] **AC: Drift guard is advisory** — `TestWriteBoard_RoundTrip` exercises the drift guard; it logs a warning but does not return an error
  - Evidence: `internal/board/board_test.go:182` (log output visible in test run)
- [x] **AC: Grep check** — `extractFrontmatterBody` and `ParseTracks` in `oracle.go` are called only in `readTrackInfos()` (fallback path), not the primary data path
  - Evidence: `grep -rn "extractFrontmatterBody\|ParseTracks" internal/board/oracle.go` shows only definitions and the fallback call at line 404-405

## Not delivered

None — all acceptance checks delivered.

## Divergence from plan

- **`internal/board/index.go`** was listed in `planned_files` but was **not changed** — no drift guard rendering logic was added to index.go because the drift guard is a simple comparison in `board.go` (`driftGuard()`), not a render-then-compare. The index.md re-rendering from board.json is a future enhancement (would require YAML serialisation); the current drift guard only warns.
- **`internal/baton/validator.go`** was not in `planned_files` but was changed — the existing `Validate()` function was hardcoded for slice-status-v1 fields; adding board-v1 validation required a schema-dispatch refactor.
- **Dark-code marker** in `validator.go` line 12 (`is deferred to a follow-up ADR`) is pre-existing from S13 — not introduced by this slice.