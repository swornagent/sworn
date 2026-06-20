# ADR 0003 — SQLite for orchestration state

Status: accepted (2026-06-26)

## Context

Safe concurrent track execution requires an ACID-guaranteed process registry.
When T2-T4 run in parallel after T1 merges, three separate `sworn run --parallel`
processes may simultaneously acquire, release, and reap track ownership on the
same machine. The registry must survive crashes, handle concurrent writes
without corruption, and provide single-owner enforcement at the storage layer.

The project has a strict stdlib-only dependency policy (ADR 0001, Rule 1). A
plain-text file or JSON-based registry cannot provide the ACID guarantees needed
for 8+ concurrent workers without implementing a write-ahead log by hand.

## Decision

Use `modernc.org/sqlite`, a pure-Go SQLite driver, as a single external
dependency. Rationale:

1. **ACID transactions** — SQLite's transaction model provides the serialisable
   isolation level needed for single-owner enforcement. Two processes racing to
   `INSERT OR FAIL` on the same primary key get exactly one winner; the loser
   gets a constraint error, not silent corruption.

2. **Zero *runtime* OS dependency** — `modernc.org/sqlite` is a CGo-free
   translation of the SQLite C library to Go. The binary statically links
   SQLite with no `libsqlite3.so` requirement. This preserves the project's
   "single binary, run anywhere" distribution model.

3. **Pure Go** — no CGo, no cross-compilation pain, no platform-specific build
   tags. Works on any OS the Go toolchain supports.

4. **Proven at this scale** — SQLite handles single-machine concurrency up to
   dozens of writers without issue. The R3 release has at most 8 concurrent
   workers; SQLite's throughput is orders of magnitude beyond that.

## Consequences

- **Binary size increases** by ~8MB (from ~15MB to ~23MB for a stripped build).
  Acceptable: the ACID guarantee at 8+ workers is a correctness requirement,
  not a nice-to-have. The increase is documented in S01's spec as an accepted
  risk.

- **Windows process-liveness detection deferred.** `syscall.Kill(pid, 0)` is
  Unix-only. On Windows, `os.FindProcess(pid).Signal(syscall.Signal(0))`
  always succeeds, making reap-on-restart unreliable. Windows support for
  the supervisor is deferred to post-R3 (tracked in R3 deferral list).

- **Schema migrations must be idempotent.** SQLite has limited ALTER TABLE
  support. All migrations use `CREATE TABLE IF NOT EXISTS` to ensure
  idempotent startup. Schema versioning is deferred until a real migration
  is needed (currently a single schema).

## References

- ADR 0001 — stdlib-only policy with documented-exception mechanism
- SwornAgent issue #5 — R3 tracking
- `modernc.org/sqlite` — https://modernc.org/sqlite