---
title: 'Slice spec — S69-lint-regress'
description: 'Port release-regression.sh from bash to Go: `sworn regress` — runs the full test suite (Go + TS + golden fixtures) against the merged release-wt worktree. Post-merge regression gate.'
---

# Slice: S69-lint-regress

## User outcome

A developer runs `sworn regress --release <name>` after all tracks have merged into release-wt. The command runs the full test suite, checks golden fixtures for divergence, and exits 0 only when everything passes. Catches semantic regressions where two independently-verified tracks break when combined.

## Entry point

New `internal/gate/regress.go`. CLI via `internal/command` registry. Invoked as `sworn regress` (separate from `sworn lint` — heavyweight, runs full test suite).

## In scope

- Run Go test suite against the merged release-wt worktree
- Run TypeScript test suite if pnpm is available
- Check golden fixture scenarios for divergence
- Report per-suite pass/fail status
- Exit 0 on all-pass, 1 on any failure

## Out of scope

- Running the test suite per-slice (that's implementer/verifier territory)
- Modifying test configuration

## Planned touchpoints

- `internal/gate/regress.go` (new)
- `internal/gate/regress_test.go` (new)
- `cmd/sworn/regress.go` (new)

## Acceptance checks

- [ ] `sworn regress --release <name>` runs all Go tests against release-wt
- [ ] Reports per-suite pass/fail status
- [ ] Golden fixture divergence detected and reported
- [ ] Exits 0 on clean, 1 on failure
- [ ] Handles missing test suites gracefully (no pnpm → skip TS tests)

## Required tests

- **Unit**: `internal/gate/regress_test.go` — fixture with mock test output
- **Reachability artefact**: `sworn regress` output on a merged test release
- **E2E gate type**: local
