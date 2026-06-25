# Design TL;DR — S36-captain-resolve-dirty-worktree

## §1. User-visible change

The Captain gains a new function `resolve-dirty-worktree`. When the coach-loop reaches a gate requiring a clean worktree (pre-merge, pre-forward-sync, pre-replan) and finds the worktree dirty, it no longer pages the Coach. Instead, the Captain assesses the uncommitted diff, **commits the work by default** (preserving worker progress), discards **only if clearly wrong** (stray build artefacts, accidental mass-deletion), and records — in the slice journal — what changed, which files, and why. The Coach is informed via the durable journal note, never via a blocking page. The only Coach escalation is the genuinely ambiguous case: a diff mixing plausible work with destructive changes where the right split is unclear.

## §2. Design decisions not in spec

1. **Function format matches existing Captain functions.** The `resolve-dirty-worktree` section follows the same structure as `/design-review` and `/replan-release` — a trigger description, inputs-loaded block, stepwise procedure, output specification, and session-end commit discipline. Rationale: consistency; implementers reading captain.md already navigate this format.

2. **Discard threshold is conservative.** Discard only on: (a) stray build artefacts (`sworn` binary, `node_modules` drift), (b) accidental mass-deletion touching files outside slice touchpoints, (c) edits to files outside the slice's declared touchpoints with no coherent intent. Everything else commits. Rationale: the spec's Risk section explicitly prefers committing — the Verifier still runs and will FAIL bad code, whereas discarding good work is irreversible.

3. **Detector specification lives in captain.md as a contract.** The loop's actual gate code (bash harness or `sworn run`) is not in this repo's scope; captain.md defines the contract it must satisfy: `git status --porcelain` at the gate point, non-empty → dispatch Captain's `resolve-dirty-worktree`.

4. **Journal entry format is prescriptive.** The Captain must record: impacted files (the `git status --porcelain` output), a one-line diff characterisation, the decision (committed/discarded/escalated), and the rationale. This is specified inline in the function.

## §3. Files I'll touch grouped by purpose

- **`internal/prompt/captain.md`** — add the `resolve-dirty-worktree` function section (after `/replan-release`, before `Failure modes to avoid`). This is the core deliverable — the contract the coach-loop dispatches and the Coach can audit.
- **`internal/prompt/prompt_test.go`** — add a test verifying `Captain()` contains the `resolve-dirty-worktree` function name and its commit-by-default rule. Golden-check pattern, consistent with existing tests for `Verifier()` and `VerifyStateless()`.

## §4. Things I'm NOT doing

- **Not writing the loop's clean-worktree gate dispatch code.** The bash `coach-loop` and `sworn run` orchestration live outside this repo's scope. The captain.md specifies the detector contract; wiring is tracked separately.
- **Not modifying any Go orchestration code** (internal/run, internal/verify, cmd/sworn). This is a prompt-only change.
- **Not handling merge conflicts or wrong-branch recovery** — those are distinct failure modes already owned by merge-track (S34) and S28 respectively.

## §5. Reachability plan

- `go test ./internal/prompt/...` — will pass, with new test asserting `Captain()` contains `resolve-dirty-worktree` and `commit by default`.
- `go build ./...` — binary compiles with updated embedded captain prompt.
- Walkthrough artefact: a documented manual exercise (recorded in proof.md) — dirty a scratch worktree, simulate the resolution path by reading the captain prompt's instructions, confirm the decision flow produces a commit/journal entry with no page.

## §6. Open questions for the Coach

*(None — the spec is clear on all points.)*