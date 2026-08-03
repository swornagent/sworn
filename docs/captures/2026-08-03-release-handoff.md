# Release handoff, 2026-08-03

Written to resume the fix-plan release in a fresh session. Read this first, then
`2026-08-01-sworn-tui-dogfood-defects.md`, which is the implementation contract.

## The blocker

**No autonomous run can start.** Not on this machine, not anywhere, and the
cause is Sworn's own certification rather than anything about the environment.

`sworn driver certify --profile codex --model gpt-5.6-sol` returns
`native_automation_surface_failed`. Because `nativeAdapter.nativeRuntime`
(`internal/driver/native.go:525`) refuses any invoke without a certificate in its
map, every dispatch then fails with `NATIVE_NOT_CERTIFIED`, surfacing as
`planner_proposal` / `runner_error` / `operational_failure`.

Full investigation, including everything already ruled out, is in **#182**.
Start there. The smoke test passes every hard check — sandbox, broker, tool
digests identical — and fails somewhere in `native_linux.go:1975-2110`.

Nothing else on the release matters until that is fixed.

## What is true right now

- `main` is at `b8bdb0b5`.
- `sworn init` shipped (#180). It sets a project up: `.sworn/` with a
  `.gitignore` excluding everything, `.sworn/runs/`, and a driver configuration
  derived from the installed agent CLI.
- A working driver configuration exists at `.sworn/drivers.json`, gitignored,
  mode 0600, reporting `PASS` / `native_binary_ready` from `sworn driver doctor`.
- A valid run manifest exists at
  `.sworn/runs/2026-08-03-sworn-run-on-ramp.json`.
- PR **#183** removes the pre-flight certification gate. Merge or close before
  resuming; the branch is `fix/no-preflight-certification`.
- The `2026-08-03-native-cli-pin-policy` release is **parked**. Its plan is on
  disk, it has no `release-wt` ref, and its candidate sits unmerged on local
  `track/2026-08-03-native-cli-pin-policy-T1` at `657c28e5`. It failed
  independent verification; see #179.

## How a run is actually driven

Established by reading the engine, after several wrong guesses:

1. Plan with the `baton-plan` skill. Planning never approves itself.
2. Approve. Today that means a comment on the GitHub issue named by the plan's
   `approval_ref`, because approval resolution is hard-coded to the GitHub API.
   That coupling is **#181** and should be removed: approval comes from whoever
   drives the command, person or agent, and belongs in a Baton receipt on the
   release ref.
3. `sworn run --manifest ABS --journal ABS --config ABS`. The engine resolves the
   approval, installs the approved plan itself through
   `authorityInstaller.install`, and dispatches from release status. On a cold
   start it dispatches a `planner_proposal` first.

`sworn run <release>` and bare `sworn run` with a release picker do not exist
yet. That is S5b in #177, and it is what removes the hand-written manifest.

## Traps that cost time today

- **The journal must be mode 0600.** The pre-existing `.sworn/sworn.db` is 644
  and fails as `JOURNAL_UNAVAILABLE`, which does not mention permissions.
- **Certification is enforced in two places.** Removing the runtime gate is not
  enough; the adapter has its own precondition.
- **A driver failure is undiagnosable from the run record.** The adapter's coded
  error is dropped at `internal/runtime/dispatch.go:1813`, becomes
  `runner_error`, then `operational_failure`, then one sentence. Six temporary
  print statements beat the entire run record. Related: #176.
- **`go test ./...` cannot resolve packages** while `scratchpad/` holds a
  vendored Go module. Use `./internal/... ./cmd/sworn/`. Three `TestTwin*` cases
  also fail locally on untracked `scratchpad/` content and pass in CI.

## Suggested order

1. Land or close #183.
2. Fix #182. Nothing autonomous runs until it is done.
3. Prove one real autonomous run using the existing manifest and configuration.
4. Then resume the fix plan: S1 (#169), S2 (#172), S3 (#173), S4 (#176),
   S5b (#177).

#181 and #179 can proceed in parallel; neither blocks a run.
