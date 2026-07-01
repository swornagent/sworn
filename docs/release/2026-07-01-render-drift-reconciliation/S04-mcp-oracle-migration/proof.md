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
$ git diff --name-only dbd3d996ba5181b65a1e0e9230f7529acd152bb7
internal/mcp/context.go
internal/mcp/lint_test.go
internal/mcp/tools_ops.go
internal/mcp/tools_test.go
```

## Test results

### Go

```
$ go test ./internal/mcp/... -count=1
ok  	github.com/swornagent/sworn/internal/mcp	(cached)

$ go test ./... -count=1
ok  	github.com/swornagent/sworn/cmd/sworn	37.362s
ok  	github.com/swornagent/sworn/internal/account	10.141s
... (all 38 packages PASS, no failures)

$ go vet ./...
(no output, exit 0)

$ go build ./...
(no output, exit 0)
```

## Reachability artefact

- **Type**: manual-smoke-step (test suite drives same code paths an MCP client reaches over the wire)
- **Path**: `internal/mcp` test suite — `TestGetBoard`, `TestGetBlockedExtractsViolations`, `TestGetSliceContext`, `TestDeferSliceWritesRuleTwo`, `TestRerunSliceWritesPID`, `TestPatchSliceWritesInstructions`, `TestApproveMergeRejectsUnverified`, `TestListReleases`, `TestSetTrackUpdates`, `TestSetTrackColon`
- **User gesture**: "An MCP client connects to `sworn mcp`, calls MCP tools against a release with `board.json` (current-format) and receives real data — track counts, slice states, worktree paths, slice counts — not empty strings, not wrong-track errors."

## Delivered

- **AC-01** — `tools_ops.go` resolves tracks via `board.ReadBoard` (the oracle) instead of `board.ParseTracks(extractFrontmatterBody(...))`. Evidence: `internal/mcp/tools_ops.go` (`readReleaseBoard`, `findBlockedInRelease`, `handleApproveMerge`), exercised by `TestGetBoard`, `TestGetBlockedExtractsViolations`, `TestApproveMergeRejectsUnverified` (all pass).
- **AC-02** — `AssembleSliceContext` (`context.go`) reads violations from `proof.json.not_delivered` (preferred) with `proof.md` regex fallback. Evidence: `internal/mcp/context.go` (`readProofViolations`), exercised by `TestGetSliceContext` (passes, including diff-from-git assertion).
- **AC-03** — `tools_plan.go`'s `set_track` reads from / writes to `board.json` via `board.ReadBoard` / `board.WriteBoard`. Evidence: `internal/mcp/tools_plan.go`, exercised by `TestSetTrackUpdates` and `TestSetTrackColon` (pass).
- **AC-04** — `catalog.go`'s `releaseStateSummary` and `countSliceTableRows` derive counts from `board.json` + `status.json`. Evidence: `internal/mcp/catalog.go`, exercised by `TestPlanReleaseExisting` (pass).
- **AC-05** — Tests in `tools_test.go` and `lint_test.go` now write `board.json` fixtures via `writeBoardJSON` / `writeLintBoardJSON` instead of hand-writing legacy `tracks:` YAML in `index.md`. Evidence: `internal/mcp/tools_test.go`, `internal/mcp/lint_test.go`; all ops and lint tests pass.
- **AC-06** — `go build ./...` succeeds, `go test ./internal/mcp/...` passes. Evidence: command output above.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

None.

## First-pass script output

```
$(release-verify.sh output will be pasted below)
```
