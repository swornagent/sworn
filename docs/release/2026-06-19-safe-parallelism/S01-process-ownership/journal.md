# Journal: S01-process-ownership

## Session 1 — 2026-06-26

**Role**: Implementer
**Worktree**: `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T1-concurrency-core`

### Decisions

1. **modernc.org/sqlite v1.52.0** — Latest version at implementation time. Pulls in
   several transitive deps (modernc.org/libc, golang.org/x/sys, etc.) all pure-Go.
   ADR-0003 documents the exception to ADR-0001's stdlib-only rule.

2. **SetMaxOpenConns(1)** — SQLite only allows one writer at a time regardless.
   Setting MaxOpenConns to 1 serialises writes at the Go pool level rather than
   relying on SQLITE_BUSY retries. WAL mode allows concurrent reads through the
   shared cache. This was necessary because the default pool behaviour created
   SQLITE_BUSY errors in concurrent test scenarios.

3. **INSERT-first pattern in Acquire** — The race-safe pattern tries INSERT first
   (PRIMARY KEY constraint provides atomic single-owner enforcement), then falls
   back to checking the existing row on constraint violation. This avoids the
   TOCTOU race of "query then insert" without requiring external locking.

4. **Reap collects into memory** — The Reap function collects stale rows into
   an in-memory slice first, closes the rowset, then deletes. This avoids the
   nested query+exec anti-pattern that deadlocks with SetMaxOpenConns(1).

5. **Run-loop integration** — DB opened at `.sworn/sworn.db` under workspace root.
   Reap called at startup. Acquire for "S01-task" called before implement loop.
   Release deferred with `MustRelease`. All optional via Options fields for
   testability.

### Trade-offs

- Binary size increase of ~8MB from modernc.org/sqlite. Accepted per ADR-0003.
- `syscall.Kill(pid, 0)` is Unix-only. Windows support deferred (documented
  in ADR-0003 as a known limitation).
- `.sworn/` gitignore uses non-anchored `.sworn/` pattern to catch any
  subdirectory (test artifacts from `go test` changing cwd).

### Deferrals (Rule 2)

None — this slice is the foundation for T1 and has no open deferrals.

### Out-of-scope discoveries

None — implementation stayed within planned touchpoints.