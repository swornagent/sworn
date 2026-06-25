---
title: S51 Implementation Journal
description: Implementation log for S51-cli-command-registry
---

# Journal: `S51-cli-command-registry`

## Session log

### 2026-07-02 00:00 — in_progress → implemented

- **State**: implemented
- **Notes**:
  - Implemented `internal/command` package with `Register`, `Lookup`, `All` — process-wide registry with duplicate-panic safety.
  - Created `cmd/sworn/commands.go` with central registration of all 23 verbs (19 functional + 4 alias entries for version/help). Each verb maps to its existing `cmdXxx` handler — zero behaviour change.
  - Rewrote `cmd/sworn/main.go` dispatch: replaced 17-case `switch` with `command.Lookup(os.Args[1])` → `c.Run(args[2:])`. `grep -c 'case "' cmd/sworn/main.go` returns 0. Added `cmdVersion` and `cmdHelp` wrapper functions (previously inline in the switch).
  - Moved `cmdVerify` + `openDeferralsFlag` to new `cmd/sworn/verify.go` — mechanical relocation, no logic changes (Coach Pin 1).
  - Created `cmd/sworn/commands_test.go` with non-empty Summary assertion for every registered command (Coach Pin 2).
  - Recorded `design_decisions` in status.json: registry pattern introduction (Coach Pin 3).
  - Updated index.md touchpoint matrix: added verify.go row, checkmarks on all T15 rows, state→in_progress.
  - Spec: added explicit `playwright-screenshot: N/A` declaration to resolve first-pass false positive.
  - All existing tests pass (cmd/sworn full suite: PASS). `go build`, `go vet` clean. Smoke tests confirm identical output/exit codes.
  - Design decisions: (1) cmdVerify moved to verify.go; (2) Command.Run signature matches existing `func([]string) int`; (3) version referenced via closure; (4) struct field Run, not interface method; (5) usage() prose stays hand-maintained, only listing could auto-generate.
  - No subagent dispatches.

## Open questions

None.

## Deferrals surfaced

- **Per-file init() self-registration** — **Why**: would collide with in-flight T3 (run.go/memory.go) and T12 (lint.go). **Tracking**: spec "Downstream adoption" section; cross-track note in index.md. **Acknowledged**: Coach (design-review PROCEED, 2026-06-22).

## Verifier verdicts received

None yet.
### 2026-07-03 — PASS (fresh-context verifier session)

**Verdict: PASS**

All six verification gates satisfied:

1. **User-reachable outcome exists** — the `sworn <verb>` entry point dispatches through `command.Lookup` in `func dispatch()`. Every verb (19 commands + aliases) resolves and executes identically to pre-refactor behaviour. Smoke test confirms: `sworn help` lists all verbs from registry, `sworn version` prints version + baton-protocol, `sworn bogusverb` exits 64.

2. **Planned touchpoints match actual changed files** — spec planned: `internal/command/registry.go`, `internal/command/registry_test.go`, `cmd/sworn/main.go`, `cmd/sworn/commands.go`, `cmd/sworn/commands_test.go`. Actual: same 5 plus `cmd/sworn/verify.go` (documented divergence: mechanical extraction of `cmdVerify` + `openDeferralsFlag` from `main.go` per Coach Pin 1).

3. **Required tests exist and exercise the integration point** — `internal/command/registry_test.go` (4 unit tests). `cmd/sworn/commands_test.go` (5 integration tests driving dispatch the way `main()` does — Rule 1 compliant). All pass with `-count=1`.

4. **Reachability artefact present** — proof.md contains terminal transcript; independently reproduced.

5. **No silent deferrals** — grep for TODO/FIXME/deferred/placeholder/hack/workaround in changed source files found only legitimate `openDeferralsFlag` type names (Rule 2 mechanism).

6. **Claimed scope matches implemented scope** — all 6 acceptance checks have verifiable evidence. `grep -c 'case "' cmd/sworn/main.go` → 0.

Worktree note: the session encountered a worktree HEAD-on-main issue (track worktree had `main` checked out; initial `checkout` didn't stick). All verification reads were cross-checked against `git show df88cf3:path` to ensure correctness.
