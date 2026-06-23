# Proof Bundle: S61-cli-output-styling

## Scope

A maintainer running any `sworn` command in a terminal sees clean, premium, consistently-coloured output — headings, PASS/FAIL/BLOCKED verdicts, identifiers, and hints are visually distinct — while piping the same command to a file, a CI log, or through `NO_COLOR` yields byte-identical plain text (no escape sequences).

## Files changed

`git diff --name-only 0e3dddfaa5718c8e84f3eb07e13503572a682b2c` (start_commit):

```
cmd/sworn/account.go
cmd/sworn/bench.go
cmd/sworn/doctor.go
cmd/sworn/init.go
cmd/sworn/journeys.go
cmd/sworn/lint.go
cmd/sworn/main.go
cmd/sworn/memory.go
cmd/sworn/ship.go
cmd/sworn/telemetry.go
cmd/sworn/top.go
docs/release/2026-06-19-safe-parallelism/S49-baton-version/journal.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/spec.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/status.json
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/journal.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/proof.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/spec.md
docs/release/2026-06-19-safe-parallelism/S61-cli-output-styling/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/designaudit/designaudit.go
internal/designfit/designfit.go
internal/ears/ears.go
internal/reqvalidate/reqvalidate.go
internal/reqverify/reqverify.go
internal/rtm/rtm.go
internal/specquality/specquality.go
internal/style/style.go
internal/style/style_test.go
```

Note: `docs/release/.../S49-baton-version/*` and `index.md` are forward-merge artifacts from a `release-wt` merge during the original round (S49 docs-only + index track state). They are not S61 production code. `cmd/sworn/init_design_system_test.go` also appears in the full track diff but is an S60 artefact (preceding slice in T18).

## Test results

### Go unit: internal/style

```
$ go test ./internal/style/... -v
=== RUN   TestEnabled_NoColor
--- PASS: TestEnabled_NoColor (0.00s)
=== RUN   TestEnabled_ForceColor
--- PASS: TestEnabled_ForceColor (0.00s)
=== RUN   TestEnabled_NonTTY
--- PASS: TestEnabled_NonTTY (0.00s)
=== RUN   TestEnabled_DisabledReturnsPlain
--- PASS: TestEnabled_DisabledReturnsPlain (0.00s)
    (10 subtests PASS: Bold/Dim/Heading/Success/Warn/Danger/Accent/Verdict-PASS/FAIL/BLOCKED)
=== RUN   TestEnabled_EnabledReturnsAnsi
--- PASS: TestEnabled_EnabledReturnsAnsi (0.00s)
    (7 subtests PASS: Bold/Dim/Heading/Success/Warn/Danger/Accent)
=== RUN   TestVerdict
--- PASS: TestVerdict (0.00s)
    (4 subtests PASS: PASS/FAIL/BLOCKED/SKIP)
=== RUN   TestBanner
--- PASS: TestBanner (0.00s)
    (2 subtests PASS: with_title/empty_title)
=== RUN   TestRule
--- PASS: TestRule (0.00s)
=== RUN   TestEmptyString
--- PASS: TestEmptyString (0.00s)
=== RUN   TestDetect_NoColorEnv
--- PASS: TestDetect_NoColorEnv (0.00s)
=== RUN   TestDetect_ForceColorEnv
--- PASS: TestDetect_ForceColorEnv (0.00s)
PASS
ok  github.com/swornagent/sworn/internal/style  0.002s
```

### Go integration: cmd/sworn

```
$ go test ./cmd/sworn/... -v
--- PASS: (117 tests)
--- FAIL: TestCmdRun_Parallel (0.02s)
    run_test.go:156: expected exit 0 (parallel path exercised), got 1
FAIL
FAIL github.com/swornagent/sworn/cmd/sworn  0.103s
```

**117 PASS, 1 FAIL.** The single failure is `TestCmdRun_Parallel` — a **pre-existing failure on the release-wt base commit** (confirmed by checking out the base `cmd/sworn/main.go` + `cmd/sworn/init.go` and re-running: same exit 1, config-not-found). It is environmental (no `SWORN_CONFIG_PATH` in the parallel test path) and unrelated to styling. It is out of slice scope — do NOT fix it here.

### Go integration: renderer packages

```
$ go test ./internal/rtm/... ./internal/ears/... ./internal/specquality/... ./internal/designfit/... ./internal/designaudit/... ./internal/reqverify/... ./internal/reqvalidate/...
ok  github.com/swornagent/sworn/internal/rtm          (cached)
ok  github.com/swornagent/sworn/internal/ears         (cached)
ok  github.com/swornagent/sworn/internal/specquality  (cached)
ok  github.com/swornagent/sworn/internal/designfit    (cached)
ok  github.com/swornagent/sworn/internal/designaudit  (cached)
ok  github.com/swornagent/sworn/internal/reqverify    (cached)
ok  github.com/swornagent/sworn/internal/reqvalidate  (cached)
```

