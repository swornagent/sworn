# Intake — sit-fixture (hermetic SIT release)

Not a real release. This intake exists so the cold-board fixture that
`internal/run.TestLoopSIT` boots is Definition-of-Ready complete: it carries a
need, a slice that covers it, a release goal, and (in the slice's status.json) a
human-ratified validation record — the same shape a genuine release presents to
the loop, so the DoR gate inside `implement.Run` (RTM + reqverify + reqvalidate)
passes on the first pass rather than being retry-bypassed.

## Release goal

Prove the assembled sworn parallel loop boots end-to-end over a real release
board from a cold board and drives a slice to a committed `verified` state
through the driver registry, so a dead loop wiring fails a test in CI instead of
shipping a DOA release.

## Needs

- N-01: The assembled parallel loop boots end-to-end over the fixture release from a cold board (no pre-made worktrees), drives one slice to a committed `verified` state through the driver registry, and never panics.

## Traceability

| Need | Slice | Acceptance |
|------|-------|------------|
| N-01 | S01-sit-slice | Loop reaches committed `verified` over the fixture from a cold board. |
