# Proof Bundle — S49-baton-version

## Scope

Reconcile the Baton protocol pin from a raw SHA to a semver tag (`v0.4.0`),
create a single `baton.Version()` accessor, delegate `prompt.BatonVersion()` to it,
and add a `sworn doctor` check that fails closed when the pin is a SHA rather than
a semver tag. Re-entered to bump the pin from v0.3.0 (original implementation) to
v0.4.0 (Baton v0.4.0 published + tagged).

## Files changed

```
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/status.json
docs/release/2026-06-19-safe-parallelism/S49-baton-version/journal.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/proof.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/spec.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/adopt/baton/VERSION
internal/baton/version.go
internal/baton/version_stub.go
internal/baton/version_test.go
internal/prompt/VERSION.txt
internal/prompt/prompt.go
```

Note: S11 files are forward-merge artefacts from a prior `/replan-release` that
amended S11's journal/status. They appear in the diff because the track branch
includes the planner's commits — not because S49 touched them.

## Test results

```
$ go test -race -run 'TestIsSemverTag|TestVersionIsSemverNotSha|TestDoctorReportsBatonTag|TestDoctorFailsOnShaPin|TestDoctorAllOK' ./internal/baton/... ./cmd/sworn/...

ok  	github.com/swornagent/sworn/internal/baton	1.017s
ok  	github.com/swornagent/sworn/cmd/sworn	1.049s
```

Full suite (`go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...`):
- `internal/baton`: all passing
- `internal/prompt`: 6 pre-existing failures (TestPlannerHasPhase2b, TestPlannerPhase2bDRYGate, TestPlannerPhase2bFastPath, TestImplementerHasDeviationCheck, TestImplementerHasDependencyDiscipline, TestVerifierHasCatalogConformance) — these test for prompt headings from T12-harness-hardening (planned, not merged); not caused by S49
- `cmd/sworn`: all passing except TestCmdRun_Parallel — pre-existing, not caused by S49

`go build ./...` — clean.

## Reachability artefact

### `sworn version` output

```
$ sworn version
sworn 0.0.0-dev
baton-protocol on Baton v0.4.0
```

Contains "on Baton v" followed by semver tag `v0.4.0` — passes AC.

### `sworn doctor` output (clean repo)

```
== Group 1: Embedded prompt integrity ==
[OK]    baton/rules/
[OK]    baton/track-mode.md
[OK]    baton/VERSION.txt
[OK]    baton/VERSION (baton-protocol)
```

Both VERSION.txt (`version=v0.4.0`) and baton-protocol pin (`on Baton v0.4.0`) pass (v0.4.0 is valid semver). 11/11 rule files present.

### `sworn doctor` forced-SHA failure (via test)

`TestDoctorFailsOnShaPin` injects a SHA via `baton.SetVersionForTest` and verifies:
- Exit code is non-zero (fail closed)
- Output contains `[ERROR]`
- Output names `baton/VERSION (baton-protocol)`

## Delivered

- [x] `baton.IsSemverTag("v0.3.0")` is true; `IsSemverTag("<SHA>")` is false (proved by `TestIsSemverTag`)
- [x] `baton.Version()` returns semver tag `v0.4.0`, not a SHA (proved by `TestVersionIsSemverNotSha`)
- [x] `internal/adopt/baton/VERSION` no longer contains a 40-hex-char SHA on `baton-protocol:` line (changed to `v0.4.0`)
- [x] `sworn version` output contains "on Baton v" + semver tag (proved by reachability artefact above — `baton-protocol on Baton v0.4.0`)
- [x] `sworn doctor` on reconciled repo prints `on Baton vX.Y.Z` and exits 0 (proved by `TestDoctorAllOK` + `TestDoctorReportsBatonTag`)
- [x] `sworn doctor` fails closed (non-zero) when pin is forced to SHA (proved by `TestDoctorFailsOnShaPin`)
- [x] `go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...` passes for S49-relevant packages; `go build ./...` clean
- [x] Single accessor: `baton.Version()` reads from `adopt.BatonDocsFS()`, `prompt.BatonVersion()` delegates to it
- [x] `cmd/sworn/main.go` NOT touched (per Coach Pin 1 / T15 ownership boundary)
- [x] `SetVersionForTest` via unexported var pattern (per Coach Pin 2)
- [x] VERSION pin bumped to v0.4.0 — vendored date updated, rules-added: 11-process-global-mutation added
- [x] `internal/prompt/VERSION.txt` agrees with baton-protocol pin (both v0.4.0)

## Not delivered

None — all spec acceptance checks met.

## Divergence from plan

- `export_test.go` renamed to `version_stub.go`: Go's `*_test.go` suffix convention treats `export_test.go` as test-only, preventing `cmd/sworn/doctor_test.go` from referencing `baton.SetVersionForTest`. Renamed to avoid the `_test.go` suffix while preserving the Coach's unexported-var pattern.
- Doctor check name uses "baton/VERSION (baton-protocol)" — both the VERSION.txt and baton-protocol checks now read from the same `baton.Version()` accessor; the names distinguish them conceptually.
- S11 files appear in the diff via forward-merge from a prior `/replan-release` — not S49 touchpoints.
- Pin bumped from v0.3.0 → v0.4.0: the original S49 implementation shipped with v0.3.0; Baton v0.4.0 was subsequently published and the planner re-routed the slice to `failed_verification` to bump the pin. This re-implementation only touches the version string, the `vendored:` date, and the `rules-added:` list in `VERSION` + `VERSION.txt`.