---
title: 'S02-lint-contracts-mock-parity'
description: '`sworn lint contracts` gains the Rule-10 mock-parity checks: when a contract names `fixtures`, a consumer slice may mock that registered boundary only with the owners recorded fix'
---

# Slice: `S02-lint-contracts-mock-parity`

## User outcome

`sworn lint contracts` gains the Rule-10 mock-parity checks: when a contract names `fixtures`, a consumer slice may mock that registered boundary only with the owner's recorded fixtures — the gate FAILs if the fixture file is missing or older than the owner's last production-code commit touching the surface, and FAILs if a consumer's tests mock the boundary without importing the fixture path AND without at least one unmocked in-process round-trip. This closes the seam where a mock encodes the consumer's own wrong assumption of the boundary.

## In scope

- Fixture-freshness check: when a contract names `fixtures`, FAIL if the fixture file does not exist OR is older (in git history) than the owner slice's last production-code commit touching the surface
- Consumer mock-parity: FAIL if a consumer slice's tests mock a registered boundary WITHOUT importing from the contract's fixture path AND WITHOUT at least one unmocked in-process round-trip against the real handler (grep-level detection to start)
- Wire the mock-parity checks into `sworn lint contracts` as an additional fail-closed check family (extends S01's internal/lint/contracts.go)
- Test fixtures reproducing the fired seam-2 case: a consumer mock that loads no owner-recorded fixture → FAIL

## Out of scope

- The registry-grading checks (S01 owns schema-validate / wire-ref / live_test / ownership)
- sworn assemble (T2)
- Deep AST-level mock detection — grep-level is the ratified starting point (proposal Rec 2); a tighter detector is a future follow-up
- The existing `sworn lint mock` subcommand — mock-parity here is contracts-registry-scoped, distinct from that gate

## Acceptance criteria

- [ ] AC-01: When a contract names `fixtures` and the fixture file is missing OR older in git history than the owner slice's last production-code commit touching the surface, `sworn lint contracts` SHALL exit non-zero naming the contract, the stale/missing fixture, and the owner commit.
- [ ] AC-02: When a consumer slice's tests mock a boundary registered in contracts.json without importing from the contract's fixture path AND without at least one unmocked in-process round-trip against the real handler, `sworn lint contracts` SHALL exit non-zero naming the consumer slice and the boundary — proven against a fired seam-2 fixture (a mock loading no owner-recorded fixture → FAIL).
- [ ] AC-03: `sworn lint contracts` SHALL exit 0 for a consumer that mocks a registered boundary but does import the owner fixture, or that includes an unmocked in-process round-trip (the escape hatch), and `go test ./internal/lint/...` SHALL pass.
