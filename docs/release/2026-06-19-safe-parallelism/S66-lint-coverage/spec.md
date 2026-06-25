---
title: 'Slice spec — S66-lint-coverage'
description: 'Port release-coverage.sh from bash to Go: `sworn lint coverage` — mechanically maps every spec AC to a test function in the slice diff, flagging uncovered ACs.'
---

# Slice: S66-lint-coverage

## User outcome

A developer runs `sworn lint coverage --slice <id> --release <name>` and receives a coverage map showing each acceptance check linked to its matching test function (file:line). Exits 0 when every AC has a matching test, non-zero with uncovered ACs enumerated.

## Entry point

New `internal/gate/coverage.go`. CLI via `internal/command` registry. Invoked as `sworn lint coverage`.

## In scope

- Extract AC checkboxes from `spec.md`
- Scan test files in the slice's diff for matching test function names
- Keyword-match AC text against test function names (Go, TypeScript, Python patterns)
- Output coverage map: AC-01 → TestFoo in bar_test.go:23
- Exit 0 on full coverage, 1 with uncovered ACs

## Out of scope

- Semantic coverage validation (is the test genuine?) — that's the LLM check (S70)
- Coverage reports for non-test files

## Planned touchpoints

- `internal/gate/coverage.go` (new)
- `internal/gate/coverage_test.go` (new)
- `cmd/sworn/lint.go` (extend)

## Acceptance checks

- [ ] `sworn lint coverage --slice <id> --release <name>` maps every AC to a test
- [ ] Flags uncovered ACs with candidates considered
- [ ] Recognises Go `func TestXxx`, TS `it('...')`/`test('...')`, Python `def test_xxx`
- [ ] Exits 0 when all ACs covered, 1 with gaps

## Required tests

- **Unit**: `internal/gate/coverage_test.go` — fixture with known ACs and test functions
- **Reachability artefact**: `sworn lint coverage` output showing PASS/FAIL
- **E2E gate type**: local
