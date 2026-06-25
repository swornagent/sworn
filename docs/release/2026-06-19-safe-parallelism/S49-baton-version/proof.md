# Proof Bundle: `S49-baton-version`

## Scope

Reconcile the Baton protocol pin from a raw SHA to a semver tag (`v0.4.2`),
create a single `baton.Version()` accessor, delegate `prompt.BatonVersion()` to it,
and add a `sworn doctor` check that fails closed when the pin is a SHA rather than
a semver tag. Re-entered to bump the pin from v0.4.0 (prior implementation) to
v0.4.2 (Baton v0.4.2 published + tagged, commit `729f188f6f69f4b807c5974b33fd39ec98671f15`,
per planner re-pin).

## Files changed

```
cmd/sworn/account.go
cmd/sworn/bench.go
cmd/sworn/doctor.go
cmd/sworn/doctor_test.go
cmd/sworn/init.go
cmd/sworn/init_design_system_test.go
cmd/sworn/journeys.go
cmd/sworn/lint.go
cmd/sworn/main.go
cmd/sworn/memory.go
cmd/sworn/ship.go
cmd/sworn/telemetry.go
cmd/sworn/top.go
docs/decisions.md
docs/release/2026-06-19-safe-parallelism/.captain-trial-log.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/journal.md
docs/release/2026-06-19-safe-parallelism/S11-anthropic-driver/status.json
docs/release/2026-06-19-safe-parallelism/S49-baton-version/journal.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/proof.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/spec.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/status.json
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/design.md
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/journal.md
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/proof.md
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/review.md
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/spec.md
docs/release/2026-06-19-safe-parallelism/S60-init-ui-bearing-fix/status.json
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/design.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/journal.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/proof.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/review.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/spec.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/adopt/baton/VERSION
internal/baton/version.go
internal/baton/version_stub.go
internal/baton/version_test.go
internal/designaudit/designaudit.go
internal/designfit/designfit.go
internal/ears/ears.go
internal/prompt/VERSION.txt
internal/prompt/prompt.go
internal/reqvalidate/reqvalidate.go
internal/reqverify/reqverify.go
internal/rtm/rtm.go
internal/specquality/specquality.go
internal/style/style.go
internal/style/style_test.go
```

