---
title: 'Slice spec — S61-cli-output-styling'
description: 'A shared, zero-dependency ANSI style package gives sworn premium, consistent, TTY/NO_COLOR-aware coloured CLI output across every command and report renderer.'
---

# Slice: `S61-cli-output-styling`

## User outcome

A maintainer running any `sworn` command in a terminal sees clean, premium, consistently-coloured output — headings, PASS/FAIL/BLOCKED verdicts, identifiers, and hints are visually distinct — while piping the same command to a file, a CI log, or through `NO_COLOR` yields byte-identical plain text (no escape sequences).

## Entry point

Every `sworn` subcommand's stdout/stderr rendering: the command files in `cmd/sworn/*.go` and the report renderers they delegate to in `internal/*/…Print()`. User gesture: invoking any `sworn` verb at a shell.

## In scope

- A new shared package `internal/style` (package `style`): a hand-rolled ANSI palette with semantic helpers (`Heading`, `Success`, `Warn`, `Danger`, `Accent`, `Bold`, `Dim`, `Verdict`, `Rule`, `Banner`, `Enabled`). Zero new dependencies — stdlib only (Rule 1 / ADR-free).
- Colour is enabled only when stdout is a TTY and `NO_COLOR` is unset; `SWORN_FORCE_COLOR` overrides the TTY check. In a pipe / file / CI / test harness, every helper returns plain text.
- Apply the palette across the CLI command surface and the delegated report renderers, with a consistent vocabulary (verdict tokens green/red/yellow; headings; identifiers accented; hints dimmed).
- Plain-text (colour-disabled) output stays byte-identical to current output, so every existing golden/output test passes unchanged.

## Out of scope

- The Bubble Tea TUI (`internal/tui/`, `sworn` no-arg cockpit) — it has its own lipgloss styling and is governed by T2; not restyled here.
- Any wording, structure, or exit-code change to command output.
- `cmd/sworn/commands.go` registry wiring and `cmd/sworn/main.go` dispatch logic (only `usage()` / `version` *presentation* is styled).

## Planned touchpoints

- `internal/style/style.go` (new), `internal/style/style_test.go` (new)
- Command surface: `cmd/sworn/{init,top,journeys,ship,lint,bench,run,main,reqverify,reqvalidate,designfit,designaudit,specquality,account,doctor,induction,login,mcp,memory,telemetry,verify}.go`
- Report renderers: `internal/{rtm,ears,specquality,designfit,designaudit,reqverify,reqvalidate}/<pkg>.go`

> Implementer note: the command surface is larger on release-wt than the original
> ad-hoc session covered (account/doctor/induction/login/mcp/memory/telemetry/verify
> were added by later tracks). All are in scope. A reference diff lives on branch
> `wip/cli-styling-reference` (init/top/journeys/ship/lint/bench/run/main + the
> renderers + the `internal/style` package) — reuse `internal/style` verbatim and
> extend the command coverage; do not port the stale main.go (it predates the
> command registry).

## Acceptance checks

- [ ] `internal/style` exposes `Heading`, `Success`, `Warn`, `Danger`, `Accent`, `Bold`, `Dim`, `Verdict(token)`, `Rule(width)`, `Banner(title)`, `Enabled()`; `Enabled()` is false when `NO_COLOR` is set and true when `SWORN_FORCE_COLOR` is set. Verified by `internal/style/style_test.go`.
- [ ] With colour disabled (the default under `go test`, where stdout is not a TTY), the full existing `cmd/sworn` and renderer test suites pass unchanged — i.e. plain output is byte-identical. Falsifiable: `go test ./cmd/sworn/... ./internal/...` is green.
- [ ] With `SWORN_FORCE_COLOR=1`, at least `sworn version`, `sworn help`, and `sworn top <release>` emit ANSI escape sequences; with `NO_COLOR=1` the same commands emit zero escape sequences. Falsifiable: pipe each through `grep -c $'\\033'`.
- [ ] No styled string is placed inside a width-padded format verb where ANSI bytes would corrupt column alignment (pad raw, then style). Verified by inspection + the renderers' table tests staying green.
- [ ] No new module dependency is added. Falsifiable: `go.mod` `require` block is unchanged; `internal/style` imports only the stdlib.
- [ ] `go build ./...` and `go vet ./...` pass.

## Required tests

- **Unit**: `internal/style/style_test.go` — palette helpers, `NO_COLOR` / `SWORN_FORCE_COLOR` / non-TTY gating, and that disabled mode returns the input unchanged.
- **Integration**: the existing `cmd/sworn/...` and `internal/{rtm,ears,specquality,designfit,designaudit,reqverify,reqvalidate}` test suites exercise the styled renderers through their command/Print entry points and must stay green with colour disabled (Rule 1).
- **Reachability artefact**: terminal transcript in `proof.md` showing `SWORN_FORCE_COLOR=1 sworn version|help|top` rendering ANSI, and the matching `NO_COLOR=1` runs showing zero escapes.
- **E2E gate type**: N/A (CLI; no Playwright).

## Risks

- **Alignment corruption**: ANSI bytes counted into a `%-*s` width break columns (already hit and fixed once in `init`'s plan table). Mitigated by the pad-then-style rule and the renderers' table tests.
- **Plain-output drift**: rephrasing text while styling would break golden tests. Mitigated by wrapping existing substrings only; divider runs of `=` stay `Dim`-wrapped rather than converted to `Rule` (which emits `─`).
- **Stream mismatch**: gating on `os.Stdout` while a renderer writes to stderr. Acceptable (single global gate); documented.

## Deferrals allowed?

No — except the TUI, which is explicitly out of scope above (governed by T2), not a mid-implementation carve-out.
