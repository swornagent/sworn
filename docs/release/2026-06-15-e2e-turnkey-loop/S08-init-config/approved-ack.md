TL;DR clean design, well-scoped. 7 pins + 2 flags:

1. **AC1 narrowing.** AC1 is cross-slice (depends on S07). Confirm S08 delivers config infra + init; AC1 closes when S07 lands.
2. **Config idempotency.** What happens when config.json exists? Pick skip-with-message, prompt-to-overwrite, or merge. Document in §2.
3. **Missing-key error UX.** Design the error path: what does `sworn verify` output when no key is configured? Don't rely on "surfaces naturally" inference.
4. **Config location docs.** Spec Risk requires documenting config location. Add to `sworn help` or README.
5. **Smoke test missing-key path.** Add a smoke test exercising the no-key error case (not just the happy path with `--api-key test-key-123`).
6. **AGENTS.md fragment source.** Where does the seven-rule fragment text come from? Hardcode as Go constant, add to `internal/prompt/` embed, or read from project AGENTS.md. Document in §3.
7. **main.go merge with S07.** Both add new case entries to same switch. Pick insertion convention (alphabetical: `init` before `run`) for trivial merge.

Flags (not pins): (a) confirm BatonVersion() format matches expected docs/baton/VERSION content; (b) consider role-name consistency between config.ModelConfig keys and embedded prompt role names.

§2 decisions 1–5 ack. §6 empty ack.

Address pins 1–7 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 7 pins are apply-inline mechanical corrections (source gaps, unspecified behaviours, missing smoke test); none requires design re-review or Coach authority.
-->
