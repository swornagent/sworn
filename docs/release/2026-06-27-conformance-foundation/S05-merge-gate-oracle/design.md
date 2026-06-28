# Design TL;DR — S05-merge-gate-oracle

## Approach

Three workstreams, all delivered in this slice:

1. **CLI merge subcommands** (`cmd/sworn/merge.go`): `sworn merge-track` and `sworn merge-release` — register with the process-wide command registry (same pattern as `cmd/sworn/board.go`, `cmd/sworn/ship.go`).
2. **Fix MCP tools_ops.go** to route verified-state checks through `board.Oracle` (git-ref reads) instead of `os.ReadFile`/`state.Read` (working-tree reads).
3. **Invariant-4 conflict classifier**: before a merge, dry-run `git merge --no-commit --no-ff`, detect conflicts via `git diff --diff-filter=U`, and block if any conflict file is NOT a documented shared file in the touchpoint matrix.

---

## 1. `cmd/sworn/merge.go` — CLI merge subcommands

### Registration pattern
```go
func init() {
    command.Register(command.Command{Name: "merge-track", Summary: "...", Run: cmdMergeTrack})
    command.Register(command.Command{Name: "merge-release", Summary: "...", Run: cmdMergeRelease})
}
```

### `sworn merge-track <track-id> [--release <name>]`
- Resolve release name from `--release` flag, or derive from `git branch --show-current` (parse `track/<release>/<id>`).
- Construct `board.NewGitOracle(repo)` and read the board state for the release.
- **Gate 1 — verified-state check**: iterate the track's slices via the oracle's `ReadSliceStatus`. Any slice not in `verified`/`deferred`/`shipped` → exit non-zero with message naming the unverified slice.
- **Gate 2 — invariant-4 classifier** (see §3 below).
- **Gate 3 — working-tree cleanliness**: assert `git status --porcelain` is empty (spec risk: dirty tree breaks dry-run).
- Execute the real merge via `repo.Merge(trackBranch)`.
- Exit 0 on success.

### `sworn merge-release [--release <name>]`
- Read the full board state via oracle.
- **Gate 1 — all slices terminal**: every slice in every track must be `verified`/`deferred`/`shipped`. If any are not, exit non-zero with the list.
- **Gate 2 — all tracks merged**: check that every track branch is an ancestor of `release-wt/<release>` via `git.IsAncestor`.
- **Gate 3 — journey gate**: call `journey.Check(projectRoot)`; if `CheckMissing` or `CheckUnratified`, exit non-zero with BLOCK message (per Rule 10).
- Merge any unmerged track branches.
- Exit 0.

### Key design choices
- **Both commands are idempotent** — re-running after a success is a no-op (already merged → `Already up to date.` is success).
- **Deriving release from branch name** as a fallback avoids needing `--release` when invoked on a track worktree.
- **The journey gate is stubbed per the spec's open deferral** — `journey.Check` is called but if `journeys.json` is absent (S17 not yet shipped), the gate fails closed with a clear message. See "Deferral allowed" in spec.

---

## 2. Fix MCP `tools_ops.go` — oracle-backed state reads

### Current problem
`handleApproveMerge` (lines 381-446) and `readReleaseBoard` (lines 207-237) read `status.json` from the **working tree** via `os.ReadFile` + `state.Read`. This is the exact bug the spec calls out: the merge gate must read committed state from git refs, not the filesystem.

### Change
1. Add a `*git.Repo` field to `OpsTools` (or a `board.Oracle` adapter).
2. In `handleApproveMerge`:
   - Replace `state.Read(statusPath)` loop with `oracle.ReadSliceStatus` via track-branch ref reads.
   - The oracle already resolves: owner track branch → release-wt → HEAD.
3. In `readReleaseBoard`:
   - Replace `os.ReadFile(indexPath)` + `board.ParseTracks(frontmatterBody)` + per-slice `state.Read` with `oracle.ReadBoard`.
4. The `RegisterOpsTools` constructor grows a `*git.Repo` parameter.

### Design rationale
- The oracle is already the production-grade reader used by `sworn board`, the router (S04), and the scheduler. MCP tools should use the same data path.
- Injecting `*git.Repo` keeps the change minimal — `board.NewGitOracle(repo)` is one line.

---

## 3. Invariant-4 conflict classifier

