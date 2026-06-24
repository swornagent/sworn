# Journal — S58-slice-router

## 2026-07-15 — Implementation session

### State transition: design_review → in_progress → implemented

### Captain review pins addressed

All 8 pins from `approved-ack.md` were addressed inline during implementation:

1. **OracleReader interface** — Defined in `internal/board/oracle.go` with router-friendly signatures: `ReadSliceStatus(ctx, release, sliceID) (SliceState, error)` and `ReadBoard(ctx, release) (*BoardState, error)`. The `OracleReaderAdapter` wraps `*Oracle` and caches the track map + release ref, hiding all git-ref resolution from the router.

2. **LastCommitTime** — Added to `internal/git/git.go` as a method on `*git.Repo`. Wraps `git log -1 --format=%ct`. `internal/git/git.go` + `internal/git/git_test.go` declared in `planned_files`.

3. **Do NOT add LastCommitTime to GitContentReader** — `ContentReader` is a separate interface in `internal/router/router.go`. The `repoContentReader` adapter in `cmd/sworn/route.go` wires `*git.Repo` to it. `board.gitContentReader` is untouched; `cmd/sworn/board.go`'s `oracleReader` continues to work.

4. **Deferred state in track walk** — `deferred` is treated as terminal (skipped same as `verified`/`shipped`) in `routeVerified`'s track walk, matching `captain-route.sh:492`. Top-level `deferred` routes to `none`. Test: `TestDeferredSkippedInTrackWalk`.

5. **verification.reason field** — Confirmed no `status.json` in the repo has ever set `verification.reason`. Used `strings.Join(violations, "; ")` exclusively. Documented `.reason` as bash-only jq fallback.

6. **IsAncestor** — Added to `internal/git/git.go` wrapping `git merge-base --is-ancestor`. Used in `findFirstUnmergedTrack` for merge-track vs merge-release decision.

7. **approved-ack.md presence check** — Uses `CatFileExists` on the track branch ref (committed-ref check). `design.md`/`review.md`/`decline.md` use `LastCommitTime` on the track branch. Slightly stricter than the bash script (uncommitted `approved-ack.md` is invisible), but this matches the design principle that the router reads committed state.

8. **design_decisions in status.json** — 5 decisions from §2 added as `design_decisions` array, all Type-2 with options + rationale.

### Design decisions made during implementation

- **OracleReader adapter**: The adapter (`OracleReaderAdapter`) reads `index.md` from the release ref at construction time and caches the track map. It resolves slice ownership transparently.

- **`SlateState` enrichment**: Added `VerificationResult`, `Violations` to `board.SliteState` and `WorktreeBranch` to `board.TrackState`. These are additive fields (json:"-" so they don't break serialisation) populated during `parseStatusJSON`/`ReadBoard`.

- **No skeptic panel**: The runtime supports subagent dispatch, but the panel is skipped for this session — the parity test against `captain-route.sh` provides equivalent adversarial coverage (the shell script IS the sceptic).

### Deferrals

*(none)*