# Design TL;DR — S05-cli-merge-regress-oracle-migration

**Slice state at authoring:** `planned` → this doc gates entry to `design_review`
(Rule 9: design review before code).

**Track:** T4-cli-merge-regress · **State target of this doc:** `design_review`
(Captain review, then Coach PROCEED, before any code is written).

## User outcome (from spec.json)

`sworn merge-release` and `sworn regress` succeed against any current-format
(`board.json`-backed) release instead of hard-erroring with `"release_worktree_path
not found in index.md frontmatter"`.

## Root cause (one sentence)

`cmd/sworn/merge.go`'s `resolveReleaseWorktree` and `cmd/sworn/regress.go`'s
`extractReleaseWorktreePath` both hand-parse `index.md` YAML frontmatter for a
`release_worktree_path:` key that `sworn render`'s current output never emits
(that field now lives in `board.json`'s top-level `release_worktree_path`) — a
hard error, not a silent failure, but a hard error against every current-format
release still means the command is broken.

## Approach

Same engine every sibling slice in this release migrates to: `internal/board`'s
oracle machinery. This slice has **two different resolution contexts** and picks
the oracle entry point that matches each command's existing resolution style,
rather than forcing both onto one mechanism:

- **`merge.go` already resolves state via git refs**, not the local working
  tree — both `cmdMergeTrack` and `cmdMergeRelease` already build a
  `board.OracleReaderAdapter` (`board.NewOracleReaderAdapterFromRepo(repo, rel,
  releaseRef)`) and call `oracleAdapter.ReadBoard(ctx, rel)` for slice/track
  state, immediately before the now-broken `resolveReleaseWorktree(repo, rel)`
  call. The release worktree path must come from the **same git ref**
  (`release-wt/<release>`) as everything else that function reads — a disk read
  would silently diverge from a launch directory sitting on a different branch
  (the exact `feedback_release_spec_forward_port` trap called out in the
  implementer harness). Fix: extend the oracle with a new
  `ReadReleaseWorktreePath` capability, alongside its existing `ReadBoard`/
  `ReadSliceStatus`, and have both merge commands call it through the
  `oracleAdapter` they already construct — no second oracle build, no new git
  read.
- **`regress.go` already resolves everything relative to the local working
  directory** (`resolveReleaseDir` joins `cwd + docs/release/<name>`, then
  reads `index.md` straight off disk) — it has no `git.Repo`/ref concept at
  all today. `board.ReadBoard(repoRoot, release) (*BoardRecord, error)` is the
  disk-based counterpart of the same oracle, already adopted for exactly this
  kind of filesystem-resolution call site by **S04** (MCP's `repo == nil`
  fallback paths — confirmed by reading S04's landed design.md/diff). It
  reads `board.json` when present and **lazily migrates** from `index.md`
  frontmatter when absent (`board.ReadBoard` → `migrateFromIndex`), which is
  also exactly the AC-03 legacy-fallback requirement, satisfied for free
  rather than reimplemented.

Both paths converge on the same on-disk/on-ref field:
`BoardRecord.ReleaseWorktreePath` (`internal/board/board.go:24`,
`json:"release_worktree_path,omitempty"`) — already populated by both the
`board.json`-read branch and the `migrateFromIndex` legacy branch. No schema
change; the field already exists and is already correctly populated. This is
a two-file, two-function fix plus one small new package-level oracle method —
matching the spec's own `effort: low / complexity: low` call.

### AC-by-AC design

- **AC-01 (`merge-release` reads via oracle, succeeds):** add
  `func (o *Oracle) ReadReleaseWorktreePath(reader gitContentReader, releaseRef,
  release string) (string, error)` in `internal/board/oracle.go`, colocated
  with `readTrackInfos` (same board.json-path-list-first, index.md-frontmatter
  -fallback shape, same "board.json exists on HEAD but not on releaseRef → fail
  closed instead of silently falling through" rule already documented on
  `readTrackInfos`, applied consistently). Add a thin adapter wrapper
  `func (a *OracleReaderAdapter) ReadReleaseWorktreePath(release string)
  (string, error)` (same release-mismatch guard as `ReadBoard`/
  `ReadSliceStatus`) so both `cmdMergeTrack` and `cmdMergeRelease` call
  `oracleAdapter.ReadReleaseWorktreePath(rel)` using the adapter they already
  built for gate 1. `resolveReleaseWorktree(repo, release)` — and its
  private, now-orphaned `extractFrontmatterBody` copy in `merge.go` — are
  **deleted**; the two call sites pass `oracleAdapter` instead of `repo`.
- **AC-02 (`regress` reads via oracle, succeeds):** in `cmdRegress`, after the
  existing `resolveReleaseDir(*releaseName)` existence check (kept — it gives
  a clearer "release directory not found" error than a raw `ReadBoard`
  failure would), call `board.ReadBoard(".", *releaseName)` and take
  `.ReleaseWorktreePath`. `extractReleaseWorktreePath` (the bespoke
  frontmatter-line parser) becomes dead code and is **deleted**. The
  `--worktree` override path is untouched (it already bypasses resolution
  entirely — AC-02 only concerns the default resolution path).
- **AC-03 (legacy no-board.json fallback preserved):** satisfied for free —
  `Oracle.ReadReleaseWorktreePath`'s index.md-frontmatter fallback (merge.go
  path) and `board.ReadBoard`'s `migrateFromIndex` (regress.go path) both
  already implement it; each gets one dedicated test proving a release with
  no `board.json` still resolves.
- **AC-04 (fixtures regenerated via the real render path):** `merge_test.go`'s
  `setupMergeFixture` currently hand-writes **both** `board.json` (as a JSON
  literal — fine, that's the canonical source) **and** an `index.md` with a
  hand-authored `release_worktree_path:` frontmatter key holding the *same*
  value — which is exactly what let the bug ship undetected: the real
  renderer never emits that key, but the test fixture puts it there anyway,
  so the pre-migration code path (frontmatter parse) silently kept passing.
  Fix: keep the direct `board.json` construction (matches S02/S04 precedent —
  constructing the canonical JSON record by hand is normal, not the bug), but
  generate `index.md` via the **real** `board.RenderToFile(repoDir, release)`
  instead of a hand-authored frontmatter string, so the fixture's `index.md`
  is provably what `sworn render` actually produces (no `release_worktree_path:`
  key). `regress_test.go` currently has **no test at all** exercising the
  default (non-`--worktree`) resolution path — add one: a temp dir with a
  `board.json` + rendered `index.md` (same helper), asserting `cmdRegress`
  resolves past the worktree-path lookup (does not hit the "not found"
  error) instead of only testing the override's fail-closed guard as today.
- **AC-05 (reachability artefact):** capture `sworn merge-release --release
  2026-07-01-loop-cli-ux` (or `sworn regress --release
  2026-07-01-loop-cli-ux`, whichever reaches the fixed code path with less
  incidental setup — decided at implementation time) run against this repo's
  own current-format release, as command output proving the frontmatter
  error is gone. Captured directly into `proof.json`
  (`reachability_artifacts`), per Rule 1.
- **AC-06 (build + tests green):** `go build ./...` and
  `go test ./cmd/sworn/...`.

## Files to touch (matches spec touchpoints exactly)

- `cmd/sworn/merge.go` — `resolveReleaseWorktree` deleted; both call sites
  switch to `oracleAdapter.ReadReleaseWorktreePath(rel)`; the private
  `extractFrontmatterBody` copy deleted (now unused).
- `cmd/sworn/regress.go` — `extractReleaseWorktreePath` deleted;
  `cmdRegress`'s default-resolution branch switches to
  `board.ReadBoard(".", *releaseName).ReleaseWorktreePath`.
- `cmd/sworn/merge_test.go` — `setupMergeFixture`'s hand-authored `index.md`
  frontmatter replaced with `board.RenderToFile`-generated output; add one
  legacy-fallback (no-`board.json`) test case.
- `cmd/sworn/regress_test.go` — add a default-resolution-path test (currently
  absent) using the same real-render fixture helper, plus one legacy-fallback
  test case.

One small addition **outside** the spec's literal touchpoint list, colocated
with the code it extends:

- `internal/board/oracle.go` — new `Oracle.ReadReleaseWorktreePath` +
  `OracleReaderAdapter.ReadReleaseWorktreePath`. This is the shared engine
  both `merge.go` call sites need; putting it anywhere else would mean
  reaching into `oracle.go`'s unexported `gitContentReader` machinery from
  `cmd/sworn`, which isn't possible without either exporting that interface
  (larger surface change) or duplicating `readTrackInfos`'s board.json/
  index.md dual-path logic a third time in `cmd/sworn` (the exact
  fixture-hides-the-bug anti-pattern this release exists to close). Flagged
  for reviewer awareness since it's a touchpoint not listed in spec.json —
  see DC-1.

## Design choices for reviewer

- **DC-1 (Type-2, local/reversible) — new oracle method vs. touchpoint list.**
  `spec.json`'s touchpoints list only the two `cmd/sworn` files and their
  tests; the actual minimal fix needs one new package-level method in
  `internal/board/oracle.go` because that's where the only usable
  `gitContentReader` implementation and `readTrackInfos`'s board.json-path
  list already live. **Alternative considered**: duplicate the board.json/
  index.md dual-path resolution directly inside `merge.go` (stay strictly
  within the listed touchpoints). **Rejected because**: it's the identical
  shape of bug this whole release exists to close — a second, drifting copy
  of "how do I read this field from board.json or its legacy fallback."
  Reusing `readTrackInfos`'s already-audited path list keeps one source of
  truth. Narrow, purely additive, no existing caller touched — Type-2, not
  escalated.
- **DC-2 (Type-2, local/reversible) — disk read for `regress.go`, git-ref
  read for `merge.go`, not one unified mechanism.** **Alternative
  considered**: give `regress.go` a `git.Repo` + ref too, so both commands
  resolve identically. **Rejected because**: `regress.go` has no git-ref
  concept anywhere else in the file today (it's a straight local-directory
  tool, including its own `--worktree` override, which is a plain path
  string) — bolting on ref resolution for one field would be a bigger,
  unrelated restructuring for zero behavioural gain, and would diverge from
  the already-precedented S04 split (git-ref oracle where a `repo` already
  exists, `board.ReadBoard` where it doesn't).
- **DC-3 (Type-2, local/reversible) — delete rather than deprecate the two
  broken parsers.** `resolveReleaseWorktree`/`extractFrontmatterBody` (in
  `merge.go`) and `extractReleaseWorktreePath` (in `regress.go`) have no
  other callers (confirmed by grep) once the two call sites switch to the
  oracle. Dead frontmatter-parsing code left in place is exactly the kind of
  "attractive nuisance" S04's design.md flagged and removed for the same
  reason.

## Design-level risks

- **Rule 11 (process-global mutation guard).** `regress_test.go` already has
  one test (`TestRegressWorktreeFlagFailsClosed`) that calls `os.Chdir` with
  a `t.Cleanup` restore — the guaranteed-restore half of Rule 11 is already
  satisfied there. The new AC-04 default-resolution test also needs a
  `os.Chdir` into a fixture dir; it must follow the same
  `t.Cleanup(func() { os.Chdir(origDir) })` pattern, and the fail-closed
  target assertion is inherited for free from the existing worktree-exists
  `os.Stat` check in `cmdRegress` (no test-side git-ref argument is involved,
  so no separate branch-target assertion is needed here).
- **`board.ReadBoard`'s migration side effect.** Same as S02/S04: calling it
  against a genuinely-legacy release writes `board.json` to disk as a side
  effect. `regress.go` inherits this (already-adopted, already-reviewed
  behaviour elsewhere in the codebase) — not a new mutation this slice
  invents, called out for awareness per the S02 precedent (DC-2 there).
- **`ReleaseRef` staleness for `merge.go`.** `Oracle.ReadReleaseWorktreePath`
  reads `release-wt/<release>` at the ref the caller already resolved for
  `ReadBoard` — no new staleness surface versus what gate 1 already reads.

## Traceability

| AC | Change | Test |
|----|--------|------|
| AC-01 | `merge.go` → `oracleAdapter.ReadReleaseWorktreePath` | `TestMergeTrack_AllVerified`, `TestMergeRelease_Pass` (regenerated fixture) |
| AC-02 | `regress.go` → `board.ReadBoard(".", release).ReleaseWorktreePath` | new default-resolution test in `regress_test.go` |
| AC-03 | legacy fallback via `Oracle.ReadReleaseWorktreePath` / `board.ReadBoard` | new no-`board.json` fixture case, both test files |
| AC-04 | fixtures generated via `board.RenderToFile`, not hand-authored frontmatter | all regenerated `merge_test.go` fixtures; new `regress_test.go` fixture |
| AC-05 | reachability artefact | `proof.json` `reachability_artifacts` — live command run against `2026-07-01-loop-cli-ux` |
| AC-06 | build + tests green | `go build ./...`, `go test ./cmd/sworn/...` |
