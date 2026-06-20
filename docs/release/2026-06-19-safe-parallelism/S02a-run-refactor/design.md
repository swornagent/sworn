# Design TL;DR — S02a-run-refactor

## §1. User-visible change

No user-facing change. The `sworn run <task>` command produces identical behaviour; the public surface is unchanged. Internally, the implement→verify retry loop is extracted into an exported `RunSlice()` function that a future goroutine worker (S02b) can call with an existing worktree and spec path, without creating branches or release directories.

## §2. Design decisions not in spec (max 5)

1. **RunSlice reads startCommit from status.json, not from a parameter.** The current `Run()` captures `startCommit` at setup and writes it to `status.json` (lines 183-194). `RunSlice()` re-reads it from status.json on each iteration (same line 287 pattern in current code). This avoids threading `startCommit` through the options struct or parameter list — both `Run()` and `RunSlice()` agree on the canonical source.

2. **RunSlice does not accept a `Task` or `Base` option.** These are setup-level concerns (branch creation, release dir naming). `RunSlice()` assumes the worktree exists and the spec/status are already on disk. Removing them from `RunSliceOptions` makes the boundary between setup (Run) and execution (RunSlice) explicit.

3. **RunSlice returns `error` only — the caller inspects status.json for state.** On success (verifier PASS), `RunSlice` transitions status.json to `verified` and returns nil. The caller (`Run()`) reads `status.json` to confirm state before merging. This avoids coupling RunSlice's return type to the verdict package.

4. **The verifier diff is still written to a temp file by RunSlice.** The current code creates a temp file for the diff patch (lines 280-283) before calling `verify.Run`. Since `RunSlice` handles the full implement→commit→diff→verify cycle, it owns this temp file lifecycle. No need to change the diff delivery mechanism.

5. **`RunSlice` does not introduce concurrency — it is the callable unit.** The function itself runs sequentially (implement → commit → diff → verify → verdict). The race detector check (AC-6) verifies that no shared mutable state (globals, unguarded maps) exists in `slice.go` that would break under concurrent callers. The existing `git.Repo` is documented as not goroutine-safe, but that's a caller concern (S02b gives each worker its own `git.Repo`).

## §3. Files I'll touch grouped by purpose

| File | Purpose |
|------|---------|
| `internal/run/run.go` | Refactor `Run()` — extract lines 196-358 (the implement→verify loop after escalation list building) into a call to `RunSlice()`, keeping setup (branch, release dir, start_commit) and post-merge in `Run()`. |
| `internal/run/slice.go` (new) | `RunSliceOptions` struct + `RunSlice(ctx, worktreeRoot, specPath, statusPath, opts) error`. Contains the retry loop, `implement.Run` call, agent commit, diff computation, `verify.Run` call, verdict handling, and state transitions to `verified`/`failed_verification`. |
| `internal/run/run_test.go` | Existing tests must pass unchanged. Add `TestRunSlice` (mock→PASS path) and `TestRunSliceFail` (mock→FAIL path). No existing test fixture or assertion changes needed — `TestRun_PassPath_Merges` etc. all go through the refactored `Run()` which internally calls `RunSlice()`. |

## §4. Things I'm NOT doing

- Not creating a `--parallel` flag or scheduler (S02b owns that).
- Not reading release boards or discovering tracks (S02b).
- Not materialising worktrees (S02b).
- Not altering `verify.Run`'s signature or behaviour.
- Not changing `implement.Run`'s signature or behaviour.
- Not changing the `model.Verifier` or `agent.Agent` interfaces.
- Not touching `internal/supervisor/`, `internal/db/`, `internal/git/`, `cmd/sworn/`.
- Not adding any new dependencies — stdlib + existing internal packages only.
- Not changing the `state.State` machine or adding new states.

## §5. Reachability plan

**Artefact type**: test output.
**Path**: `cd <worktree> && go test -race ./internal/run/...` output captured in proof.md.
**Coverage**: All 8 existing tests + 2 new tests pass. The race detector passes. The `TestRun_*` tests prove `Run()` still works end-to-end after refactoring; `TestRunSlice` and `TestRunSliceFail` prove the extracted function works when called directly with a mock worktree.

## §6. Open questions for the Coach

None.