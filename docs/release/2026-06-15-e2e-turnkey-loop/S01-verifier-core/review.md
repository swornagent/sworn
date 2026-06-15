# Captain review — S01-verifier-core
Date: 2026-06-15
Captain version: 0.1
Design TL;DR commit: 0848e400fe3bbcdbd8145a437797c106b3922fdb

## Pins

1. **[mechanical] §1 — AC2/AC3/AC4 BLOCKED paths not reflected in user-visible change paragraph**
   What I observed: §1 says "A developer runs `sworn verify --spec <path> --diff <path>` and gets a JSON verdict printed to stdout. The process exits 0 only on PASS; FAIL exits 1 and BLOCKED exits 2." It does not mention what happens with empty/missing inputs (AC2: `first_pass:*` BLOCKED), an unconfigured model (AC3: `verifier_dispatch` BLOCKED), or an unparseable verdict (AC4: `unparseable_verdict` BLOCKED). All three are described in §2/§3/§4 and already implemented and tested in code.
   What to ask the implementer: Add a one-line note to §1 covering the BLOCKED paths, or confirm the current §1 scope (PASS/FAIL exit codes only) is intentional for a "user-visible" summary.

2. **[mechanical] §2.5 — `--proof` and `--verifier-model` CLI flags are forward-compatible scope not in spec**
   What I observed: The code wires `--proof` (optional proof bundle) and `--verifier-model` (model selection) flags. The spec's In Scope is: verdict contract + verifier interface stub + deterministic first-pass. Neither flag is required by the spec; design §2.5 acknowledges --proof as forward-compatible. The spec mentions proof bundles arrive in S05+ and real model dispatch in S02, so these are benign pre-wiring.
   What to ask the implementer: Confirm the Coach accepts these forward-compatible flags landing in S01 (they're zero-cost, already pass tests, and avoid later CLI churn).

## Summary

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none (both are documentation polish; all 4 ACs pass in code)

## Smaller flags (not pins, worth one-line ack)

- `TestRun_MissingSpecBlocks` tests empty content but not a missing file path. A test with a non-existent path would tighten the first-pass gate. Code handles it correctly (os.ReadFile error → BLOCKED), so this is coverage polish, not a gap.
- `internal/verify/verify_test.go` is in `actual_files` but not in `planned_files`. The spec's Required Tests section names it, so it should be in `planned_files` for tracking accuracy.

## Suggested ack reply

TL;DR clean — scaffold matches spec and all 4 ACs pass. 2 pins + 2 flags:

1. **§1 BLOCKED paths.** §1 covers PASS/FAIL exit codes but not the three BLOCKED paths (empty input, unconfigured model, unparseable verdict). Add a one-liner to §1 or ack as-is.
2. **Forward-compatible --proof and --verifier-model flags.** Code pre-wires these; spec doesn't require them until S02/S05. Ack that the pre-wiring is acceptable.

Flags (not pins): (a) missing-file test gap — test empty content but not non-existent path; (b) verify_test.go missing from planned_files in status.json.

§2 decisions all clean — typed constants, Unconfigured sentinel, prefix parse, fail-closed exit codes, optional --proof flag — all ack. §6 empty — no open questions.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 ACs pass; 2 mechanical pins are documentation polish only; scaffold code is sound
-->