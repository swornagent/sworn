# Design TL;DR — S01-render-drift-guard

## User outcome

`sworn doctor` fails closed (non-zero exit, ERROR not WARN) when any
board.json-backed release's committed `index.md` doesn't match
`render(board.json + slice records)` — replacing the existing advisory,
already-broken `driftGuard`.

## Approach

Add a new doctor check, `checkRenderDrift`, in `cmd/sworn/doctor.go`:

1. Scan `docs/release/*` for release directories.
2. For each, `os.Stat` its `board.json`. If absent, skip (AC-03 — no JSON
   source to render from, not a genuinely-broken release).
3. If present, call the **existing, untouched** `board.Render(repoRoot,
   releaseName)` (`internal/board/render.go:46`) to get the expected
   `index.md` content in-memory — no new render implementation (AC-01, and
   matches the rationale's "render.go itself is untouched").
4. If `Render` itself errors (e.g. malformed board.json, missing slice
   record), surface that as an ERROR naming the release — a release that
   can't render can't be proven non-drifted, so this fails closed too rather
   than being silently skipped.
5. Read the committed `docs/release/<release>/index.md` from disk and
   compare byte-for-byte against `Render`'s output.
6. Mismatch → ERROR result naming the release, with a one-line remediation
   hint (`re-render via 'sworn render <release>'`). Match → OK result.

Wire `checkRenderDrift` into `cmdDoctor` under the existing **Group 2: Repo
artifact audit** heading (per the planning decision recorded in
`intake.md`), immediately after the current `checkRepoArtifacts` call.
Unlike the current Group 2 loop (which never sets `hasError` — its checks
are OK/WARN-only today), this new call's results **do** feed `hasError`,
following the same pattern Group 2b (`checkStatusTimestamps`) already uses.
This is the one place fail-closed behavior must be wired correctly for
AC-02 to hold — a doctor check that reports ERROR but never flips the exit
code would be the same "advisory, not fail-closed" bug this slice exists to
fix, just moved one layer up.

Remove `driftGuard` (`internal/board/board.go:224-261`) and its call site
inside `WriteBoard` (`board.go:173`) entirely (AC-04) — not left running
alongside the new check. `WriteBoard` becomes marshal → write → validate,
with no post-write drift check. The `log` import in `board.go` becomes
unused once `driftGuard`'s five `log.Printf` calls are gone and must be
dropped too. `trackInfosToBoardTracks` / `boardTracksToTrackInfos` stay —
both are used elsewhere (`migrateFromIndex`, `oracle.go:391`), not solely by
`driftGuard`.

## Files touched

- `internal/board/board.go` — delete `driftGuard`, its call site, the `log`
  import.
- `internal/board/board_test.go` — no existing test asserts on `driftGuard`
  behavior directly (only exercised indirectly via `WriteBoard`, which never
  asserted on its `log.Printf` output); removal needs no test changes beyond
  confirming the package still compiles/passes.
- `cmd/sworn/doctor.go` — add `checkRenderDrift`, wire into Group 2 with
  `hasError` feed.
- `cmd/sworn/doctor_test.go` — new tests: clean release (OK), drifted
  release (ERROR + non-zero exit), no-board.json release (skipped, AC-03),
  render-error release (ERROR).

## Design-level risks / pins

- **Byte-for-byte vs normalised comparison**: `RenderToFile` writes
  `Render`'s output with no post-processing, so a straight byte comparison
  against the committed file should be exact for any release actually
  produced by `sworn render`. Risk: a release whose `index.md` was
  hand-edited with trailing-whitespace/line-ending differences that a human
  wouldn't consider "drift." Decision: byte-for-byte, no normalisation —
  `index.md` is a build artifact per ADR-0009, so any hand-edit at all,
  even whitespace-only, is exactly the class of drift this guard exists to
  catch. Flagging for reviewer awareness, not proposing an alternative.
- **AC-05 reachability**: after implementation, `sworn doctor` must be run
  for real against this repo's live `docs/release/*` — this is the
  reachability artefact, not a synthetic fixture. Requires the currently
  in-flight sibling tracks (T2-T5) to not be regressing other releases'
  board.json/index.md pairs mid-flight, but since this check only reads
  already-committed state on this track's own branch, that's not a
  cross-track coupling risk.
- **Group 2 `hasError` wiring is the one subtle spot**: easy to add the
  check's results to the `g2` slice's print loop without also adding it to
  `hasError` (mirroring the existing gap where Group 2 today has no ERROR
  path at all). Calling this out explicitly so it isn't missed.

## Out of scope (unchanged from spec)

- No new render implementation — `internal/board/render.go` is untouched.
- No changes to the 4 other tracks' touchpoints (TUI, MCP, CLI
  merge/regress, core-loop/RTM) — touchpoint-disjoint per the planning
  decision.
