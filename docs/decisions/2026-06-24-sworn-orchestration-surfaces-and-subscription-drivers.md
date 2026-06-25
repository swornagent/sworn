# Decision: Orchestration Surfaces and Subscription Drivers

**Date:** 2026-06-24  
**Status:** Accepted  
**Context:** S59-scheduler-relayer (T17-orchestration-core, release 2026-06-19-safe-parallelism)

## Decision

The cooperative pause/resume control lives in the **engine layer** (`internal/scheduler.PauseEngine`), not in any surface (CLI, TUI, MCP). All three surfaces call the same engine functions:

- `scheduler.PauseRelease(release)` — signal workers to stop at the next router-poll boundary
- `scheduler.ResumeRelease(release)` — clear the pause signal (workers resume on next `sworn run --parallel`)

This means the same engine function is invokable identically from:
- **CLI**: `sworn pause <release>` / `sworn resume <release>` subcommands
- **TUI**: keyboard shortcut mapped to `scheduler.PauseRelease`
- **MCP run-control tools**: `sworn_pause_release` / `sworn_resume_release` tool implementations

## Why this matters

If the pause mechanism were implemented in each surface independently (e.g., a TUI key that sends SIGINT, a CLI command that writes a file, an MCP tool that calls a REST endpoint), the three surfaces would diverge in semantics and the engine would have no single authoritative state. Putting the signal in `internal/scheduler.PauseEngine` ensures:

1. **Single truth**: one closed channel per release signals all workers for that release, regardless of which surface triggered it.
2. **Cooperative, not abrupt**: the channel is checked at the top of each router-poll loop iteration — after any in-flight dispatch completes, committed state is always consistent.
3. **Crash-safe**: pause state is not persisted across process restarts. A paused release's workers return `TrackPaused`; re-running `sworn run --parallel` starts a fresh engine and resumes from committed state (the router re-derives the next action from `status.json`).

## Subscription drivers (out of scope for S59)

The reference coach loop drives subscription via webhook/ntfy paging when tracks pause or fail (S07, T3). S59 surfaces pause states to `RunParallel`'s return value; the paging transport is wired separately and is out of scope for this slice.

## Alternatives considered

- **File-based pause flag**: `sworn pause` writes `.sworn/paused/<release>`, worker polls the file. Rejected: more I/O, harder to unit-test, process restart could leave stale flag.
- **OS signal (SIGTSTP)**: pauses the whole process. Rejected: CLI only (TUI and MCP can't send signals to an in-process engine), and pauses all releases not just one.
- **Context cancellation**: propagates to all goroutines including dependency logic. Rejected: overly broad — we want to pause one release's workers while other releases (if any) continue.
