---
title: 'S04-driver-registry-resolution'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S04-driver-registry-resolution`

## User outcome

Model selection resolves to a registered Driver with a fail-fast capability check at startup ('no driver for model X' / 'driver lacks Chat'), replacing the provider×capability matrix with 'is a driver registered for this model'.

## Entry point

internal/driver/registry.go + the resolution call in internal/run/run.go (ResolveImplementerModel).

## In scope

- New `internal/driver/registry.go`: name→Driver registration + `Resolve(modelID, requiredCap) (Driver, error)`.
- Resolution fails fast with a descriptive error when no driver matches or the driver lacks the required capability.
- Default agentic role → subprocess driver; OAI models → oai driver; keyless/verify-only mapped explicitly.

## Acceptance checks

- [ ] WHEN `Resolve` is called with a model whose provider has no registered driver, THE SYSTEM SHALL return a descriptive error before any dispatch.
- [ ] WHEN the resolved driver lacks the capability the role requires (e.g. Chat for implementer), THE SYSTEM SHALL fail fast at resolution with the driver name and missing capability.
- [ ] WHEN resolution succeeds, THE SYSTEM SHALL return a Driver ready to Dispatch.

## Planned touchpoints

- `internal/driver/registry.go`
- `internal/driver/registry_test.go`
- `internal/run/run.go`

## Required tests

- `go test ./internal/driver/... ./internal/run/... -run TestResolve`

## Deferrals allowed?

No.
