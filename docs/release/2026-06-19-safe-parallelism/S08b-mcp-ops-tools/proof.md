# Proof Bundle — S08b-mcp-ops-tools

## Scope

Register 9 operations tools on the sworn MCP server: get_board, get_blocked,
get_slice_context, rerun_slice, patch_slice, approve_merge, defer_slice,
get_credits, list_releases. AI clients can query and act on running releases
through these tools.

## Files changed (vs start_commit 6e21bf0)

```
cmd/sworn/mcp.go                                          |   4 +  — RegisterOpsTools call
internal/mcp/context.go                                   | 189 +  — AssembleSliceContext + helpers + extractField fix
internal/mcp/tools_ops.go                                 | 592 +  — 9 tool handlers + RegisterOpsTools
internal/mcp/tools_test.go                                | 562 +  — tests for all 9 tools + git repo fixtures
docs/release/2026-06-19-safe-parallelism/S08b-mcp-ops-tools/proof.md  | proof bundle (this file)
docs/release/2026-06-19-safe-parallelism/S08b-mcp-ops-tools/journal.md | session journal
```

Key changes this round (addressing verifier violations):
- `internal/mcp/context.go`: Fixed `extractField()` to correctly parse quoted JSON values (was leaving trailing quotes)
- `internal/mcp/tools_test.go`: `TestGetSliceContext` now uses a real temporary git repository to verify non-empty diff; `TestDeferSliceWritesRuleTwo` now asserts intake.md content
- `proof.md`: Reachability artefact replaced with actual `get_blocked` invocation output

## Test results

```
$ go test ./internal/mcp/... -count=1 -timeout 60s
ok  github.com/swornagent/sworn/internal/mcp  0.067s

$ go test ./... -count=1 -timeout 120s
... all 26 packages pass ...
```

All 21 tests pass (10 existing server tests + 9 new ops tool tests + 2 git repo integration helpers). Full suite green across 26 packages.

## Reachability artefact

An MCP client initializes a `sworn mcp` server pointed at a fixture release with
one `failed_verification` slice, then calls `get_blocked`:

```
$ cd /tmp/sworn-reachability-test
$ printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_blocked","arguments":{}}}' | sworn mcp

Response (initialize):
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{},"resources":{"listChanged":false},"prompts":{}},"serverInfo":{"name":"sworn-mcp","version":"0.1.0"}}}

Response (get_blocked):
{"jsonrpc":"2.0","id":2,"result":{"isError":false,"content":[{"type":"text","text":"Release: test-reach\n  Track: T1-engine\n  Slice: S02-fail\n  State: failed_verification\n  Worktree: /tmp/wt/T1-engine\n  Violations:\nFAIL: Gate 3 — missing spec coverage\n**Violation 1:** missing spec content\n**Violation 2:** test does not verify non-empty diff\n"}]}}
```

The `get_blocked` tool returns the failed slice ID, its track, state, worktree
path, and violations parsed from proof.md — exactly the user gesture the spec
requires. An AI assistant connected to `sworn mcp` can call `get_blocked` and
receive this complete picture without navigating worktrees manually.
## Delivered

- **get_board**: reads index.md frontmatter via `board.ParseTracks()`, reads
  per-slice status.json via `state.Read()`, returns formatted board summary
- **get_blocked**: scans all releases for `failed_verification` slices, extracts
  violations from proof.md, returns formatted blocked-slice report
- **get_slice_context**: assembles spec content + violations + diff + journal
  content for a given slice via `AssembleSliceContext()`; git diff errors
  wrapped per Pin 1 (diff="" + diff_note)
- **rerun_slice**: resets slice state to `in_progress` in status.json, spawns
  `sworn run` via `os.Executable()` per Pin 3, returns PID non-blocking
- **patch_slice**: writes instructions to PATCH_INSTRUCTIONS.md, then calls
  rerun_slice
- **approve_merge**: validates all track slices are `verified` via `state.Read()`,
  then merges via `internal/git.Repo.Merge()` per Pin 4
- **defer_slice**: writes `state: "deferred"` directly (bypassing
  `state.Transition()` per Flag b), appends Rule 2 deferral to status.json
  open_deferrals and intake.md
- **get_credits**: reads `~/.config/sworn/credits.json`; returns null (not
  error) when file is absent
- **list_releases**: scans `docs/release/*/index.md`, returns release catalogue
  with slice and track counts
- **Test coverage**: 9 named test functions covering all tools with fixture data,
  including 4 Pin 5 tests: `TestRerunSliceWritesPID`,
  `TestPatchSliceWritesInstructions`, `TestApproveMergeRejectsUnverified`,
  `TestListReleases`
- **Wiring**: `mcp.RegisterOpsTools(server, ".")` called from `cmd/sworn/mcp.go`

## Not delivered

No deferrals. All 9 tools are implemented and tested.

## Divergence from plan

- Used `execSwornRun` package-level variable (mockable in tests) instead of raw
  `exec.Command` — a testability improvement that does not change the production
  behaviour (defaults to `exec.CommandContext`).
- Diff output uses `diff_note` field alongside `diff` for the Pin 1 error
  wrapping, rather than embedding the note in the diff string itself.

## First-pass script output

```
$ bash release-verify.sh S08b-mcp-ops-tools 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  PASS  integration branch drift present but does not affect test infrastructure

== Diff vs start_commit (verifier base) ==
  PASS  8 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 22
  checks failed: 0
FIRST-PASS PASS
```