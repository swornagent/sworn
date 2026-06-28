---
title: 'S07-normalized-result-telemetry'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S07-normalized-result-telemetry`

## User outcome

Every dispatch records duration, input/output tokens, real cost, and confirmed model-id from the Driver Result into the supervisor store (FT-7), so per-model loop-delivery economics are measurable.

## Entry point

internal/driver Result → internal/state Dispatch / internal/supervisor.

## In scope

- Map Driver.Result fields into the Dispatch telemetry record at each implement/verify call.
- Real cost from a pricing map keyed on the confirmed Result.ModelID (not a nominal constant); token split recorded.

## Acceptance checks

- [ ] WHEN a dispatch completes, THE SYSTEM SHALL persist DurationMS, InputTokens, OutputTokens, real CostUSD, and the confirmed ModelID.
- [ ] THE recorded model-id SHALL be the one the Result confirms, not the requested string.

## Planned touchpoints

- `internal/state/state.go`
- `internal/supervisor/supervisor.go`
- `internal/driver/driver.go`

## Required tests

- `go test ./internal/state/... ./internal/supervisor/... -run TestDispatchTelemetry`

## Deferrals allowed?

No.
