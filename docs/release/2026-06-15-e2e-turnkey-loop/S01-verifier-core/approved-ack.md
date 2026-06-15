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
