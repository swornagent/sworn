# Journal — S08b-mcp-ops-tools

## State transitions

| Date | From | To | By | Reason |
|------|------|----|----|--------|
| 2026-06-21 | design_review | in_progress | implementer | Coach approved via approved-ack.md (5 pins, PROCEED) |
| 2026-06-21 | in_progress | implemented | implementer | All 9 tools implemented and tested |

## Decisions

1. **execSwornRun package-level variable** — Made the subprocess spawn function
   a package-level variable (`execSwornRun`) so tests can replace it with a
   fake returning a known PID. Production default uses `os.Executable()` per
   Pin 3. This is a testability improvement, not a behaviour change.

2. **Flag b: "deferred" bypasses state.Transition()** — The `defer_slice` tool
   sets `s.State = "deferred"` directly on the Status struct rather than going
   through `state.Transition()`, because "deferred" is not a constant in the
   state machine (constants are Planned, InProgress, Implemented, Verified,
   FailedVerification). This bypass was approved by the Coach in the
   approved-ack.md (Flag b). It is an intentional design decision, not a
   silent deferral.

## Design review (Captain + Coach)

- **Captain pin 1**: git diff error wrap in AssembleSliceContext — implemented
  via `runDiff()` returning safe (diff="", diff_note) on error.
- **Captain pin 2**: use `internal/board.ParseTracks()` instead of
  `gopkg.in/yaml.v3` — no yaml.v3 dependency added; all index.md frontmatter
  parsing uses board.ParseTracks().
- **Captain pin 3**: use `os.Executable()` for rerun_slice — implemented via
  `execSwornRun` which defaults to `exec.CommandContext(os.Executable(), ...)`.
- **Captain pin 4**: use `internal/git.Repo.Merge()` for approve_merge — implemented
  in handleApproveMerge, validates slices first then calls repo.Merge().
- **Captain pin 5**: name 4 missing test stubs before production code —
  TestRerunSliceWritesPID, TestPatchSliceWritesInstructions,
  TestApproveMergeRejectsUnverified, TestListReleases all implemented.

## Dark-code acknowledgements

The following code patterns were flagged by release-verify.sh as dark-code
markers but are **acknowledged design decisions** (approved by Coach):

| File | Line | Pattern | Status |
|------|------|---------|--------|
| tools_ops.go:396 | `// Flag b: "deferred" bypasses state.Transition() — set directly` | Comment referencing Coach-approved bypass | Acknowledged (approved-ack.md Flag b) |
| tools_ops.go:397 | `s.State = "deferred"` | Direct state write (intentional bypass) | Acknowledged (approved-ack.md Flag b) |

## Deferrals

None. All 9 tools implemented with full test coverage.