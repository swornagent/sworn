---
title: 'Slice S51 — CLI command registry'
description: 'Replace the hand-maintained `cmd/sworn/main.go` dispatch switch with a self-registration command registry, so adding a subcommand never edits a shared file. Makes `main.go` owned by exactly one track and ends the recurring touchpoint collision.'
---

# Slice: `S51-cli-command-registry`

## User outcome

A developer runs any `sworn <verb>` (e.g. `sworn lint`, `sworn version`, `sworn doctor`) and gets exactly the same behaviour as before; `sworn help` lists every command. Internally, a new subcommand is added by registering it from its own `cmd/sworn/<verb>.go` file — `cmd/sworn/main.go` is never edited to add a command again.

## Entry point

The `sworn` CLI binary (`cmd/sworn`). User gesture: `sworn <verb> [args]` at a shell, and `sworn help` / `sworn version`. The dispatch path in `func main()` is the surface under change; every existing verb must remain reachable through it.

## Why this slice exists

`cmd/sworn/main.go` was declared a *DOCUMENTED SHARED* file ("additive dispatch only") with every track adding a `case` to one contiguous `switch os.Args[1]`. Additive `case` insertions into the same block **do** collide in git, so forward-merging `release-wt` into any in-flight track that also added a case conflicts on `main.go`. This recurred across T2/T4/T8 syncs and finally hard-paged the coach loop on T3-commercial's S07-paging verify (release-wt → T3 forward-merge conflict on `main.go`). The registry removes `main.go` as a shared edit surface at the root: it becomes owned by exactly one track (`T15-cli-registry`) and is never touched per-command again.

## In scope

- A new `internal/command` package: a process-wide command registry with `Register`, `Lookup`, and `All` (sorted), where double-registration of a name is a programming error (panic).
- `cmd/sworn/main.go` reduced to: argument check → registry `Lookup(os.Args[1])` → dispatch `Run(os.Args[2:])` → unknown-command fallback. No per-command `case` statements remain.
- A single new T15-owned file `cmd/sworn/commands.go` that registers every command **already present on `release-wt`** by reference to its existing `cmdXxx` function: `init`, `verify`, `run`, `bench`, `mcp`, `lint`, `reqverify`, `reqvalidate`, `designfit`, `journeys`, `ship`, `specquality`, `designaudit`, `top`, `doctor`, `telemetry`, `memory`, plus `version` (and aliases `--version`/`-v`) and `help` (and `--help`/`-h`).
- `usage()` enumerates the registry (`command.All()`) rather than a hand-maintained string block, so help output stays in sync with registered commands automatically.

## Out of scope

- Migrating each existing `cmd/sworn/<verb>.go` to its own self-registering `init()` (would collide with in-flight T3 on `run.go`/`memory.go` and T12 on `lint.go`). Existing verbs are registered centrally in `commands.go`; opportunistic per-file migration is a later, separate concern. Tracked: cross-track note in `index.md`.
- Converting T3's `login`/`account` commands to `register()` — that is T3's own merge-resolution work, folded into S07-paging's implement re-entry (see "Downstream adoption").
- Any change to command behaviour, flags, output, or exit codes. This is a pure dispatch refactor; every verb is byte-for-byte equivalent in behaviour.

## Planned touchpoints

- `internal/command/registry.go` (new — Register / Lookup / All)
- `internal/command/registry_test.go` (new)
- `cmd/sworn/main.go` (rewrite dispatch switch → registry lookup loop; sole owner: T15)
- `cmd/sworn/commands.go` (new — central registration of all pre-existing verbs + version/help)
- `cmd/sworn/commands_test.go` (new — dispatch + coverage test)

## Acceptance checks

