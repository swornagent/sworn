---
title: S01-verifier-core
description: Fail-closed verdict contract + deterministic first-pass + model stub. Already implemented (the scaffold).
---

# Slice: `S01-verifier-core`

## User outcome

A developer runs `sworn verify --spec <p> --diff <p>` and receives a fail-closed
JSON verdict (PASS/FAIL/BLOCKED); the process exits `0` only on PASS.

## Entry point

CLI: `sworn verify` (`cmd/sworn`).

## In scope

- Verdict contract (`internal/verdict`): PASS/FAIL/BLOCKED + exit-code mapping
  (0/1/2; anything not PASS is non-zero).
- Verifier interface + `Unconfigured` stub (`internal/model`) — fails closed.
- Verification orchestration (`internal/verify`): a stub deterministic first-pass
  (spec/diff non-empty) → model dispatch → conservative verdict parse.

## Out of scope

- Real model dispatch (S02), enriched first-pass (S03), embedded prompt (S04).

## Planned touchpoints

- `cmd/sworn/main.go`, `internal/verdict/`, `internal/model/client.go`,
  `internal/verify/`

## Acceptance checks

- [x] PASS→exit 0, FAIL→1, BLOCKED→2.
- [x] Missing/empty spec or diff → BLOCKED (`first_pass:*`).
- [x] Unconfigured model → BLOCKED (fail-closed).
- [x] A reply not starting with PASS/FAIL/BLOCKED → BLOCKED (`unparseable_verdict`).

## Required tests

- **Unit**: `internal/verify/verify_test.go` (PASS, empty-spec block,
  unconfigured-model block, garbled-verdict block).
- **Reachability artefact**: `sworn verify --spec <f> --diff <f>` prints a JSON
  verdict and sets the exit code.

## Risks

- Verdict-parse false PASS — mitigated by conservative prefix match + fail-closed
  default (covered by `TestRun_GarbledVerdictBlocks`).

## Deferrals allowed?

No.
