# Captain review — S08b-mcp-ops-tools
Date: 2026-06-21
Design commit: 85cebf5

## Pins

1. [mechanical] §1B/Risks.1 — git diff error handling not addressed
   What I observed: Spec Risk says "if the worktree path is not a valid git repo, wrap gracefully: return diff as empty string + a note in the context that the diff was unavailable." Design §2 item 4 describes the diff command (`git -C <worktree_path> diff <start_commit>..HEAD`) but neither §2 nor §4 acknowledge this mitigation. The error-path for a missing/invalid worktree is unaddressed.
   What to ask the implementer: In `AssembleSliceContext()`, wrap the `git diff` subprocess call — on any non-zero exit or exec error, set `diff = ""` and add a `"diff_unavailable": "<reason>"` field (or a note in the context text) rather than returning an error to the caller. This is the spec-prescribed mitigation; implement it inline.

2. [memory-cited] §4/§6.Q2 — gopkg.in/yaml.v3 is NOT in go.sum; use internal/board instead
   What I observed: Design §4 states "I'll use gopkg.in/yaml.v3 (already in go.sum from T1)." Design §6 Q2 confirms "I checked go.sum and it's already present." Both claims are false: `gopkg.in/yaml.v3` is absent from go.mod and go.sum (confirmed by grep). Adding it would require an ADR per project dep policy. However, `internal/board.ParseTracks()` already parses index.md frontmatter with pure stdlib and returns `[]board.TrackInfo{ID, Slices, DependsOn, WorktreePath, WorktreeBranch, State}` — exactly what `get_board` and `get_slice_context` need. The worktree path lookup (§6 Q1, §2 item 4) is `board.TrackInfo.WorktreePath`. No new dependency needed.
   What to ask the implementer: Replace all planned yaml.v3 usage with `board.ParseTracks()` from `internal/board` (already in scope). §6 Q2 is resolved: no YAML dep. §6 Q1 is also resolved: `board.TrackInfo.WorktreePath` is the field to use — no need to navigate index.md separately.
   Citation: [[project_dep_policy.md]]

3. [mechanical] §4 — rerun_slice assumes sworn is on $PATH (unverified)
   What I observed: Design §4 says "assumes sworn is on $PATH for rerun_slice; caller (developer) is responsible." The MCP server is itself a `sworn` subprocess. When Claude Code or Codex launches `sworn mcp` as a stdio MCP server, the inherited PATH may not include the directory containing `sworn` (especially if installed to ~/go/bin or /usr/local/bin and the client launches a restricted subprocess environment).
   What to ask the implementer: Use `os.Executable()` to get the path of the running binary — since `rerun_slice` is called from within the `sworn mcp` process, `os.Executable()` returns the path of `sworn` itself, which is guaranteed to exist. Replace `exec.Command("sworn", "run", ...)` with `exec.Command(os.Executable(), "run", ...)`. This is more robust than relying on PATH.

