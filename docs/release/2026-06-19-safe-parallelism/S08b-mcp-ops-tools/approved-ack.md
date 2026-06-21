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
