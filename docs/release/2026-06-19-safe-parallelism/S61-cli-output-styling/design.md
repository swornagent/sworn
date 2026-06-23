# Design TL;DR — S61-cli-output-styling

## §1. User-visible change

Every `sworn` command emitting text output — `sworn lint`, `sworn top`, `sworn verify`, `sworn version`, `sworn init`, and all 17 others — renders with consistent, premium colour coding. PASS tokens and success messages appear green; FAIL tokens and error counts red; warnings yellow; section headings bold-cyan; identifiers accented cyan; secondary text dimmed. These colours appear only when stdout is a real terminal and `NO_COLOR` is unset (or when `SWORN_FORCE_COLOR=1` overrides). When piped to a file, a CI log, or a non-TTY test harness, every command produces byte-identical plain text — zero escape sequences — so all existing golden-string and table-alignment tests pass unchanged.

## §2. Design decisions not in spec (max 5)

1. **Reuse `internal/style/style.go` verbatim from `wip/cli-styling-reference`.** The spec explicitly directs this. The package is clean, tested in the reference session, and needs zero changes. The only addition is `style_test.go`, which the reference branch lacks.

2. **Command-file styling follows a consistent wrapping pattern.** Every styled output wraps an existing `fmt.Print*` argument in a `style.*()` call, never rephrases the string. This guarantees byte-identical plain-text output. Example: `fmt.Printf("\n%d violation(s) found:\n", n)` → `fmt.Printf("\n%s\n", style.Danger(fmt.Sprintf("%d violation(s) found:", n)))`.

3. **`main.go` touchpoints are `usage()`, `cmdVersion()` and `cmdHelp()` only.** The registry-based dispatch (`dispatch()`) and TUI launch path have no output text to style. Do NOT touch `dispatch()` or the registry import — T15 owns that pattern.

4. **Divider runs of `=` stay `Dim`-wrapped rather than converted to `style.Rule`.** The spec's Risk section explicitly calls this out: `style.Rule()` emits `─` (box-drawing) which differs from the existing `=` characters. Using `style.Dim(strings.Repeat("=", n))` preserves byte-identical plain output.

5. **The `usage()` text block in `main.go` gets styled once at the top (Banner) and the bottom (hint line).** The large middle block of usage prose stays plain — it is reference material, not a verdict or heading. Over-styling it would degrade readability.

## §3. Files I'll touch grouped by purpose

- **`internal/style/style.go` (new)** — copied verbatim from `wip/cli-styling-reference`
- **`internal/style/style_test.go` (new)** — unit tests for all helpers, TTY/NO_COLOR/FORCE_COLOR gating, and disabled-mode identity
- **Command files (21 files)** — add `style` import and wrap output strings: `account.go`, `bench.go`, `designaudit.go`, `designfit.go`, `doctor.go`, `induction.go`, `init.go`, `journeys.go`, `lint.go`, `login.go`, `main.go` (usage/version/help only), `mcp.go`, `memory.go`, `reqvalidate.go`, `reqverify.go`, `run.go`, `ship.go`, `specquality.go`, `telemetry.go`, `top.go`, `verify.go`
- **Report renderers (7 files)** — add `style` import and wrap output strings: `internal/rtm/rtm.go`, `internal/ears/ears.go`, `internal/specquality/specquality.go`, `internal/designfit/designfit.go`, `internal/designaudit/designaudit.go`, `internal/reqverify/reqverify.go`, `internal/reqvalidate/reqvalidate.go`

## §4. Things I'm NOT doing

- **NOT restyling the TUI** (`internal/tui/`, `sworn` no-arg cockpit) — T2 owns it; explicitly out of scope.
- **NOT changing any wording, structure, or exit code** — the spec forbids it; every existing test must pass byte-identically with colour disabled.
- **NOT touching `commands.go` or `commands_test.go`** — the command registry (T15) has no output text.
- **NOT touching non-renderer internal packages** (`internal/verify/`, `internal/run/`, `internal/scheduler/`, etc.) — they output no user-facing text; they are not in the planned_files list.
- **NOT porting the stale `main.go` from `wip/cli-styling-reference`** — that predates the command registry (T15) and would regress dispatch.

## §5. Reachability plan

Terminal transcript in `proof.md` showing `SWORN_FORCE_COLOR=1 sworn version`, `SWORN_FORCE_COLOR=1 sworn help`, and `SWORN_FORCE_COLOR=1 sworn top <release>` each emitting ANSI escape sequences (verified via `grep -c $'\033'`), and the matching `NO_COLOR=1` runs showing zero escapes. Plus `go test ./cmd/sworn/... ./internal/...` green — the existing test suites exercising every styled code path through their command/renderer entry points.

## §6. Open questions for the Coach

None.