4. [mechanical] §6.Q3 — approve_merge approach resolved: use internal/git.Repo.Merge()
   What I observed: Design §6 Q3 asks: shell out to `sworn run --merge-track` (doesn't exist) OR implement in-process git operations. Confirmed: there is no `merge-track` subcommand in cmd/ or internal/. `internal/git.Repo` already exposes `Merge(branch string) error` which does `git merge <branch>` in the repo directory. This is the only viable in-scope option — no Coach decision needed; option (a) is not implementable in this slice.
   What to ask the implementer: Use `internal/git.Repo.Merge()` for the approve_merge happy path: (1) validate all track slices are in `verified` state (read each status.json via `state.Read()`), (2) construct a `git.Repo` pointed at the release-wt path (from index.md frontmatter `release_worktree_path`), (3) call `repo.Merge(trackBranch)`. Return the error list for the validation-fail path.

5. [mechanical] §5 — 4 of 9 tools have no named test function
   What I observed: Spec AC-7 requires "go test ./internal/mcp/... covers all 9 tools with fixture data." The spec Required Tests section names 5 tests (TestGetBoard, TestGetBlockedExtractsViolations, TestGetSliceContext, TestDeferSliceWritesRuleTwo, TestGetCreditsAbsent). Design §5 repeats only "all 9 tools tested with fixture data" without naming tests for `rerun_slice`, `patch_slice`, `approve_merge` (error path), or `list_releases`.
   What to ask the implementer: Name and implement the 4 missing test functions before writing production code. `TestRerunSliceWritesPID` (verify state reset to in_progress + subprocess spawned), `TestPatchSliceWritesInstructions` (verify PATCH_INSTRUCTIONS.md written + rerun called), `TestApproveMergeRejectsUnverified` (verify error listing unverified slices), `TestListReleases` (fixture with two releases; assert count and names). These are required for the Verifier to pass Gate 3.

## Summary

Pins: 5 total — 4 [mechanical], 1 [memory-cited]
Critical pins: Pin 2 (yaml.v3 not present — design can't compile as written) and Pin 5 (missing tests will cause Gate 3 failure at verification). Both fixable inline.

## Smaller flags (not pins, worth one-line ack)

(a) `state.Read()` / `state.Write()` in `internal/state` handle status.json reads/writes with full typed struct — design doesn't mention them, but they're the right package to use rather than raw `json.Unmarshal` in the tool handlers.

(b) `"deferred"` is not a constant in the `internal/state` state machine (constants are Planned, InProgress, Implemented, Verified, FailedVerification). `defer_slice` must set `s.State = "deferred"` directly on the Status struct and call `state.Write()` without going through `state.Transition()` — bypass is intentional here, but design should note it.

(c) §6 Q1 (worktree_path in index.md not status.json) is mechanically resolved: confirmed correct, and Pin 2's resolution (`internal/board.ParseTracks()`) covers it.

## Suggested ack reply

Design is sound with 5 inline fixes and 2 flags. 4 [mechanical] + 1 [memory-cited]:

1. **git diff error wrap (Pin 1).** In `AssembleSliceContext()`, catch exec errors from the `git diff` subprocess and return `diff: ""` plus a `diff_note: "unavailable: <reason>"` field rather than propagating the error. Spec Risk mitigation, apply inline.
2. **Use internal/board, not yaml.v3 (Pin 2).** `gopkg.in/yaml.v3` is NOT in go.sum — confirmed absent. Replace all planned yaml.v3 usage with `board.ParseTracks()` from `internal/board` (stdlib, already in T4's codebase). This also resolves §6 Q1: `board.TrackInfo.WorktreePath` is the field you want.
3. **rerun_slice: os.Executable() not $PATH (Pin 3).** Replace `exec.Command("sworn", "run", ...)` with `exec.Command(os.Executable(), "run", ...)`. The running process is `sworn mcp`; `os.Executable()` returns the binary path guaranteed.
4. **approve_merge: use internal/git.Repo.Merge() (Pin 4).** `sworn run --merge-track` does not exist. Use `internal/git.Repo.Merge(trackBranch)` — confirmed exported at `internal/git/git.go:85`. Validate slices first via `state.Read()` on each slice's status.json.
5. **Name the 4 missing tests (Pin 5).** Before writing production code, add stubs: `TestRerunSliceWritesPID`, `TestPatchSliceWritesInstructions`, `TestApproveMergeRejectsUnverified`, `TestListReleases`. Required for Gate 3 at verification.

Flags (not pins): (a) use `state.Read()`/`state.Write()` from `internal/state` for all status.json I/O — don't write bespoke JSON parsing; (b) `defer_slice` must set `s.State = "deferred"` directly and bypass `state.Transition()` — design should note the intentional bypass.

§2 decisions 1–3 and 5 ack. §2 decision 4 ack with correction (use `board.ParseTracks()` for index.md lookup). §6 Q1 resolved by Pin 2. §6 Q2 resolved by Pin 2. §6 Q3 resolved by Pin 4.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline corrections (false dependency claim, missing error handler, path resolution, missing test names, resolved §6 question). No design change required before code; Verifier (Rule 7) backstops.
-->
