<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR clean second-round design — existing code is correct, scope narrows to proof production. 3 pins + 3 flags:

1. **No rewrite.** anthropic.go and anthropic_test.go are already committed at `810d7ce` from round 1. The round-1 BLOCKED was process-level (cmd/sworn/run.go touchpoint collision), not a code issue. Scope for round 2: run `go test ./internal/model/... -run Anthropic` (confirm 4/4 pass), run `go test ./internal/model/...` (no OAI regression), populate `actual_files` + `reachability_artifacts` from existing state, produce proof.md citing existing tests and commit `810d7ce`, transition to `implemented`. Do NOT rewrite anthropic.go or anthropic_test.go.
2. **Error taxonomy non-HTTP gap.** `anthropicStatusCode()` has a fallback path (returns `(0, false)`) that emits `fmt.Errorf` instead of a typed `model.Error`. Confirm S44's retry policy handles plain errors gracefully (e.g., treats them as Transient). If not, add `NewProviderError(0, "anthropic", a.Model, nil)` with `Kind=KindTransient` for that path. Add a code comment documenting the intent either way.
3. **SDK and error-taxonomy memory acks.** §2.1 SDK choice confirmed against [[project_dep_policy]] + [[feedback_dep_justification_test]]. §2.3 NewProviderError() confirmed against [[project_provider_error_taxonomy]] for the HTTP path. Both acked.

Flags (not pins): (a) `design_decisions` absent from status.json — designfit passes (empty → skip), but worth back-filling the 5 §2 decisions as Type-2 for audit completeness; (b) T5 134 commits behind release-wt — verified: zero touches on S11 artefacts, no stale-spec concern; (c) round-1 proof.md must be replaced with a fresh bundle for round 2.

§2 decisions 1–5 acked (all Type-2, no human_decision required). §6 empty (no open questions).

Address Pin 1 as the primary implementation directive; Pins 2–3 inline during proof production. Proceed to `in_progress`.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are apply-inline — Pin 1 is a clear no-rewrite directive for an already-working implementation; Pins 2-3 are memory acks with a documentation nudge for the non-HTTP error path. No design change required before code; existing implementation is correct.
-->
