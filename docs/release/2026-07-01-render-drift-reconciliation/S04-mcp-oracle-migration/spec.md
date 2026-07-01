# S04-mcp-oracle-migration

> The machine-readable contract for this slice lives at
> `docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/spec.json`.
> This file mirrors its `user_outcome`, `covers_needs`, and `acceptance_criteria`
> in human-readable form so the deterministic first-pass script
> (`scripts/release-verify.sh`) — which still expects `spec.md` — has
> something to point at.

## User outcome

Every MCP tool an agent uses to read or act on release state (board reads,
get_blocked, get_slice_context, approve_merge, plan mutation, catalog counts)
returns real data for a current-format release instead of silently-empty
results or a wrong-track error.

## Covers needs

- N-01

## Acceptance criteria

- **AC-01** — When any MCP tool in `tools_ops.go` (board read, get_blocked,
  approve_merge) is invoked against a release with a committed `board.json`,
  it SHALL resolve tracks via the `internal/board` oracle instead of
  `board.ParseTracks(extractFrontmatterBody(...))` on raw `index.md`.
- **AC-02** — When `AssembleSliceContext` (`context.go`) resolves a slice's
  worktree path or violations, it SHALL read them from `board.json` (via the
  oracle) and `proof.json.not_delivered` respectively, not from `index.md`
  frontmatter or a `proof.md` regex scrape.
- **AC-03** — If `tools_plan.go`'s plan-mutation path updates a release's
  tracks, it SHALL read the existing tracks from `board.json` (via the
  oracle) before mutating and writing back — it SHALL NOT read from the stale
  frontmatter parse, which for any current-format release returns zero tracks
  and would silently wipe the board's track data on write.
- **AC-04** — When `catalog.go`'s `releaseStateSummary` / `countSliceTableRows`
  count slices for a release, they SHALL derive counts from
  `board.json` / `status.json` data (via the oracle) instead of grepping for
  a Markdown table header literal that does not match
  `internal/board/render.go`'s actual output.
- **AC-05** — Every test in `tools_test.go`, `lint_test.go`, and
  `catalog_test.go` that currently hand-writes an `index.md` fixture with the
  legacy `tracks:` YAML shape SHALL be regenerated to build its fixture via
  the real sworn render path, proving these MCP tools work against what the
  renderer actually produces.
- **AC-06** — After the change, `go build ./...` SHALL succeed and
  `go test ./internal/mcp/...` SHALL pass.
