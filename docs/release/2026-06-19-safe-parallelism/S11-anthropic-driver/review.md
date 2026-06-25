# Captain review — S11-anthropic-driver
Date: 2026-07-08
Design commit: 1b4c8748c9c5eefdf082e807bc3754a43df4af11

> **Context note:** This is the **second-round** design review. S11 was fully
> implemented in commit `810d7ce`, then the verifier returned BLOCKED for a
> process-level reason (cmd/sworn/run.go listed in `planned_files` but not
> touched — track-mode invariant 4 conflict). The replan at `3210e0a` corrected
> `planned_files` and reset `state → planned`. The code from round 1 is still
> committed and present on the branch. This review checks the revised design
> (cmd/sworn/run.go absent) for soundness before the second implement pass.

## Pins

1. **[mechanical] §3 — Re-entry semantics: anthropic.go and anthropic_test.go are NOT new files**
   What I observed: Design §3 labels `internal/model/anthropic.go` as "New driver" and `internal/model/anthropic_test.go` as "New tests." Both files are already committed at `810d7ce` and present in the worktree. The round-1 verifier BLOCKED was a *process* issue (cmd/sworn/run.go collision), NOT a code-quality issue — all 4 unit tests passed in round 1 (`TestAnthropicVerify_ReturnsTextBlock`, `TestAnthropicVerify_MultiBlock`, `TestAnthropicVerify_APIError` with KindRateLimit assertion, `TestAnthropicNewClient_RoutedCorrectly`).
   What to ask the implementer: Do NOT rewrite anthropic.go or anthropic_test.go. Correct scope for round 2: (a) run `go test ./internal/model/... -run Anthropic` to confirm existing 4 tests pass, (b) run `go test ./internal/model/...` to confirm no OAI regression, (c) populate `actual_files` and `reachability_artifacts` in status.json from the round-1 state (commit `810d7ce`), (d) produce proof.md citing existing tests and commit, (e) transition status.json to `implemented`. Rewriting risks silent regression against a working implementation.

2. **[memory-cited] §2.1 — SDK adoption via ADR-0007 aligns with dep policy memory**
   What I observed: Design §2.1 cites ADR-0007 to justify `github.com/anthropics/anthropic-sdk-go` at v1.51.1. The dep policy was revised (2026-06-20) to allow provider SDKs with ADR backing.
   What to ask the implementer: Confirm the citations are current. [[project_dep_policy]] confirms SDKs allowed under revised policy. [[feedback_dep_justification_test]] confirms the SDK qualifies: it replaces hard/reusable logic (auth headers, JSON wire format, error response parsing from the internal `*apierror.Error` type) — not a narrow one-shot transform. v1.51.1 pin satisfies spec Risk 1. No conflict found; ack confirms these citations.
   Citation: [[project_dep_policy]], [[feedback_dep_justification_test]]

3. **[memory-cited] §2.3 — error taxonomy: HTTP path aligned, non-HTTP fallback exits the taxonomy**
   What I observed: Design §2.3 routes HTTP errors through `NewProviderError()` — the correct consumption point per [[project_provider_error_taxonomy]] ("every driver (S11–S16) inherits [the taxonomy]"). Confirmed in live code: `anthropicStatusCode(err)` parses the formatted error string; on success, calls `NewProviderError(code, "anthropic", a.Model, nil)` so S44's `IsTerminal`/`IsTransient` work. BUT: when `anthropicStatusCode` returns `(0, false)` (non-HTTP errors: TLS failure, DNS failure, network timeout), the code falls through to `fmt.Errorf("model: anthropic dispatch: %w", err)` — a plain error, not a typed `model.Error`. S44's retry policy uses `errors.As(err, &me)` to classify; on a plain error, classification fails silently.
   What to ask the implementer: Confirm whether S44's retry policy treats unclassified (non-`model.Error`) errors as Transient, Terminal, or unknown. If S44 has no fallback for plain errors, the non-HTTP path should route through `NewProviderError(0, "anthropic", a.Model, nil)` with `Kind=KindTransient` (network errors are transient by default). If S44 already handles this gracefully, document the intent in a code comment at the fallback line.
   Citation: [[project_provider_error_taxonomy]]

---

## Summary

Pins: 3 total — 1 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: Pin 1 — if the implementer re-writes existing files, they risk regressing a working implementation that passed all 4 unit tests in round 1.

## Smaller flags (not pins, worth one-line ack)

(a) **`design_decisions` absent from status.json.** `sworn designfit` skips slices with no `design_decisions` array, so the gate technically passes. But the 5 §2 decisions (especially SDK choice backed by ADR-0007) are worth recording as Type-2 entries for audit completeness and to give future captains a traceable decision record. Not blocking — can be added in proof.md or as a separate follow-up.

(b) **T5 track is 134 commits behind release-wt.** Verified at review start: zero of those 134 commits touch S11 artefacts or spec.md. Drift gate's "stale spec" concern does not apply. Proceeding was correct; flag noted for audit trail.

(c) **`proof.md` exists in the artefact directory** but was produced for the round-1 implementation that was BLOCKED on a process issue. The implementer must produce a fresh proof.md for round 2 (the harness will not re-use the round-1 bundle).

## Suggested ack reply
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
