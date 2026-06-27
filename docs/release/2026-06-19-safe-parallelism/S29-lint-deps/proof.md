---
title: "Slice proof bundle for S29-lint-deps"
---

# Proof Bundle: S29-lint-deps

## Scope

`sworn lint deps <slice-id> <release>` fails closed on undeclared `go.mod`/`go.sum` changes — exits non-zero when a dep file changes but is absent from the slice's `status.json` `planned_files`.

## Files changed

```
$ git diff --name-only 8c85353
cmd/sworn/lint.go
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/proof.md
docs/release/2026-06-19-safe-parallelism/S29-lint-deps/status.json
internal/lint/deps.go
internal/lint/deps_test.go
internal/prompt/planner.md
```

Production files: `internal/lint/deps.go`, `internal/lint/deps_test.go`, `cmd/sworn/lint.go`, `internal/prompt/planner.md`.

## Test results

### Go test (slice-scoped)

```
$ go test ./internal/lint/... -run Deps -v
=== RUN   TestDepsUndeclaredFails
--- PASS: TestDepsUndeclaredFails (0.03s)
=== RUN   TestDepsDeclaredPasses
--- PASS: TestDepsDeclaredPasses (0.02s)
=== RUN   TestDepsNoChangePasses
--- PASS: TestDepsNoChangePasses (0.02s)
PASS
ok  	github.com/swornagent/sworn/internal/lint	0.067s
```

### Build + vet

```
$ go build ./...
$ go vet ./internal/lint/...
BUILD OK
VET OK
```

## Reachability artefact

- **Type**: manual-smoke-step (CLI binary invocation)
- **Binary**: `bin/sworn` built via `make build`
- **Fixture**: temp git repo with a slice whose `go.mod` changes but `planned_files` omits it

**Undeclared case (exit 1):**

```
$ cd /tmp/sworn-deps-reach-final
$ sworn lint deps --base HEAD~2 S01-test test-rel
sworn lint deps: undeclared dependency file(s): go.mod
exit: 1
```

**Declared case (exit 0):**

```
$ cd /tmp/sworn-deps-reach
$ sworn lint deps --base HEAD~3 S01-test test-rel
deps: all dependency files declared in planned_files for S01-test
exit: 0
```

**No-change case (exit 0):**

```
$ cd /tmp/sworn-deps-reach2
$ sworn lint deps --base HEAD S01-test test-rel
deps: all dependency files declared in planned_files for S01-test
exit: 0
```

## Delivered

- `sworn lint deps <slice> <release>` exits non-zero when a changed `go.mod`/`go.sum` is not in `planned_files`; message names the undeclared file(s) — evidence: `TestDepsUndeclaredFails` + reachability artefact (undeclared case, exit 1, names `go.mod`).
- `sworn lint deps <slice> <release>` exits 0 when dep files are declared or unchanged — evidence: `TestDepsDeclaredPasses`, `TestDepsNoChangePasses` + reachability artefact (declared case exit 0, no-change case exit 0).
- `internal/prompt/planner.md` contains a checklist line directing the planner to add `go.mod` and `go.sum` to `planned_files` on any dep change — evidence: Phase 4 step 7 in `internal/prompt/planner.md`.
- `go build ./...` and `go vet ./internal/lint/...` pass — evidence: Build + vet output above.

## Not delivered

- None.

## Divergence from plan

- Rewrote `internal/lint/deps.go` from the crashed-dispatch WIP: replaced the local `Status` struct with `internal/state.Read` (consistent with the rest of the codebase), switched from three-dot to two-dot git diff, fixed indentation to tabs, and sorted undeclared file names in the error message for deterministic output.
- Added `--base` flag to `cmdLintDeps` for testability (spec Risks section mentions "accept an explicit base-ref flag for tests").

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S29-lint-deps 2026-06-19-safe-parallelism
  slice:       S29-lint-deps
  slice dir:   docs/release/2026-06-19-safe-parallelism/S29-lint-deps
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

== Integration branch drift ==
  integration branch: release/v0.1.0
  WARNING: worktree is 1 commit(s) behind release/v0.1.0 (no test-infra overlap)
  PASS  integration branch drift present but does not affect test infrastructure

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 8c85353
  PASS  6 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~3) consistent with diff vs start_commit (6)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```