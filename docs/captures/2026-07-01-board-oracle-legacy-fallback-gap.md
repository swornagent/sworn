# Board oracle: legacy index.md fallback bypasses S05 strict release reader

2026-07-01. Found during `/merge-release 2026-06-30-sworn-operational-readiness`.
Tracked: sworn#42.

## What happened

S05-board-canonical-emit (merged as part of `2026-06-30-sworn-operational-readiness`)
claims: "sworn always EMITS, VALIDATES, and now READS only the canonical baton
object form for a board's `release` (strict) — a legacy bare-string release
fails closed on read."

While running `/merge-release`, a manual test of `sworn board --release <name>
--json` against a release whose primary-worktree `board.json` was (at that
moment) still the legacy bare-string form succeeded — appearing to contradict
S05's claim.

## First conclusion was wrong

That manual test was misleading, not a real repro. `cmd/sworn/board.go:48`
resolves `releaseRef := "refs/heads/release-wt/" + release` and reads via
`git show <releaseRef>:<path>` — **never the working tree**. The release-wt
branch for that release had already been migrated to the canonical object
form by its T4/S05 track merge, so the command legitimately read good data
from a different ref than the stale file being eyeballed. No defect in this
narrow case — confirmed by `TestReadTrackInfos_BareStringRelease_FailsClosed`,
which passes even on the pre-fix code.

## The real gap

`internal/board/oracle.go:readTrackInfos` tries `board.json` at `releaseRef`
first (both the canonical and Fumadocs-prefixed paths). On ANY failure to
find it there, it silently falls through to the legacy `index.md`
YAML-frontmatter parser (`ParseTracks`), which never constructs a `Release`
value — so the S05 strict check never runs on that path.

Concretely: if a release has migrated to `board.json` (a copy exists on
`HEAD`) but `releaseRef` (release-wt) hasn't absorbed that commit — and a
stale legacy `index.md` is still committed there — the oracle silently
returns board state parsed from the stale legacy file instead of erroring.
This bypasses S05 entirely for a release that has, in fact, migrated.

Genuinely-legacy releases (never had `board.json`, e.g.
`2026-06-19-safe-parallelism`, `2026-06-16-fidelity-layer`) legitimately need
the `index.md` fallback and must keep working.

## Why it passed verification (Rule 1 gap)

S05's own tests (`internal/board/board_release_test.go` —
`TestRelease_StringForm_FailsClosed`, `TestRelease_BareStringRead_FailsClosed`)
call `json.Unmarshal` directly on the `Release` / `BoardRecord` type in
isolation. They never go through `readTrackInfos`, `oracle.ReadBoard`, or
`cmd/sworn board` — the actual integration point an operator (or
`/merge-release`'s Step 1 oracle gate) exercises. This is the Baton Rule 1
("Reachability Gate") red flag by name: "a component imported only by its
own test file."

S05's proof/reachability artefact only ran `sworn board` against a
well-formed canonical board (the positive case) — never a migrated-but-desynced
one. S05 also has no `review.md`, unlike its siblings S01/S02/S03/S06 in the
same release — no human design-review pass existed to ask whether the
negative test reached the real entry point.

## Fix

`internal/board/oracle.go:readTrackInfos`: before falling back to legacy
`index.md`, check `reader.CatFileExists("HEAD", boardPath)` for both the
canonical and Fumadocs-prefixed paths. If `board.json` exists on `HEAD` but
not on `releaseRef`, return a hard error instead of silently using the
legacy parser. If it exists nowhere (including `HEAD`), fall back to
`index.md` as before — genuinely-legacy releases are unaffected.

Added to `internal/board/oracle_test.go` (exercising `readTrackInfos`
directly — the real integration point, not the leaf `Release` type):

- `TestReadTrackInfos_BareStringRelease_FailsClosed` — already passed
  pre-fix; confirms the S05 strict type itself was never broken.
- `TestReadTrackInfos_MigratedOnHeadButMissingOnReleaseRef_FailsClosed` —
  failed pre-fix (TDD red), passes post-fix.
- `TestReadTrackInfos_NeverMigrated_UsesLegacyIndexMD` — regression guard;
  confirms genuinely-legacy releases still resolve via `index.md`.

## Reachability artefact

Built `sworn` binaries from before and after the fix, ran both against a
hand-built scratch git repo reproducing the exact scenario (board.json on
`HEAD`, legacy `index.md` only on `release-wt/demo-release`):

- Before: `sworn board --release demo-release --json` → exit 0, returns
  stale/legacy track data derived from `index.md`.
- After: same command → exit 2, `sworn board: board.json exists on HEAD
  (docs/release/demo-release/board.json) but not on
  refs/heads/release-wt/demo-release — this release has migrated to
  board.json; sync releaseRef before reading it rather than falling back to
  legacy index.md`.

Full `go test ./...` passes after the change.
