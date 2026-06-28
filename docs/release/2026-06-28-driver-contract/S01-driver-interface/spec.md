---
title: 'S01-driver-interface'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S01-driver-interface`

## User outcome

The engine has a single `Driver` contract at the process boundary; an agentic dispatch is a `Driver.Dispatch(spec, worktree) -> Result` call, not an in-process ChatMessage loop.

## Entry point

internal/driver (new) — the package that defines the contract every runtime driver implements; consumed later by RunSlice/scheduler.

## In scope

- New `internal/driver/driver.go`: `type Driver interface { Dispatch(ctx, DispatchInput) (Result, error); Capabilities() Capability }`.
- `DispatchInput`{Role, ModelID, SpecPath, WorktreeRoot, Command, Timeout}; `Result`{Status (ok|blocked|error), Subtype, ResultText, Verdict, CostUSD, InputTokens, OutputTokens, ModelID, DurationMS}.
- Contract doc `docs/baton/runtime-drivers.md` (vendored/aligned) states the behavioural spec the conformance suite (S09) enforces.
- No provider wire types (ChatMessage etc.) appear in this package or its imports.

## Acceptance checks

- [ ] WHEN a caller constructs a `DispatchInput` and calls `Dispatch`, THE SYSTEM SHALL return a `Result` whose `Status` is one of ok|blocked|error and never panic on a nil field.
- [ ] THE `internal/driver` package SHALL NOT import `internal/model` request/response wire types (compile-time assertion / import test).
- [ ] Result SHALL carry DurationMS, InputTokens, OutputTokens, CostUSD, and the confirmed ModelID for telemetry.

## Planned touchpoints

- `internal/driver/driver.go`
- `internal/driver/driver_test.go`
- `docs/baton/runtime-drivers.md`

## Required tests

- `go test ./internal/driver/... -run TestDriverContract`

## Deferrals allowed?

No.