### Location
New function `Invariant4Check(repo *git.Repo, trackBranch string, documentedShared map[string]bool) error` in `internal/router/router.go` (near the existing `routeMergeDecision`, per spec's audit ref router.go:381-429).

### Algorithm
1. Assert working tree is clean (`git status --porcelain`).
2. Run `git merge --no-commit --no-ff <trackBranch>`.
3. If exit code is 0 (clean merge) → `git reset --merge HEAD` to undo → return nil (pass).
4. If non-zero, detect conflicted files:
   - `git diff --name-only --diff-filter=U` returns the list of unmerged files.
5. For each conflicted file:
   - Check if it matches any key in `documentedShared` (the touchpoint matrix's documented-shared-file set). Use prefix matching: a conflict on `internal/model/oai.go` matches the documented-shared entry `internal/model/oai.go + drivers`.
   - If NOT in the set → `git merge --abort` → return error: `"BLOCK: invariant-4 violation — conflict on <filename> (not a documented shared file)"`.
6. If all conflicts are on documented shared files → `git merge --abort` → return nil (invariant-4 satisfied — the conflict is expected and will be resolved by the human).

### Building the documented-shared-file set
Parse the touchpoint matrix from `index.md` (markdown table below the frontmatter). A file/surface is "documented shared" when **≥2 tracks** have a checkmark (✓) in the matrix. The keys are the file paths (first column, normalized by trimming DOCUMENTED SHARED suffix and leading backticks). The matrix also explicitly marks some rows as `(DOCUMENTED SHARED)` — those rows are always included.

For the current release, the documented shared files (from `index.md` § Touchpoint matrix):
- `internal/model/oai.go`
- `internal/run/slice.go`
- `internal/verify/verify.go`
- `internal/model/openai_responses.go`
- `internal/verify/verify_test.go`
- `internal/state/state.go`

### Risk mitigation
- **Working-tree cleanliness**: the function aborts the merge (`git merge --abort` or `git reset --merge HEAD`) in every path, so a dirty-tree check at entry should catch most issues.
- **Concurrent merges**: the caller (CLI or MCP tool) must serialize merges — the function itself is not goroutine-safe (it mutates the working tree).

---

## Files touched

| File | Change |
|---|---|
| `cmd/sworn/merge.go` | **New** — CLI subcommands `merge-track` and `merge-release` |
| `internal/mcp/tools_ops.go` | Fix `handleApproveMerge` and `readReleaseBoard` to use `board.Oracle`; add `*git.Repo` to `OpsTools` |
| `internal/router/router.go` | Add `Invariant4Check` function; parse touchpoint matrix to build documented-shared set |
| `internal/git/git.go` | Add `MergeDryRun(branch) (conflictFiles []string, err error)` method; add `ResetMerge()`, `MergeAbort()` helpers |

### New test files
- `cmd/sworn/merge_test.go` — CLI table tests with mock oracle + mock git
- `internal/router/router_test.go` — add invariant-4 classifier tests
- `internal/mcp/tools_test.go` — add oracle-backed merge tests

---

## Risks / pins for reviewer

1. **Touchpoint matrix parser fragility**: the markdown table parser must handle the current `index.md` table format. If the format changes, the parser breaks. Mitigation: parse defensively (column-based matching, normalize whitespace). An alternative is to read `planned_files` from each slice's `status.json` on the release-wt ref and compute the intersection — this is more robust but requires reading every slice's status.json. **Design choice**: use the index.md touchpoint matrix (it's the canonical declaration of shared files) with a fallback to status.json planned_files.

2. **Merge abort state**: `git merge --no-commit --no-ff` followed by conflict detection and `git merge --abort` should always restore to a clean state. If `merge --abort` fails (e.g., the repo was already in a conflicted merge), the function must surface the error. The dirty-tree pre-check catches this.

3. **Oracle injection into MCP tools**: `RegisterOpsTools` currently takes only `repoRoot`. Adding a `*git.Repo` parameter changes the function signature, which affects the server startup code. Need to audit all callers of `RegisterOpsTools`.

## AC traceability

Each acceptance check maps to a planned change:
- **AC1** (all verified → merge, exit 0) → `cmd/sworn/merge.go` cmdMergeTrack gate-1
- **AC2** (unverified → exit non-zero) → `cmd/sworn/merge.go` cmdMergeTrack gate-1 + `internal/mcp/tools_ops.go` handleApproveMerge
- **AC3** (invariant-4 conflict → BLOCK message) → `internal/router/router.go` Invariant4Check
- **AC4** (journey gate → BLOCK on absent/unratified) → `cmd/sworn/merge.go` cmdMergeRelease gate-3
- **AC5** (use oracle not os.ReadFile) → `internal/mcp/tools_ops.go` fixes
- **AC6** (MCP tools pass same gates) → `internal/mcp/tools_ops.go` handleApproveMerge uses Invariant4Check + oracle reads