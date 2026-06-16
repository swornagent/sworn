# Design TL;DR — S07-run-loop

## §1. User-visible change

`sworn run --task "<description>"` becomes the turnkey entry point. A developer
describes what to build in plain language, provides model credentials via
environment variables, and the binary sequences implementation (agentic tool
loop → proof bundle → `implemented` state), adversarial verification
(fresh-context, fail-closed), and gated merge. On verification failure it
automatically retries with a progressively more capable (and expensive)
implementer model — nano → mini → 4.1 → 4o → o3-mini → o3 — up to a
configurable cap, then surfaces the blocking verdict to the human. Unverified
work is structurally incapable of merging: the merge gate checks
`state == verified` directly.

## §2. Design decisions not in spec (max 5)

1. **Single-slice, auto-generated release structure.** v0.1 takes a task string
   (not a pre-authored spec.md). The run loop creates a minimal slice directory
   under `docs/release/<run-YYYYMMDD-HHMMSS>/S01-task/` with an auto-generated
   spec.md and status.json, then operates on it. The user doesn't manage slice
   folders — `sworn run` owns the lifecycle end-to-end. Rationale: the spec says
   "v0.1 takes a single task/spec"; auto-generation is the simplest path to no
   prior setup.

2. **Model escalation is implementer-only.** The verifier model stays fixed
   across retries (the adversarial property requires a capable model; downgrading
   it would weaken the gate). Only the implementer model escalates. Rationale:
   the implementer is the creative engine that benefits from more capability on
   retry; the verifier is a judgement engine that must remain strong.

3. **Merge target is configurable via `--base` flag (default: `main`).** The run
   loop creates a feature branch off the base, implements on it, and on PASS
   merges back. The branch name is auto-derived from the task. Rationale: running
   on the current branch and merging into `main` is the most common
   CI/GitHub-flow pattern; hardcoding "main" with an override flag covers both
   local dev and CI.

4. **Retry resets implementation state, not cumulative.** Each retry re-runs
   `implement.Run()` from scratch (state reset to `in_progress`). The escalated
   model gets the same spec and workspace — not the prior model's output as
   context. Rationale: a fresh start avoids compounding a bad approach; the
   escalated model's stronger reasoning is what breaks the deadlock, not
   iterative refinement of a flawed first attempt.

5. **Branch naming:** `sworn/<sanitised-task-slug>`. Dashes and lowercase
   alphanumeric only, truncated to 50 chars. Rationale: predictable,
   human-readable, and won't collide with release-wt/track branches.

## §3. Files I'll touch grouped by purpose

- **Orchestration engine** (`internal/run/run.go`, `internal/run/run_test.go`) —
  new package. The core loop: setup slice, call implement, diff, call verify,
  handle verdict, retry/escalate, merge.
- **CLI surface** (`cmd/sworn/run.go`) — new file. Flag parsing, model
  resolution from env, dispatch to `internal/run`.
- **Command dispatch** (`cmd/sworn/main.go`) — add `"run"` case to the switch.

## §4. Things I'm NOT doing

- TUI (`sworn top`) — explicit out-of-scope in spec.
- Multi-slice planning — explicit out-of-scope in spec.
- Reading/writing `index.md` for the auto-generated release — the auto-generated
  release is ephemeral and single-slice; no board tracking needed.
- Auto-selecting the verifier model — the user must provide `--verifier-model`
  (fail-closed: no model = no verification = no merge).
- Cost tracking across retries in the CLI output — cost is available per-verdict
  in the result struct but not accumulated in the user-facing output for v0.1.

## §5. Reachability plan

**Integration test** at `internal/run/run_test.go` exercises the full PASS and
FAIL paths end-to-end with fake agents/verifiers:
- PASS path: fake implementer creates a file, fake verifier returns "PASS" →
  assert merge commit exists on base branch, assert state is `verified`.
- FAIL path: fake verifier returns "FAIL" × 3 → assert no merge, assert exit
  with escalation message.
- FAIL-then-PASS path: first verifier returns "FAIL", second returns "PASS" →
  assert merge after retry.

These are Go tests (`go test ./internal/run/`) that run without network/model
calls.

## §6. Open questions for the Coach

*(empty)*