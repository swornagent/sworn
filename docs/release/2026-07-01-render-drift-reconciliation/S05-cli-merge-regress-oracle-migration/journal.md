# Journal — S05-cli-merge-regress-oracle-migration

## 2026-07-02 — Implementer session start

Design review (`.captain-trial-log.md`, commit `c5634da`) returned `DECISION:
PROCEED`, 4 pins (3 mechanical, 1 memory-cited, 0 escalate). No
`approved-ack.md` marker convention exists in this repo yet (same gap S01/S04
noted) — recording the ack here as the durable artefact per Rule 9.

Applying the 4 pins inline during implementation:
1. **AC-05 target correction (mechanical).** `2026-07-01-loop-cli-ux` carries
   no `release_worktree_path` in board.json or index.md (verified live —
   `grep release_worktree_path` on both = 0 hits). Retargeting the
   reachability artefact to `2026-06-30-sworn-operational-readiness`, whose
   board.json does carry the field, and using `sworn regress` (not
   `merge-release`, which cannot reach `resolveReleaseWorktree` this release
   — gate 1 "all slices terminal" fails first) as the vehicle.
2. **Fail-closed empty path (mechanical / Rule 11).**
   `Oracle.ReadReleaseWorktreePath` returns an error (not "") when
   board.json has no top-level `release_worktree_path`, preserving
   `resolveReleaseWorktree`'s existing "not found" failure mode so
   `git.New("")` never runs against the ambient cwd. `regress.go` keeps its
   existing `if worktreePath == "" { return 2 }` guard after the swap to
   `board.ReadBoard(".", release).ReleaseWorktreePath`.
3. **AC-04 fixture regeneration is not a drop-in (mechanical).**
   `board.RenderToFile` fails closed unless every board.json slice has both
   spec.json and status.json on disk. `setupMergeFixture` now writes
   S01-verified's spec.json (with touchpoints) before calling
   `board.RenderToFile`, and status.json is written by each test before the
   commit that includes the rendered index.md. Same pattern used for the new
   `regress_test.go` fixture helper.
4. **AC-06 test scope (memory-cited —
   `feedback_releaseverify_specmd_false_fail` /
   `project_newline_eating_edit_corruption`).** Slice-relevant test command
   widened to `go test ./internal/board/... ./cmd/sworn/...` — this slice
   modifies `internal/board/oracle.go`, and memory records a prior
   strict-reader change regressing fixtures in that package while
   `./cmd/sworn/...` alone stayed green.

No Type-1 escalations in the 4 pins — proceeding straight to `in_progress`.

## 2026-07-02 — verifier verdict (fresh context)

- **State transition**: `implemented` → `verified`.
- **Verdict**: `PASS`
- **Verified against**: `3c9ae51` (track/2026-07-01-render-drift-reconciliation/T4-cli-merge-regress, HEAD at review time; diff base `start_commit` c5634da).
- **Own execution, not trusted captured output**:
  - `go build ./...` — exit 0.
  - `go test ./internal/board/... ./cmd/sworn/... -timeout 100s` — exit 0 (re-ran).
  - `go test ./... -timeout 260s` — 41 packages, all ok (re-ran; matches proof.json's claim).
  - `go vet ./internal/board/... ./cmd/sworn/...` — clean.
  - `gofmt -l` on all six touched Go files — clean.
  - Named tests re-run individually with `-v`: `TestMergeTrack_AllVerified`,
    `TestMergeTrack_OracleRouting`, `TestMergeRelease_Pass`,
    `TestMergeTrack_LegacyIndexMDFallback`,
    `TestRegressDefaultResolution_BoardJSON`,
    `TestRegressDefaultResolution_LegacyIndexMDFallback` — all PASS.
  - Reachability artefact reproduced live: rebuilt the binary from this
    worktree's HEAD and re-ran
    `sworn regress --release 2026-06-30-sworn-operational-readiness` —
    exit 0, output byte-identical in shape to
    `reachability-regress-output.txt` (Go tests PASS, TS SKIP, Golden
    fixtures PASS). Confirmed live: that release's committed `board.json`
    carries `release_worktree_path` (1 grep hit) and its `index.md` carries
    zero occurrences of the key — the pre-fix scraper would have hard-erred.
- **Diff read (not recalled)**: `git diff c5634da..HEAD` for `merge.go`,
  `regress.go`, `oracle.go` confirms AC-01/AC-02 exactly as described —
  both `cmdMergeTrack`/`cmdMergeRelease` call sites reuse the
  already-in-scope `oracleAdapter` (no second oracle build); `regress.go`
  switches to `board.ReadBoard(".", *releaseName).ReleaseWorktreePath` with
  the existing empty-path fail-closed guard preserved. `oracle.go`'s new
  `ReadReleaseWorktreePath`/`OracleReaderAdapter.ReadReleaseWorktreePath`
  mirror `readTrackInfos`'s board.json-first/index.md-fallback/
  fail-closed-on-HEAD-migrated shape. `grep` confirms the deleted
  `resolveReleaseWorktree`/`extractFrontmatterBody`/
  `extractReleaseWorktreePath` have zero remaining references in the
  touched files (dark-code check: the new oracle methods are reachable only
  from the two production CLI call sites, not just their own tests).
- **Scope check**: `git diff --name-only c5634da..HEAD` (9 files) matches
  `proof.json.files_changed` exactly (the bundle's own three files —
  `proof.json`/`proof.md`/`reachability-regress-output.txt` — are
  self-referentially omitted, same convention as S01/S04). No touchpoint
  drift; the one addition beyond spec.json's literal list
  (`internal/board/oracle.go` + its test) is declared as a Type-2 design
  decision in `status.json` and in `proof.json`'s Divergence section.
- **Render discipline**: `sworn render 2026-07-01-render-drift-reconciliation`
  at HEAD produces byte-identical `index.md` (no drift); `sworn doctor`
  reports zero render-drift errors for this release.
- **Gate walk**: AC-01 PASS, AC-02 PASS, AC-03 PASS (legacy fallback unit +
  integration tests both green), AC-04 PASS (fixtures now generated via real
  `board.RenderToFile`, confirmed the real renderer emits zero
  `release_worktree_path:` occurrences), AC-05 PASS (reachability
  reproduced live, retarget rationale sound and verified), AC-06 PASS (build
  + full test suite green, re-run independently).
- **Next step**: track `T4-cli-merge-regress` is complete —
  `/merge-track T4-cli-merge-regress`.
