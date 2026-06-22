<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is sound and grounded in the live codebase. 5 mechanical pins to apply inline:

1. **styles.go in planned_files.** Add `internal/tui/styles.go` to `planned_files` in `status.json` before writing any code. Otherwise the verifier Gate 2 will FAIL.

2. **Released-merge test (CRITICAL).** The SQL in `ActiveMerges()` must find only the *most-recent* event per `merge:*` track_id and check that event is `acquired`. A naive `WHERE event = 'acquired'` query silently shows stale completed merges. Add a third render test (`TestLiveViewNoMergeActorAfterRelease`): insert `(merge:T1-engine, acquired)` then `(merge:T1-engine, released-done)` in the events table, assert `lv.View()` has no merge row. Use `MAX(id)` subquery pattern: `SELECT track_id FROM events WHERE id IN (SELECT MAX(id) FROM events WHERE release = ? AND track_id LIKE 'merge:%' GROUP BY track_id) AND event = 'acquired'`.

3. **Confirm track_id format from private harness.** Before coding the SQL filter, check the coach-loop's merge-track dispatch script for the exact `track_id` string it INSERTs (or has sqlite3 INSERT) into the events table. The `LIKE 'merge:%'` filter and the test fixture `'merge:T1-engine'` must match the confirmed format.

4. **Board view test needs DB fixture.** The board view merge badge test must set up both a filesystem fixture (index.md + status.json) AND a SQLite DB at `db.DefaultPath(dir)` with a `merge:T1-engine` acquired event. See `TestConcurrentStatusPoll` (tui_test.go:188) for the existing DB-setup pattern.

5. **Add design_decisions to status.json.** Populate all 5 decisions (all Type-2, `human_decision: ""`) from design.md §2 before transitioning to in_progress.

Flags (not pins): (a) §6 "confirmed from codebase" is accurate for supervisor lifecycle and events schema; `merge:<track>` format is confirmed from spec — verify the private harness in Pin 3; (b) `CurrentSlice` field for PID display requires returning `detail` from the events query — or leave it as `"—"` (both are fine); (c) `colWarn` already exists in styles.go.

§2 decisions D1–D5 all ack'd as Type-2 (no human call pending). §6 question: none stated.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: all 5 pins are mechanical apply-inline fixes (SQL correctness, test coverage gap, planned_files declaration, format verification, status.json field); no Coach authority call required
-->