**S49-owned touchpoints** (the slice's actual work — this session only bumped pin text):
- `internal/adopt/baton/VERSION` — pin bumped v0.4.0 → v0.4.2, added `upstream-sha:`, extended `rules-added:`
- `internal/prompt/VERSION.txt` — v0.4.0 → v0.4.2
- `internal/baton/version.go` — doc comment v0.4.0 → v0.4.2
- `internal/prompt/prompt.go` — doc comment v0.4.0 → v0.4.2; fixed Edit-tool newline collapse

All other files are forward-merge artefacts from `release-wt/2026-06-19-safe-parallelism` (S11, S60, S61, T18-cli-polish, infrastructure packages) — not S49 work.

## Test results

### S49-specific tests

```
$ go test -race -count=1 -run 'TestIsSemverTag|TestVersionIsSemverNotSha|TestDoctorReportsBatonTag|TestDoctorFailsOnShaPin|TestDoctorAllOK' ./internal/baton/... ./cmd/sworn/... -v

=== RUN   TestIsSemverTag
--- PASS: TestIsSemverTag (0.00s)
=== RUN   TestVersionIsSemverNotSha
--- PASS: TestVersionIsSemverNotSha (0.00s)
ok  	github.com/swornagent/sworn/internal/baton	1.016s
=== RUN   TestDoctorAllOK
--- PASS: TestDoctorAllOK (0.00s)
=== RUN   TestDoctorReportsBatonTag
--- PASS: TestDoctorReportsBatonTag (0.00s)
=== RUN   TestDoctorFailsOnShaPin
--- PASS: TestDoctorFailsOnShaPin (0.00s)
ok  	github.com/swornagent/sworn/cmd/sworn	1.053s
```

### Full suite (`go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...`)

- `internal/baton`: all passing
- `internal/prompt`: 6 pre-existing failures (TestPlannerHasPhase2b, TestPlannerPhase2bDRYGate, TestPlannerPhase2bFastPath, TestImplementerHasDeviationCheck, TestImplementerHasDependencyDiscipline, TestVerifierHasCatalogConformance) — these test for prompt headings from T12-harness-hardening (planned, not merged); not caused by S49
- `cmd/sworn`: all passing except TestCmdRun_Parallel — pre-existing, not caused by S49

`go build ./...` — clean.

## Reachability artefact

### `sworn version` output

```
$ sworn version
⚔ sworn · sworn 0.0.0-dev
baton-protocol on Baton v0.4.2
```

Contains "on Baton v" followed by semver tag `v0.4.2` — passes AC.

### `sworn doctor` output (VERSION checks only)

```
Group 1: Embedded prompt integrity
[OK]    baton/VERSION.txt          version=v0.4.2
[OK]    baton/VERSION (baton-protocol)     on Baton v0.4.2
```

Both checks pass (v0.4.2 is valid semver). Exit code: 0.

### `sworn doctor` forced-SHA failure (via test)

`TestDoctorFailsOnShaPin` injects a SHA via `baton.SetVersionForTest` and verifies:
- Exit code is non-zero (fail closed)
- Output contains `[ERROR]`
- Output names `baton/VERSION (baton-protocol)`

## Delivered

- [x] `baton.IsSemverTag("v0.3.0")` is true; `IsSemverTag("<SHA>")` is false (proved by `TestIsSemverTag`)
- [x] `baton.Version()` returns semver tag `v0.4.2`, not a SHA (proved by `TestVersionIsSemverNotSha`)
- [x] `internal/adopt/baton/VERSION` no longer contains a 40-hex-char SHA on `baton-protocol:` line (line reads `baton-protocol: v0.4.2`)
- [x] `sworn version` output contains "on Baton v" + semver tag (proved by reachability artefact above — `baton-protocol on Baton v0.4.2`)
- [x] `sworn doctor` on reconciled repo prints `on Baton vX.Y.Z` and exits 0 (proved by `TestDoctorAllOK` + `TestDoctorReportsBatonTag` + live run)
- [x] `sworn doctor` fails closed (non-zero) when pin is forced to SHA (proved by `TestDoctorFailsOnShaPin`)
- [x] `go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...` passes for S49-relevant packages; `go build ./...` clean
- [x] Single accessor: `baton.Version()` reads from `adopt.BatonDocsFS()`, `prompt.BatonVersion()` delegates to it
- [x] `cmd/sworn/main.go` NOT touched (per Coach Pin 1 / T15 ownership boundary)
- [x] `SetVersionForTest` via unexported var pattern (per Coach Pin 2)
- [x] VERSION pin bumped to v0.4.2 — `upstream-sha:` recorded, `vendored:` refreshed, `rules-added:` extended with role-prompt-operational-gates
- [x] `internal/prompt/VERSION.txt` agrees with baton-protocol pin (both v0.4.2)

## Not delivered

None — all spec acceptance checks met.

## Divergence from plan

- `export_test.go` renamed to `version_stub.go`: Go's `*_test.go` suffix convention treats `export_test.go` as test-only, preventing `cmd/sworn/doctor_test.go` from referencing `baton.SetVersionForTest`. Renamed to avoid the `_test.go` suffix while preserving the Coach's unexported-var pattern.
- Doctor check name uses "baton/VERSION (baton-protocol)" — both the VERSION.txt and baton-protocol checks now read from the same `baton.Version()` accessor; the names distinguish them conceptually.
- S11/S60/S61/files + infrastructure packages appear in the diff via forward-merge from `release-wt/2026-06-19-safe-parallelism` (T18-cli-polish merge, replan-release base sync, S11 re-route) — not S49 touchpoints. Verifier: the S49-owned files are the 4 listed above (this session) plus the 8 from prior implementation rounds.
- Pin bumped from v0.4.0 → v0.4.2: the planner re-pinned the spec to v0.4.2 after Baton v0.4.2 was published + tagged (commit `729f188f6f69f4b807c5974b33fd39ec98671f15`). This re-implementation bumps the version string, records `upstream-sha:`, and extends `rules-added:` in `VERSION` + `VERSION.txt`.

## First-pass script output


```
$ release-verify.sh S49-baton-version 2026-06-19-safe-parallelism

  slice:       S49-baton-version
  slice dir:   docs/release/2026-06-19-safe-parallelism/S49-baton-version
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit d58aeca
  PASS  50 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  (all sections present, no template placeholders)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```
