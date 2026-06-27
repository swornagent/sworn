---
title: 'S05 — Merge gate routes through oracle; invariant-4 classifier; sworn merge CLI'
description: 'Fix the merge gate to route verified-checks through board.Oracle (not working-tree status.json); add invariant-4 conflict classifier; expose sworn merge-track / merge-release CLI subcommands; wire journey gate into merge-release.'
---

# Slice: `S05-merge-gate-oracle`

## User outcome

`sworn merge-track <track-id>` and `sworn merge-release` are available as CLI commands; the merge gate checks slice states via the board oracle (not working-tree status.json), catches invariant-4 file conflicts and reports them by name, and invokes the journey gate before merge-release succeeds.

## Entry point

`sworn merge-track <track-id> [--release <name>]` and `sworn merge-release [--release <name>]` CLI subcommands (new in `cmd/sworn/merge.go`); also called by the scheduler's auto-merge step (S04).

## In scope

- `cmd/sworn/merge.go` (new): implements `sworn merge-track` and `sworn merge-release` subcommands
- `internal/mcp/tools_ops.go`: fix the existing `merge_track` and `merge_release` MCP tools to call `board.Oracle.ReadTrackState()` instead of reading `status.json` from the working tree
- Invariant-4 classifier: before merging, run `git merge --no-commit --no-ff` dry-run and detect conflict files; if any conflict file is NOT in the touchpoint matrix's documented-shared-file list for that track, BLOCK with the conflicting file names listed
- Journey gate: before `sworn merge-release` succeeds, call `journey.Check(release)` — fail closed if journeys.json is absent or not ratified
- All verified-state checks go through `board.Oracle.ReadSliceState(sliceID)` which reads committed status.json from the track branch (not the working-tree copy)

## Out of scope

- Changing the oracle's data source from index.md to board.json (that is S14) — this slice uses the existing oracle, just calls it correctly
- `sworn merge-release` pushing to the version integration branch (merges to `release-wt`, not beyond)
- The auto-merge trigger from S04 — S05 provides the CLI/MCP gate; S04 calls it

## Planned touchpoints

- `cmd/sworn/merge.go` (new)
- `internal/mcp/tools_ops.go` (fix verified-check to use oracle)
- `internal/router/router.go` (add invariant-4 conflict classifier, per audit ref router.go:381-429)

## Acceptance checks

- [ ] WHEN `sworn merge-track T1-orchestration` is run and all T1 slices have state `verified` in the oracle, THE SYSTEM SHALL perform the git merge and exit 0
- [ ] WHEN `sworn merge-track T1-orchestration` is run and any T1 slice has state != `verified` in the oracle, THE SYSTEM SHALL exit non-zero with a message naming the unverified slice(s)
- [ ] WHEN a merge dry-run detects a conflict on a file NOT in the touchpoint matrix documented-shared-file list, THE SYSTEM SHALL exit non-zero with message "BLOCK: invariant-4 violation — conflict on <filename> (not a documented shared file)"
- [ ] WHEN `sworn merge-release` is run and `journey.Check()` returns false (no journeys.json or not ratified), THE SYSTEM SHALL exit non-zero with message "BLOCK: no ratified journeys.json — Rule 10 gate"
- [ ] WHEN `sworn merge-track` reads slice states, THE SYSTEM SHALL use `board.Oracle.ReadSliceState()` (committed status.json on the track branch), not `os.ReadFile` on the working-tree path
- [ ] The existing MCP `merge_track` and `merge_release` tools pass the same gates via `internal/mcp/tools_ops.go`

## Required tests

- **Unit**: `cmd/sworn/merge_test.go` or similar — mock oracle + mock git; table test for unverified-block and oracle-routing
- **Integration**: add a scenario to existing `cmd/sworn/` tests: mock a track with all slices verified → merge succeeds; mock a track with one non-verified → merge blocked
- **Reachability artefact**: `go test ./cmd/sworn/... -v -run TestMergeTrack` exits 0; smoke step: `sworn merge-track --dry-run` on a real release board

## Risks

- invariant-4 classifier uses `git merge --no-commit`; if the working tree is dirty this may fail — the merge gate must first assert the working tree is clean before the dry-run

## Deferrals allowed?

Yes for journey.Check wiring in merge-release if journeys.json does not yet exist in the repo (S17 creates it); the merge-release journey gate may be stubbed with a clear warning until S17 ships. Rule 2: Why = journeys.json not yet declared (S17 is in the same track T1); Tracking = S17-journeys-declare; Acknowledged = Brad, 2026-06-27.
