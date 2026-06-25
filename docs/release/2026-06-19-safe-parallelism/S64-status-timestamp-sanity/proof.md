# Proof Bundle — S64-status-timestamp-sanity

## Scope

Add a fail-closed lint/doctor gate that scans release `status.json` files and rejects future `last_updated_at` / `verification.verifier_verdict_at` timestamps beyond a 5-minute clock-skew allowance. The board should no longer be the first place a human notices impossible dates; deterministic tooling names the offending slice and field.

## Files changed

```
cmd/sworn/doctor.go           | 538 +++++++++++++++++++++++++++++++++++++++++++++-
cmd/sworn/doctor_test.go      |  81 +++++++
cmd/sworn/lint.go             | 233 +++++++++++++++++-----
cmd/sworn/lint_trace_test.go  | 134 ++++++++++++
internal/lint/status_time.go  | 221 ++++++++++++++++++++
internal/lint/status_time_test.go | 263 +++++++++++++++++++++++
```

## Test results

### `go test -race ./internal/lint/...`

```
ok  	github.com/swornagent/sworn/internal/lint	1.137s
```

All 11 table-driven test cases pass: valid past, within-skew, skew-boundary, future-beyond-skew, far-future, malformed, missing-field, valid-verdict-at, future-verdict-at, malformed-verdict-at, both-future. Plus multi-slice, empty-release, skew-edge (4m59s pass / 5m1s fail), nil-clock, and JSON-extraction tests.

### `go test -race -run "TestLintStatus|TestDoctorStatus" ./cmd/sworn/...`

```
ok  	github.com/swornagent/sworn/cmd/sworn	1.071s
```

Rule 1 reachability: all command-level tests pass, driving the actual `cmdLintStatus` and `cmdDoctor` entry points, not just the lint package.

### `go build ./...`

```
(no output — exit 0)
```

## Reachability artefact (Rule 1)

Command-level tests exercising the user-facing affordance:

- `TestLintStatusCmd_MissingReleaseArg` — `sworn lint status` exits 64
- `TestLintStatusCmd_NonexistentRelease` — exits 2
- `TestLintStatusCmd_ValidRelease` — exits 0 on clean fixture
- `TestLintStatusCmd_FutureTimestamp` — exits non-zero on future `last_updated_at`
- `TestLintStatusCmd_FutureVerdictAt` — exits non-zero on future `verifier_verdict_at`
- `TestLintStatusCmd_MalformedTimestamp` — exits non-zero on unparsable timestamp
- `TestDoctorStatusTimestamps` — `sworn doctor` reports `[ERROR]` on future timestamps
- `TestDoctorStatusTimestamps_Clean` — `sworn doctor` has no `[ERROR]` on clean data

## Delivered

- [x] Reusable status-timestamp validator under `internal/lint` → `internal/lint/status_time.go`
- [x] `last_updated_at` validated (future beyond 5m = fail; malformed = fail)
- [x] `verification.verifier_verdict_at` validated (when present; future beyond 5m = fail; malformed = fail)
- [x] Unparsable RFC3339 timestamps fail closed → `checkTimestampField` returns violation on parse error
- [x] Fixed clock in tests (`fixedClock` struct); no test depends on wall-clock time
- [x] Wired through `sworn lint status <release>` → `cmdLintStatus` in `cmd/sworn/lint.go`
- [x] Wired through `sworn doctor` → Group 2b `checkStatusTimestamps` in `cmd/sworn/doctor.go`
- [x] Error messages name release, slice id, field path (`last_updated_at` / `verification.verifier_verdict_at`), raw value, and allowed maximum → `StatusTimeViolation.String()`
- [x] `go test -race ./internal/lint/... ./cmd/sworn/...` and `go build ./...` pass

## Not delivered

*(none — all in-scope acceptance checks satisfied)*

## Divergence from plan

*(none)*

## First-pass script output

See below (will be re-run at session end).

## Design decisions

- **Clock interface:** `lint.Clock` with `Now() time.Time` — tests inject `fixedClock`, production uses `DefaultClock` (real wall clock).
- **JSON extraction:** Lightweight string-scan extraction (`extractJSONField`) rather than `encoding/json` unmarshal. This lets us surface unparsable values exactly as written in the JSON, and avoids depending on a specific struct shape for the raw field extraction.
- **Doctor group placement:** Added as Group 2b ("Release status timestamp sanity") after existing Group 2 ("Repo artifact audit"). This groups repository-artefact hygiene checks together.
- **Doctor scanning scope:** Scans all directories under `docs/release/`; each release is checked independently. A summary result with total violation count precedes per-slice detail lines.
- **5-minute skew allowance:** Accepted as specified. Timestamps exactly at `now+5m` pass (inclusive boundary).