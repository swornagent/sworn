# Design TL;DR — S06-invariant2-enforcement

## Approach

Add a **track-disjointness pre-launch check** to `RunParallel` in `internal/run/parallel.go`. Before launching each track goroutine, collect that track's `planned_files` union (from the committed `status.json` of every slice in the track on the `release-wt` ref), and check it against the union of `planned_files` from already-launched tracks in the same phase. Files listed as "(DOCUMENTED SHARED)" in the index.md touchpoint matrix are exempt. An overlapping file blocks the second track with a named report; the blocked track retries after the conflicting track completes and auto-merges to `release-wt`.

## Key design choices

### 1. Where to read planned_files from

**Choice:** Read from the **committed `status.json` on the `release-wt/<release>` ref** (via `git show`) at RunParallel startup, cache per-track.

**Rationale:** The spec says "via the oracle" and "committed status.json". The release-wt ref has the planner-authored `planned_files` — they are immutable specs, not runtime state. Reading once at startup (not per-goroutine) avoids repeated git operations. The oracle/git reader already exists in `parallel.go` via the `repo` variable constructed at line 175.

### 2. How to parse documented shared files

**Choice:** Parse the **index.md markdown table** rows where the first column contains "(DOCUMENTED SHARED)". Extract the file path from backtick-quoted text.

**Rationale:** The spec says "files listed as DOCUMENTED SHARED in the release's index.md touchpoint matrix". The index.md is already read and parsed in `RunParallel` (line 121-143), so extending the frontmatter parser to extract the touchpoint matrix is incremental. A regex-based row parser (`| \`<path>\` (DOCUMENTED SHARED) |`) is simple, stdlib-only, and matches the existing line-oriented parsing style in `internal/board/index.go`.

### 3. Block mechanic

**Choice:** In the phase fan-out loop, before `wg.Add(1)` / `go func()`, check the new track's `planned_files` against a running union. If overlap exists, log `"INVARIANT-2: tracks <T_a> and <T_b> both write <file> — blocked T_b until T_a merges"` to stderr and append the track to a `blockedTracks` list (don't launch it). After `wg.Wait()` on the launched goroutines, if `blockedTracks` is non-empty, launch them in a follow-up phase (new `wg` + goroutines).

**Rationale:** The invariant-2 check is at dispatch time (before goroutine launch), per spec. The blocked track waits for the conflicting track to finish, which triggers `finishTrack` → auto-merge → release-wt. By the time the follow-up phase starts, the conflicting track has merged, so the re-check passes. This re-uses the existing phase-barrier ordering mechanism from S04 without adding a polling loop.

### 4. Retry mechanic

**Choice:** Re-use the existing **phase barrier** pattern (already proven in S04). The follow-up phase launches blocked tracks only after all launched tracks' goroutines return. Since `finishTrack` auto-merges on completion, the follow-up phase's disjointness re-check will pass.

**Rationale:** The spec says "same retry mechanic as depends_on wait from S04", which is the phase barrier. No new polling, no new goroutine patterns — the phase barrier already guarantees ordering.

### 5. Oracle failure mode (AC-4)

**Choice:** If a slice's `status.json` cannot be read (missing, parse error, empty `planned_files`), treat that slice as having **zero planned files** — fail open.

**Rationale:** Spec AC-4: "IF the oracle cannot read a slice's planned_files, THE SYSTEM SHALL treat that slice as having no planned files (fail open on the check to avoid blocking on data absence)". A tight invariant-2 that blocks on missing data would prevent valid parallel runs.

## Files to touch

- `internal/run/parallel.go` — add disjointness check in the phase fan-out loop, plus helper functions:
  - `collectTrackPlannedFiles()` — reads all slices' status.json from release-wt, extracts planned_files
  - `parseDocumentedSharedFiles()` — extracts DOCUMENTED SHARED paths from index.md touchpoint matrix
  - `checkDisjointness()` — intersects two planned_files sets, returning overlapping files
- `internal/run/parallel_test.go` — extend with:
  - `TestInvariant2_OverlapBlocksSecondTrack` — mock oracle returns overlapping planned_files → assert second track blocked + correct error message
  - `TestInvariant2_NoOverlapBothRun` — disjoint planned_files → both tracks launch
  - `TestInvariant2_DocumentedSharedExempt` — overlap on documented-shared file → both tracks launch
  - `TestInvariant2_OracleReadFailureFailsOpen` — oracle error → track launches (fail open)

## Design-level risks / pins

- **Pin 1 — index.md parsing fragility:** The documented-shared-file extraction is a regex on the markdown table. If the index.md table format changes (e.g., column order shifts), the parser silently misses files. Mitigation: make the parser tolerant — search for `(DOCUMENTED SHARED)` anywhere in the row, extract backtick-quoted path from the first column.
- **Pin 2 — planned_files may be empty for un-specced slices:** Per spec Risks section, this is acceptable (fail open). No action needed.
- **Pin 3 — first-track-wins semantics:** The first track in iteration order gets to run; the second is blocked. If two tracks share a file, track ordering in the board (not the execution plan) determines priority. This is consistent with the spec's "blocked T_b until T_a merges" language.

## AC traceability

| AC | Planned change |
|---|---|
| AC1: Overlap blocks second track + named report | `checkDisjointness()` in phase loop + stderr log |
| AC2: Retry after T_a merges | Follow-up phase after wg.Wait + auto-merge |
| AC3: Documented-shared exempt | `parseDocumentedSharedFiles()` fed into disjointness check |
| AC4: Oracle failure → fail open | Error path in `collectTrackPlannedFiles()` returns empty set |
| AC5: Unit tests (overlap, no-overlap, documented-shared exempt, fail-open) | `parallel_test.go` — TestInvariant2_* functions |