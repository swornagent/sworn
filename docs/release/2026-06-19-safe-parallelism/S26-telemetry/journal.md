# S26-telemetry — Journal

## Session 1: Implementation (2026-06-21)

### State transitions
- `design_review` → `in_progress` (commit 208ec07)
  - Transitioned to in_progress after Coach approved via approved-ack.md
  - Added open_deferrals entry for AC1/AC2 (T3/S09 cross-track dependency)
  - Cleared verification.result (stale from prior round)
  - set start_commit

### Coach decisions applied (from approved-ack.md)
1. **Pin 1** — AC1/AC2 Rule 2 deferral: Added to status.json open_deferrals
2. **Pin 2** — main.go four-way collision: Added §4 note in design.md
3. **Pin 3** — dispatch() version/help cases: Confirmed both return 0 explicitly
4. **Pin 4** — Meta-command exclusion: Coach pick (a) — exclude sworn telemetry *; version/help still fire
5. **Pin 5** — Config path: Coach pick (a) — hardcode ~/.config/sworn/
6. **Pin 6** — ShowDisclosure neutrality: Intentional — only shows in neutral state
7. **Pin 7** — ShowConsent contract: Documented signature ShowConsent(r io.Reader, w io.Writer) (bool, error); added TestShowConsent tests
8. **Pin 8** — sworn_version delivery: Coach pick (a) — Fire(cmd, sub, version string, durationMS int64, exitCode int)

### Flags applied
- (a) Renamed TestIsEnabled_Neither → TestIsEnabled_OptedIn_NoOverrides
- (b) go_version trimmed to major.minor in trimGoVersion()
- (c) sworn version/help fire telemetry (consistent with Pin 4)

### Files created
- `internal/telemetry/telemetry.go` — core telemetry package
- `internal/telemetry/telemetry_test.go` — 18 tests (all pass, no race)
- `cmd/sworn/telemetry.go` — sworn telemetry on|off|status subcommand

### Files modified
- `cmd/sworn/main.go` — extracted dispatch(), added ShowDisclosure + telemetry.Fire
- `docs/release/2026-06-19-safe-parallelism/S26-telemetry/design.md` — updated per Coach

### Test results
- `go test -race ./internal/telemetry/...` — PASS (18 tests)
- `go build ./cmd/sworn/...` — compiles
- `go test ./...` — all existing tests pass

### Open items (to be verified externally)
- AC1/AC2 (sworn init consent) will be verified when T3/S09 lands the init flow wiring
- api.sworn.sh/v1/events backend is not yet live — telemetry silently drops on connection refused
- TestFireSchema uses HTTP transport rewriting (not httptest on the real URL) which is correct for client-side verification