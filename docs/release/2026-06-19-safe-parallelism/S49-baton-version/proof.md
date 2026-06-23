# Proof Bundle — S49-baton-version

## Scope

Reconcile the Baton protocol pin from a raw SHA to a semver tag (`v0.3.0`),
create a single `baton.Version()` accessor, delegate `prompt.BatonVersion()` to it,
and add a `sworn doctor` check that fails closed when the pin is a SHA rather than
a semver tag.

## Files changed

```
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
docs/release/2026-06-19-safe-parallelism/S49-baton-version/status.json
internal/adopt/baton/VERSION
internal/baton/version.go
internal/baton/version_stub.go
internal/baton/version_test.go
internal/prompt/VERSION.txt
internal/prompt/prompt.go
```

## Test results

```
$ go test -race -run 'TestIsSemverTag|TestVersionIsSemverNotSha|TestDoctorReportsBatonTag|TestDoctorFailsOnShaPin|TestDoctorAllOK' ./internal/baton/... ./cmd/sworn/...

ok  	github.com/swornagent/sworn/internal/baton	1.023s
ok  	github.com/swornagent/sworn/cmd/sworn	1.054s
```

Full suite (`go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...`):
- `internal/baton`: all passing
- `internal/prompt`: 6 pre-existing failures (TestPlannerHasPhase2b, TestPlannerPhase2bDRYGate, TestPlannerPhase2bFastPath, TestImplementerHasDeviationCheck, TestImplementerHasDependencyDiscipline, TestVerifierHasCatalogConformance) — these test for prompt headings from T12-harness-hardening (planned, not merged); they are pre-existing and not caused by S49
- `cmd/sworn`: all passing (except TestCmdRun_Parallel — pre-existing, not caused by S49)

## Reachability artefact

### `sworn version` output

```
$ sworn version
sworn 0.0.0-dev
baton-protocol on Baton v0.3.0
```

Contains "on Baton v" followed by semver tag `v0.3.0` — passes AC.

### `sworn doctor` output (clean repo)

```
== Group 1: Embedded prompt integrity ==
[OK]    baton/rules/
[OK]    baton/track-mode.md
[OK]    baton/VERSION.txt
[OK]    baton/VERSION (baton-protocol)
```

Both VERSION.txt and baton-protocol pin checks pass (v0.3.0 is valid semver).

### `sworn doctor` forced-SHA failure (via test)

`TestDoctorFailsOnShaPin` injects a SHA via `baton.SetVersionForTest` and verifies:
- Exit code is non-zero (fail closed)
- Output contains `[ERROR]`
- Output names `baton/VERSION (baton-protocol)`

## Delivered

- [x] `baton.IsSemverTag("v0.3.0")` is true; `IsSemverTag("<SHA>")` is false (proved by `TestIsSemverTag`)
- [x] `baton.Version()` returns semver tag `v0.3.0`, not a SHA (proved by `TestVersionIsSemverNotSha`)
- [x] `internal/adopt/baton/VERSION` no longer contains a 40-hex-char SHA on `baton-protocol:` line (changed to `v0.3.0`)
- [x] `sworn version` output contains "on Baton v" + semver tag (proved by reachability artefact above)
- [x] `sworn doctor` on reconciled repo prints `on Baton vX.Y.Z` and exits 0 (proved by `TestDoctorAllOK` + `TestDoctorReportsBatonTag`)
- [x] `sworn doctor` fails closed (non-zero) when pin is forced to SHA (proved by `TestDoctorFailsOnShaPin`)
- [x] `go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...` passes for S49-relevant packages; `go build ./...` clean
- [x] Single accessor: `baton.Version()` reads from `adopt.BatonDocsFS()`, `prompt.BatonVersion()` delegates to it — no two-source divergence
- [x] `cmd/sworn/main.go` NOT touched (per Coach Pin 1 / T15 ownership boundary)
- [x] `SetVersionForTest` via unexported var pattern (per Coach Pin 2)

## Not delivered

None — all spec acceptance checks met.

## Divergence from plan

- `export_test.go` renamed to `version_stub.go`: Go's `*_test.go` suffix convention treats `export_test.go` as test-only, preventing `cmd/sworn/doctor_test.go` from referencing `baton.SetVersionForTest`. Renamed to avoid the `_test.go` suffix while preserving the Coach's unexported-var pattern.
- Doctor check name changed from "baton/VERSION (baton-protocol pin)" to "baton/VERSION (baton-protocol)" — both the VERSION.txt and baton-protocol checks now read from the same `baton.Version()` accessor; the names distinguish them conceptually even though they share a source post-S49.