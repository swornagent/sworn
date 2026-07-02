# Design TL;DR — S06-core-loop-and-rtm-oracle-migration

## Outcome

`sworn loop --release <name> --parallel` (`internal/run.RunParallel`) currently
never reads `board.json` at all — it parses raw `index.md` YAML frontmatter by
hand, with three independent bugs (dead "no tracks found" error path, a
worktree-path resolver that can't see an already-recorded path, and a
documented-shared-file detector that only matches an explicit annotation, not
genuine ≥2-track overlap). `internal/rtm`'s release-level vertical-trace
parsers are similarly stuck reading markdown headings that no longer exist
post-ADR-0009. This slice makes the loop's own dispatch/safety-invariant logic
and Rule 8's traceability matrix both read from `board.json`, the real source
of truth, instead of silently degrading.

## Key finding that reshaped the design: no `internal/board/*.go` changes needed

My first-pass design assumed I'd need a new accessor on `board.Release` (for
`vertical_trace`) and possibly an exported track-info converter. Checking the
release's touchpoint matrix (`index.md`) before writing code: **`T1-drift-guard`
already owns `internal/board/board.go` and `internal/board/board_test.go`**
exclusively. Editing those files from T5 would be an undeclared track
collision (track-mode.md invariant 2) — the implementer role forbids silently
absorbing a touch outside the track's declared touchpoints.

Re-examined against the already-**exported** surface of `internal/board`
(`ReadBoard`, `BoardRecord`, `BoardTrack`, `TrackInfo` — all public, all
usable read-only from `internal/run`/`internal/rtm` without modification),
every AC is achievable with zero `internal/board` edits:

- AC-01/AC-02: `board.ReadBoard(repoRoot, release)` is already exported and
  already does exactly what's needed — reads `board.json` if present, lazily
  migrates from `index.md` frontmatter if not (so legacy pre-migration
  releases keep working unchanged). `BoardRecord.Tracks []BoardTrack` and
  `.ReleaseWorktreePath` are both already public fields.
- AC-04: `board.Release` stores the canonical JSON verbatim in an unexported
  `raw` field with no accessor — but I don't need one. `board.json` lives as
  a plain sibling file of `index.md`/`intake.md` inside the release
  directory in every real layout, so `internal/rtm` can read+unmarshal it
  directly with a tiny local anonymous struct, exactly the same
  `os.ReadFile`-then-parse idiom `rtm.go` already uses for `index.md`/
  `intake.md`/`spec.md`/`status.json`. No shared type needed.

This keeps the change entirely inside this track's six declared touchpoints
and avoids the collision. Flagging it explicitly below as a pin, since it's
exactly the kind of cross-track risk Rule 9 wants a human eye on before code
— even though the outcome here is "no expansion needed," a reviewer should
confirm the touchpoint-matrix reasoning holds.

## Per-file design

### `internal/run/parallel.go` (AC-01, AC-02, AC-03)

**Track/worktree resolution — replace the frontmatter-parsing block:**

Today (`RunParallel`, ~line 144-173):
```go
indexData, err := os.ReadFile(indexPath)
fm := extractFrontmatter(string(indexData))
releaseWorktreePath := extractReleaseWorktreePath(fm)   // only ever reads index.md frontmatter
if releaseWorktreePath == "" { /* cold-start default */ }
tracks := board.ParseTracks(fm)                          // legacy line-oriented parser, no board.json awareness
if len(tracks) == 0 { return fmt.Errorf("no tracks found") }
```

New:
```go
br, err := board.ReadBoard(absRoot, releaseName)
if err != nil {
    return fmt.Errorf("RunParallel: read release board: %w", err)
}
releaseWorktreePath := br.ReleaseWorktreePath
if releaseWorktreePath == "" {
    // unchanged cold-start default (eval finding 1) — only fires when
    // board.json genuinely has no path recorded, not unconditionally.
    releaseWorktreePath = filepath.Join(filepath.Dir(absRoot), filepath.Base(absRoot)+"-worktrees", "release-"+releaseName)
    fmt.Fprintf(os.Stderr, "RunParallel: release_worktree_path unset — defaulting to %s (cold-start)\n", releaseWorktreePath)
}
tracks := trackInfosFromBoardTracks(br.Tracks)   // new small local converter, see below
if len(tracks) == 0 {
    return fmt.Errorf("RunParallel: no tracks found in release board")
}
```

