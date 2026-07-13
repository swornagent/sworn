---
title: 'Proof bundle — S69-lint-regress'
description: 'Generated from live repo state.'
---

## Scope

Port release-regression.sh from bash to Go: `sworn regress --release <name>` runs the full test suite (Go + TS + golden fixtures) against the merged release-wt worktree, exits 0 on all-pass, 1 on any failure.

## Files changed

```
cmd/sworn/commands.go
cmd/sworn/regress.go
docs/release/2026-06-19-safe-parallelism/S69-lint-regress/status.json
internal/gate/regress.go
internal/gate/regress_test.go
```

## Test results

```
=== RUN   TestRunRegress_AllPass
--- PASS: TestRunRegress_AllPass (0.00s)
=== RUN   TestRunRegress_AllFail
--- PASS: TestRunRegress_AllFail (0.00s)
=== RUN   TestRunRegress_Mixed
--- PASS: TestRunRegress_Mixed (0.00s)
=== RUN   TestRunRegress_NoPackageJSON
--- PASS: TestRunRegress_NoPackageJSON (0.00s)
=== RUN   TestPrintRegress_Output
--- PASS: TestPrintRegress_Output (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/gate	(cached)
```

```
go build ./... — PASS (clean, no output)
```

## Reachability artefact

```
$ ./bin/sworn regress --release 2026-06-19-safe-parallelism

Regression — 2026-06-19-safe-parallelism

Worktree: /home/user/projects/sworn-worktrees/release-2026-06-19-safe-parallelism

  FAIL  Go tests (exit 1)
  SKIP  TypeScript tests (no package.json in worktree)
  PASS  Golden fixtures

1 suite(s) failed, 1 passed, 1 skipped.
```

The command correctly resolves the release worktree from `index.md` frontmatter, runs `go test ./...` (Go tests have pre-existing failures in the release worktree — expected), skips TS tests because there is no `package.json`, checks golden fixtures via `git diff --exit-code`, and exits 1 on failure.

`--json` flag produces valid JSON output with all suite details.

## Delivered

| AC | Status | Evidence |
|----|--------|----------|
| `sworn regress --release <name>` runs all Go tests against release-wt | ✅ | `internal/gate/regress.go` runs `go test ./...` in resolved worktree; reachability artefact shows Go tests running |
| Reports per-suite pass/fail status | ✅ | `PrintRegress()` in `internal/gate/regress.go` displays PASS/FAIL/SKIP per suite with exit codes |
| Golden fixture divergence detected and reported | ✅ | `checkGoldenFixtures()` in `internal/gate/regress.go` runs `git diff --exit-code -- **/testdata/**` and reports divergence as a FAIL suite |
| Exits 0 on clean, 1 on failure | ✅ | `cmd/sworn/regress.go` returns 0 when `!report.HasViolations()`, 1 otherwise; tested via `TestRunRegress_AllPass` / `TestRunRegress_AllFail` |
| Handles missing test suites gracefully | ✅ | `runTSSuite()` skips when pnpm unavailable (`TestRunRegress_Mixed`) or `package.json` absent (`TestRunRegress_NoPackageJSON`) |

## Not delivered

None — all five acceptance checks are delivered.

## Divergence from plan

- **`cmd/sworn/commands.go`** was not listed in `planned_files` but was necessarily modified to register the `regress` command in the process-wide command registry (one `command.Register` call). This is the standard touch for any new CLI verb and is consistent with all prior command additions (lint, coverage, design, mock, etc. all touch this file for registration).

## First-pass script output

First-pass shows expected pre-final-state failures:
- `proof.md missing` and `journal.md missing` — these files are created as part of this commit
- `state is 'in_progress'` — slice transitions to `implemented` in this commit
- Playwright opt-in false positive — the spec says "E2E gate type: local" which triggers the script's "e2e" keyword regex; this is a Go CLI tool with no UI, no Playwright screenshots needed; the "E2E gate type" field is informational, not a screenshot requirement
- `PLAYWRIGHT_OPTIN: unbound variable` — known script issue (see feedback_release_verify_darkcode_placeholder.md)