# Journal — S02b-concurrent-scheduler

## 2026-06-27 — First session

### State transition: design_review → in_progress

Coach approved via approved-ack.md. Two mechanical pins applied:

1. **Pin 1**: Added `internal/board/track.go` + `internal/board/track_test.go` to status.json.planned_files. These files are new touchpoints not in the original spec — board track parsing extracted to a separate file for reusability (used by scheduler, per design §2 Decision 1). Confirmed no touchpoint-matrix collision via Captain review.

2. **Pin 2**: Pre-declared sync.Map divergence in proof.md (see "Divergence from plan" section). The spec prescribes `sync.WaitGroup + channels` for dependency signalling; implemented with `sync.Map` outcome store instead. All ACs pass; sync.Map avoids N×M channel fan-out while delivering equivalent ordering and failure-cascade semantics.

**Flag (a)**: Clarified S01 scope — orphan DB rows are reaped by supervisor.Reap(); orphan git-worktree directories left on disk (no cleanup in this slice — future concern).

### Design decisions carried forward from design.md §2

- Parsed tracks via `internal/board/track.go` (ParseTracks from frontmatter)
- One worktree per track, materialised by worker goroutine; release worktree as pre-flight
- Failure cascade via context cancellation (direct dependents only)
- sync.Map outcome store (declared divergence)
- RunParallel in internal/run/parallel.go

### Implementation notes

- BuildPlan topologically sorts tracks into phases (concurrent execution sets)
- Each phase starts only when all tracks in the previous phase are done
- Worker goroutines use a shared outcome map + context tree for cancellation
- exit code: 0 iff all tracks PASS; 1 if any FAIL