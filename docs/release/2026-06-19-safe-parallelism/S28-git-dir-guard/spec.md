---
title: 'S28-git-dir-guard — internal/git fails closed on empty Repo.Dir (fixes workers writing to main, sworn#6)'
description: 'internal/git.Repo.run() defaults cmd.Dir to the ambient cwd when Repo.Dir is empty, so a git mutation (Checkout/Branch) runs in the calling worktree and can flip it to main. Make run() return an error on empty Dir, add a regression test, and audit callers. Structural fix for sworn#6.'
---

# Slice: `S28-git-dir-guard`

## User outcome

A `sworn` run that exercises any code path or test touching `internal/git` can
**never** silently operate on the ambient working directory. A `git.Repo` with no
`Dir` set fails loudly instead of running `git checkout main` (or any mutation) in
the calling worktree. This closes the root cause of sworn#6 — a track worker's
commits landing on `main` because a `go test` flipped the worktree.

## Entry point

`internal/git.Repo` construction + every mutating method. Verifiable by: a unit
test that constructs a zero-`Dir` `Repo`, calls a mutating op, asserts it returns
the guard error, and asserts the **current working directory's git HEAD is
unchanged** (proving the op did not touch the ambient repo).

## Background (sworn#6)

`internal/git/git.go`:

```go
func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir   // r.Dir == "" → exec uses the AMBIENT cwd
	...
}
```

`Repo.Checkout("main")` then runs `git checkout main` in whatever dir `go test`
executes in — the track worktree — flipping its branch. Observed 2026-06-21 on
`T8-memory`/`S23` (commit `ec97408` stranded on `main`).

## In scope

### `internal/git/git.go` — fail closed on empty Dir

`run()` returns an error before exec when `r.Dir == ""`:

```go
func (r *Repo) run(args ...string) (string, error) {
	if r.Dir == "" {
		return "", fmt.Errorf("git %s: refusing to run with empty Repo.Dir "+
			"(would operate on the ambient working directory / calling worktree)",
			strings.Join(args, " "))
	}
	...
}
```

(Optionally also reject in `New("")`, but guarding `run()` is the single chokepoint
every method funnels through — guard there at minimum.)

### Audit callers

Grep for `git.New(` and `git.Repo{` across the tree. Any caller that constructs a
`Repo` with an empty/zero `Dir` and *relied* on cwd behaviour must be given an
explicit directory. If none exist, note that in `proof.md` (the guard is then purely
defensive).

## Out of scope

- The harness defence-in-depth guard (coach-loop post-dispatch worktree-branch
  assertion) — already landed separately in `~/.claude/bin/coach-loop` (private
  harness, not this repo). This slice is the in-repo structural fix.
- Broader git-isolation refactors. The minimal fail-closed guard is sufficient.

## Planned touchpoints

- `internal/git/git.go` (add the empty-Dir guard in `run()`)
- `internal/git/git_test.go` (regression test; create if absent)
- any caller found by the audit that passed an empty `Dir` (expected: none)

## Acceptance checks

- [ ] `git.New("").Checkout("main")` (and `.Branch("x")`, `.Commit("m")`) returns a
  non-nil error mentioning empty `Repo.Dir`; the git binary is NOT invoked against cwd
- [ ] the regression test captures the cwd's `git rev-parse HEAD` before and after the
  guarded call and asserts they are identical (the ambient repo is untouched)
- [ ] all existing `internal/git` tests still pass (no caller legitimately relied on
  empty Dir; if one did, it is fixed to pass an explicit dir)
- [ ] `go build ./...` and `go vet ./internal/git/...` pass

## Required tests

- **Unit** `internal/git/git_test.go`:
  - `TestRunRejectsEmptyDir`: `(&git.Repo{}).Checkout("main")` returns the guard error
  - `TestEmptyDirDoesNotTouchCwd`: from a temp git repo set as cwd, call a mutating op
    on a zero-`Dir` Repo; assert the error AND that the temp repo's HEAD/branch is
    unchanged
  - existing happy-path tests continue to pass using an explicit temp `Dir`
- **Reachability artefact**: run the two new tests; capture output showing the guard
  fires and the cwd repo is untouched. Document in `proof.md`.

## Risks

- A legitimate caller relying on the empty-Dir/cwd default would break. The audit step
  exists to find and fix any such caller before the guard lands. Do not skip it.

## Deferrals allowed?

None.
