---
title: 'S08b-mcp-ops-tools — MCP operations tools (query + act on running releases)'
description: 'Registers 9 operations tools on the sworn MCP server: get_board, get_blocked, get_slice_context, rerun_slice, patch_slice, approve_merge, defer_slice, get_credits. AI clients can query and act on running releases.'
---

# Slice: `S08b-mcp-ops-tools`

## User outcome

An AI assistant connected to `sworn mcp` can call `get_blocked()` and receive a
complete picture of all blocked slices with their violations, call `get_slice_context()`
to get spec + diff + journal assembled automatically, call `rerun_slice()` to trigger
a re-run after proposing a fix, and see their credit balance — all without the
developer manually navigating worktrees.

## Entry point

`tools/call` on the MCP server (S08a) routing to the registered operations handlers.

## In scope

Nine operations tools registered via `server.RegisterTool()`:

1. **`get_board`** `(release?: string)` → for each release (or the specified one):
   tracks list with state, slices list with state + last_updated_at. Reads from
   `index.md` frontmatter + each slice's `status.json`.

2. **`get_blocked`** `()` → all slices in `failed_verification` or `BLOCKED` state
   across all releases; for each: release, track, slice_id, violations (parsed from
   proof.md), worktree_path.

3. **`get_slice_context`** `(release: string, slice_id: string)` → assembled context
   object: spec_content (full spec.md text), violations (from proof.md), diff (git diff
   from start_commit to HEAD in the track worktree), journal_content (journal.md text).
   This is the full package an AI needs to propose a fix.

4. **`rerun_slice`** `(release: string, slice_id: string)` → resets slice state to
   `in_progress` in status.json; shells out to `sworn run` as a subprocess; returns
   the run's PID and a message. Non-blocking — returns immediately after spawn.

5. **`patch_slice`** `(release: string, slice_id: string, instructions: string)` →
   writes `instructions` to `<worktree>/<slice_dir>/PATCH_INSTRUCTIONS.md`; then calls
   rerun_slice. The implementer agent is expected to read PATCH_INSTRUCTIONS.md.

6. **`approve_merge`** `(release: string, track_id: string)` → validates all track
   slices are in `verified` state; runs the merge-track logic (merges track branch to
   release-wt); returns success or list of unverified slices blocking the merge.

7. **`defer_slice`** `(release: string, slice_id: string, reason: string)` → writes
   state `deferred` to status.json; appends a Rule 2 deferral block to intake.md with
   the provided reason, "Tracking: TBD", and current timestamp as Acknowledged.

8. **`get_credits`** `()` → reads `~/.config/sworn/credits.json` cache; returns
   balance and last_refreshed timestamp; returns `null` if not logged in.

9. **`list_releases`** `()` → scans `docs/release/*/index.md`; returns list of release
   names, slice counts, aggregate states.

- `internal/mcp/context.go`: `AssembleSliceContext(release, sliceID, repoRoot string)
   (ContextResult, error)` — does the heavy lifting for `get_slice_context`: reads
   status.json for start_commit and worktree_path, reads spec.md, parses proof.md for
   violations, runs `git diff <start_commit>..HEAD` in the worktree.

## Out of scope

- Planning tools — create/mutate release artefacts (S08c)
- Resource reads (S08c)
- Prompt resources (S08c)

## Planned touchpoints

- `internal/mcp/tools_ops.go` (new — 9 tool handler functions)
- `internal/mcp/context.go` (new — AssembleSliceContext)
- `internal/mcp/tools_test.go` (new — ops tool tests with fixture releases)

## Acceptance checks

- [ ] `get_board` returns correct track + slice structure for a fixture release
- [ ] `get_blocked` on a fixture with one `failed_verification` slice returns that
  slice's violations (parsed from a fixture proof.md)
- [ ] `get_slice_context` returns non-empty spec_content, violations, and diff for
  a fixture slice with a known start_commit and worktree with uncommitted changes
- [ ] `defer_slice` with reason "blocked on backend" writes `state: deferred` to
  status.json and appends a deferral block to intake.md containing the reason string
- [ ] `get_credits` returns the balance from a fixture `~/.config/sworn/credits.json`;
  returns null (not error) when the file is absent
- [ ] `approve_merge` returns an error listing unverified slices when any slice is not
  `verified`; (happy path merge tested via integration test or deferred to S02b
  testing if merge logic is already covered there)
- [ ] `go test ./internal/mcp/...` covers all 9 tools with fixture data

## Required tests

- **Unit**: `internal/mcp/tools_test.go`
  — `TestGetBoard`: fixture release; assert response structure matches index.md
  — `TestGetBlockedExtractsViolations`: fixture proof.md with known violations;
    assert violations list in response
  — `TestGetSliceContext`: fixture with start_commit + worktree containing a file
    change; assert diff is non-empty and spec_content matches fixture spec.md
  — `TestDeferSliceWritesRuleTwo`: call defer_slice; assert status.json + intake.md
  — `TestGetCreditsAbsent`: no credits file; assert null (not error) in response
- **Reachability artefact**: configure sworn mcp in Claude Code; ask "what's blocked in
  the safe-parallelism release?"; observe AI calls get_blocked and returns the blocked
  slice list. Screengrab or log in proof.md.

## Risks

- `git diff` in `get_slice_context` runs a subprocess from within the MCP server. If
  the worktree path is not a valid git repo, it returns an error. Wrap gracefully:
  return diff as empty string + a note in the context that the diff was unavailable.

## Deferrals allowed?

No. These are the core tools that make MCP useful for resolution.
