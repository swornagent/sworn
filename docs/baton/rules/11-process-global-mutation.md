---
title: Rule 11 — Process-Global Mutation Guard
description: Any design touching process-global state must guarantee restore, assert a non-empty/expected working directory before git ops, and show a reachability artefact proving the guard.
---

# Rule 11 — Process-Global Mutation Guard

## The rule

Any code — test or production — that mutates **process-global state** must
satisfy all three of the following before the owning slice can reach `verified`:

1. **Guaranteed restore.** State mutated (`os.Chdir`, a raw `git` invocation
   with a cwd argument, worktree creation/switching, or a global env/cwd
   mutation in tests) must be restored before the owning function returns.
   Acceptable patterns: `t.Chdir` (test-scoped), `defer <restore>()`,
   or a `cleanup` callback that runs irrespective of test outcome.

2. **Non-empty / expected-dir assertion.** Any `git` operation that carries a
   cwd argument must first assert that the target directory exists, is
   non-empty, or matches an expected path. The assertion must fail closed —
   if the directory is empty or missing, the operation must not proceed.

3. **Reachability artefact showing the guard.** The slice cannot be marked
   `verified` without evidence that the guard exists and fires: a test that
   exercises the restore path (e.g. asserts the working directory after a
   test that calls `os.Chdir`), a screenshot of a test run showing the
   assertion firing, or an explicit smoke step verifying the non-empty-dir
   check.

A design that touches any of these patterns without addressing (1), (2), and
(3) is incomplete. The Captain design-review must flag it.

## Why

The sworn#6 class (a git op run with an empty directory that flipped a worktree
to `main`) and its recurrence on S28-git-dir-guard (`os.Chdir` → `t.Chdir`)
demonstrate that process-global mutation in tests and CLI code is a
**systematic** failure class, not an incidental one.

- S28 fixed the code (git fails closed on empty `Dir`).
- This rule (S35) is the **process** guard — it makes the catch systematic
  rather than incidental. Every design that touches process-global state is
  now subject to a standing review check.

Mutating process-global state without restore is a silent corruption risk: the
next test, or the next CLI operation in the same process, inherits the mutated
state. Without a non-empty-dir assertion, a git op can run in an unexpected
directory and produce nondeterministic outcomes. Without a reachability
artefact, the guard itself is unverifiable.

## How to apply

### For implementers

- When a design touches `os.Chdir`, wrap it in `t.Chdir` (test-scoped) or
  `defer os.Chdir(originalDir)`.
- When a design invokes `git` with a cwd argument, assert the directory is
  non-empty (e.g. `_, err := os.Stat(path); if os.IsNotExist(err) { ... }`)
  before the git invocation.
- When a design creates or switches worktrees, ensure the worktree path is
  verified and the original working directory is restored.
- In `proof.md`, cite the specific test or smoke step that proves the guard.

### For Captains

See the `/design-review` function's Step 7 — "Process-global mutation guard".
For any design under review, scan for the four patterns (`os.Chdir`, raw
`git` with cwd, worktree creation/switching, global env/cwd mutation in tests).
Flag any occurrence that lacks (1) restore, (2) non-empty-dir assertion, and
(3) a reachability artefact.

### For verifiers

Gate 4 (reachability) already covers the artefact requirement. This rule adds
a **named check class** to Gate 4: the reachability artefact must specifically
demonstrate the process-global-mutation guard when the slice's diff touches
any of the four patterns.

## When this rule applies

- Any slice whose `planned_files` include code that calls `os.Chdir`, `os.Setenv`,
  `os.Setwd`, or invokes `exec.Command("git", ...)` with a `Dir` field set.
- Any slice that creates, switches, or removes git worktrees.
- Any test file that mutates `os.Environ()`, `os.Args`, or the process working
  directory.

## When this rule does NOT apply

- Tests that mutate only test-local state (scoped `t.TempDir()`, `t.Setenv`,
  `t.Chdir`) with automatic framework-level restore — the framework itself is
  the guard. No extra check needed beyond confirming the framework's restore
  guarantee applies.

## Provenance

- **sworn#6**: A git op run with an empty directory (`git worktree remove`
  invoked after the target worktree's path was stale) flipped the parent
  worktree to `main` silently. Tracked at github.com/swornagent/sworn#6.
- **S28-git-dir-guard**: The code-side fix — `internal/git.Repo.run()` now
  fails closed on an empty `Dir`. Caught a recurrence: `os.Chdir` without
  restore in a test helper.
- **Trial-log harvest §5 (theme T-F)**: The trial-log analysis identified
  process-global mutation as a recurring theme and recommended codifying the
  guard. This rule is the durable artefact of that harvest.