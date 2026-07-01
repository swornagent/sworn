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