`board.ReadBoard` still needs `indexPath`/raw `indexData` for nothing else
except the documented-shared-files parse below, so that read stays for now
(still needed to compute `indexPath` for `router.ParseDocumentedShared`).

**`trackInfosFromBoardTracks` (new, unexported, local to `internal/run`):**
mechanical field-for-field copy from `board.BoardTrack` to `board.TrackInfo`
(`ID`, `Slices`, `WorktreePath`, `WorktreeBranch`, `State` copy directly;
`DependsOn` needs `[]string(t.DependsOn)` since `BoardTrack.DependsOn` is
`board.StringList`). Both types are already exported; this is a converter,
not new shared library surface — deliberately kept local rather than added
to `internal/board` to avoid the T1 collision above.

**Documented-shared-file detection — delete, delegate:**

Delete `parseDocumentedSharedFiles` (line ~593-629) entirely — it's a second,
weaker, independent reimplementation of exactly what
`router.ParseDocumentedShared`/`parseTouchpointMatrix` already do correctly
(explicit `(DOCUMENTED SHARED)` marker **and** ≥2-checkmark inference; the
old function only matched the explicit marker, which is the actual AC-03
defect). `internal/run` already imports `internal/router` (used for
`router.OracleReader` a few lines down) — no new import, no cycle
(`internal/router` does not import `internal/run` or `internal/board`... — wait,
confirmed: `internal/router` does not import `internal/run`; `internal/board`
does not import `internal/router`; `internal/run` already imports
`internal/router`. Clean).

Call site (~line 247) becomes:
```go
docShared, err := router.ParseDocumentedShared(indexPath)
if err != nil {
    // Fail open — a release with no "Touchpoint matrix" section (rare;
    // e.g. a single-track release, or an unrendered index.md) has no
    // documented-shared exemptions, not a fatal error. Matches the existing
    // fail-open precedent for oracle-read failures in this same function.
    fmt.Fprintf(os.Stderr, "RunParallel: parse documented shared files: %v (treating as no exemptions)\n", err)
    docShared = nil
}
```

**Delete as dead code (only called from the three replaced call sites above,
confirmed via repo-wide grep — no other caller in `internal/run` or
elsewhere):** `extractFrontmatter`, `extractReleaseWorktreePath`,
`stripInlineComment`.

### `internal/run/parallel_test.go` (touchpoint)

- Remove `TestExtractFrontmatter`, `TestExtractReleaseWorktreePath` (test the
  deleted functions).
- Existing fixtures already lay out `<tmp>/docs/release/<name>/index.md` with
  full frontmatter including `worktree_branch:` on every track (verified
  against all 10 track entries across `TestRunParallel_Basic`,
  `_FailureCascade`, `_TimingConcurrency`, `_DependentTrackRunsAfterSuccess`,
  `_TrackPaused`) — this is exactly what `board.ReadBoard`'s lazy-migration
  path (`migrateFromIndex` → `WriteBoard` → board-v1 schema validation,
  which requires non-empty `worktree_branch`) needs, so **no fixture changes
  needed** for these tests; `board.ReadBoard` transparently migrates them the
  same way it does in production.
- `TestRunParallel_NoTracks` / `_ReleaseWorktreePathMissing` both use
  `tracks: []` — empty tracks list migrates fine (no per-track required
  fields to fail), both still hit `len(tracks) == 0` and error as expected.
  No change needed.
- `TestInvariant2_OverlapBlocksSecondTrack` / `_NoOverlapBothRun` /
  `_OracleReadFailureFailsOpen`: none of their fixtures contain a
  "Touchpoint matrix" section at all, so `router.ParseDocumentedShared` will
  return the "not found" error on all three — caught by the new fail-open
  handling above, same net effect (`docShared == nil`) as today. No fixture
  changes needed.
