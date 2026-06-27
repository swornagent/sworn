---
title: 'S06 — Track-mode invariant-2 enforcement in the loop'
description: 'The autonomous loop enforces touchpoint disjointness at track-dispatch time; an attempted concurrent dispatch of two tracks with overlapping planned_files is blocked with a named report.'
---

# Slice: `S06-invariant2-enforcement`

## User outcome

When `sworn run` attempts to dispatch two tracks concurrently, it checks the union of their `planned_files` (from each slice's status.json); if any file appears in both, the loop blocks the second track with a named report listing the colliding file(s) and the two track IDs, preventing silent concurrent writes to the same file.

## Entry point

`sworn run --release <name>` → `internal/run/parallel.go` — the parallel track launcher checks disjointness before starting each track goroutine.

## In scope

- `internal/run/parallel.go`: before launching each track goroutine, collect the `planned_files` union for all already-running tracks; if the new track's any-slice `planned_files` intersects, block the new track and emit a named report
- The check uses `planned_files` from each slice's committed `status.json` (via the oracle)
- Exception: files listed as "DOCUMENTED SHARED" in the release's index.md touchpoint matrix are exempt from the disjointness check
- The blocked track is reported via log/TUI with message: "INVARIANT-2: tracks <T_a> and <T_b> both write <file> — blocked T_b until T_a merges"
- A blocked track retries the check once T_a merges (same retry mechanic as depends_on wait from S04)

## Out of scope

- Retroactive detection (files already modified mid-run by a concurrently-running track) — the check is at dispatch time only
- Planner-time enforcement — the planner already builds the touchpoint matrix; this is runtime backstop only
- Any modification to triage or the orchestrator

## Planned touchpoints

- `internal/run/parallel.go` (add disjointness check before track goroutine start)

## Acceptance checks

- [ ] WHEN `sworn run` attempts to start two tracks in parallel that share a file in `planned_files` (and the file is not in the documented-shared-file list), THE SYSTEM SHALL block the second track and emit "INVARIANT-2: tracks <T_a> and <T_b> both write <file>"
- [ ] WHEN the first conflicting track merges to `release-wt`, THE SYSTEM SHALL retry starting the previously blocked track
- [ ] WHEN a file appears in `planned_files` of two tracks but is listed as a documented shared file in index.md, THE SYSTEM SHALL NOT block on that file
- [ ] IF the oracle cannot read a slice's planned_files, THE SYSTEM SHALL treat that slice as having no planned files (fail open on the check to avoid blocking on data absence)
- [ ] Test: parallel.go test with a mock oracle returning overlapping planned_files → assert second track blocked + correct error message

## Required tests

- **Unit**: `internal/run/parallel_test.go` (new or extend existing) — overlap scenario + no-overlap scenario + documented-shared-file exempt scenario
- **Reachability artefact**: `go test ./internal/run/... -v -run TestInvariant2` exits 0

## Risks

- Reads planned_files from committed status.json at dispatch time; if slices have empty planned_files (not yet specced), the check will not block — this is acceptable (fail open per AC above)

## Deferrals allowed?

No.
