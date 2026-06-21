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

None yet.
