# Design TL;DR — `S37-telemetry-tui-exclusion`

## §1. User-visible change

Running `sworn` with no subcommand (which launches the TUI) no longer emits a
telemetry event. Previously, the no-args path sent an event with an empty
`cmd` field and a `duration_ms` equal to the entire interactive TUI session
— junk data. After this change, the no-args/TUI path is excluded from firing,
consistent with the existing `sworn telemetry *` meta-command exclusion. All
real commands (`verify`, `run`, `lint`, etc.) continue to fire normally.

## §2. Design decisions not in spec (max 5)

1. **Mirror the existing exclusion shape exactly.** The existing `cmd == "telemetry"`
   early-return (line 205-207 of `telemetry.go`) is the pattern: check, return before
   spawning the goroutine. Adding `cmd == ""` immediately after is the obvious, minimal
   extension. Rationale: consistency with the existing codebase and zero new patterns.

2. **Empty string is the precise no-args signal.** Every real subcommand has a name
   (the command registry at `internal/command` resolves it). An empty `cmd` means
   `os.Args` had ≤1 element — exactly the TUI launch path in `main.go`'s `dispatch()`.
   No risk of false positives.

3. **No new exported API or config toggle.** The spec says "extend `Fire()`" — this
   is a hard-coded exclusion, not a configurable policy. Adding a toggle would be
   over-engineering for a slice this narrow.

4. **Timing: check-and-return happens synchronously, inside `Fire()`.** This matches
   the existing `cmd == "telemetry"` check — the exclusion is decided before the
   goroutine spawn. No race window, no goroutine to cancel.

5. **Test strategy: one negative test, one positive guard.** `TestFireSkipsEmptyCmd`
   asserts the transport is never called for `cmd=""`. `TestFireStillFiresRealCmd`
   asserts `Fire("verify", ...)` still hits the transport — this guards against an
   over-broad exclusion that accidentally catches everything.

## §3. Files I'll touch grouped by purpose

- **`internal/telemetry/telemetry.go`** — add the `cmd == ""` exclusion check
  immediately after the existing `cmd == "telemetry"` check. One line change
  (a new `if` block), same shape as the existing one.
- **`internal/telemetry/telemetry_test.go`** — two new tests:
  `TestFireSkipsEmptyCmd` and `TestFireStillFiresRealCmd`. Both follow the
  existing `TestFireTelemetryMetaCommandExcluded` test pattern (httptest
  server, custom transport, verify hit/no-hit on the transport).

## §4. Things I'm NOT doing

- **NOT editing `cmd/sworn/main.go`.** The spec explicitly rules this out.
  The exclusion lives in the telemetry package, not in the shared dispatch file.
- **NOT changing the telemetry data model, API endpoint, or event schema.**
- **NOT adding a configuration flag, environment variable, or user-facing
  toggle** to control TUI telemetry exclusion — it's unconditional.
- **NOT touching any other package** — no cross-package imports or refactors.

## §5. Reachability plan

Run `go test ./internal/telemetry/... -v` and capture output. The two new tests
(`TestFireSkipsEmptyCmd`, `TestFireStillFiresRealCmd`) plus the existing
`TestFireTelemetryMetaCommandExcluded` provide direct, deterministic evidence
that:
- Empty cmd → no event fired
- Real cmd → event fired
- Meta-command exclusion still works

No screenshot or E2E spec needed — the tests exercise `Fire()` directly through
its public API with a real `httptest.Server` transport.

## §6. Open questions for the Coach

None.