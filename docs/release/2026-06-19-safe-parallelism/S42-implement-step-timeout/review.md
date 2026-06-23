# Captain review — S42-implement-step-timeout
Date: 2026-07-06
Design commit: cc674991c94f755c3a1e36745494e8ad619c2663

## Pins

1. [mechanical] §4.5 / AC5 — Config-file precedence tier silently dropped (CRITICAL)
   What I observed: Design §4 NOT-doing item 5 states "NOT adding a config-file reader — there's no precedent in the codebase today." This is false. `internal/config/config.go` already provides a JSON config reader; its package comment reads "loads sworn's configuration with precedence: env > file > default." Moreover, `cmd/sworn/run.go`'s `resolveVerifierModel()` (line 110-117) already calls `config.Load()` — the pattern is established. Spec AC5 requires **flag > env > config > default** precedence. The design's resolved precedence is **flag > env > default**, silently dropping the config tier. A verifier running AC5 will FAIL.
   What to ask the implementer: (a) Add `Implementer struct { Timeout string \`json:"timeout"\` }` (or `Timeout duration.Duration`) to `internal/config/Config`. Because `time.Duration` is not native JSON, parse it as a string via `time.ParseDuration`. (b) Add `internal/config/config.go` to `planned_files`. (c) Implement `resolveImplementTimeout(flagVal, envVal string, cfgTimeout time.Duration) time.Duration` with precedence: flag (if non-zero) > env (if set) > cfg (if non-zero) > default constant. This brings AC5 into compliance.

2. [mechanical] Step 2b — `design_decisions` absent from status.json
   What I observed: S42's `status.json` has no `design_decisions` array. The design.md §2 lists 5 decisions. Per the established T12 pattern (S41, S38, S37, S36, S35, S35 all had this absence flagged), `sworn designfit` reads the `design_decisions` field from status.json — with the field absent the gate trivially passes even for architecturally-significant choices. This is the 6th consecutive T12 slice with this absence.
   What to ask the implementer: Populate `design_decisions` in `status.json` before transitioning to `in_progress`, mirroring the 5 §2 decisions. Use the structure from S41 as the template (choice, stake_class, options, rationale). All 5 decisions are plausibly Type-2 (reversible, confined to this slice).

3. [memory-cited] §2.D2 — DeadlineExceeded vs model.Error{Kind} orthogonality
   What I observed: Design Decision 2 uses `errors.Is(err, context.DeadlineExceeded)` to detect a timeout and escalate via the existing implementer-error path. Memory [[project_provider_error_taxonomy]] records that S44-feedback-driven-retry will layer `model.Error{Kind}` onto the same escalation path in `slice.go` — terminal Kind (Auth/Credits) → PAGE (no escalation), transient Kind → backoff on same model. A `context.DeadlineExceeded` is a sworn-internal signal, not a provider error, so it will not carry a `model.Error{Kind}`. Confirm the intended S44 interaction: when S44 adds Kind-based routing, `DeadlineExceeded` should still fall through to the existing "escalate to next model" path (i.e. it's treated as a non-typed implementer error), unless S44 specifically handles it as a dedicated Kind. This is not blocking S42 but needs acknowledgement so S44 is spec'd consistently.
   Citation: [[project_provider_error_taxonomy]]

Pins: 3 total — 2 [mechanical], 0 [memory-cited cited without confirmation], 1 [memory-cited]
Critical pins: Pin 1 (AC5 fails as designed; slice ships incomplete without the config tier).

## Summary

3 pins (2 mechanical, 1 memory-cited). Pin 1 is critical — the config-file precedence tier is demonstrably present in the codebase and required by AC5; shipping without it fails the verifier. Pin 2 is a recurring T12 hygiene miss (6th consecutive occurrence). Pin 3 is a forward-compatibility ack for the S44 interaction. All three are apply-inline.

## Smaller flags (not pins, worth one-line ack)

(a) **RetryCap analogy description slightly misleading**: Design Decision 3 says "matches the existing `RetryCap: -1` pattern where a zero/sentinel triggers the default." In live code, `RetryCap: -1` (negative) triggers the default; `RetryCap: 0` means "single attempt." The proposed `ImplementTimeout: 0 = use default` is a different convention (zero-value as sentinel, which is idiomatic for `time.Duration`). The approach is sound; just strike "matches the existing RetryCap: -1 pattern" or rephrase to "uses Go's zero-value-as-unset convention."

(b) **S44 same-file sequencing**: S44-feedback-driven-retry (T12, planned) also touches `internal/run/slice.go` and `internal/run/slice_test.go`. Being in the same serial worktree, there is no merge conflict risk, but S44's hunk must confine itself to the Kind-based routing layer and leave S42's `DefaultImplementTimeout` constant and `ImplementTimeout` field intact. No action needed for S42; worth noting for the S44 design brief.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Solid design with 3 apply-inline pins:

1. **Config tier missing from AC5 precedence chain (critical).** `internal/config/config.go` exists and has a JSON reader; `resolveVerifierModel()` in `cmd/sworn/run.go` already calls `config.Load()`. Add `Implementer.Timeout string \`json:"timeout"\`` to `Config`, parse it with `time.ParseDuration`, add `internal/config/config.go` to `planned_files`, and implement `resolveImplementTimeout()` with precedence flag > env > config.Load() > default. This brings AC5 into compliance.

2. **Populate `design_decisions` in `status.json` before transitioning to `in_progress`.** Mirror the 5 §2 decisions using S41's structure (choice, stake_class "Type-2", options, rationale). `sworn designfit` reads this field; absent = trivially-passes, which defeats the gate.

3. **S44 DeadlineExceeded interaction — add one-line ack.** In design.md §2.D2 or §4, note that `context.DeadlineExceeded` is a sworn-internal signal (not a `model.Error{Kind}`), so S44's Kind-based routing will leave it on the existing "escalate to next model" path. This forward-documents the seam for the S44 implementer.

Flags (not pins): (a) Strike "matches RetryCap: -1 pattern" from Decision 3 — use "Go zero-value-as-unset (`time.Duration`)." (b) S44 shares `slice.go` and `slice_test.go` — second-lander confines hunks; no action for S42.

§2 decisions D1/D3/D4/D5 ack (all Type-2, mechanical). D2 ack per [[project_provider_error_taxonomy]] — orthogonal to model.Error{Kind} as documented above. §6 open questions: none — ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both critical pins are mechanical apply-inline corrections (add config tier to AC5 chain, populate design_decisions); no design re-check needed before code.
-->
