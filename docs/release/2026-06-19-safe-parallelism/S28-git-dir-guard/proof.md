# Proof Bundle: S28-git-dir-guard

## Scope

A `sworn` run that exercises any code path or test touching `internal/git` can **never** silently operate on the ambient working directory. A `git.Repo` with no `Dir` set fails loudly instead of running `git checkout main` (or any mutation) in the calling worktree. This closes the root cause of sworn#6 — a track worker's commits landing on `main` because a `go test` flipped the worktree.

## Files changed

```
$ git diff --name-only b7425e9..584e9d9
docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard/status.json
internal/git/git.go
internal/git/git_test.go
```

## Test results

### Go (internal/git)

```
$ go test -count=1 -v ./internal/git/...
=== RUN   TestInit
--- PASS: TestInit (0.01s)
=== RUN   TestBranchAndCheckout
--- PASS: TestBranchAndCheckout (0.02s)
=== RUN   TestStageAndCommit
--- PASS: TestStageAndCommit (0.01s)
=== RUN   TestRevParse
--- PASS: TestRevParse (0.02s)
=== RUN   TestDiffRange
--- PASS: TestDiffRange (0.02s)
=== RUN   TestDiffRangeStat
--- PASS: TestDiffRangeStat (0.02s)
=== RUN   TestCommit_AllowEmpty
--- PASS: TestCommit_AllowEmpty (0.01s)
=== RUN   TestDiffRange_Empty
--- PASS: TestDiffRange_Empty (0.01s)
=== RUN   TestRunRejectsEmptyDir
--- PASS: TestRunRejectsEmptyDir (0.00s)
=== RUN   TestEmptyDirDoesNotTouchCwd
--- PASS: TestEmptyDirDoesNotTouchCwd (0.02s)
=== RUN   TestMerge
--- PASS: TestMerge (0.03s)
PASS
ok  	github.com/swornagent/sworn/internal/git	0.173s
```

### Go (build + vet)

```
$ go build ./...
# (exit 0, no output)

$ go vet ./internal/git/...
# (exit 0, no output)
```

### New tests (guard-specific)

```
$ go test -count=1 -v -run 'TestRunRejectsEmptyDir|TestEmptyDirDoesNotTouchCwd' ./internal/git/...
=== RUN   TestRunRejectsEmptyDir
--- PASS: TestRunRejectsEmptyDir (0.00s)
=== RUN   TestEmptyDirDoesNotTouchCwd
--- PASS: TestEmptyDirDoesNotTouchCwd (0.02s)
PASS
ok  	github.com/swornagent/sworn/internal/git	0.033s
```

## Reachability artefact

- **Type**: manual-smoke-step  
- **Path**: N/A — unit test (no UI)  
- **User gesture**: `go test -v -run 'TestRunRejectsEmptyDir|TestEmptyDirDoesNotTouchCwd' ./internal/git/...` passes, demonstrating the guard fires and the ambient cwd is untouched.

## Delivered

- [AC1] `git.New("").Checkout("main")`, `.Branch("x")`, `.Commit("m")` each return a non-nil error mentioning "empty Repo.Dir"; the git binary is NOT invoked against cwd — evidence: `TestRunRejectsEmptyDir` exercises all three methods and asserts the guard error message
- [AC2] The regression test captures cwd `git rev-parse HEAD` before and after the guarded call and asserts they are identical — evidence: `TestEmptyDirDoesNotTouchCwd` (uses `t.Chdir()` per Captain pin #1, captures original SHA and branch, asserts both unchanged after zero-Dir Checkout)
- [AC3] All existing `internal/git` tests still pass — evidence: `go test -count=1 -v ./internal/git/...` shows all 11 tests PASS (9 existing + 2 new)
- [AC4] `go build ./...` and `go vet ./internal/git/...` pass — evidence: both exit 0 with no output

## Not delivered

- None. All four acceptance checks are delivered. (Note: the guard is in `run()` which also covers `DiffRangeStat` — the 9th method — automatically, as noted in the Captain review flag (a).)

## Divergence from plan

None. Implementation matches the spec and design exactly:
- Guard added in `run()` (as specified, the single chokepoint)
- `t.Chdir()` used per Captain pin #1 (instead of `os.Chdir()` as originally drafted in design.md)
- `design_decisions` added to `status.json` per Captain pin #2
- Caller audit confirmed zero callers needed fixing

## First-pass script output

```
$ ~/.claude/bin/release-verify.sh S28-git-dir-guard 2026-06-19-safe-parallelism
  slice:       S28-git-dir-guard
  slice dir:   docs/release/2026-06-19-safe-parallelism/S28-git-dir-guard
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
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit b7425e9...
  PASS  5 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~3) consistent with diff vs start_commit (5)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```