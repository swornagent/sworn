# Journal: S03-crash-recovery

## Session 1: 2026-07-28 — Initial implementation

### Decisions

- **MaxTurnsError sentinel placement**: Defined `ErrMaxTurns` in `internal/agent/agent.go` (the package where max-turns is first detected) and `MaxTurnsSentinel` (string constant) there too. The worker detects the sentinel via `strings.Contains` rather than importing `run` (which would create an import cycle: `run` → `scheduler` → `run`).
- **Circuit breaker DB table**: Added `circuit_failures` to the existing DB schema (`internal/db/db.go`) rather than a separate DB. The spec's deferral note says "record circuit breaker events anyway; they will be durable after S25 merges" — the table is already in the schema.
- **RecordPage function**: Added as a public function to `internal/supervisor/supervisor.go` rather than keeping it private (`logEvent`). The worker needs to call it from outside the supervisor package.
- **Worker integration points**: Circuit breaker and max-turns checks inserted at 3 locations in `worker.go` (router implement path, router redesign path, legacy path) to ensure consistent behavior across all code paths.

### Trade-offs

- **String matching vs import**: Used `strings.Contains(err.Error(), agent.MaxTurnsSentinel)` instead of `run.IsMaxTurnsExhausted(err)` to avoid import cycle. This follows the same pattern as the interpreter INCONCLUSIVE sentinel detection.
- **Hardcoded threshold 3**: Per spec, circuit breaker threshold is hardcoded at 3. Configurable threshold is out of scope for this slice.
- **Fail-open on DB unavailable**: ShouldBreak returns `false` when DB is nil or query fails, per AC4. This means the circuit breaker won't fire without a DB, but PAGE emit (RecordPage) is best-effort anyway.

### Test coverage

- 11 circuit tests covering: 3 consecutive → true, <3 → false, interleaved → false, reset after diff → true, nil DB → false, empty DB → false, fingerprint determinism, different slice/error → different FP, first-line-only, cross-slice isolation
- 2 max-turns worker tests: legacy path and router-driven path