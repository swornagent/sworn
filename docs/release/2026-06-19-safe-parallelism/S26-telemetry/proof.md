# Proof Bundle: S26-telemetry

## Scope

During sworn init, the user is asked a single consent question for anonymous usage telemetry. Users manage consent post-init via `sworn telemetry on|off|status`. After opting in, every sworn invocation that is not a telemetry meta-command fires a non-blocking telemetry event to api.sworn.sh/v1/events. Telemetry is always non-blocking, silently drops on error, and never collects code, paths, or user identity.

## Files changed

```
$ git diff --name-only 659332370db3e763f1cb4457d89ae28e52dc5215 HEAD
cmd/sworn/main.go
cmd/sworn/telemetry.go
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/journal.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/spec.md
docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/status.json
docs/release/2026-06-19-safe-parallelism/S26-telemetry/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S26-telemetry/design.md
docs/release/2026-06-19-safe-parallelism/S26-telemetry/journal.md
docs/release/2026-06-19-safe-parallelism/S26-telemetry/proof.md
docs/release/2026-06-19-safe-parallelism/S26-telemetry/status.json
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/journal.md
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/spec.md
docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/status.json
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/journal.md
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/spec.md
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/prompt/implementer.md
internal/telemetry/telemetry.go
internal/telemetry/telemetry_test.go
```

Note: 12 of the 21 files are planning artefacts that entered the T9 track via forward-merges from `release-wt/2026-06-19-safe-parallelism` — not from T9 implementation work. See "Divergence from plan" for group explanations.

The tracked binary `sworn` that was committed in the original implementation was removed via `git rm --cached sworn` (verified: `git ls-files sworn` returns empty). `.gitignore` has `/sworn` to prevent re-addition.

## Test results

### Go

```
$ go test -race ./internal/telemetry/...
ok  	github.com/swornagent/sworn/internal/telemetry	1.227s
```

All 19 tests pass with no data races:

- TestIsEnabled_EnvVar — PASS
- TestIsEnabled_Sentinel — PASS
- TestIsEnabled_Neither — PASS (newly added per Gate 3 violation fix)
- TestIsEnabled_OptedIn_NoOverrides — PASS
- TestInstallIDIdempotent — PASS
- TestInstallIDWriteFailure — PASS
- TestFireSchema — PASS
- TestFireNonBlocking — PASS (threshold tightened to 10ms per AC8)
- TestFireSilentOnError — PASS
- TestFireTelemetryMetaCommandExcluded — PASS
- TestShowDisclosure_FirstRun — PASS
- TestShowDisclosure_SubsequentRun — PASS
- TestShowDisclosure_NeutralPrecondition — PASS
- TestShowDisclosure_OptedOutPrecondition — PASS
- TestShowConsent_Yes — PASS
- TestShowConsent_No — PASS
- TestShowConsent_Enter — PASS
- TestShowConsent_NoLong — PASS
- TestTrimGoVersion — PASS

### Build

```
$ go build ./cmd/sworn/...
(compiles successfully, no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-19-safe-parallelism/S26-telemetry/proof.md` (this document)
- **User gesture**: Run `rm -f ~/.config/sworn/.telemetry-disclosed && sworn version` against a clean config dir (no `.telemetry-enabled` or `.no-telemetry` present). The one-time disclosure text appears on stderr before the version output.

## Delivered

- AC3 — After opting in via `sworn init` (or `sworn telemetry on`), the next `sworn run` fires a telemetry event; after opting out, no event fires. Evidence: `TestIsEnabled_OptedIn_NoOverrides` (enabled when .telemetry-enabled exists), `TestIsEnabled_Sentinel` (disabled when .no-telemetry exists), `TestIsEnabled_EnvVar` (disabled when SWORN_NO_TELEMETRY=1).
- AC4 — `sworn telemetry on` creates `~/.config/sworn/.telemetry-enabled` and removes `.no-telemetry` if present; `sworn telemetry off` does the reverse. Evidence: `cmd/sworn/telemetry.go` -> `telemetryOn()` and `telemetryOff()` functions.
- AC5 — `sworn telemetry status` prints `telemetry: enabled` or `telemetry: disabled` and the mechanism. Evidence: `cmd/sworn/telemetry.go` -> `telemetryStatus()`.
- AC6 — `SWORN_NO_TELEMETRY=1 sworn run --task "x"` completes without firing any HTTP request even when `.telemetry-enabled` exists. Evidence: `TestIsEnabled_EnvVar` (env var wins).
- AC7 — A successful telemetry event POSTed to `httptest.NewServer` contains exactly the fields in the schema and no others. Evidence: `TestFireSchema` (validates exact field set via JSON marshal/unmarshal of event struct + full HTTP flow validation).
- AC8 — `sworn run` exits within 10ms of the run completing regardless of whether the telemetry endpoint is reachable (non-blocking confirmed). Evidence: `TestFireNonBlocking` (5s server sleep, Fire() returns in <10ms).
- AC9 — `install-id` file contains a valid UUIDv4; running `sworn` twice produces the same install-id. Evidence: `TestInstallIDIdempotent` (same UUID on repeated calls, file written once).
- AC10 — If `~/.config/sworn/` cannot be created, sworn runs normally and telemetry fires with `install_id: ""`; no panic. Evidence: `TestInstallIDWriteFailure` (returns "" without panic).
- AC11 — `go test -race ./internal/telemetry/...` passes. Evidence: full test suite above.
- AC (new, implicit from Gate 3 fix) — `TestIsEnabled_Neither`: no sentinel files exist -> `IsEnabled()` returns false (telemetry disabled until init runs). Evidence: `TestIsEnabled_Neither`.

