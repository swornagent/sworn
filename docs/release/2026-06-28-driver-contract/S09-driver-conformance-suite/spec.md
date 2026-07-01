---
title: 'S09-driver-conformance-suite'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S09-driver-conformance-suite`

## User outcome

Every Driver implementation passes one behavioural conformance suite (content always emitted, exit-on-no-tools, normalized Result shape, fail-closed on error), so a new driver is provably contract-correct.

## Entry point

internal/driver conformance suite run against subprocess + oai (+ a fake) drivers.

## In scope

- A table-driven conformance suite asserts the contract for any Driver: Result shape, never-nil, error-not-panic, content-always-present (oai), exit-on-no-tools.
- Run the suite against the subprocess driver, the oai driver, and a fake driver.

## Acceptance checks

- [ ] EACH registered Driver SHALL pass the conformance suite (Result well-formed, no panic on error paths).
- [ ] THE oai driver SHALL emit a present `content` field on tool-only turns under the conformance check.
- [ ] A driver that violates the contract SHALL fail the suite with the violated clause.

## Planned touchpoints

- `internal/driver/conformance_test.go`

## Required tests

- `go test ./internal/driver/... -run TestDriverConformance`

## Deferrals allowed?

No.
