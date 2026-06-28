---
title: 'S08-differential-validation'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S08-differential-validation`

## User outcome

The Go engine is validated against the coach-loop reference: the same release + inputs produce the same routing decisions, state transitions, and final verified-set as the reference (golden traces); divergence is a hard test failure.

## Entry point

internal/run + internal/router differential test harness fed by captured coach-loop reference traces.

## In scope

- Capture coach-loop reference traces (routing decision + state per tick) for a fixture release into testdata.
- A differential test runs the Go engine over the same fixture and asserts routing/state/verified-set parity with the reference trace.
- Divergence from the reference fails the test with the first differing decision.

## Acceptance checks

- [ ] WHEN the Go engine runs the fixture release, THE SYSTEM SHALL produce the same sequence of routing decisions as the captured coach-loop reference trace.
- [ ] WHEN the Go engine finishes, THE SYSTEM SHALL reach the same verified-set as the reference.
- [ ] WHEN a decision diverges from the reference, THE test SHALL fail citing the first divergent tick.

## Planned touchpoints

- `internal/run/differential_test.go`
- `internal/run/testdata/coachloop-reference/`

## Required tests

- `go test ./internal/run/... -run TestDifferentialVsReference`

## Deferrals allowed?

No.
