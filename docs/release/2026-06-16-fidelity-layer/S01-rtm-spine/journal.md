---
title: Slice journal template
description: Implementation log for one slice. Append-only. Visible to verifier as context, but verifier verdict is based on proof.md and repo state, not journal prose.
---

# Journal: `S01-rtm-spine`

> Copy this file to `docs/release/<release-name>/<slice-id>/journal.md`. Append entries chronologically. Do not delete history. Decisions captured here must also land in commit message bodies per Rule 4 — this journal is a working surface, not a substitute for durable capture.

## Session log

### `2026-06-17 20:15` — session start

- **State**: `planned -> in_progress`
- **Notes**:
  - Materialised track worktree at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T1-fidelity-core` for track `T1-fidelity-core`.
  - First `/implement-slice` in the release and track. Release worktree already existed at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer`.
  - Recorded worktree_path in index.md frontmatter on release-wt branch.

### `2026-06-17 20:30` — implementation

- **State**: `in_progress`
- **Notes**:
  - Designed the RTM data model: 2-D matrix with horizontal (need -> AC -> test -> proof) and vertical (org objective -> release benefit -> slice) axes.
  - Need id scheme: N-NN (e.g. N-01), stable, never reused, assigned by planner at intake time, cited inline in AC text.
  - No separate datastore: the RTM builds from existing artefacts (intake.md, spec.md, status.json, index.md) alone.
  - Fail-closed: orphaned need, orphaned AC (no need or no test), and slice with no vertical link each cause non-zero exit.
  - Lightweight floor: slice -> release goal satisfies the vertical trace without an org-objective link (solo/small-team).
  - Added trace fields to state.Status: NeedIDs, ReleaseBenefit, OrgObjective.
  - Added ParseVerticalTrace to board package for release_benefit / org_objective frontmatter fields.
  - Updated planner prompt to instruct need-id assignment in intake and citation in acceptance checks.
  - Created Rule 8 doc (08-requirements-fidelity.md) in the adopted baton rules.
  - Updated adopt.go to include the new rule in Materialise and embed.
  - Updated baton VERSION and README.

### `2026-06-17 21:15` — state transition to implemented

- **State**: `in_progress -> implemented`
- **Notes**:
  - All 13 RTM unit tests pass (orphaned need, orphaned AC no need, orphaned AC no test, slice no vertical, solo floor, non-existent need, fully traced, print, parse needs, parse ACs, parse tests, isSliceID, truncate).
  - All 5 integration tests pass (cmdRtm missing arg, nonexistent release, fully traced, orphaned need, solo floor).
  - State trace fields round-trip test passes.
  - Board ParseVerticalTrace test passes.
  - Full suite green: `go test ./...` all pass.
  - `go vet ./...` clean.
  - `gofmt` clean on all changed files.
  - Reachability artefact: live `sworn rtm 2026-06-16-fidelity-layer` run produces the matrix and correctly fails with 70 violations (all ACs lack need ids because the release was specced before the RTM existed).
  - Proof bundle generated from live repo state.

## Open questions

None. All acceptance checks are delivered.

## Deferrals surfaced

None.

## Verifier verdicts received

### `2026-06-17` — Verifier verdict: FAIL

```
FAIL

Slice: `S01-rtm-spine`

Violations:
1. Gate 2 — `proof.md` "Divergence from plan" does not adequately explain functional changes to
   `internal/adopt/adopt.go` and `internal/adopt/baton/README.md`, which are not listed in the
   spec's "Planned touchpoints".
   - `internal/adopt/adopt.go`: mentioned in divergence only as a release-verify.sh false-positive
     concern; the actual functional change (adding Rule 8 to the embed directive and `Materialise`
     file list so the rule is embedded in the binary and written to repos that run `sworn init`)
     is not described.
   - `internal/adopt/baton/README.md`: not mentioned in the divergence section at all; this file
     received a functional documentation change (Rule 8 added to the embedded Baton README index).

Required to address:
1. Update `proof.md` "Divergence from plan" to explain `internal/adopt/adopt.go`: the file was
   changed to add `08-requirements-fidelity.md` to the `//go:embed` directive and to the
   `Materialise` file list, so the new rule is bundled into the `sworn` binary and written to
   consumer repos on `sworn init`. No code change required — proof.md update only.
2. Update `proof.md` "Divergence from plan" to explain `internal/adopt/baton/README.md`: the
   file was changed to document Rule 8 in the embedded Baton README (the index listing of all
   rules). No code change required — proof.md update only.
```

Gates 1, 3, 4, 5, 6 all PASS. Only Gate 2 fails.

### `2026-06-18 02:00` — Verifier verdict: FAIL (second round)

```
FAIL

Slice: `S01-rtm-spine`

