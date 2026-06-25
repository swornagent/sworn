# Captain review — S09-per-role-model-config
Date: 2026-06-23
Design commit: e1f0b2eb5ac91f374411d7701591dd2821a12c2b

## Pins

1. [mechanical] §2b/status.json — `design_decisions` field absent from status.json
   What I observed: Five decisions documented in design.md §2, but status.json has no
   `design_decisions` key at all. The trial log flagged this same gap for S23 ("Most
   valuable: design_decisions absent from status.json — Type-1 decisions bypass designfit
   gate"). Decisions D1 (shared ModelSetting struct) and D4 (replacing resolveVerifierModel
   in run.go) are architecturally significant choices; they must be classified and recorded.
   What to ask the implementer: Add `design_decisions` array to status.json before
   transitioning to in_progress. Use the S51 status.json as a format reference (single
   string per entry, one per §2 decision).

2. [mechanical] §5/reachability — `sworn init --yes` behavior for new prompts is unresolved
   What I observed: Design §5 smoke step says "Run `sworn init --yes` with piped stdin
   providing implementer model, escalation list, and max attempts." But the existing
   init.go pattern guards every interactive prompt with `!*yes`: API key prompt fires only
   when `key == "" && !*yes` (line 189); `config.PromptDesignSystem` is passed `*yes`
   (line 200). If S09 follows this pattern, `--yes` would skip the new prompts and use
   defaults — the smoke test needs no piped stdin, just a check that defaults appear in the
   written config. If S09 breaks the pattern (always prompts), that inconsistency should be
   explicit. Either way, §5's procedure must match the actual implementation choice.
   What to ask the implementer: Choose: (a) new prompts respect `--yes` → smoke step
   asserts defaults in written config, no piped stdin needed; OR (b) new prompts always
   fire → document the deliberate break from existing pattern. Update §5 / proof.md
   smoke step accordingly.

3. [memory-cited] §2 Decision 3 — `ResolveEscalationModels` shape must be S44-compatible
   What I observed: Decision 3 introduces the EscalationModels fallback. The resolved
   `[]string` slice passes directly into `run.Options.EscalationModels`. S44-feedback-driven-
   retry consumes this slice via `model.Error{Kind}` to control which model to try next
   (slice.go:110 already uses `DefaultEscalationModels` when EscalationModels is nil).
   The design does not acknowledge this downstream consumer.
   What to ask the implementer: Confirm `ResolveEscalationModels` returns the slice
   unmodified (no dedup, no filtering), so S44 inherits the ordered escalation path
   as configured. The existing `run.Options.EscalationModels` wire in run.go is the
   correct connection point.
   Citation: [[project_provider_error_taxonomy]]

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: none (no pin causes the slice to ship broken if unaddressed, though
Pin 2 will produce an invalid proof.md smoke step if the procedure isn't reconciled)

## Summary

Design is sound: all 7 ACs are addressed, spec risks are acknowledged, `DefaultEscalationModels`
confirmed at `run/run.go:29` (§4 NOT-doing claim verified), `resolveVerifierModel` replacement
is a net simplification (confirmed from live code). Three lightweight mechanical/memory pins.

## Smaller flags (not pins, worth one-line ack)

(a) `config.ResolveVerifierModel` returns `(string, error)` vs run.go's `resolveVerifierModel`
    returning `string`. The error path in cmdRun needs to change from `if verifier == ""` to
    `if err != nil`. Trivial but easy to overlook.

(b) S10, S17, S54, S56 all list `internal/config/config.go` in planned_files. No active
    collision — all are planned and downstream of T3-commercial. Downstream implementers
    should know S09 expanded `ModelSetting` to add `EscalationModels []string` and
    `MaxAttempts int`.

(c) Design Decision 1 notes verifier JSON will gain zero-valued `escalation_models: []` and
    `max_attempts: 0` after round-trip. If downstream consumers parse `escalation_models`
    from verifier config (they shouldn't per Decision 1's rationale), they'll see an empty
    slice, not absence. Confirm no consumer assumes `escalation_models` absent = verifier
    section (it won't be absent after S09 writes the file).

## Suggested ack reply
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
