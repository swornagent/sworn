TL;DR Clean design, well-scoped. 4 pins + 2 flags:

1. **INCONCLUSIVE verdict gap.** The new verifier prompt supports `INCONCLUSIVE: <reason>` as a fourth verdict, but `parseVerdict` and `internal/verdict/verdict.go` only handle PASS/FAIL/BLOCKED. Add `INCONCLUSIVE` to the verdict contract + parseVerdict, with exit code 3 (non-zero, fail-closed, distinct from BLOCKED's 2). This is in-scope because S04 owns the prompt→parser boundary. If you believe INCONCLUSIVE belongs in a future slice, add a NOT-doing item and a tracking note.

2. **Memory ack — Baton protocol alignment.** Vendor all four prompts (including captain) confirmed as aligned with [[project_baton_extraction]] and [[project_swornagent_baton_brand]] — both MIT-licensed, public/open. Acked. One incidental: the captain prompt has a single "S21 stall, 2026-05-30" reference — generic enough for open-source, but Coach may want to scrub.

3. **VERSION.txt bump tracking.** The "future updates must bump VERSION.txt" rule lives only in this design doc. Add a comment at the top of `VERSION.txt` itself so the rule is co-located with the version number it governs.

4. **Prompt test negative check.** The planned unit test checks for PASS/FAIL/BLOCKED — but so does the old placeholder. Add one assertion that the embedded prompt ≠ the old const, or check for a token the placeholder lacks (e.g., `INCONCLUSIVE`).

Flags (not pins): (a) `cmd/sworn/main.go` is documented shared — S02 touched the `verify` case, S04 touches `version` — additive, region-separable, no collision; (b) `version` inline in main.go is fine despite the "one file per subcommand" convention — two-line print doesn't warrant its own file.

§2 decisions 2–5 ack (no memory conflicts). §6 empty — ack.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline corrections (add INCONCLUSIVE to verdict contract + parseVerdict, memory-ack, VERSION.txt comment, negative test assertion) — no design re-review needed.
-->
