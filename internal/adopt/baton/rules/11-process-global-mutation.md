---
title: Rule 11 — Process-Global Mutation Guard
description: Any change that mutates process-global state (working directory, environment, or which worktree/branch a tool acts on) must guarantee restore, assert the target before acting, and show a reachability artefact proving the guard.
---

# Rule 11 — Process-Global Mutation Guard

## The rule

Any change — test or production — that mutates **process-global state** (the
working directory, environment variables, or which git worktree/branch the
process operates on) must satisfy all three of the following before the owning
slice can reach `verified`:

1. **Guaranteed restore.** Mutated state must be restored before the owning
   unit of work returns — via a test-framework scoped helper, a deferred
   restore, or a cleanup callback that runs irrespective of outcome. Prefer
   *scoped* mutation (invoking the tool with an explicit working-directory
   argument, or a child process) over mutating the ambient process and
   restoring it.

2. **Fail-closed target assertion.** Any operation that acts on a path or
   worktree — especially a `git` operation carrying a directory argument —
   must first assert the target exists and is the expected directory. If the
   path is empty, missing, or unexpected, the operation must not proceed.

3. **Reachability artefact.** The slice cannot be marked `verified` without
   evidence the guard exists and fires: a test exercising the restore path, or
   an explicit smoke step demonstrating the assertion firing on a bad target.

## Why

In a parallel or multi-worktree harness, process-global state is shared across
units of work: a mutation left unrestored is silently inherited by the next
test, or the next operation in the same process. The worst case is a git
operation that runs in an unexpected (or empty) directory and corrupts branch
state — a worktree silently flipped to its base branch — surfacing later as an
unrelated-looking failure. Wherever sessions run concurrently against a shared
base, this is a systematic failure class, not an incidental one.

## Resumed-loop restore contract

The same fail-closed principle extends to **resumption after an unclean exit**. A crashed or interrupted run can leave a track worktree holding uncommitted implementer output — debris that is process-global state by another name: it is silently inherited by whatever acts in that worktree next.

**A resumed loop must restore each track worktree to its committed slice state before it re-dispatches into that worktree.** Concretely: `git reset --hard` to the slice's committed head and `git clean` untracked debris, having first asserted the target is the expected worktree on the expected branch (clause 2 above — a `reset --hard` in the wrong directory is exactly the high-blast-radius case). Only then may the resumed unit of work re-dispatch.

Restoring to committed state is not the same as re-bootstrapping. A correct resume **preserves** committed progress and the board (the release plan is intact, verified slices stay verified); it discards only the *uncommitted* leftovers of the interrupted attempt. "Recovers without corrupting" is the floor; "recovers **cleanly**" — no crash debris surviving into the retry — is the contract, because leftover code contaminates the new attempt's diff and every diff-scanning gate that reads it (the 2026-07-12 dogfood: leftover implementer output tripped a boundary-mock detector on code the retry never wrote).

Any Baton engine that runs concurrent worktrees and supports resume inherits this hazard; the restore is therefore part of the protocol, not an engine detail.

## How to apply

- **Implementers:** prefer scoped mutation (pass an explicit working directory
  to the tool, or use a framework directory/env helper that auto-restores) over
  mutating the ambient process; when you must mutate, pair it with a deferred
  restore. Assert any path/worktree target before acting on it. Cite the
  guarding test or smoke step in `proof.md`.
- **Captains (design review):** scan any design that touches the working
  directory, environment, or worktree selection. Flag any occurrence lacking
  (1) restore, (2) a fail-closed target assertion, and (3) a reachability
  artefact.
- **Verifiers:** the reachability gate must specifically demonstrate the guard
  when the slice's diff touches process-global state.

## When this rule applies

- Any slice that changes the process working directory, environment variables,
  or which worktree/branch a tool operates on.
- Any slice that creates, switches, or removes git worktrees.
- Any test that mutates the working directory, environment, or process
  arguments without a framework-scoped, auto-restoring helper.

## When this rule does NOT apply

- Tests that mutate only framework-scoped state with automatic restore — the
  framework itself is the guard.
- Single-worktree, single-session workflows with no shared process state: the
  failure class does not arise, though the discipline remains good practice.

## Provenance

Codified after a recurring failure class in multi-worktree release harnesses: a
git operation run against a stale or empty directory silently flipped a
worktree to its base branch, and the pattern recurred across slices until the
guard was made a standing design-review check. It composes Rule 9 (design
review flags the unsafe design) with Rules 1/6 (reachability/proof that the
guard fires), specialised onto one high-blast-radius pattern.
