<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is a faithful port of captain-route.sh with the right edge-case priorities (commit-time-newest, ghost filter, blocked-precedes-state). 8 pins, all mechanical — interface wiring and missing git methods to declare before code:

1. **OracleReader interface signature.** The existing `Oracle.ReadSliceStatus` takes 7 params (ctx, gitContentReader, trackBranch, releaseWTRef, release, sliceID, trackMap) — not a clean interface method. Define `OracleReader` with router-friendly signatures that hide ref/track-map resolution, or have the CLI construct the full param set and pass `*Oracle` directly. Confirm approach before code.
2. **LastCommitTime needs internal/git/git.go.** `git.Repo` has no `Log`/`LastCommitTime` method. Add it to `internal/git/git.go` and add `internal/git/git.go` + `internal/git/git_test.go` to `planned_files`.
3. **Do NOT add LastCommitTime to GitContentReader.** `cmd/sworn/board.go`'s `oracleReader` satisfies the current interface; adding `LastCommitTime` breaks it. Use a separate `CommitTimeReader` interface in `internal/router/` or call `*git.Repo` directly for commit-time queries.
4. **Deferred state in track walk.** The `verified` walk must treat `deferred` as terminal (skip it), matching `captain-route.sh:492`. Top-level `deferred` routes to `none` (bash fall-through). Add a test case.
5. **verification.reason field.** `state.Verification` has no `Reason` field. Check whether any status.json has ever set `verification.reason` — if not, use `strings.Join(violations, "; ")` and document the `.reason` fallback as bash-only jq. If some do, add `Reason string` to `state.Verification` and add `internal/state/state.go` to `planned_files`.
6. **IsAncestor for merge-track/merge-release.** `internal/git/git.go` has no `MergeBase`/`IsAncestor`. Add `IsAncestor(branch, ancestor string) (bool, error)` wrapping `git merge-base --is-ancestor`, and declare `internal/git/git.go` in `planned_files`.
7. **approved-ack.md presence check semantics.** Bash uses working-tree `-f`; design claims "not reading working tree." Resolve: use `CatFileExists` on the track branch for `approved-ack.md` (committed-ref), and `LastCommitTime` on the track branch for design/review/decline. Document the resolution.
8. **design_decisions in status.json.** Add the 5 §2 decisions as `design_decisions` in `status.json`, all Type-2 with options + rationale. Recurring trial-log pattern (8th+ occurrence).

§2 decisions 1-5 ack (all Type-2, appropriate for a pure-function port). §6 questions: none.

Address pins 1-8 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 8 pins are mechanical wiring/declaration fixes the implementer applies inline (add missing files to planned_files, define interface signatures, add git methods); no design re-review needed before code is safe.
-->
