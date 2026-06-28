# Journal: S21-sworn-run-task

## 2026-07-28 — Implementation session

### Decisions

- **New file `cmd/sworn/task.go`**: Chose to create a new file rather than heavily modifying `cmd/sworn/run.go` to avoid collision with T1 S07 (which owns the run loop). The `cmdRun` function delegates to `cmdRunTask` for the single-slice `--task` path.
- **`--dry-run` short-circuit**: Added early in `cmdRun` (before config/model loading) so the dry-run path is reachable without a configured model. Dry-run prints "planner dispatch would be called" and exits 0.
- **Planner dispatch via `model.Verify()`**: Used `model.Verify()` (not `model.Chat()`) for the single-shot planner dispatch. This works with any OAI-compatible driver — no Chat capability required.
- **Directory structure**: Created under `.sworn/task-runs/<timestamp>/S01-task/` — ephemeral by default. A git repo is initialized in the task-runs directory for RunSlice's diff computation.
- **Git Config method**: Added `Config()` method to `internal/git/git.go` for setting `user.email`/`user.name` in the ephemeral task-run repo.

### Trade-offs

- The `resolvePlannerModel` fallback uses a hardcoded `openai/gpt-4o` when no model is configured. This avoids a hard crash but means the model choice may not match the user's actual provider setup.
- The spec extraction (`extractSpecFromReply`) handles several common output formats but is not exhaustive — a planner that returns an unusual format may produce a degraded result.
- `--base` flag is still accepted but ignored for `--task` mode (task runs are ephemeral and don't merge anywhere).

### Subagent dispatches

None — implemented directly.