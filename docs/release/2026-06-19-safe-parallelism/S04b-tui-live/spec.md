---
title: 'S04b-tui-live — concurrent status view + credits (DB poll)'
description: 'Extends the sworn TUI with a live concurrent status view that polls SQLite every second to show which tracks are running, which slice each is on, elapsed time, and credit balance.'
---

# Slice: `S04b-tui-live`

## User outcome

A developer navigating to an active release in the `sworn` TUI sees a live concurrent
status view: each running track, its current slice, elapsed time, and credits consumed —
updating every second from the SQLite DB without the developer refreshing manually.

## Entry point

The TUI board view from S04a, when the selected release has tracks in `in_progress`
state in the SQLite DB — transitions to the concurrent status sub-view automatically,
or user presses `l` (live) to switch.

## In scope

- `internal/tui/concurrent.go`: Bubble Tea component that polls the SQLite DB (via
  `internal/db`) every 1 second via a `time.Tick`-based `tickMsg`; renders a table
  showing:
  - Track ID, current slice ID, state
  - Elapsed time since `started_at` (from DB events table)
  - Live status badge: `running` / `verifying` / `blocked` / `done`
- Credit balance display: reads `~/.config/sworn/credits.json` cache (or "–" if not
  logged in); shown in the TUI header bar alongside the release name
- Integration into `internal/tui/model.go`: the root model gains a `viewLive` state;
  board view auto-transitions to live view when in-progress tracks are detected (or
  user presses `l`); pressing `b` returns to board view
- The concurrent view is read-only — no actions in this slice (actions are S04c)

## Out of scope

- Blocked-slice TL;DR panel (S04c)
- Notification of state changes (S07 — polling is the display mechanism here)
- Credits purchase flow (S06b)
- Web dashboard

## Planned touchpoints

- `internal/tui/concurrent.go` (new)
- `internal/tui/model.go` (touch — add viewLive state, auto-transition, `l`/`b` keys)
- `internal/tui/tui_test.go` (touch — add concurrent view tests)

## Acceptance checks

- [ ] When the selected release has at least one track row in the SQLite DB with
  `state = 'in_progress'`, the TUI transitions to (or offers `l` to view) the
  concurrent status view
- [ ] The concurrent status table updates its elapsed time column every ~1 second
  (verified in tests by advancing the tick counter)
- [ ] If a track row transitions from `in_progress` to `done` in the DB, the TUI
  reflects the new state within 2 seconds (one poll cycle)
- [ ] Credit balance shown in header when `~/.config/sworn/credits.json` exists;
  "–" shown when absent — no error
- [ ] `go test ./internal/tui/...` passes; `TestConcurrentStatusPoll` verifies the
  model state updates correctly on tick with a fake DB

## Required tests

- **Unit**: `internal/tui/tui_test.go`
  — `TestConcurrentStatusPoll`: inject a fake DB returning known track state; advance
    one tick; assert the concurrent view model contains the expected rows
  — `TestAutoTransitionToLive`: model initialised with a release that has in-progress
    tracks; assert `viewLive` is active without user input
  — `TestCreditBalanceDisplayed`: fixture credits file with balance 42; assert header
    contains "42"
  — `TestCreditBalanceAbsent`: no credits file; assert header shows "–" not error
- **Reachability artefact**: smoke step — run `sworn run --parallel` on a fixture
  release (does not need to complete); in a separate terminal, run `sworn` and navigate
  to the release; observe the live view showing running tracks with incrementing elapsed
  time. Document in proof.md.

## Risks

- Polling the DB from a goroutine while the scheduler writes to it requires the DB
  connection to support concurrent readers. SQLite's WAL mode (enabled in S01's
  `internal/db/db.go`) allows concurrent reads and writes. Verify WAL is enabled
  before implementing.

## Deferrals allowed?

No. S04c's blocked panel is the next step and it appears in this same view.
