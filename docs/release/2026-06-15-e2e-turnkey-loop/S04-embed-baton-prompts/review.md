# Captain review — S04-embed-baton-prompts
Date: 2026-06-16
Captain version: 0.1
Design TL;DR commit: 37b8ddfb13bdeefe76eeda74ca569d639712f85c

## Pins

1. **[mechanical] §3 (internal/verify/verify.go) — INCONCLUSIVE verdict gap**
   What I observed: The embedded Baton verifier prompt supports `INCONCLUSIVE: <reason>` as a fourth verdict type (the prompt says: "You return exactly one of: PASS, FAIL, BLOCKED, or INCONCLUSIVE"). But `parseVerdict` in `internal/verify/verify.go` only handles PASS/FAIL/BLOCKED. The verdict contract in `internal/verdict/verdict.go` also only defines `Pass`, `Fail`, `Blocked`. An INCONCLUSIVE reply will fall through to the default unparseable→BLOCKED path — but the verifier prompt explicitly says "Do NOT write verification.result: blocked for an INCONCLUSIVE outcome." The recovery semantics differ: BLOCKED → replan, INCONCLUSIVE → re-verify in a clean session.
   What to ask the implementer: Either add `INCONCLUSIVE` to the verdict contract (`internal/verdict/verdict.go`) and `parseVerdict`, or explicitly document in a NOT-doing item that S04 intentionally defers INCONCLUSIVE support to a future slice. If adding, the exit code should be non-zero (fail-closed) but distinct from BLOCKED — consider exit code 3.

2. **[memory-cited] §2.1 — All four prompts align with Baton protocol**
   What I observed: Decision 1 (vendor all four role prompts: verifier, planner, implementer, captain) aligns with [[project_baton_extraction]] (baton is public/open at github.com/sawy3r/baton) and [[project_swornagent_baton_brand]] (SwornAgent on open Baton protocol, MIT licence). The captain prompt is part of the open Baton protocol and carries an MIT licence.
   What to ask the implementer: Ack confirms the citation. One incidental note: the captain prompt contains a single historical incident reference ("S21 stall, 2026-05-30") — generic enough for open-source, but flag if Coach wants to scrub before going public.
   Citation: [[project_baton_extraction]], [[project_swornagent_baton_brand]]

3. **[mechanical] §2.2 — VERSION.txt bump process has no tracking**
   What I observed: The design says "future prompt updates must bump this [VERSION.txt]" but provides no mechanism, hook, or documentation for ensuring this happens. The requirement lives only in this design doc's prose.
   What to ask the implementer: Add a comment in `VERSION.txt` itself (e.g., `# Bump this version whenever prompt files are re-vendored from upstream Baton`) so the rule is co-located with the thing it governs. Or add a section to `internal/prompt/README.md`.

4. **[mechanical] §5 — Reachability plan: prompt test doesn't distinguish from placeholder**
   What I observed: The reachability plan says the unit test "asserts `prompt.Verifier()` is non-empty and contains the PASS/FAIL/BLOCKED verdict-contract instruction." But the current placeholder `const systemPrompt` also contains PASS/FAIL/BLOCKED. A bug that silently re-embeds the placeholder (e.g., a wrong vendoring path) would pass this test. The test needs a negative check.
   What to ask the implementer: Add one assertion that the embedded prompt does NOT equal the old placeholder string, or check for a distinctive token the placeholder lacks (e.g., `INCONCLUSIVE`, `adversarial verification`, or `track worktree precondition`). This catches a silent vendoring failure at test time.

## Summary

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): Pin 1 — if unaddressed, INCONCLUSIVE verdicts from the model will be silently misrouted to BLOCKED, causing spurious replans and breaking the verifier prompt's contract.

## Smaller flags (not pins, worth one-line ack)

- `cmd/sworn/main.go` is the documented shared file; S02 already modified the `verify` case. S04's change to the `version` case is additive and region-separable — no collision, just a note to the implementer to keep the edit minimal.
- The `version` subcommand currently lives inline in `main.go` (not its own file). The AGENTS.md convention says "one file per subcommand" — but for a two-line print, inline is fine. No action needed.

## Suggested ack reply

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