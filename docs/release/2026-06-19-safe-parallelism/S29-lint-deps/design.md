# Design TL;DR: S29-lint-deps

## §1. User-visible change
Running `sworn lint deps <slice-id> <release>` will check if `go.mod` or `go.sum` have changed since the slice's `start_commit` (or a provided base ref). If they have changed but are not listed in the slice's `status.json` `planned_files`, the command will exit with a non-zero status and print an error message naming the undeclared files.

## §2. Design decisions not in spec (max 5)
1. `internal/lint` package will expose a `CheckDeps(sliceDir, baseRef string) error` function. If `baseRef` is empty, it will read `start_commit` from `status.json`. If `start_commit` is null, it will default to `release-wt/<release>`.
2. The git diff will be performed using `git diff --name-only <baseRef>...HEAD` (or similar) to find changed files.
3. The `sworn lint deps` command will accept an optional `--base` flag to override the base ref, which is useful for testing without relying on the slice's `start_commit`.
4. The planner prompt update will be a simple checklist addition in `internal/prompt/planner.md` under the appropriate section.

## §3. Files I'll touch grouped by purpose
- `cmd/sworn/lint.go`: Add the `deps` subcommand to the `lint` command, parsing the slice ID and release, and calling `lint.CheckDeps`.
- `internal/lint/deps.go`: Implement the core logic to read `status.json`, run `git diff`, and check for undeclared `go.mod`/`go.sum` changes.
- `internal/lint/deps_test.go`: Unit/integration tests for `CheckDeps`.
- `internal/prompt/planner.md`: Add the checklist line for planners to auto-add `go.mod`/`go.sum`.

## §4. Things I'm NOT doing
- I am not auto-editing `status.json` to add the missing files.
- I am not validating the contents of `go.mod` or `go.sum`.
- I am not implementing `touchpoints` or `symbols` lint targets.

## §5. Reachability plan
I will create a temporary git repository, initialize a slice with a `status.json` that omits `go.mod`, modify `go.mod`, and run `sworn lint deps` against it to capture the non-zero exit code and error message. This will be documented in `proof.md`.

## §6. Open questions for the Coach
None.