---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S34-tui-merge-actor`

## 2026-06-21 — planned (replan)

Added during `/replan-release`, appended to the tail of track `T2-monitoring`. The
coach-loop now tags merge-track dispatches with `WORKER_TRACK="merge:<track>"`, so merge
activity emits as a distinct actor in the event stream rather than as `coordinator`;
previously merge activity was invisible in monitoring. This slice renders the
`merge:<track>` actor as its own highlighted row in the TUI live concurrent-status view
(`internal/tui/concurrent.go`, built by S04b) and the board view (`internal/tui/board.go`,
built by S04a).

**Rationale:** make merge activity first-class in the live monitor — a merge in flight
should be visibly its own actor, not folded into coordinator activity.

Placed at the tail of `T2-monitoring` (after S04a/S04b/S04c/S17-tui-provider-config) so the
`internal/tui/` live view and board surfaces it extends already exist; within-track
serialisation means no parallel `internal/tui/` collision. The upstream `merge:<track>`
tag is produced by the coach-loop (private harness), not this repo — this slice only
consumes/renders it.

## Open questions

- Confirm the actor/track field name and value format S04b's poller surfaces to the
  renderer (read `internal/tui/concurrent.go` before coding) — see spec Risks.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-06-28 — PASS

**Verifier role active. No prior implementer context loaded.** Fresh session loading only spec.md, proof.md, status.json, and live repo state from the track worktree.

**Gate 1: User-reachable outcome exists** — PASS. Entry points `internal/tui/concurrent.go` (live view) and `internal/tui/board.go` (board view) are wired through `LiveView.poll()`/`View()` and `BoardView.LoadBoard()`/`View()` respectively — both are real user-facing TUI surfaces, not intermediate functions.

**Gate 2: Planned touchpoints match actual changed files** — PASS. Planned: `concurrent.go`, `board.go`, `tui_test.go`. Actual adds `styles.go` (natural home for the distinct lipgloss style/badge the spec calls for) and `releases.go`/`tui.go` (gofmt-only whitespace normalisation; no functional change). The styles.go addition is logically required by the planned touchpoints — the existing styles pattern predates this slice.

**Gate 3: Required tests exist and exercise the integration point** — PASS. All 5 merge-actor tests (`TestLiveViewRendersMergeActorRow`, `TestLiveViewNoMergeActorNoRow`, `TestLiveViewNoMergeActorAfterRelease`, `TestBoardViewShowsMergeBadge`, `TestBoardViewNoMergeBadge`) exercise through real TUI components (`StartLiveView`/`View()`, `BoardView.LoadBoard()`/`View()`), not leaf functions. Re-ran: 27/27 PASS, `go build ./...` exit 0, `go vet ./internal/tui/...` exit 0.

**Gate 4: Reachability artefact proves the user path** — PASS. Tests render through actual TUI `View()` methods from DB fixtures — the full data-to-render path is exercised. User gesture: "Developer opens `sworn` TUI, selects a release with active merge, observes amber-bold `merge:<track>` row in live view and `⟪merge⟫` badge on board track headers."

**Gate 5: No silent deferrals or placeholder logic** — PASS. Zero dark-code markers (TODO/FIXME/deferred/placeholder/hack/workaround) in any changed file under `internal/tui/`.

**Gate 6: Claimed scope matches implemented scope** — PASS. All 5 acceptance checks have verifiable evidence references in proof.md. Not-delivered list is empty. Divergences are gofmt-only (documented) and an additive test (Pin 2, documented).

**Verdict: PASS**
## 2026-06-28 — implemented

### Design review outcome

Captain reviewed the design TL;DR and issued 5 mechanical pins (all PROCEED, no
Coach authority call required). Coach approved via `approved-ack.md`. All 5 pins
addressed inline:

1. **styles.go in planned_files** — added `internal/tui/styles.go` to
   `planned_files` in `status.json` before any code.
2. **Released-merge test (CRITICAL)** — `ActiveMerges()` uses a `MAX(id)`
   subquery to find the latest event per `merge:*` track_id, then filters for
   `event = 'acquired'`. Added `TestLiveViewNoMergeActorAfterRelease` which
   inserts `acquired` then `released-done` and asserts no merge row renders.
3. **Confirm track_id format** — verified in the private coach-loop
   (`/home/brad/.claude/bin/coach-loop` line 2230: `WORKER_TRACK="merge:$_PENDING_MERGE"`,
   line 2260: `WORKER_TRACK="merge:$READY_TRACK"`). Format is `merge:<track-id>`
   (e.g. `merge:T1-engine`). The supervisor's `Acquire()` writes `acquired`
   events and `Release()` writes `released-done`/`released-failed` events to
   the events table with that `track_id`.
4. **Board view test needs DB fixture** — `TestBoardViewShowsMergeBadge` sets
   up both a filesystem fixture (index.md + status.json) AND a SQLite DB at
   `db.DefaultPath(dir)` with a `merge:T1-core` acquired event, following the
   `TestConcurrentStatusPoll` pattern.
5. **design_decisions in status.json** — all 5 decisions (D1-D5) populated as
   Type-2 with `human_decision: ""`.

### Implementation decisions

- `IsMerge bool` field added to `TrackRow` struct. The `poll()` method now
  queries both the `tracks` table (existing behaviour) and the `events` table
  (new merge-actor query). Merge rows get `State = "merging"` and
  `CurrentSlice` set to the event `detail` (e.g. "PID 12345").
- `ActiveMerges(repoRoot, releaseName) []string` exported in `concurrent.go`.
  Opens, queries, closes its own connection — mirrors `HasInProgressTracks`.
- `MergeActive map[string]bool` added to `BoardView`. Populated in `LoadBoard`
  via `ActiveMerges()`. The track_id prefix `merge:` is stripped to get the
  bare track-id for matching against `TrackInfo.ID`.
- `MergeRowStyle` (amber, bold) and `MergeBadge` (amber, bold) added to
  `styles.go`, using the existing `colWarn` colour.
- `releases.go` and `tui.go` were touched by `gofmt -w` only (whitespace
  normalisation); no functional changes. Recorded in `actual_files` for
  transparency.

### Tests added

- `TestLiveViewRendersMergeActorRow` — AC1: active merge renders distinct row
- `TestLiveViewNoMergeActorNoRow` — AC3: no merge events, no merge row
- `TestLiveViewNoMergeActorAfterRelease` — Pin 2: completed merge (acquired
  then released-done) does not render
- `TestBoardViewShowsMergeBadge` — AC2: board view shows merge badge
- `TestBoardViewNoMergeBadge` — AC3: board view without merge shows no badge

### Skeptic panel

skeptic_panel: skipped — runtime does not support subagent dispatch

### Trade-offs

- The merge query runs inside `poll()` on every tick (1s). The `MAX(id)`
  subquery is efficient on the events table (indexed by AUTOINCREMENT PK) and
  the result set is small (at most one row per active merge). No performance
  concern for the expected scale (tens of tracks, not thousands).
- `CurrentSlice` is repurposed for the merge detail (PID string) rather than
  adding a new field. This keeps the struct minimal and the renderer unchanged
  beyond the `IsMerge` style switch.