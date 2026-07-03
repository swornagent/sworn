# Journal — S02-claude-subprocess-driver

## 2026-07-03 — design_review → in_progress → implemented

**Design review resolution (prior session, this session inherits it).** Captain
design review surfaced 4 pins (2 mechanical, 1 memory-cited, 1 escalate). Brad
ratified pin 2 as a Type-1 decision: `internal/driver`'s new `ErrKind`
vocabulary maps a non-zero CLI exit to `ErrKindAuth` (not a generic `provider`
label), matching `internal/model/cli.go`'s existing coarse heuristic exactly,
to preserve `internal/run/slice.go:487`'s terminal-halt-on-auth fail-fast
through the driver rewire. Recorded in `status.json.design_decisions` and
`spec.json` AC-04/R-03 before this session started. `review.md` carries
`DECISION: PROCEED`.

**Implementation.** Built `internal/driver/subprocess.go` (provider-neutral
spawn/env-hygiene/error-classification plumbing, `ErrKind` constants) and
`internal/driver/claude.go` (`ClaudeDriver`, the first real
`driver.Driver` implementation). Followed design.md's file split exactly so
S03 (codex) can reuse the plumbing without copy-paste.

Grounded every design choice against live code before writing it:
`internal/driver/driver.go`/`worktree.go` for the S01 contract and
`AssertWorktree`; `internal/model/cli.go`/`cli_test.go` for the exact
error-classification precedent (exec.Error/fs.PathError → config,
exec.ExitError → auth, context.DeadlineExceeded → transient) and the
fake-binary `TestMain` re-exec convention; `internal/run/run.go:353` to
confirm the `CapChat` gate reads `Capabilities()` (not a static table), so
retiring `cliDriver`'s `CapChat` fails fast with a clear error instead of a
silent toolless dispatch (R-02's own mitigation).

**Decisions made during implementation (not already fixed by design.md):**

1. **Binary resolution is a struct field, not an env var.** `cli.go`'s
   `cliDriver` reads `CLAUDE_BIN`/`SWORN_CLI_TIMEOUT` from the environment;
   `ClaudeDriver` takes `Binary string` directly instead. Wiring
   env-var-driven configuration into the driver itself would anticipate
   S05's registry/config work, which owns that concern. Tests set `Binary`
   directly on the struct literal — no env var needed for the fake-CLI
   harness.
2. **Envelope's own `duration_ms` is preferred over the measured wall-clock
   time, with the measured value as fallback only.** Not explicitly stated
   in spec.json/design.md; chosen because AC-01 lists `duration_ms` as one
   of the envelope fields "populated from the CLI's JSON result envelope,"
   and R-01's defensive-parsing principle (degrade gracefully on absence)
   implies the CLI's own reported value is authoritative when present.
3. **`fakeClaudeHang` uses `time.Sleep(24h)`, not `select{}`.** Discovered
   empirically: `internal/model/cli_test.go`'s existing fake-hang uses a bare
   `select{}` and it works there only because that package's test binary has
   other background goroutines (import-graph side effect) that keep the Go
   runtime from concluding all goroutines are permanently asleep.
   `internal/driver`'s test binary has no such goroutines, so a bare
   `select{}` is a genuine, single-goroutine deadlock and the Go runtime
   kills the process immediately with "fatal error: all goroutines are
   asleep" — misclassified as `ErrKindAuth` (a non-zero exit) instead of
   `ErrKindTransient` (a timeout). `time.Sleep` registers a pending timer,
   which the runtime treats as guaranteed future progress, so it blocks for
   real instead of deadlocking instantly. Caught by the table-driven
   `TestClaudeErrorMapping`/`TestSpawn_Timeout` tests failing deterministically
   on first run — not a flake, a correctness bug in the test double.

**Verification run before marking implemented:** `go build ./...`,
`go vet ./...`, `go test ./internal/driver/... ./internal/model/...` (AC-06's
literal required command), and the full `go test ./...` (no regressions in
any other package from retiring `cliDriver`'s `CapChat`/`Chat`).

**No out-of-scope work performed.** Codex (S03), registry wiring (S05), and
real-CLI integration proof (S10) are untouched, per spec.json's
`out_of_scope`.

State: `in_progress` → `implemented`.
