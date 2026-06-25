<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is solid — `DefaultEscalationModels` verified at `run/run.go:29`, AC coverage clean,
Risk 1/2 acknowledged. 3 pins (2 mech, 1 mem) — apply inline before writing code:

1. **status.json design_decisions.** Add `design_decisions` array to status.json before
   transitioning to in_progress. One string per §2 decision (5 entries). See S51 status.json
   for format.

2. **`--yes` behavior for new prompts.** Pick one: (a) new prompts respect `!*yes` → use
   defaults when `--yes` is passed, smoke test just inspects written config for defaults; OR
   (b) always prompt → document the deliberate break. Update proof.md smoke step to match
   whichever choice you make. The existing pattern (API key, design system) is (a).

3. **EscalationModels pass-through.** `ResolveEscalationModels` must return the configured
   slice unmodified. S44-feedback-driven-retry inherits this slice via `run.Options.EscalationModels`
   (slice.go:110) — no dedup, no filtering.

Flags (not pins): (a) `ResolveVerifierModel` returns `(string, error)` — change run.go's
`if verifier == ""` guard to `if err != nil`; (b) S10/S17/S54/S56 also touch config.go —
expand ModelSetting cleanly; (c) verifier round-trip will gain zero-valued escalation_models
field — confirm no consumer checks for absence.

§2 decisions D1–D5 ack — clean after status.json update. §6 empty ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline corrections (missing status.json field, smoke-step procedure choice, memory-cited shape confirmation) — none require re-reviewing the design before code.
-->
