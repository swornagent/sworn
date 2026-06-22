# Design TL;DR — S34-tui-merge-actor

## §1. User-visible change

A developer watching an active release in the `sworn` TUI sees merge activity as its own distinct, highlighted row labelled `merge:<track>` in the live concurrent-status view. Previously, merge-track dispatches were invisible because the LiveView only polled the `tracks` table; the coach-loop now emits `merge:<track>` events into the `events` table, and this slice surfaces them. The board view also shows a merge indicator next to any track header that has an active merge in flight.

## §2. Design decisions not in spec (max 5)

1. **Merge actors come from the `events` table, not the `tracks` table.** The coach-loop (private harness) writes events with `track_id = "merge:<track>"` via the supervisor's `logEvent`. The `tracks` table is not populated for merge dispatches. Rationale: the spec says "from the polled event stream"; the events table IS the event stream. The LiveView's `poll()` currently only queries `tracks`; we extend it to also query `events`.

2. **A merge actor is "active" if its most recent event is `acquired` (not `released-*`).** The supervisor logs `acquired` on start and `released-<state>` on completion. We query for distinct `merge:%` track_ids whose latest event is `acquired`. Rationale: matches the supervisor's lifecycle; a released merge is done and should not show.

3. **Merge rows reuse `TrackRow` with an `IsMerge bool` flag** rather than a separate type. Rationale: minimal structural change; the renderer just checks the flag to apply a different style. The `CurrentSlice` field shows the merge detail (e.g., "PID 12345") since merge actors don't have a slice.

4. **Board view merge indicator is a styled badge appended to the track header line** (e.g., `▸ T1-engine  [in_progress] ⟪merge⟫`). Rationale: the board already groups slices under track headers; a merge is a track-level activity, so the indicator belongs on the header, not as a separate slice row.

5. **`ActiveMerges(repoRoot, releaseName) []string` is an exported function** in `concurrent.go` so the board view can check for active merges without holding a persistent DB connection. Rationale: mirrors the existing `HasInProgressTracks` pattern — open, query, close.

## §3. Files I'll touch grouped by purpose

- **`internal/tui/concurrent.go`** (touch) — Add `IsMerge` to `TrackRow`; extend `poll()` to query the `events` table for active `merge:*` actors; add `ActiveMerges()` exported helper; render merge rows with distinct style in `View()`.
- **`internal/tui/board.go`** (touch) — Add `MergeActive map[string]bool` to `BoardView`; populate it in `LoadBoard` via `ActiveMerges()`; render a merge badge next to track headers in `View()`.
- **`internal/tui/styles.go`** (touch) — Add `MergeRowStyle` (live view row) and `MergeBadge` (board view indicator) lipgloss styles, visually distinct from worker/coordinator rows (amber/warn colour, bold).
- **`internal/tui/tui_test.go`** (touch) — Add `TestLiveViewRendersMergeActorRow`, `TestLiveViewNoMergeActorNoRow`, and a board view merge indicator test.

## §4. Things I'm NOT doing

- Not producing the `merge:<track>` event tag — that's upstream in the coach-loop (private harness, not this repo).
- Not changing the DB poll interval or the tick mechanism — the merge query runs inside the existing `poll()` on each tick.
- Not changing merge semantics or the merge-track / merge-release flow.
- Not adding a DB connection to the BoardView struct — `ActiveMerges()` opens, queries, and closes its own connection, same as `HasInProgressTracks`.

## §5. Reachability plan

The reachability artefact is the TUI render test output: `TestLiveViewRendersMergeActorRow` feeds a SQLite DB with a `merge:T1-engine` event, calls `StartLiveView`, and asserts `lv.View()` contains a distinct `merge:T1-engine` row. The test output (pass/fail with the rendered string) is captured in `proof.md`. A second test (`TestLiveViewNoMergeActorNoRow`) verifies no spurious merge row when no merge events exist. A board view test verifies the merge badge appears next to the track header.

## §6. Open questions for the Coach

None. The events table schema (`track_id`, `release`, `event`, `detail`, `ts`) and the supervisor's `acquired`/`released-*` lifecycle are confirmed from the codebase. The `merge:<track>` format is confirmed from the spec.