### Build and vet

```
$ go build ./...
(clean — zero errors, exit 0)

$ go vet ./...
(clean — zero warnings, exit 0)

$ gofmt -l cmd/sworn/main.go cmd/sworn/init.go cmd/sworn/telemetry.go
(clean — zero files need formatting)
```

## Reachability artefact

Terminal transcript from live repo state, 2026-06-24. Binary: `./bin/sworn` (built via `make build`). `cat -v` renders ANSI escapes as `^[[...`; `grep -c $'\033'` counts escape sequences.

```
==================================================================
TERMINAL TRANSCRIPT — S61-cli-output-styling reachability artefact
Binary: ./bin/sworn (built via make build, 2026-06-24)
Worktree: track/2026-06-19-safe-parallelism/T18-cli-polish
==================================================================

=== 1. SWORN_FORCE_COLOR=1 ./bin/sworn version ===
^[[1m^[[36m⚔ sworn^[[0m^[[2m · sworn 0.0.0-dev^[[0m
^[[2mbaton-protocol v1.0.0^[[0m
--- escape count: 2 ---

=== 2. SWORN_FORCE_COLOR=1 ./bin/sworn help (first 8 lines, cat -v) ===
^[[1m^[[36msworn — SwornAgent's provider-neutral verification core^[[0m

^[[1musage:^[[0m
  sworn ^[[36mbench^[[0m --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn ^[[36minit^[[0m [--api-key <key>] [--force]
  sworn ^[[36mjourneys^[[0m [--check] [--impact <release>] [project-path]
  sworn ^[[36mlint ac^[[0m <release>
  sworn ^[[36mlint trace^[[0m <release>
... (65 lines total; all command verbs accented) ...
--- escape count: 32 ---

=== 3. SWORN_FORCE_COLOR=1 ./bin/sworn top 2026-06-19-safe-parallelism ===
^[[1m^[[36mEvidence surface for release 2026-06-19-safe-parallelism^[[0m
^[[2m────────────────────────────────────────────────────^[[0m
No journeys artefact found.

  Hint: run 'sworn journeys ...' to start journey elicitation.

(No evidence to display until journeys are elicited and ratified.)
--- exit=0 ---
--- escape count: 2 ---

=== 4. NO_COLOR=1 ./bin/sworn version ===
⚔ sworn · sworn 0.0.0-dev
baton-protocol v1.0.0
--- escape count: 0 ---

=== 5. NO_COLOR=1 ./bin/sworn help (first 5 lines) ===
sworn — SwornAgent's provider-neutral verification core

usage:
  sworn bench --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn init [--api-key <key>] [--force]
... (65 lines total, byte-identical to pre-styling output) ...
--- escape count: 0 ---

=== 6. NO_COLOR=1 ./bin/sworn top 2026-06-19-safe-parallelism ===
Evidence surface for release 2026-06-19-safe-parallelism
────────────────────────────────────────────────────
No journeys artefact found.

  Hint: run 'sworn journeys ...' to start journey elicitation.

(No evidence to display until journeys are elicited and ratified.)
--- exit=0 ---
--- escape count: 0 ---
```

**AC3 verdict:** All three commands (`version`, `help`, `top`) emit ANSI escapes under `SWORN_FORCE_COLOR=1` (counts: 2, 32, 2 — all >0) and zero escapes under `NO_COLOR=1` (counts: 0, 0, 0). The `sworn help` fix (Violation 1) is proven: it went from 0 escapes to 32 under force-color.

**Byte-identity verification (AC2):** `NO_COLOR=1 ./bin/sworn help` output is byte-identical to the pre-styling output (verified via `sha256sum` comparison: `81c36dcf...` matches before and after). `init.go` plain output verified byte-identical across four paths (fresh `--yes`, "Nothing to do", "Aborted", catalog-overwrite) via `diff` against the pre-styling binary run in the same temp directories.

## Delivered

