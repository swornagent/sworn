---
title: 'S34-tui-merge-actor — render the `merge:<track>` actor as a distinct row in the TUI live view + release board'
description: 'The coach-loop now tags merge-track dispatches with WORKER_TRACK="merge:<track>" so they emit as a distinct actor in the event stream (not "coordinator"); previously merge activity was invisible in monitoring. Render the merge:<track> actor as its own highlighted row/actor in the sworn TUI live concurrent-status view and the release board. Appends to track T2-monitoring.'
---

# Slice: `S34-tui-merge-actor`

## User outcome

A developer watching an active release in the `sworn` TUI sees **merge activity** as its
own distinct, highlighted row/actor — labelled `merge:<track>` — in the live
concurrent-status view and the release board. Previously, merge-track dispatches emitted
as `coordinator` (or were invisible); now the coach-loop tags them
`WORKER_TRACK="merge:<track>"`, so a merge in flight is visible as a first-class actor
rather than hidden inside coordinator activity.

## Entry point

The `sworn` TUI live concurrent-status view (`internal/tui/concurrent.go`, built by S04b)
and the board view (`internal/tui/board.go`, built by S04a). Verifiable by: a TUI render
unit test (`internal/tui/tui_test.go`) that feeds a live-status snapshot containing an
event whose actor/track is `merge:<track>` and asserts the rendered output shows a
distinct, highlighted `merge:<track>` row.

## In scope

- In the live concurrent-status view (`internal/tui/concurrent.go`): recognise an
  actor/track of the form `merge:<track>` from the polled event stream and render it as a
  distinct, highlighted row (its own actor), not folded into `coordinator`.
- In the board render surface (`internal/tui/board.go` / the board view): show merge
  activity for a track as its own highlighted indicator/row.
- Styling: the merge actor row is visually distinguished from worker and coordinator rows
  (e.g. a distinct lipgloss style/badge), consistent with the existing live-view styling.

## Out of scope

- Producing the `merge:<track>` event tag — that already happens upstream in the
  coach-loop (`WORKER_TRACK="merge:<track>"`, private harness, not this repo). This slice
  only consumes/renders it.
- The DB poll / event-stream plumbing (built by S04b) — this slice extends the renderer,
  not the polling.
- Any change to merge semantics or the merge-track / merge-release flow.

## Planned touchpoints

- `internal/tui/concurrent.go` (touch — detect `merge:<track>` actor; render distinct row)
- `internal/tui/board.go` (touch — show merge activity as its own highlighted row/indicator)
- `internal/tui/tui_test.go` (touch — add the merge-actor render test)

> **Touchpoint note:** `internal/tui/concurrent.go` is created by S04b-tui-live and
> `internal/tui/board.go` by S04a-tui-foundation — both earlier in track T2-monitoring.
> S34 appends to T2's tail, so those files exist before this slice runs; T2's
> within-track serialisation means no parallel collision on `internal/tui/`. If the live
> view exposes a dedicated row-render seam, prefer touching that over `model.go`; confirm
> against the S04b implementation before coding.

## Acceptance checks

- [ ] Given a live-status snapshot whose event stream contains an actor/track
  `merge:<track>`, the live concurrent-status view renders a **distinct, highlighted**
  row labelled `merge:<track>` (not merged into `coordinator`)
- [ ] The board view shows merge activity for that track as its own highlighted
  row/indicator
- [ ] A snapshot with **no** merge actor renders unchanged (no spurious merge row)
- [ ] `go test ./internal/tui/...` passes (including the new merge-actor render test)
- [ ] `go build ./...` passes

## Required tests

- **Unit (TUI render)** `internal/tui/tui_test.go`:
  - `TestLiveViewRendersMergeActorRow`: feed a snapshot with a `merge:<track>` event;
    assert the rendered string contains a distinct `merge:<track>` row
  - `TestLiveViewNoMergeActorNoRow`: snapshot with only worker/coordinator actors → no
    merge row rendered
- **Reachability artefact**: capture the TUI render test output (or a rendered-string
  golden) showing the highlighted `merge:<track>` row. Document in `proof.md`.

## Risks

- The exact actor/track field shape (`merge:<track>`) must match what the coach-loop emits
  and what S04b's poller surfaces. Mitigation: read S04b's `internal/tui/concurrent.go`
  event/snapshot struct before coding to confirm the field name and value format the
  renderer receives; the upstream tag is `WORKER_TRACK="merge:<track>"` (coach-loop,
  private harness) and surfaces via the SQLite events table S04b already polls.

## Deferrals allowed?

None.
