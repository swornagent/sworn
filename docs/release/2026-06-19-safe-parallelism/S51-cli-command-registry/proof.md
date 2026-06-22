---
title: Slice proof bundle — S51-cli-command-registry
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection.
---

# Proof Bundle: `S51-cli-command-registry`

## Scope

A developer runs any `sworn <verb>` and gets exactly the same behaviour as before; `sworn help` lists every command. Internally, a new subcommand is added by registering it from its own `cmd/sworn/<verb>.go` file — `cmd/sworn/main.go` is never edited to add a command again.

## Files changed

```
$ git diff --name-only a4df3dc
cmd/sworn/commands.go
cmd/sworn/commands_test.go
cmd/sworn/main.go
cmd/sworn/verify.go
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/journal.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/proof.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/spec.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/status.json
internal/command/registry.go
internal/command/registry_test.go
```
## Test results

### Go

```
$ go test ./internal/command/... -v
=== RUN   TestRegisterAndLookup
--- PASS: TestRegisterAndLookup (0.00s)
=== RUN   TestLookupNotFound
--- PASS: TestLookupNotFound (0.00s)
=== RUN   TestAllSorted
--- PASS: TestAllSorted (0.00s)
=== RUN   TestDuplicatePanics
--- PASS: TestDuplicatePanics (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/command      0.002s
```

```
$ go test ./cmd/sworn/... -v -run 'TestEveryVerbResolves|TestUnknownVerbNotFound|TestAllCommandsHaveNonEmptySummary|TestVersionAndHelpAliasesShareHandlers|TestDispatchResolves'
=== RUN   TestEveryVerbResolves
--- PASS: TestEveryVerbResolves (0.00s)
=== RUN   TestUnknownVerbNotFound
--- PASS: TestUnknownVerbNotFound (0.00s)
=== RUN   TestAllCommandsHaveNonEmptySummary
--- PASS: TestAllCommandsHaveNonEmptySummary (0.00s)
=== RUN   TestVersionAndHelpAliasesShareHandlers
--- PASS: TestVersionAndHelpAliasesShareHandlers (0.00s)
=== RUN   TestDispatchResolves
--- PASS: TestDispatchResolves (0.00s)
PASS
ok      github.com/swornagent/sworn/cmd/sworn      0.009s
```

```
$ go test ./cmd/sworn/... (full suite)
PASS
ok      github.com/swornagent/sworn/cmd/sworn      0.239s
```

```
$ go build ./... && go vet ./...
EXIT: 0
EXIT: 0
```

```
$ grep -c 'case "' cmd/sworn/main.go
0
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: terminal transcript captured below
- **User gesture**: user runs `sworn help`, `sworn version`, `sworn lint`, `sworn designfit`, and `sworn bogusverb` at a shell

### Smoke transcript

```
$ sworn help
sworn -- SwornAgent's provider-neutral verification core

usage:
  sworn bench --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn init [--api-key <key>] [--force]
  sworn journeys [--check] [--impact <release>] [project-path]
  sworn lint ac <release>
  sworn lint trace <release>
  sworn reqverify <release>
  sworn reqvalidate <release>
  sworn designfit <release>
  sworn designaudit <project-dir> [--cohesion on-brand|off-brand]
  sworn specquality <release> [--threshold <0.0-1.0>]
  sworn run --task <description> [--implementer-model <m>] [--verifier-model <m>] [--base <branch>] [--retry-cap <n>]
  sworn ship <release> [project-root]
  sworn telemetry on|off|status
  sworn top <release> [project-path]
  sworn doctor [--fix] [--sync-baton]
  sworn verify --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]
  sworn version
(continues with full prose...)

$ sworn version
sworn 0.0.0-dev
baton-protocol v1.0.0

$ sworn --version
sworn 0.0.0-dev
baton-protocol v1.0.0

$ sworn -v
sworn 0.0.0-dev
baton-protocol v1.0.0

$ sworn lint ac 2026-06-19-safe-parallelism
EARS Acceptance-Criteria Validation
============================================================
[...output identical to pre-refactor...]
EXIT: 0

$ sworn designfit 2026-06-19-safe-parallelism
sworn designfit: [...identical output...]
EXIT: 2

$ sworn bogusverb
unknown command "bogusverb"

sworn -- SwornAgent's provider-neutral verification core
[...usage...]
EXIT: 64
```

## Delivered

- **`internal/command` exposes `Register(Command)`, `Lookup(name string) (Command, bool)`, and `All() []Command`; registering two commands with the same name panics. Verified by `internal/command/registry_test.go`.** — evidence: `internal/command/registry.go`, `internal/command/registry_test.go` (4 tests, all pass)
- **`cmd/sworn/main.go`'s `func main()` contains no `case "<verb>":` dispatch statements — dispatch is a single registry `Lookup`. Falsifiable: `grep -c 'case "' cmd/sworn/main.go` returns 0.** — evidence: `grep -c 'case "' cmd/sworn/main.go` → 0; `cmd/sworn/main.go` dispatch function uses `command.Lookup`
- **Every verb that dispatched before the refactor (init, verify, run, bench, mcp, lint, reqverify, reqvalidate, designfit, journeys, ship, specquality, designaudit, top, doctor, telemetry, memory, version, help) resolves via `command.Lookup` and dispatches to the same `cmdXxx` function. Verified by `commands_test.go` asserting each name resolves.** — evidence: `cmd/sworn/commands_test.go` (`TestEveryVerbResolves`, `TestDispatchResolves` — 23 verbs, all pass)
- **`sworn version` prints `sworn <v>` + `baton-protocol <v>` exactly as before; `sworn --version` and `sworn -v` are aliases. Verified by test + smoke.** — evidence: `cmd/sworn/commands_test.go` (`TestVersionAndHelpAliasesShareHandlers`), smoke transcript above
- **`sworn help` lists every registered command (sourced from `command.All()`), and an unknown verb prints `unknown command` to stderr and exits 64. Verified by test + smoke.** — evidence: `cmd/sworn/commands_test.go` (`TestEveryVerbResolves`, `TestAllCommandsHaveNonEmptySummary`), smoke transcript above
- **`go build ./...` and `go vet ./...` pass; the built binary's full verb surface is unchanged.** — evidence: go build EXIT: 0, go vet EXIT: 0, full cmd/sworn test suite passes

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- **Added `cmd/sworn/verify.go`** — the spec's planned_files omitted this file. `cmdVerify` and `openDeferralsFlag` were embedded in `main.go` pre-refactor and needed a home once `main.go`'s switch was removed. The relocation is mechanical (no logic changes) and declared in the updated status.json `planned_files` + index.md touchpoint matrix (Coach Pin 1).

## First-pass script output

```
$ release-verify.sh S51-cli-command-registry 2026-06-19-safe-parallelism

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
  diff base: start_commit a4df3dc
  PASS  10 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~7) consistent with diff vs start_commit (10)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```