---
title: 'S05-runslice-via-driver'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S05-runslice-via-driver`

## User outcome

`RunSlice` performs implement and verify by calling `Driver.Dispatch`, never by constructing an agent/ChatMessage directly; the orchestration path has no provider-wire coupling.

## Entry point

internal/run/slice.go — replace NewAgent/NewVerifier direct use with Driver dispatch.

## In scope

- RunSlice resolves a Driver (via registry) for the implement and verify roles and calls Dispatch; remove direct internal/agent construction from RunSlice.
- Verdicts come from Result.Verdict/Status; the slice state machine consumes the normalized Result.
- The nil-factory class is gone by construction (no factory fields; resolution always returns a Driver or errors).

## Acceptance checks

- [ ] WHEN RunSlice runs a slice, THE SYSTEM SHALL implement and verify exclusively through `Driver.Dispatch`.
- [ ] THE `internal/run` package SHALL NOT import `internal/model` wire types after this slice (import test).
- [ ] WHEN a driver cannot be resolved, RunSlice SHALL return a descriptive error (no nil dereference).

## Planned touchpoints

- `internal/run/slice.go`
- `internal/run/slice_test.go`

## Required tests

- `go test ./internal/run/... -run TestRunSliceDriver`

## Deferrals allowed?

No.
