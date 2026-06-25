# Design TL;DR: S28-git-dir-guard

## §1. User-visible change

Every `git.Repo` method (Checkout, Branch, Commit, Merge, Stage, RevParse, DiffRange, Init) will fail with a descriptive error when `Dir` is empty — before executing the git binary. Callers that always pass an explicit directory (all three production callers audited) see no change. The regression test proves the ambient working directory is untouched by a zero-`Dir` call.

## §2. Design decisions not in spec (max 5)

1. **Guard in `run()` not `New()`** — `run()` is the single chokepoint every method funnels through, so one check covers all mutation paths without changing the constructor API. Spec lists this as optional; I'm making it the sole location.
2. **Error message includes the git args** — so the operator sees *which* operation was blocked (e.g. `git checkout main: refusing to run with empty Repo.Dir`), not just a generic error.
3. **Test uses `setupRepo`-style temp dir as cwd** — the regression test `TestEmptyDirDoesNotTouchCwd` creates a temp git repo, `os.Chdir`s into it, then calls a mutating op on a zero-`Dir` `Repo`. Captures `git rev-parse HEAD` before/after to prove the temp repo is untouched.
4. **No callers need fixing** — audit found zero production callers passing empty `Dir`. All three callers (`internal/run/slice.go`, `internal/run/run.go`, `internal/implement/implement.go`) pass concrete `worktreeRoot`/`workspaceRoot` paths.
5. **Return error not panic** — consistent with the package's existing error-return pattern; caller decides how to handle (typically log + abort in the run-loop).

## §3. Files I'll touch grouped by purpose

- **`internal/git/git.go`** — add the empty-Dir guard in `run()` (3 lines). Core change.
- **`internal/git/git_test.go`** — add two new tests (`TestRunRejectsEmptyDir`, `TestEmptyDirDoesNotTouchCwd`). Regression coverage.

## §4. Things I'm NOT doing

- Not adding a guard in `New()` — `run()` is the single chokepoint; redundant check adds no safety.
- Not changing the `Repo` struct or constructor signature — pure additive change.
- Not adding a panic-based guard (e.g., `if r.Dir == "" { panic(...) }`) — package returns errors, not panics.
- Not fixing any caller — audit found none broken.
- Not fixing the harness defence-in-depth guard — that's in `~/.claude/bin/coach-loop` (separate, private, already landed).

## §5. Reachability plan

- **Artefact**: `go test -v -run 'TestRunRejectsEmptyDir|TestEmptyDirDoesNotTouchCwd' ./internal/git/...` output captured in `proof.md`.
- **Secondary**: `go build ./...` and `go vet ./internal/git/...` pass output.
- **Existing test suite**: `go test ./internal/git/...` full output (all existing tests still pass).

## §6. Open questions for the Coach

None.