- [ ] `internal/command` exposes `Register(Command)`, `Lookup(name string) (Command, bool)`, and `All() []Command`; registering two commands with the same name panics. Verified by `internal/command/registry_test.go`.
- [ ] `cmd/sworn/main.go`'s `func main()` contains **no** `case "<verb>":` dispatch statements — dispatch is a single registry `Lookup`. Falsifiable: `grep -c 'case "' cmd/sworn/main.go` returns 0.
- [ ] Every verb that dispatched before the refactor (`init`, `verify`, `run`, `bench`, `mcp`, `lint`, `reqverify`, `reqvalidate`, `designfit`, `journeys`, `ship`, `specquality`, `designaudit`, `top`, `doctor`, `telemetry`, `memory`, `version`, `help`) resolves via `command.Lookup` and dispatches to the same `cmdXxx` function. Verified by `commands_test.go` asserting each name resolves.
- [ ] `sworn version` prints `sworn <v>` + `baton-protocol <v>` exactly as before; `sworn --version` and `sworn -v` are aliases. Verified by test + smoke.
- [ ] `sworn help` lists every registered command (sourced from `command.All()`), and an unknown verb prints `unknown command` to stderr and exits 64. Verified by test + smoke.
- [ ] `go build ./...` and `go vet ./...` pass; the built binary's full verb surface is unchanged.

## Required tests

- **Unit**: `internal/command/registry_test.go` — Register/Lookup/All behaviour; duplicate-name panic; `All()` sorted and complete.
- **Integration**: `cmd/sworn/commands_test.go` — drives dispatch the way `main()` does: for each expected verb, assert `command.Lookup(verb)` resolves and its `Run` is the expected handler; assert unknown verb is not found; assert `help`/`version` resolve. This exercises the entry point (`main`'s dispatch path) per Rule 1, not a leaf in isolation.
- **Reachability artefact**: explicit smoke step — build `sworn`, run `sworn help` (observe all verbs listed from the registry), `sworn version` (observe version + baton-protocol line), `sworn lint` and `sworn designfit` against a sample release (observe identical behaviour to pre-refactor), and `sworn bogusverb` (observe `unknown command`, exit 64). Capture terminal transcript in `proof.md`.
- **E2E gate type**: N/A (CLI, no Playwright).

## Risks

- **A verb is dropped in migration** — a command registered in `commands.go` is missed, so `sworn <verb>` regresses to "unknown command". Mitigated by `commands_test.go` asserting the full known-verb set resolves; the list of 19 verbs above is the checklist.
- **`init()` ordering / import cycle** — central registration in `commands.go` references `cmdXxx` funcs in sibling files; all are package `main`, so no cycle. `internal/command` must not import `cmd/sworn`. Verified by `go build`.
- **Help text drift** — `usage()` now derives from the registry; a command registered without a `Summary` yields a blank help line. Mitigated by `commands_test.go` asserting every registered command has a non-empty `Summary`.

## Downstream adoption (not part of this slice's diff)

- **T3-commercial / S07-paging**: on its next implement re-entry, T3 forward-merges `release-wt` (now carrying this registry), resolves the `main.go` conflict by converting its `login` and `account` dispatch cases into `command.Register(...)` calls in `cmd/sworn/login.go` and `cmd/sworn/account.go`, then re-verifies. This is the mechanism that clears the coach-loop pause.
- **T3 / S19-sworn-induction, T14 / S48-baton-vendor, S49-baton-version**: their specs are corrected (this replan) to register their verbs from their own command files instead of editing `main.go`.
- **Convention going forward**: any track adding a top-level CLI command self-registers from its own `cmd/sworn/<verb>.go` via `init()`; `cmd/sworn/main.go` and `cmd/sworn/commands.go` are owned solely by `T15-cli-registry` and are not edited to add a command. Enforced by the `lint touchpoints` gate (S30) against the touchpoint matrix.

## Deferrals allowed?

No. The verb-coverage checklist (19 names) and the `grep -c 'case "'` check are both falsifiable from artefacts; nothing here may be carved out mid-implementation without a Rule 2 surface.
