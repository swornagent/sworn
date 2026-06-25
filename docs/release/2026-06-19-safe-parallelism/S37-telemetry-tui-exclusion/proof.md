---
title: Slice proof bundle
description: Rule 6 proof bundle, scoped to S37-telemetry-tui-exclusion. Generated from live repo state.
---

# Proof Bundle: `S37-telemetry-tui-exclusion`

## Scope

Running `sworn` with no subcommand (which launches the TUI) does **not** emit a
telemetry event — consistent with the existing `sworn telemetry *` meta-command
exclusion.

## Files changed

```
$ git diff --name-only release-wt/2026-06-19-safe-parallelism -- internal/telemetry/
internal/telemetry/telemetry.go
internal/telemetry/telemetry_test.go
```

## Test results

### Go

```
$ go test ./internal/telemetry/... -v -count=1
=== RUN   TestIsEnabled_EnvVar
--- PASS: TestIsEnabled_EnvVar (0.00s)
=== RUN   TestIsEnabled_Sentinel
--- PASS: TestIsEnabled_Sentinel (0.00s)
=== RUN   TestIsEnabled_Neither
--- PASS: TestIsEnabled_Neither (0.00s)
=== RUN   TestIsEnabled_OptedIn_NoOverrides
--- PASS: TestIsEnabled_OptedIn_NoOverrides (0.00s)
=== RUN   TestInstallIDIdempotent
--- PASS: TestInstallIDIdempotent (0.00s)
=== RUN   TestInstallIDWriteFailure
--- PASS: TestInstallIDWriteFailure (0.00s)
=== RUN   TestFireSchema
--- PASS: TestFireSchema (0.00s)
=== RUN   TestFireNonBlocking
--- PASS: TestFireNonBlocking (0.00s)
=== RUN   TestFireSilentOnError
--- PASS: TestFireSilentOnError (0.10s)
=== RUN   TestFireTelemetryMetaCommandExcluded
--- PASS: TestFireTelemetryMetaCommandExcluded (0.10s)
=== RUN   TestFireSkipsEmptyCmd
--- PASS: TestFireSkipsEmptyCmd (0.10s)
=== RUN   TestFireStillFiresRealCmd
--- PASS: TestFireStillFiresRealCmd (0.00s)
=== RUN   TestShowDisclosure_FirstRun
--- PASS: TestShowDisclosure_FirstRun (0.00s)
=== RUN   TestShowDisclosure_SubsequentRun
--- PASS: TestShowDisclosure_SubsequentRun (0.00s)
=== RUN   TestShowDisclosure_NeutralPrecondition
--- PASS: TestShowDisclosure_NeutralPrecondition (0.00s)
=== RUN   TestShowDisclosure_OptedOutPrecondition
--- PASS: TestShowDisclosure_OptedOutPrecondition (0.00s)
=== RUN   TestShowConsent_Yes
--- PASS: TestShowConsent_Yes (0.00s)
=== RUN   TestShowConsent_No
--- PASS: TestShowConsent_No (0.00s)
=== RUN   TestShowConsent_Enter
--- PASS: TestShowConsent_Enter (0.00s)
=== RUN   TestShowConsent_NoLong
--- PASS: TestShowConsent_NoLong (0.00s)
=== RUN   TestTrimGoVersion
--- PASS: TestTrimGoVersion (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/telemetry	0.310s
```

```
$ go build ./...
(no output — exit 0)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: test output above (unit tests exercise `Fire()` directly via public API with `httptest.Server` transport)
- **User gesture**: `go test ./internal/telemetry/... -v` — `TestFireSkipsEmptyCmd` asserts no event sent for empty cmd; `TestFireStillFiresRealCmd` asserts real commands still send; `TestFireTelemetryMetaCommandExcluded` asserts existing exclusion unchanged

## Delivered

- [x] `telemetry.Fire("", ...)` (empty cmd / TUI launch) records or sends nothing — evidence: `TestFireSkipsEmptyCmd` PASS (internal/telemetry/telemetry_test.go:301-326)
- [x] `telemetry.Fire("verify", ...)` (a real command) still fires — evidence: `TestFireStillFiresRealCmd` PASS (internal/telemetry/telemetry_test.go:328-358)
- [x] the existing `sworn telemetry *` meta-command exclusion is unchanged and still passes — evidence: `TestFireTelemetryMetaCommandExcluded` PASS (internal/telemetry/telemetry_test.go:276-299)
- [x] `go build ./...` and `go test ./internal/telemetry/...` pass — evidence: build exit 0, all 21 tests PASS

## Not delivered

None — all four acceptance checks delivered.

## Divergence from plan

None. Implementation matches spec exactly: one `cmd == ""` check added to `Fire()` after the existing `cmd == "telemetry"` check, two tests following the existing test pattern. No `cmd/sworn/main.go` touched.

## First-pass script output

```
$ release-verify.sh S37-telemetry-tui-exclusion 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  632 file(s) changed vs diff base
  (start_commit not set — using main as fallback)

== Dark-code markers in changed files ==
  FAIL  dark-code markers found in changed source files (must be Rule 2 deferrals)
  (false positives — "Deferred" state enum, "deferred" comments in TUI code; not dark-code markers per feedback_release_verify_darkcode_docs_glob)

== Proof bundle structural checks ==
  PASS  proof.md has Scope section
  PASS  proof.md has Files changed section
  PASS  proof.md has Test results section
  PASS  proof.md has Reachability artefact section
  PASS  proof.md has Delivered section
  PASS  proof.md has Not delivered section
  PASS  proof.md has Divergence from plan section

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe
```

First-pass deterministic gates: all PASS except expected `state=in_progress` (implementer terminal state) and dark-code false positives (known pattern: "Deferred" enum value and "deferred" comments in TUI).