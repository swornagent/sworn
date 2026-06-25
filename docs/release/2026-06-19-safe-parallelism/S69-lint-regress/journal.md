---
title: 'Journal — S69-lint-regress'
description: 'Session notes for implementing sworn regress.'
---

## 2026-07-15T20:00:00Z — implementation

### State transition: planned → in_progress → implemented

### Decisions

- **testRunner interface for testability.** The `runRegress` internal function accepts a `testRunner` interface; `realRunner` shells out with `exec.Command`, and `mockRunner` (with map-based dispatch) lets unit tests exercise every pass/fail/skip path without touching a real worktree or spawning real processes. This keeps the gate tests fast and deterministic.

- **Three-suite model: Go, TypeScript, Golden fixtures.** The regression runner runs three independent suites: `go test ./...` in the worktree, `pnpm test` if available, and `git diff --exit-code -- **/testdata/**` for golden fixture divergence. Each suite reports its own pass/fail/skip status independently — a failure in one does not prevent the others from running.

- **Graceful TS skip.** TypeScript suite is skipped (not failed) when pnpm is unavailable or no `package.json` exists in the worktree. This avoids a hard dependency on a JS toolchain for a primarily-Go project.

- **Release worktree resolution.** The CLI resolves the release worktree path from `index.md` frontmatter (`release_worktree_path` field), following the same pattern used by `internal/run/parallel.go` and `internal/mcp/tools_ops.go`. No new config surface.

### Trade-offs

- `extractReleaseWorktreePath` duplicates the logic from `internal/run/parallel.go` (which is unexported). Could be pulled into a shared helper if a third consumer appears, but duplication of a 10-line extractor is preferable to premature extraction across package boundaries.

### Touchpoints note

- `cmd/sworn/commands.go` was touched to register the `regress` command. This is not in `planned_files` but is the standard registration touch for every new CLI verb — the command registry file is a documented shared surface.

### Out of scope (per spec)

- Running the test suite per-slice (that's implementer/verifier territory)
- Modifying test configuration