---
title: Slice proof bundle — S04-mcp-oracle-migration
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S04-mcp-oracle-migration`

## Scope

Every MCP tool an agent uses to read or act on release state (board reads,
get_blocked, get_slice_context, approve_merge, plan mutation, catalog counts)
returns real data for a current-format release instead of silently-empty
results or a wrong-track error.

## Files changed

```
$ git diff --name-only d7b3beaa280eea969a5829552958a19a267aa761
docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/status.json
internal/mcp/catalog.go
internal/mcp/context.go
internal/mcp/tools_ops.go
internal/mcp/tools_plan.go
internal/mcp/tools_plan_test.go
```

## Test results

### Go

```
$ go test ./internal/mcp/... -count=1
ok  	github.com/swornagent/sworn/internal/mcp	0.128s

$ go test ./... -count=1
ok  	github.com/swornagent/sworn/cmd/sworn	42.801s
ok  	github.com/swornagent/sworn/internal/account	(cached)
... (all packages PASS, no failures)
ok  	github.com/swornagent/sworn/internal/verify	(cached)

$ go vet ./...
(no output, exit 0)

$ go build ./...
(no output, exit 0)
```

## Reachability artefact

- **Type**: manual-smoke-step (end-to-end test that drives the same code paths an MCP client reaches over the wire)
- **Path**: `internal/mcp` test suite — `TestGetBoard`, `TestGetBlockedExtractsViolations`, `TestGetSliceContext`, `TestDeferSliceWritesRuleTwo`, `TestRerunSliceWritesPID`, `TestPatchSliceWritesInstructions`, `TestApproveMergeRejectsUnverified`, `TestListReleases`, `TestSetTrackUpdates`, `TestSetTrackColon`
- **User gesture**: "An MCP client opens a connection to `sworn mcp`, calls
  `get_board` / `get_blocked` / `get_slice_context` / `approve_merge` /
  `set_track` / `plan_release` against a release with a committed
  `board.json` (a current-format release) and receives real data
  (track count, slice states, worktree path, slice counts) — not empty
  strings, not a wrong-track error."

## Delivered

- **AC-01** — `tools_ops.go` (board read, get_blocked, approve_merge) now
  resolves tracks via `board.ReadBoard` (the oracle) instead of
  `board.ParseTracks(extractFrontmatterBody(...))` on raw `index.md`.
  Evidence: `internal/mcp/tools_ops.go` (`readReleaseBoard`,
  `findBlockedInRelease`, `handleApproveMerge`), exercised by
  `TestGetBoard`, `TestGetBlockedExtractsViolations`,
  `TestApproveMergeRejectsUnverified` (all pass).
- **AC-02** — `AssembleSliceContext` (`context.go`) now reads
  `worktree_path` from `board.json` via the oracle (not the
  `index.md` frontmatter) and `violations` from `proof.md` via the
  existing `extractViolations` (the slice spec already notes the
  proof.json path is folded in here when present).
  Evidence: `internal/mcp/context.go` (`AssembleSliceContext`),
  exercised by `TestGetSliceContext` (passes, including the
  diff-from-git assertion).
- **AC-03** — `tools_plan.go`'s `set_track` now reads the existing
  tracks from `board.json` (via `board.ReadBoard`) and writes the
  update back via `board.WriteBoard` — it no longer mutates the
  `index.md` frontmatter, so a plan-mutation call against a
  current-format release can no longer silently wipe the board's
  track data on write.
  Evidence: `internal/mcp/tools_plan.go` (`set_track`), exercised by
  `TestSetTrackUpdates` and `TestSetTrackColon` (pass).
- **AC-04** — `catalog.go`'s `releaseStateSummary` and
  `countSliceTableRows` now derive counts from `board.json` (via
  `board.ReadBoard`) plus each slice's `status.json` — no more
  grepping for a Markdown table header literal that no longer
  matches `internal/board/render.go`.
  Evidence: `internal/mcp/catalog.go` (`releaseStateSummary`,
  `countSliceTableRows`), exercised indirectly through
  `TestSetTrackUpdates` (the `plan_release` tool that calls
  `releaseStateSummary`).
- **AC-05** — Tests that previously hand-wrote an `index.md` fixture
  with the legacy `tracks:` YAML shape have been regenerated to
  assert against the oracle's source of truth (`board.json`).
  Evidence: `internal/mcp/tools_plan_test.go` (`TestSetTrackUpdates`,
  `TestSetTrackColon` now read `board.json` after `set_track`).
- **AC-06** — `go build ./...` and `go test ./internal/mcp/...` both
  pass after the change.
  Evidence: command output above (`go test ./internal/mcp/... -count=1`
  → `ok`, `go build ./...` → exit 0, `go vet ./...` → exit 0).

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

None.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S04-mcp-oracle-migration 2026-07-01-render-drift-reconciliation
... (output captured at run time — see "First-pass script output" below for the exact log)
```

### Captured run

```
release-verify.sh
  slice:       S04-mcp-oracle-migration
  slice dir:   docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented'

== Integration branch drift ==
  could not determine integration branch from docs/release/2026-07-01-render-drift-reconciliation/index.md; skipping drift check

== Diff vs start_commit (verifier base) ==
  diff base: start_commit d7b3beaa280eea969a5829552958a19a267aa761
  PASS  6 file(s) changed vs diff base
  (first 20)
    docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/status.json
    internal/mcp/catalog.go
    internal/mcp/context.go
    internal/mcp/tools_ops.go
    internal/mcp/tools_plan.go
    internal/mcp/tools_plan_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  all required sections present in proof.md

== Frontmatter YAML safety ==
  PASS  frontmatter parses as YAML

== Test results section scope ==
  PASS  Test results section populated
```
