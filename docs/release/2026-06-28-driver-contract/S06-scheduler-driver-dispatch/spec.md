---
title: 'S06-scheduler-driver-dispatch'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S06-scheduler-driver-dispatch`

## User outcome

The parallel scheduler/worker drives dispatches through the Driver contract; the model layer is an implementation detail behind drivers, invisible to the scheduler.

## Entry point

internal/scheduler/worker.go + cmd/sworn/run.go (parallel runSliceFn).

## In scope

- Worker/runSliceFn dispatch via the resolved Driver; the parallel path wires no factories (the S27 wiring gap cannot recur).
- Driver selection is per-role and per-model from config; no hardcoded provider in the scheduler.

## Acceptance checks

- [ ] WHEN the parallel loop dispatches a slice, THE SYSTEM SHALL use a resolved Driver and SHALL NOT pass nil agent/verifier factories.
- [ ] THE scheduler package SHALL NOT import provider wire types.
- [ ] WHEN a model has no driver, the loop SHALL fail fast at startup, not mid-run.

## Planned touchpoints

- `internal/scheduler/worker.go`
- `cmd/sworn/run.go`
- `internal/scheduler/worker_test.go`

## Required tests

- `go test ./internal/scheduler/... ./cmd/sworn/... -run TestSchedulerDriver`

## Deferrals allowed?

No.
