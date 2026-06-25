---
title: 'Slice spec — S68-lint-mock'
description: 'Port release-mock-check.sh from bash to Go: `sworn lint mock` — Rule 10 no-mock boundary enforcement, scanning test files for undeclared mock/stub/fixture usage.'
---

# Slice: S68-lint-mock

## User outcome

A developer runs `sworn lint mock --slice <id> --release <name>` and receives a report of any undeclared mock boundaries: test files using mocks/stubs/fixtures/seeded data without declaring the boundary. Declared boundaries (via `@mock-boundary` comment, `open_deferrals`, or `architecture-overrides.json`) are accepted. Exits 0 on clean, 1 with violations.

## Entry point

New `internal/gate/mock.go`. CLI via `internal/command` registry. Invoked as `sworn lint mock`.

## In scope

- Scan test files in the slice's diff for mock/stub/fixture/keyword patterns
- Detect real-infra references alongside mock usage (undeclared boundary)
- Check for boundary declaration markers (`@mock-boundary`, `open_deferrals` entries, `architecture-overrides.json`)
- Output: structured JSON + human-friendly text

## Out of scope

- Runtime enforcement of mock boundaries (static analysis only)
- Validating test data is actually fake (semantic check — LLM territory)

## Planned touchpoints

- `internal/gate/mock.go` (new)
- `internal/gate/mock_test.go` (new)
- `cmd/sworn/lint.go` (extend)

## Acceptance checks

- [ ] Detects mock/stub/fixture/seed usage in test files
- [ ] Detects real-infra references (localhost:5432, DB_URL, AUTH0_DOMAIN) alongside mocks
- [ ] Accepts `@mock-boundary` comments as declared boundaries
- [ ] Accepts `open_deferrals` entries mentioning mocks
- [ ] Accepts `architecture-overrides.json` suppressed rules
- [ ] Exits 0 on clean, 1 with violations

## Required tests

- **Unit**: `internal/gate/mock_test.go` — fixture test files with declared and undeclared mocks
- **Reachability artefact**: `sworn lint mock` output showing violations
- **E2E gate type**: local
