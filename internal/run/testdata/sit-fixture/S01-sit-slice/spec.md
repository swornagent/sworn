# S01-sit-slice — cold-board reference slice

This is the single slice of the hermetic `sit-fixture` release that
`internal/run.TestLoopSIT` drives through the assembled parallel loop. It has no
production surface of its own; it exists so the loop has a real release to boot
over from a cold board (no pre-made worktrees), with every role dispatch served
by the offline reference driver.

## User outcome

The assembled `sworn` parallel loop boots over this fixture release, materialises
the release and track worktrees, dispatches the captain / implement / verify
legs through the driver registry, consumes the verdict via the state machine,
and drives this slice to `verified` — committing that transition to the track
ref — without any network access, paid dispatch, or panic.

## Acceptance checks

- [ ] The parallel loop reaches `verified` for this slice from a cold board (N-01).
- [ ] The `verified` transition is committed to the track ref, so a router re-read observes `verified` and does not re-dispatch the verify leg (N-01).

## Required tests

- **SIT**: `go test ./internal/run/ -run TestLoopSIT`