- `TestInvariant2_DocumentedSharedExempt`: **needs a one-line fixture
  addition** — its raw `indexContent` jumps straight from the frontmatter
  close to `# Test\n\n` then the table, with no "Touchpoint matrix" heading
  text, so `router.parseTouchpointMatrix` won't find the table at all (it
  requires the literal substring "Touchpoint matrix" somewhere in the body).
  Add `## Touchpoint matrix\n\n` immediately before the existing
  `| File | T1 | T2 |` line. No other change to this test.
- **New test** (AC-03): build a real 2-track, 2-slice fixture under
  `<tmp>/docs/release/<name>/` — `board.json` with tracks `T1`/`T2`, each
  with one slice; each slice's `spec.json` declaring the **same** file path
  in its `touchpoints` array (e.g. `internal/shared/thing.go`); each slice's
  `status.json` present (any valid state). Call `board.Render(tmp, release)`
  (real renderer, not hand-authored markdown) to produce `index.md`, write
  it to disk, then run `RunParallel` (with a no-op `RunSliceFn`, same
  pattern as existing tests) and assert the run completes without an
  invariant-2 block on that file — proving the ≥2-checkmark inference path
  (the actual previously-broken path — the old function only recognized the
  explicit annotation) now works end-to-end from a genuinely rendered file.

### `internal/run/cold_start_test.go` (touchpoint)

Remove `TestStripInlineComment` and
`TestExtractReleaseWorktreePath_CommentPlaceholder` (test the deleted
functions). `TestRunSlice_ColdStartBootstrapsStartCommit` is unrelated
(tests `start_commit` bootstrap, not frontmatter parsing) — untouched.

### `internal/rtm/rtm.go` (AC-04)

`Build(releaseDir string)` already reads `intake.md`/`index.md` directly from
`releaseDir` with no `repoRoot`/`release`-name split — and `internal/implement/ready.go:69`
is the one external caller, passing `releaseDir` with no other params. To
avoid changing `Build`'s signature (which would ripple into `ready.go`, an
undeclared touchpoint), the fix reads `board.json` as a **sibling file of
releaseDir** (`filepath.Join(releaseDir, "board.json")`) — which is exactly
where it lives in every real release layout — rather than reconstructing a
repo root and calling `board.ReadBoard`.

New unexported helper:
```go
func readBoardVerticalTrace(releaseDir string) (benefit, orgObjective string, ok bool) {
    data, err := os.ReadFile(filepath.Join(releaseDir, "board.json"))
    if err != nil {
        return "", "", false
    }
    var doc struct {
        Release struct {
            VerticalTrace struct {
                Benefit      string `json:"benefit"`
                OrgObjective string `json:"org_objective"`
            } `json:"vertical_trace"`
        } `json:"release"`
    }
    if json.Unmarshal(data, &doc) != nil {
        return "", "", false
    }
    return doc.Release.VerticalTrace.Benefit, doc.Release.VerticalTrace.OrgObjective, true
}
```

In `Build`, replace:
```go
m.ReleaseBenefit = parseReleaseBenefit(string(indexText))
m.OrgObjective = parseOrgObjective(string(indexText))
```
with:
```go
if benefit, orgObj, ok := readBoardVerticalTrace(releaseDir); ok {
    m.ReleaseBenefit = benefit
    m.OrgObjective = orgObj
} else {
    // Legacy fallback: no board.json (pre-ADR-0009 release) — keep the old
    // markdown-heading parse so releases that never migrated still trace.
    m.ReleaseBenefit = parseReleaseBenefit(string(indexText))
    m.OrgObjective = parseOrgObjective(string(indexText))
}
```

`parseReleaseBenefit`/`parseOrgObjective` are **kept** (not deleted) as the
legacy-release fallback — unlike `parallel.go`'s frontmatter helpers, these
still have a live call site. No real `board.json` today carries
`org_objective` (confirmed by inspecting both `2026-07-01-render-drift-reconciliation/board.json`
and `2026-07-01-loop-cli-ux/board.json` — only `vertical_trace.benefit`
exists in practice); `readBoardVerticalTrace` returning `""` for an absent
`org_objective` key is correct (empty because genuinely unauthored, not
because the wrong file was read) and matches AC-04's wording precisely.

Per-slice `parseSliceReleaseBenefit`/`parseSliceOrgObjective` (rtm.go:446-464,
already reading `status.json`) are out of scope — confirmed correct already,
not touched.