Violations:
1. Gate 2 — `start_commit` in `status.json` is set to `925cb07` (the re-implementation
   restart doc commit), which sits AFTER the actual implementation commit `67f287b`. As a
   result, `git diff --name-only 925cb07..HEAD` returns only 4 release-artefact docs files;
   all 8 planned touchpoints (internal/rtm/rtm.go, internal/rtm/rtm_test.go,
   internal/state/state.go, internal/board/index.go, cmd/sworn/rtm.go, cmd/sworn/main.go,
   internal/prompt/planner.md, internal/adopt/baton/rules/08-requirements-fidelity.md) are
   absent from the live diff. proof.md "Files changed" silently uses
   release-wt/2026-06-16-fidelity-layer as the diff base (not start_commit) to surface the
   implementation files, and "Not delivered" lists "None" — no Rule 2 deferral explains the
   planned touchpoints being absent from the start_commit..HEAD diff.

Required to address:
1. Set start_commit in status.json to 8767fc7 (the original
   docs(release/2026-06-16-fidelity-layer/S01-rtm-spine): start implementation commit) so
   that git diff --name-only 8767fc7..HEAD covers the full slice scope.
2. Regenerate proof.md "Files changed" from git diff --name-only 8767fc7.
3. Update proof.md "Divergence from plan" to acknowledge that the verifier-verdict and
   re-implementation doc commits (28ad590, 925cb07, 9b3ff7f, db7feff) now appear within
   start_commit..HEAD but are not slice implementation scope — they are doc-only bookkeeping.

Note: implementation code is present and correct on the branch — all tests pass (13 unit,
5 integration, state round-trip, board vertical-trace). FAIL is a metadata issue
(start_commit pointing after the code), not a substantive implementation defect.
```

### `2026-06-18 00:10` — re-implementation after failed_verification

- **State**: `failed_verification -> in_progress`
- **Notes**:
  - Discovered that a prior bad merge (commit `49d9eda`) had dropped all implementation files from the track branch's working tree. The merge of `release-wt/2026-06-16-fidelity-layer` into the track branch resolved in favour of the pre-implementation tree, silently removing `internal/rtm/`, `internal/state` changes, `internal/board` changes, `cmd/sworn/rtm.go`, and all other implementation artefacts.
  - Fix: reset track branch to `dac5ec8` (the "implemented" commit with all source files), cherry-picked `9f2f7bb` (verifier verdict FAIL recording). This restored all implementation files while preserving the verifier verdict.
  - The verifier FAIL was purely a proof.md Divergence section gap: two changed files (`internal/adopt/adopt.go` and `internal/adopt/baton/README.md`) were not explained as functional changes. No code change needed.
  - Rewrote the Divergence section to explain both files:
    - `internal/adopt/adopt.go`: added `08-requirements-fidelity.md` to the `Materialise` file list so the rule is written to consumer repos on `sworn init`. The `//go:embed baton/rules/*` wildcard already covers it for the binary.
    - `internal/adopt/baton/README.md`: added Rule 8 to the embedded Baton README's numbered rule index so the documentation surface stays consistent with the rules directory.
  - Also documented the test-file divergences (`internal/state/state_test.go` gofmt alignment, `internal/board/index_test.go` and `cmd/sworn/rtm_test.go` as new test files) that were not in the planned touchpoints.
  - All tests pass, vet clean, gofmt clean. No code changes in this session — proof.md update only.
### `2026-06-18 03:00` — re-implementation after second FAIL (metadata fix)

- **State**: `failed_verification -> in_progress -> implemented`
- **Notes**:
  - The second verifier FAIL was purely a metadata issue: `start_commit` was set to `925cb07` (the re-implementation restart doc commit, AFTER the actual implementation commit `67f287b`), causing the verifier's diff base to miss all 8 planned touchpoints. The verifier explicitly noted "implementation code is present and correct on the branch — all tests pass. FAIL is a metadata issue (start_commit pointing after the code), not a substantive implementation defect."
  - Fix: set `start_commit` to `8767fc7` (the original "start implementation" commit) so `8767fc7..HEAD` covers the full slice scope including all 18 changed files.
  - Regenerated `proof.md` "Files changed" from `git diff --name-only 8767fc7..HEAD` (18 files, all 8 planned touchpoints present).
  - Added "Bookkeeping commits within start_commit..HEAD" subsection to "Divergence from plan" acknowledging that 10 doc-only/merge commits in the range are not implementation scope.
  - Discovered and fixed a worktree state corruption: the track worktree had been re-registered to `main` instead of `track/2026-06-16-fidelity-layer/T1-fidelity-core`. Removed and re-added the worktree on the correct branch.
  - All tests pass (13 RTM unit, 5 integration, state round-trip, board vertical-trace). `go vet ./...` clean. `gofmt -l .` clean.
  - No code changes in this session — metadata and proof.md fix only.
