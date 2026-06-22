# Captain review — S34-tui-merge-actor
Date: 2026-06-22
Design commit: f2806bc19a8ef00e84c19bb89e3c239d389dd642

## Pins

1. **[mechanical] §3 — `styles.go` missing from `planned_files`**
   What I observed: `design.md §3` explicitly names `internal/tui/styles.go` as a touchpoint (adding `MergeRowStyle` and `MergeBadge` lipgloss styles). `status.json.planned_files` lists only 3 files: `concurrent.go`, `board.go`, `tui_test.go`. `styles.go` is absent.
   What to ask the implementer: Add `internal/tui/styles.go` to `planned_files` in `status.json` before transitioning to `in_progress`. A verifier Gate 2 check will flag unexpected actual_files not declared in planned_files.

2. **[mechanical] §2.D2 / AC3 — Released-merge actor test missing (CRITICAL)**
   What I observed: The spec's AC3 is "A snapshot with **no** merge actor renders unchanged (no spurious merge row)." The design's two named tests cover: (a) active merge → row visible; (b) no merge events at all → no row. Neither test covers a third case: the events table has BOTH an `acquired` AND a `released-done` event for the same `merge:T1-engine` track_id. A naive SQL query like `WHERE event = 'acquired' AND track_id LIKE 'merge:%'` would return that track regardless of later release events, producing a stale merge row in the live view for a completed merge.
   What to ask the implementer: Add a third render test (`TestLiveViewNoMergeActorAfterRelease` or extend `TestLiveViewNoMergeActorNoRow`) that inserts `(merge:T1-engine, acquired)` followed by `(merge:T1-engine, released-done)` in the events table, then asserts `lv.View()` contains no merge row. The SQL for `ActiveMerges` must check that the **most-recent** event per `merge:*` track_id is `acquired`, not merely that any `acquired` event exists. Suggested pattern: subquery on `MAX(id)` to get the latest event per track_id.

3. **[mechanical] §2.D1 — Coach-loop event write mechanism and track_id format unverified**
   What I observed: Design §2.D1 states "the coach-loop writes events with `track_id = 'merge:<track>'` via the supervisor's `logEvent`." But the sworn Go binary has no `WORKER_TRACK` env-var consumer (grep confirms), and `supervisor.logEvent` is a Go method, not callable from bash. The private coach-loop is a bash harness. The actual mechanism for writing `merge:<track>` events to the DB is not verified in this repo. If the coach-loop uses a different format (e.g. `"coordinator"`, `"merge/T1-engine"`, or the branch name), the `LIKE 'merge:%'` SQL filter in `ActiveMerges` would silently return nothing in production.
   What to ask the implementer: Before writing `ActiveMerges()`, check the private coach-loop's merge-track dispatch script (look for the `sqlite3` INSERT or equivalent) to confirm the exact `track_id` string written for merge actors. Ensure the SQL filter (`LIKE 'merge:%'`) and the test fixture string (`merge:T1-engine`) both match the confirmed format.

4. **[mechanical] §5 — Board view test needs DB fixture, not just filesystem fixture**
   What I observed: The existing `TestBoardViewShowsSlices` uses a filesystem fixture (index.md + status.json files) but no SQLite DB. The design adds `ActiveMerges(repoRoot, releaseName)` called from `LoadBoard`; the board view merge badge test therefore needs a populated events table at `db.DefaultPath(dir)` in addition to the index.md fixture. The design's §5 says "A board view test verifies the merge badge appears next to the track header" but doesn't make this DB dependency explicit.
   What to ask the implementer: Confirm the board view test sets up a DB at `db.DefaultPath(dir)` with an `acquired` event for `merge:T1-engine`, alongside the standard index.md fixture. See `TestConcurrentStatusPoll` (tui_test.go:188) for the existing pattern.

5. **[mechanical] §2 — `status.json` missing `design_decisions` array**
   What I observed: S34's `status.json` has no `design_decisions` field. All 5 decisions in design.md §2 are untracked by the designfit gate. `sworn designfit 2026-06-19-safe-parallelism` will silently skip this slice (designfit.go:127: "No design decisions means no design-fit gate to enforce"). The decisions are all Type-2 with rationale already in design.md; no human decisions pending.
   What to ask the implementer: Add `design_decisions` to `status.json` with the 5 entries (all `stake_class: "Type-2"`, `human_decision: ""`), populated from design.md §2. This is a documentation step with no code implication; it ensures the audit trail is complete.

---

**Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]**
**Critical pins: Pin 2 (released-merge actor test) and Pin 1 (styles.go in planned_files) would cause the slice to ship with observable bugs or a verifier Gate 2 FAIL if unaddressed.**

## Smaller flags (not pins, worth one-line ack)

(a) The §6 confirmation "confirmed from the codebase" applies to the supervisor's lifecycle (`acquired`/`released-*`) and events table schema — both confirmed from `supervisor.go:135,212` and `db.go:41`. The `merge:<track>` format is confirmed from the **spec**, not the codebase (since the coach-loop is private). This is fine as long as Pin 3 is addressed.

(b) The `CurrentSlice` field repurposing for PID display (Decision 3: "shows the merge detail, e.g. 'PID 12345'") is clever but note that the PID is in the `detail` column of events, not `current_slice`. The implementer will need to return `detail` from the events query alongside `track_id` to populate this field — or leave `CurrentSlice` as `"—"`. Both are fine; the spec doesn't require the PID in the live view row.

(c) `colWarn` (`#F59E0B`, amber-500) already exists in `styles.go`. The new `MergeRowStyle` and `MergeBadge` can reference it directly — no new colour palette entry needed.

## Suggested ack reply

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
