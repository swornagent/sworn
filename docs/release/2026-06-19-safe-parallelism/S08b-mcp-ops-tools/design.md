# Design TL;DR — S08b-mcp-ops-tools

## §1. User-visible change

An AI assistant (Claude Code, Codex, Cline) connected to `sworn mcp` can call 9
operations tools to introspect and act on running releases without the developer
manually navigating worktrees: `get_board` (full release structure), `get_blocked`
(all stuck slices with violations), `get_slice_context` (spec + diff + journal),
`rerun_slice` (re-trigger a failed slice), `patch_slice` (instruct + re-run),
`approve_merge` (merge a verified track), `defer_slice` (formal deferral),
`get_credits` (credit balance), and `list_releases` (release catalogue).

## §2. Design decisions not in spec (max 5)

1. **Wiring: `cmd/sworn/mcp.go` registers ops tools before `server.Run()`.** The
   slice spec says "tools/call routing to registered handlers." `cmdMcp()` currently
   creates a bare `mcp.New()` with zero tools. I'll add `RegisterOpsTools(s)`
   calls inside `cmdMcp()` before `server.Run()`. This keeps tool registration
   visible in the entry point and follows the S08a pattern of keeping the server
   package transport-agnostic.

2. **`get_board` reads index.md + per-slice status.json via filesystem, not a DB.**
   The spec says "reads from index.md frontmatter + each slice's status.json." S08b
   does not duplicate the release-board oracle from `internal/run/` — it reads the
   YAML frontmatter of `docs/release/<name>/index.md` (for track-level structure)
   and each slice's `status.json` (for per-slice state). This keeps the MCP server
   stateless: no DB dependency, no schema coupling, and total correctness as all
   reads hit the authoritative files on the release-wt branch.

3. **`get_blocked` parses violations from `proof.md` with a simple regex/section
   scan.** The spec says "parsed from proof.md." The verifier's standard output
   format for FAIL is `FAIL: <numbered concrete violations>` — each on its own
   line at the start of proof.md. I'll scan for `FAIL:` lines and `**Violation
   <N>:**` section markers. If proof.md is absent or empty, the function returns
   an empty violations list (the slice isn't failed yet).

4. **`get_slice_context` resolves the worktree path from status.json, not
   index.md.** The spec says "reads status.json for start_commit and worktree_path."
   But worktree_path lives in `index.md` (frontmatter), not `status.json`. The
   design: `AssembleSliceContext()` looks up status.json for `start_commit` and
   `spec_path`, then reads the release's `index.md` frontmatter to find the
   slice's track's `worktree_path`. The diff runs `git -C <worktree_path> diff
   <start_commit>..HEAD`.

5. **`rerun_slice` uses `exec.Command("sworn", "run", ...)` not an in-process
   call.** The spec says "shells out to `sworn run` as a subprocess." This gives
   the subprocess its own stdout/stderr, its own model config, and isolation from
   the MCP server's lifecycle. The function returns immediately with a PID — the
   caller polls via `get_board` to see state transitions.

## §3. Files I'll touch grouped by purpose

| File | Why |
|---|---|
| `internal/mcp/tools_ops.go` (new) | 9 tool handler functions + `RegisterOpsTools(server)` entry point |
| `internal/mcp/context.go` (new) | `AssembleSliceContext()` — the heavy lifter for `get_slice_context` |
| `internal/mcp/tools_test.go` (new) | Test coverage for all 9 tools with fixture releases |
| `cmd/sworn/mcp.go` (modify) | Add `RegisterOpsTools()` call before `server.Run()` |

## §4. Things I'm NOT doing

- **Not writing planning tools** (S08c owns those).
- **Not implementing resource reads or prompt resources** (S08c).
- **Not building a proper YAML parser** for index.md frontmatter — I'll use a
  simple `---` delimiter + `gopkg.in/yaml.v3` (already in go.sum from T1) for
  frontmatter parsing. If `yaml.v3` is not vendored, I'll use a lightweight
  key-value scan (the frontmatter is small and predictable).
- **Not making `rerun_slice` block until completion** — spec says non-blocking,
  returns PID.
- **Not adding a DB dependency** — all reads hit filesystem + git subprocesses.
- **Not resolving the sworn binary path** — assumes `sworn` is on `$PATH` for
  `rerun_slice`; the caller (developer) is responsible for ensuring this.

## §5. Reachability plan

1. `go test ./internal/mcp/...` — all 9 tools tested with fixture data
2. `go build -o sworn . && echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' | ./sworn mcp` — start the MCP server, then send `tools/list` to see all 9 ops tools registered
3. Screenshot or log showing an AI assistant (Claude Code) calling `get_blocked` on the safe-parallelism release and receiving the blocked slice list

## §6. Open questions for the Coach

- **`internal/mcp/context.go`**: The spec says "reads status.json for... worktree_path" but worktree_path is in index.md frontmatter, not status.json. My design resolves this by reading index.md (see §2 item 4). OK?
- **YAML dependency**: `gopkg.in/yaml.v3` — I checked go.sum and it's already present. If the Coach prefers no new import (stdlib-only constraint), I'll scan `---` delimited frontmatter manually. Which approach?
- **`approve_merge` logic**: The spec says "validates all track slices are in verified state; runs the merge-track logic." S02b's merge logic (`mergeTrack` in `internal/run/parallel.go`) lives in the run package behind `track/` branch operations with git worktree orchestration. `approve_merge` as an MCP tool would need to replicate this. Should it shell out to `sworn run --merge-track` (once that subcommand exists), or should I implement a lightweight in-process `verifyAllVerifiedAndMerge()` that does the git operations directly?