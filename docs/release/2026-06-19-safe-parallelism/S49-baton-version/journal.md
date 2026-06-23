# Journal — S49-baton-version

## 2026-07-09: Implementation

### State transition: design_review → in_progress

Coach-approved design (3 pins, all addressed):
1. Dropped `cmd/sworn/main.go` from planned_files — `BatonVersion()` returns `"on Baton " + baton.Version()` so the existing `baton-protocol %s` format produces output containing "on Baton v0.3.0" without touching T15-owned main.go.
2. `SetVersionForTest` via unexported var pattern — `version_stub.go` (renamed from `export_test.go` because Go treats `*_test.go` as test-only).
3. Single accessor (baton.Version() from adopt embed) confirmed.

### Key decisions

- **Pin reconciliation**: Changed `internal/adopt/baton/VERSION` baton-protocol line from SHA `cf158423...` to `v0.3.0`, and `internal/prompt/VERSION.txt` from `v1.0.0` to `v0.3.0`.
- **`baton.Version()`**: Reads from `adopt.BatonDocsFS() → baton/VERSION`, parses `baton-protocol:` line. Returns `""` if embed missing.
- **`baton.IsSemverTag()`**: Strict `^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$` — no pre-release/build suffixes.
- **`prompt.BatonVersion()`**: Now delegates to `baton.Version()`, returns `"on Baton " + baton.Version()`.
- **Doctor checks**: Existing VERSION.txt check tightened to ERROR on non-semver; new `baton/VERSION (baton-protocol)` check added — fails closed on SHA.
- **`version_stub.go`**: Renamed from `export_test.go` because Go's `*_test.go` suffix convention makes it test-only, and `cmd/sworn/doctor_test.go` needs `baton.SetVersionForTest`.
- **`cmd/sworn/main.go` NOT touched**: Coach Pin 1 — `BatonVersion()` prefix handles the output reframing without touching T15-owned main.go.

### Pre-existing test failures (not S49-caused)

- `internal/prompt`: TestPlannerHasPhase2b, TestPlannerPhase2bDRYGate, TestPlannerPhase2bFastPath, TestImplementerHasDeviationCheck, TestImplementerHasDependencyDiscipline, TestVerifierHasCatalogConformance — these check for prompt headings from T12-harness-hardening (planned, not merged).
- `cmd/sworn`: TestCmdRun_Parallel — pre-existing.

### Deferrals

None — all spec acceptance checks met.

### Skeptic panel

Skipped — runtime does not support subagent dispatch (single-threaded API call mode).

## 2026-06-23: Design review

Captain reviewed design.md (commit 7093b0c0e4d1b28e1e8b9460ecb51588474dc9be). 3 pins:
1. Drop main.go from planned_files/design §3 (4th recurrence of Gate 2 failure pattern)
2. SetVersionForTest via export_test.go (not production code)
3. Single accessor confirmed — honours [[project_baton_sworn_architecture]]

Coach approved with CAPTAIN-VERDICT: PROCEED.