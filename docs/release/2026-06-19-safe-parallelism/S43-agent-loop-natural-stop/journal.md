# S43-agent-loop-natural-stop — Implementation Journal

## 2026-06-23 — Session start

- Slice state: planned → in_progress.
- Anchor: S42 in T12-harness-hardening verified; S43 is next.
- Start commit: `d4f6729` (HEAD of track branch).
- Worktree: `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T12-harness-hardening`.
- Plan: change `agent.Run` to return on any turn with no tool calls (content may be empty), keep `MaxTurns` cap for non-terminating tool-call loops, add tests, document in `implement.Run` that the agent's prose is optional because the diff/test output is the artifact.

## Design trade-off captured

A model that returns empty content before doing useful work now terminates early with a thin or empty diff. This is acceptable because downstream `verify.Run` evaluates the actual diff and tests; an empty diff will FAIL and the escalation loop advances. The previous behavior (spin to `MaxTurns` and error) discarded potentially good work and forced a blind escalation, so the new behavior is strictly better for the common case where the model did the work and then stopped silently.
