# Journal — S14-board-json

## 2026-06-28 09:12 — Implementation session

**State transition:** planned → in_progress → implemented

### Decisions

- **Schema-dispatch in validator.go**: The existing `Validate()` was hardcoded for slice-status-v1. Refactored to a `switch` on schemaName so board-v1 gets track-level required-field validation (`id`, `state`, `worktree_branch`) distinct from slice-level checks (`slice_id`, `release`, `track`, `state`, `verification`).
- **Drift guard is advisory only**: Per the spec's open deferral and risks section, the drift guard in `WriteBoard()` logs a warning via `log.Printf` but does not return an error. Promoting to BLOCK would require all existing releases to migrate first.
- **Oracle git-ref path preserved**: The oracle's `readTrackInfos()` reads `board.json` from git refs (via `reader.Show()`), same as the old index.md path. The filesystem-level `ReadBoard()` is separate — it handles lazy migration (writing board.json to disk) that the oracle can't do from git-ref space.
- **index.go not changed**: Contrary to the planned_files list, `index.go` was not modified. The drift guard is a simple comparison (not render-then-compare), so no YAML serialisation was needed. Re-rendering index.md from board.json is deferred to a future slice.

### Trade-offs

- **No Fumadocs prefix for `migrateFromIndex()`**: The filesystem-level lazy migration only checks `docs/release/<release>/index.md`, not the Fumadocs prefix. The oracle's git-ref fallback does handle both prefixes. This is acceptable because lazy migration runs in a worktree where `docs/release/` is the canonical path.
- **`log.Printf` for drift warnings**: Uses stdlib `log` package. Could be noisier than a structured log, but consistent with the rest of the codebase (zero external deps).

### Subagent dispatches

None.

### Fix applied

- **Pre-existing syntax error**: `oracle.go` line 427 had `trackMap := make(...)	for ...` (missing newline/semicolon). Fixed by splitting onto two lines. This was a pre-existing bug that may affect other tracks — verified all existing tests pass after fix.
## Verifier verdicts received

### 2026-06-28T09:19:00Z — Verifier session (fresh, artefact-only)

**Verdict: PASS**

Slice: `S14-board-json`
Verified against: `3bac35f` (track/2026-06-27-conformance-foundation/T4-records-as-json)
Verifier session: fresh, artefact-only

**Gate walkthrough:**
1. **Gate 1 — User-reachable outcome**: PASS. `board.Oracle` reads `board.json` first via `readTrackInfos()`, with `index.md` frontmatter as legacy fallback. All oracle data paths (`ReadSliceStatus`, `ReadTracks`, `ReadBoard`) route through `readTrackInfos()`.
2. **Gate 2 — Planned touchpoints**: PASS. `index.go` not changed (explained), `validator.go` and `embed.go` added (explained). All divergences acknowledged in proof.md.
3. **Gate 3 — Required tests**: PASS. All 27 board tests pass, `go vet` clean. `TestOracleReadBoard_BoardJSONFirst` exercises the integration point.
4. **Gate 4 — Reachability artefact**: PASS. Test run demonstrates oracle path through board.json.
5. **Gate 5 — No silent deferrals**: PASS. Only hit is pre-existing S13 dark-code marker in `validator.go:55`.
6. **Gate 6 — Design conformance**: PASS. Non-UI project, no design-fidelity config.
7. **Gate 7 — Claimed scope**: PASS. All 11 Delivered items have verifiable evidence.

**Next step:** `/implement-slice S15-spec-proof-records 2026-06-27-conformance-foundation` (next incomplete slice in T4-records-as-json).
