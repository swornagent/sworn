# Journal — S57-oracle-reader

## 2026-07-14: Implementation session (round 1)

**State transition**: `design_review` → `in_progress` → `implemented`

**Coach pins addressed:**
1. Added `Routing` field to `state.Verification` struct (`internal/state/state.go`)
2. Added `internal/state/state.go` to `planned_files`
3. Added `design_decisions` array (5 Type-2 entries) to `status.json`
4. Introduced `gitContentReader` interface in `oracle.go` — `oneShotEmptyReader` fake enables `TestTransientReadRetry` with a one-shot empty read
5. Added `internal/git/git.go` and `internal/git/git_test.go` to `planned_files`

**Coach flags:**
a. `ReadBoard` output struct includes `actionable`, `dependsOnTracks`, `owner`, `lastUpdated`, `track` for parity with `release-board-status.sh --json`
b. `ReadBoard` reads `index.md` from git ref first, then passes frontmatter body to `ParseTracks`
c. `ResolvedFrom` return type explicitly declared

**Implementation decisions:**
- `gitContentReader` interface with `Show` and `CatFileExists` methods; production adapter wraps `*git.Repo` via method values
- `NewGitOracle(repo *git.Repo)` constructor for production; `NewOracle(reader gitContentReader)` for tests
- Docs prefix probe via `git cat-file -e` against the committed tree (coach pin 7, avoids Fumadocs symlink trap)
- Transient-read retry: 50ms sleep + one re-read on empty content
- Ghost-slice filter: ReadBoard skips slices whose resolved track doesn't match the iterating track
- `cmd/sworn/board.go` self-registers via `init()` with `command.Register` (S51 pattern)
- `sworn board --json` outputs shape compatible with `release-board-status.sh --json`
- Blocked visibility: `BlockedReason` from first violation, `BlockedOwner` from `verification.routing` or inferred

**Pre-existing issues noted:**
- `ParseTracks` bug: `inDependsBlock` is not cleared by `slices:` line when depends_on precedes slices in frontmatter. Worked around in tests; not fixed here (out of scope for S57).

**Deferrals:** None.

**Skeptic panel:** skipped — runtime does not support subagent dispatch.
## 2026-07-14: First-pass PASS

`release-verify.sh` — 23/23 checks passed, 0 failed. State: `implemented`. Ready for fresh-context verifier session.
