---
title: 'Slice spec — S67-lint-design'
description: 'Port release-audit-design.sh from bash to Go: `sworn lint design` — hardcoded colour detection + architecture rule engine (grep, touchpoints, diff-size, external check types).'
---

# Slice: S67-lint-design

## User outcome

A developer runs `sworn lint design --slice <id> --release <name>` and receives a structured report of design conformance violations: hardcoded colour values, architecture rule violations, touchpoint discipline breaks, file size growth, and external SAST/lint tool results.

## Entry point

New `internal/gate/design.go`. CLI via `internal/command` registry. Invoked as `sworn lint design`. Reads project-level `architecture.json` for rule configuration.

## In scope

- Hardcoded colour detection (hex, rgb, hsl) in UI files from the diff
- Architecture rule engine supporting 4 check types:
  - `grep`: regex pattern match in changed files
  - `touchpoints`: verify changed files are in planned touchpoints
  - `diff-size`: file growth and absolute size limits
  - `external`: invoke external tool and parse exit code
- Read `docs/baton/architecture.json` for rule config
- Read `docs/baton/design-fidelity.json` for design system config
- Read per-slice `design-allowlist.json` for escape hatch
- Output: structured JSON + human-friendly text
- Exit 0 on clean pass, 1 with enumerated violations

## Out of scope

- Executing external tools that aren't installed (graceful skip with warning)
- Modifying rule config (read-only)

## Planned touchpoints

- `internal/gate/design.go` (new)
- `internal/gate/design_test.go` (new)
- `internal/gate/archrules.go` (new — architecture rule engine)
- `cmd/sworn/lint.go` (extend)

## Acceptance checks

- [ ] Detects hardcoded hex/rgb/hsl colours in UI files
- [ ] Reads and applies architecture.json rules
- [ ] grep check: flags regex matches in changed files
- [ ] touchpoints check: flags files outside planned touchpoints
- [ ] diff-size check: flags files exceeding growth/absolute limits
- [ ] external check: invokes tool and flags on non-zero exit
- [ ] Reads design-allowlist.json and suppresses matching violations
- [ ] Skips test files by default

## Required tests

- **Unit**: `internal/gate/design_test.go` — fixture with known colour violations and architecture rules
- **Unit**: `internal/gate/archrules_test.go` — each check type with fixture data
- **Reachability artefact**: `sworn lint design` output showing violations
- **E2E gate type**: local