## Not delivered

- AC1 — `sworn init` (interactive) presents the telemetry consent question as its final step. **Why**: `cmd/sworn/init.go` is owned by T3/S09. S26 ships `ShowConsent()` as a callable function; the init-flow wiring is a cross-track dependency. **Tracking**: S09-per-role-model-config planned_files includes cmd/sworn/init.go. **Acknowledged**: Coach review 2026-06-21.
- AC2 — `sworn init --non-interactive` skips the consent question and creates `~/.config/sworn/.no-telemetry` (defaults to off). **Why**: Same as AC1 — init.go is owned by T3/S09. **Tracking**: S09-per-role-model-config. **Acknowledged**: Coach review 2026-06-21.

## Divergence from plan

- **Config path**: Changed from `os.UserConfigDir()` (design $2.2, initial proposal) to hardcoded `~/.config/sworn/` per Coach Pin 5 option (a), matching spec ACs exactly. Updated in implementation.
- **Fire() signature**: Added `swornVersion string` parameter (Coach Pin 8 option (a)) — `Fire(cmd, sub, swornVersion string, durationMS int64, exitCode int)` — so the caller in `cmd/sworn/main.go` passes the build-time version, avoiding circular imports.
- **Meta-command exclusion**: Coach Pin 4 option (a) — only `sworn telemetry *` is excluded from firing; `sworn version` and `sworn help` still fire (spec was silent on this).
- **TestFireNonBlocking threshold**: Tightened from 100ms to 10ms to match spec AC8 ("sworn run exits within 10ms"). The spec's Required Tests section originally said <100ms; the AC is binding. The goroutine launch overhead is well under 1ms in practice, so 10ms is a generous margin.

### Out-of-scope files on T9 branch (from forward-merges — not T9 implementation work)

The following files appear in `git diff --name-only start_commit..HEAD` but are **not** part of S26-telemetry's implementation. They entered the T9-telemetry track branch via standard forward-merges from `release-wt/2026-06-19-safe-parallelism` (planner/replan activity on the release board). Any track that forward-merges from release-wt during a concurrent replan window will accumulate similar planning-artefact entries in its diff — this is expected track-mode behaviour.

- **S21-canonical-baton planning artefacts** (`docs/release/2026-06-19-safe-parallelism/S21-canonical-baton/*`): Entered via forward-merge commit b97e925 ("merge: sync release-wt replan (S21 re-scope to 10 rules + S27 public-readiness gate)"). The replan commit d4f886b authored these files on release-wt; the merge propagated them to all active tracks.
- **S27-public-readiness-scrub planning artefacts** (`docs/release/2026-06-19-safe-parallelism/S27-public-readiness-scrub/*`): Same forward-merge — S27 was added as a new slice in the same replan commit.
- **S28-git-dir-guard planning artefacts** (`docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/*`): Entered via forward-merge commit cea048f (added T11 structural fix for sworn#6).
- **`docs/release/2026-06-19-safe-parallelism/index.md`**: Updated by every replan forward-merge — tracks the release board state. Expected to differ in every active track's diff.
- **`internal/prompt/implementer.md`** and **`internal/adopt/baton/rules/10-customer-journey-validation.md`**: Modified by commit 5139882 ("docs(rules): tie no-mock boundary to Rule 10 as its enforcement"), which was picked up from the integration base `release/v0.1.0` via the base sync merge 32b9054 ("sync base release/v0.1.0 — pick up no-mock->Rule10 reconciliation (5139882) before S21 re-scope"). This commit reconciles a Rule 10 documentation inconsistency and is owned cross-project (not by any single track). It arrived on the T9 branch as a mechanical base sync, not as slice implementation work.
- **`docs/release/2026-06-19-safe-parallelism/S26-telemetry/approved-ack.md`**: Deleted by commit 1729f7b ("drop approved-ack — Gate 1/2/6 FAIL requires design re-review"). The verifier's FAIL verdict caused the design-review artefacts to be dropped; when a subsequent session re-enters implementation with a fresh design review, a new approved-ack.md will be created. This deletion is part of the normal verify->fail->fix loop.

## First-pass script output

```
$ ~/.claude/bin/release-verify.sh S26-telemetry 2026-06-19-safe-parallelism
...
FIRST-PASS: 22/23 checks PASS, 1 FAIL (expected — state is in_progress)
```

Note: The single FAIL is `state is in_progress` — the script correctly refuses to verify a slice that is not yet `implemented`. The state transition happens after first-pass. All structural/documentation checks pass, including the proof.md diff count (21 files) matching the actual start_commit diff (21 files). No dark-code markers. No integration drift. All sections present and correct.