### `internal/rtm/rtm_test.go` (touchpoint)

New test: extend or add a fixture variant that also writes a `board.json`
alongside the existing `intake.md`/`index.md`/slice fixtures (with
`release.vertical_trace.benefit` set to a known string, no `org_objective`
key), then assert `Build(dir).ReleaseBenefit` matches the board.json value
(not whatever `index.md` might separately say) and `OrgObjective == ""`.
Existing tests (`TestBuild_FullyTraced` et al., no `board.json` in their
fixtures) continue to exercise the legacy markdown-fallback branch unchanged
— confirmed no existing test references `parseReleaseBenefit`/
`parseOrgObjective` directly, so no test breaks from their retention.

### `internal/router/router_test.go` (AC-05, touchpoint)

New regression test, parallel in spirit to the new `parallel_test.go` test
above but exercising `router.ParseDocumentedShared`/`parseTouchpointMatrix`
directly: build a real 2-track/2-slice `board.json` + `spec.json` fixture
with a shared touchpoint file, call `board.Render` to get real rendered
`index.md` content (not the pre-migration `2026-06-27-conformance-foundation`
fixture the existing `TestParseDocumentedSharedFromFile` uses, which predates
the board.json migration and is a different, still-valid, still-untouched
test), and assert the shared file is present in the returned map. This test
file already imports test-only helpers freely; adding `internal/board` as a
test-file import is safe (`internal/board` doesn't import `internal/router`,
confirmed, no cycle).

## Out of scope (Rule 2 deferrals — noted, not actioned this slice)

- `internal/board/board.go`'s `migrateFromIndex` explicitly discards
  `vertical_trace` when lazily migrating a legacy release
  (`vt := ParseVerticalTrace(...); _ = vt // vertical trace not stored in board.json`).
  This means a release that migrates via `RunParallel`'s (or anything else's)
  first `board.ReadBoard` call loses its vertical trace forever. This is a
  real latent gap, but it's inside `internal/board/board.go` — T1-drift-guard's
  exclusive touchpoint — and out of this slice's AC set. Flagging for the
  Coach; likely belongs to T1 or a follow-up.
- No changes to `internal/board/oracle.go`'s git-ref-based `Oracle` type
  (used by the router/verifier for git-ref reads) — AC-01/AC-02's "via the
  oracle" is satisfied by the on-disk `board.ReadBoard`, the correct
  counterpart for a function that already operates on worktree paths on
  disk, not git refs. Terminology note for the reviewer: "the oracle" in the
  spec's rationale reads most naturally as "board.json is the source of
  truth, read via the board package" rather than literally the `Oracle`
  struct — flagging so the reviewer can confirm this reading.

## Pins for the Captain / Coach

1. **Touchpoint-matrix collision, resolved by avoidance, not by decision** —
   see "Key finding" above. Confirms this slice touches zero files T1 also
   claims. Ask: does the reviewer agree the design correctly stays inside
   the six declared touchpoints, or is there a reason `internal/board.go`
   changes are unavoidable that I've missed?
2. **AC-06 reachability mechanics** — the spec's reachability artefact is
   "running `sworn loop --release <name> --parallel` ... shows the loop finds
   its tracks." Running the real CLI against this repo's own
   `2026-07-01-render-drift-reconciliation` release would actually dispatch
   live slice work against the other four tracks' worktrees (T1-T4), which
   are separate in-flight sessions — a real side effect, not a safe
   dry-run. Proposed default: capture the reachability artefact via a small
   Go-level invocation of `run.RunParallel` against the real release's
   `board.json` with a no-op `RunSliceFn` (the exact same harness pattern
   the unit tests already use, just pointed at real repo data instead of a
   temp-dir fixture) — proves track-loading succeeds without touching any
   other track's live state. Needs explicit sign-off before implementation:
   is this substitute acceptable, or does the Coach want the literal CLI
   invocation (accepting the side effects on T1-T4)?

## Files touched (all within declared touchpoints)

- `internal/run/parallel.go`
- `internal/run/parallel_test.go`
- `internal/run/cold_start_test.go`
- `internal/rtm/rtm.go`
- `internal/rtm/rtm_test.go`
- `internal/router/router_test.go`
