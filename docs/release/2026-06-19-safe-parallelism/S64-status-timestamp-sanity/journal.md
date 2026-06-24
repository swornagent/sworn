# Journal — S64-status-timestamp-sanity

## 2026-06-25 — planned

- Added after the release board showed future-dated status metadata on the live board.
- Scope intentionally Sworn-only: this is a deterministic lint/doctor implementation hygiene gate, not an upstream Baton protocol change.
- Track placement: new `T19-status-hygiene`, depending on merged `T4-mcp`, `T12-harness-hardening`, and `T15-cli-registry`.

## 2026-07-15 — implemented

### Decisions

- **Clock injection:** `lint.Clock` interface with `Now() time.Time`. Tests use `fixedClock` pinned to `2026-06-25T12:00:00Z`; production uses `DefaultClock` (real wall clock). All time-sensitive test cases are deterministic.
- **JSON field extraction:** Lightweight string-scan (`extractJSONField`) rather than `encoding/json` unmarshal. This surfaces unparsable values exactly as they appear in the JSON file, which is the key requirement for diagnostic error messages naming the offending field+value.
- **Lint target name:** `status` (not `status-time` or `timestamps`). Short, discoverable; the usage string clarifies it checks timestamps.
- **Doctor group:** Added as Group 2b ("Release status timestamp sanity") between existing Group 2 (Repo artifact audit) and Group 3 (Local Baton sync). Groups repository-artefact hygiene checks together.
- **Doctor scanning scope:** Walks all directories under `docs/release/`, checking each release independently. A summary result with total violation count precedes per-slice detail lines.
- **Boundary:** `maxFutureSkew = 5 * time.Minute`. Timestamps at exactly `now+5m` pass (inclusive); `now+5m+1s` fails. Tests confirm both edges.

### Trade-offs

- **String-scan JSON extraction vs full unmarshal:** The extractor handles only the specific fields we check (`last_updated_at`, `verification.verifier_verdict_at`). If status.json format changes significantly, the extractor may need updating. This is acceptable because (a) the fields are stable Baton protocol fields, (b) the test suite covers both present and absent cases, and (c) a false-negative (extractor returning "") means the field is silently skipped, which is safe — it means no violation, not a missed violation that would fail open.
- **Doctor scans ALL releases:** Potentially slow for repos with many historical releases. Acceptable because `sworn doctor` is a human-initiated health check, not a hot-path operation. The walk is O(releases × slices) and each status.json is a few hundred bytes.
- **No auto-repair (`--fix`):** As specified in "Out of scope." The gate reports defects; repair stays human-owned. Adding `--fix` would require rewriting status.json files, which crosses a trust boundary this slice intentionally preserves.

### Pre-existing conditions

- The live repo contains future-dated status metadata (the motivation for this slice). Running `sworn doctor` on the live repo will report these as `[ERROR]` — this is the intended behaviour. The defect exists today and should be explicit.
## Verifier verdicts received

### 2026-06-24T15:25:19Z — PASS (verifier)

All six gates passed.

- Gate 1: User-reachable outcome exists — `sworn lint status <release>` and `sworn doctor` wired and exercised by command tests.
- Gate 2: Planned touchpoints match actual changed files — exact match to spec (internal/lint/status_time*.go, cmd/sworn/lint*.go, cmd/sworn/doctor*.go).
- Gate 3: Required tests exist and exercise the integration point — table tests + Rule 1 command-level tests (`TestLintStatusCmd_*`, `TestDoctorStatusTimestamps*`) re-run and green.
- Gate 4: Reachability artefact proves the user path — command tests drive `cmdLintStatus` and doctor groups directly.
- Gate 5: No silent deferrals or placeholder logic — no TODO/FIXME/deferred in changed Go files.
- Gate 6: Claimed scope matches implemented scope — all "Delivered" items have evidence in tests and code.

Tests re-run: `go test -race ./internal/lint/... ./cmd/sworn/...` and `go build ./...` — PASS.

Verified against: bb4ea79 (implementation commit)
