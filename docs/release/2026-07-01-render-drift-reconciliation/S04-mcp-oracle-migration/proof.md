---
title: Slice proof bundle — S04-mcp-oracle-migration
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `S04-mcp-oracle-migration`

Rendered from `proof.json` (proof-v1). Third implementation pass — addresses the
second-pass verifier FAIL (proof.md fallback removal + proof.json.not_delivered
test coverage).

## Scope

Every MCP tool an agent uses to read or act on release state (board reads,
get_blocked, get_slice_context, approve_merge, plan mutation, catalog counts)
returns real data for a current-format release instead of silently-empty
results or a wrong-track error.

## Files changed

```
$ git diff --name-only dbd3d9900faf00cb96d7ef1acf6d6f89fc59b901
docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/journal.md
docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/proof.md
docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/status.json
internal/mcp/context.go
internal/mcp/lint_test.go
internal/mcp/tools_ops.go
internal/mcp/tools_test.go
```

(`proof.json` and this regenerated `proof.md` land with the bundle commit.)

## Test results

### Go

```
$ go test ./internal/mcp/... -count=1
ok  	github.com/swornagent/sworn/internal/mcp	0.102s

$ go test ./... -count=1 -timeout 600s
ok — all 39 test packages PASS, 0 failures
(cmd/sworn 34.757s, internal/account 10.124s, internal/mcp 0.151s, ...;
 only internal/baton/schemas and internal/verdict have no test files)

$ go vet ./...
(no output, exit 0)

$ go build ./...
(no output, exit 0)
```

## Reachability artefact

- **Type**: manual-smoke-step (test suite drives the same code paths an MCP client reaches over the wire)
- **Path**: `internal/mcp` test suite — `TestGetBoard`, `TestGetBlockedExtractsViolations`, `TestGetSliceContext`, `TestDeferSliceWritesRuleTwo`, `TestRerunSliceWritesPID`, `TestPatchSliceWritesInstructions`, `TestApproveMergeRejectsUnverified`, `TestListReleases`, `TestSetTrackUpdates`, `TestSetTrackColon`
- **User gesture**: "An MCP client connects to `sworn mcp`, calls `get_blocked` / `get_slice_context` against a release whose failing slice carries a proof-v1 `proof.json`, and receives the slice's violations from `proof.json.not_delivered` — while a stray legacy `proof.md` in the same slice dir is ignored entirely."

## Delivered

- **AC-01** — `tools_ops.go` resolves tracks via `board.ReadBoard` (the oracle) instead of `board.ParseTracks(extractFrontmatterBody(...))`. Evidence: `internal/mcp/tools_ops.go` (`readReleaseBoard`, `findBlockedInRelease`, `handleApproveMerge`, `handleListReleases`), exercised by `TestGetBoard`, `TestGetBlockedExtractsViolations`, `TestApproveMergeRejectsUnverified`, `TestListReleases` (all pass).
- **AC-02** — `AssembleSliceContext` (`context.go`) resolves the worktree path via `board.ReadBoard` and reads violations **exclusively** from `proof.json.not_delivered`: `readProofViolations` has no `proof.md` fallback and the `extractViolations` regex scraper is deleted as dead code. Evidence: `internal/mcp/context.go` (`readProofViolations`), exercised by `TestGetSliceContext` and `TestGetBlockedExtractsViolations` — both write a `proof.json` fixture **and** a decoy `proof.md` carrying `LEGACY-SCRAPE-MARKER`, and fail if the marker ever surfaces.
- **AC-03** — `tools_plan.go`'s `set_track` reads from / writes to `board.json` via `board.ReadBoard` / `board.WriteBoard`; the frontmatter write-back (silent track-wipe footgun) is gone. Evidence: `internal/mcp/tools_plan.go`, exercised by `TestSetTrackUpdates` and `TestSetTrackColon` (pass).
- **AC-04** — `catalog.go`'s `releaseStateSummary` and `countSliceTableRows` derive counts from `board.json` + `status.json`. Evidence: `internal/mcp/catalog.go`, exercised by `TestPlanReleaseExisting` (pass).
- **AC-05** — Tests in `tools_test.go` and `lint_test.go` build fixtures via `writeBoardJSON` / `writeLintBoardJSON` (current `board.BoardRecord` shape) instead of hand-writing legacy `tracks:` YAML in `index.md`; proof fixtures are proof-v1 `proof.json` via `writeProofJSON`. Evidence: `internal/mcp/tools_test.go`, `internal/mcp/lint_test.go`; all ops and lint tests pass.
- **AC-06** — `go build ./...` succeeds, `go test ./internal/mcp/...` passes. Evidence: command output above.

## Not delivered

None. Every acceptance check is delivered.

## Divergence from plan

- `sworn llm-check --type ac-satisfaction` could not run in the implementer session — no `SWORN_ANTHROPIC_API_KEY` / `$SWORN_MODEL` credential is available here (`sworn llm-check: model setup: model: SWORN_ANTHROPIC_API_KEY not set`). Rule 2 deferral recorded in `journal.md`; the fresh-context `/verify-slice` pass is the model-backed check for this slice, consistent with both prior passes.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S04-mcp-oracle-migration 2026-07-01-render-drift-reconciliation
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
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  could not determine integration branch from docs/release/2026-07-01-render-drift-reconciliation/index.md; skipping drift check

== Diff vs start_commit (verifier base) ==
  diff base: start_commit dbd3d9900faf00cb96d7ef1acf6d6f89fc59b901
  PASS  7 file(s) changed vs diff base
  (first 20)
    docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/journal.md
    docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/proof.md
    docs/release/2026-07-01-render-drift-reconciliation/S04-mcp-oracle-migration/status.json
    internal/mcp/context.go
    internal/mcp/lint_test.go
    internal/mcp/tools_ops.go
    internal/mcp/tools_test.go

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
  PASS  deferrals (proof 'Not delivered' + spec 'Out of scope') carry concrete tracking refs
  PASS  proof.md 'Files changed' count (~7) consistent with diff vs start_commit (7)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```