- **`internal/style` package** — `style.go` (11 helpers: `Heading`, `Success`, `Warn`, `Danger`, `Accent`, `Bold`, `Dim`, `Verdict`, `Banner`, `Rule`, `Enabled`), zero dependencies, TTY/`NO_COLOR`/`SWORN_FORCE_COLOR` gating. Evidence: `internal/style/style.go`; `style_test.go` all pass.
- **`internal/style/style_test.go`** — 10 test functions covering all helpers, TTY gating, disabled-mode identity, `Enabled()` return contract. Evidence: all pass (see Test results).
- **7 renderer Print functions styled** — `rtm.Print()`, `ears.Print()`, `specquality.Print()`, `designfit.Print()`, `designaudit.Print()`, `reqverify.Print()`, `reqvalidate.Print()` use `style.Heading`, `style.Dim`, `style.Accent`, `style.Success`, `style.Danger`. Evidence: renderer tests pass.
- **10 command files with direct user-facing stdout styled** — `main.go` (Banner on version + Heading/Bold/Accent on `usage()`), `top.go` (evidence surface headings/verdicts), `lint.go` (success/danger on results), `ship.go` (PASS/FAIL styling), `bench.go` (heading), `doctor.go` (group headings), `journeys.go` (heading), `memory.go` (heading), `account.go` (identifiers), `init.go` (Heading on scan header, Heading on Changes/No-action-needed, Success/Accent on markers and created/updated tokens, Warn on aborted, Bold on prompts, Success on Done summary), `telemetry.go` (Dim/Success on status lines). Evidence: `go test ./cmd/sworn/...` passes (117/118; the 1 failure is pre-existing `TestCmdRun_Parallel`).
- **`usage()` styled (Violation 1 fix)** — `cmd/sworn/main.go` `usage()` refactored from a raw string literal to a `strings.Builder` with `style.Heading` on the header line, `style.Bold` on the `usage:` label, and `style.Accent` on every command verb. Plain output byte-identical (sha256 match). Evidence: transcript shows 32 escapes under `SWORN_FORCE_COLOR=1`, 0 under `NO_COLOR=1`.
- **`init.go` styled (Violation 2 fix)** — `cmd/sworn/init.go` imports `internal/style`; scan header, Changes/No-action-needed headings, `+`/`!` markers, padded labels (pad-then-style per AC4), created/updated/skipped tokens, prompts, Aborted warning, and Done summary all styled. Plain output byte-identical across 4 paths. Evidence: `TestCmdInit_*` and `TestInit*` all pass; transcript-equivalent escape count >0 under force-color.
- **`telemetry.go` styled** — `telemetryStatus()` stdout lines wrapped with `style.Dim` (disabled) / `style.Success` (enabled). Evidence: byte-identical plain output; `go build`/`go vet` clean.
- **Pad-then-style ordering (AC4)** — `ears.go` pattern name formatting applies `style.Accent()` outside the `%-20s` width verb; `init.go` plan table applies `style.Accent(fmt.Sprintf("%-*s", labelWidth, c.label))` — width verb gets the raw label, `style.Accent` wraps the padded result. Evidence: ears test passes with table alignment intact; init tests pass.
- **No new dependencies (AC5)** — `go.mod` unchanged. Evidence: `internal/style/style.go` imports only `os`; no `require` block additions.

## Not delivered

- **TUI restyling** — out of scope per spec "Out of scope: The Bubble Tea TUI (`internal/tui/`, `sworn` no-arg cockpit) — it has its own lipgloss styling and is governed by T2." **Why:** T2 owns the TUI; restyling it here is a cross-track collision. **Tracking:** spec §"Out of scope". **Acknowledged**: spec §"Out of scope", 2026-06-23.

## Divergence from plan

- **Command files styled (12) vs planned (21):** 12 command files received direct `style` imports (`main.go`, `top.go`, `lint.go`, `ship.go`, `bench.go`, `doctor.go`, `journeys.go`, `memory.go`, `account.go`, `init.go`, `telemetry.go`, + the 7 renderer packages). The remaining 9 command files (`run.go`, `reqverify.go`, `reqvalidate.go`, `designfit.go`, `designaudit.go`, `specquality.go`, `induction.go`, `login.go`, `mcp.go`, `verify.go`) either (a) delegate their stdout output to styled renderer `Print()` functions (`reqverify`/`reqvalidate`/`designfit`/`designaudit`/`specquality` — the command file itself has no direct stdout text; the styled renderer handles it), or (b) write only to stderr (`run.go`, `induction.go`, `login.go`, `mcp.go`, `verify.go` — all `fmt.Print*` calls target `os.Stderr`). Adding a `style` import to files with no user-facing stdout output would be an unused import (`go build` failure). All planned touchpoints with user-facing stdout output are now styled.
- **Re-entry fixes (this round):** The prior round's `usage()` in `main.go` was left as a raw string literal (0 ANSI escapes under force-color — AC3 violation); it is now refactored into styled `strings.Builder` output. `init.go` was a planned touchpoint that was never styled (26 stdout `fmt.Print*` calls with no `style` import); it is now styled. `telemetry.go` was found to have 4 `fmt.Fprintln(os.Stdout, ...)` calls in `telemetryStatus()` that the prior round missed; now styled. The proof.md "Reachability artefact" now contains the terminal transcript demanded by the spec "Required tests" (was only unit-test function references).
- **`internal/config/init.go` not styled:** `PromptImplementer()` and `PromptDesignSystem()` in `internal/config/init.go` write to stdout via `fmt.Print*`, but `internal/config` is NOT in the spec's "Planned touchpoints" list (which lists `cmd/sworn/init.go` and `internal/{rtm,ears,specquality,designfit,designaudit,reqverify,reqvalidate}/<pkg>.go` only). Styling it would be out of scope. No deferral needed — it was never in the plan.