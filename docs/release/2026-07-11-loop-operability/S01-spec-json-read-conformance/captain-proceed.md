# Coach acknowledgement — S01-spec-json-read-conformance

Date: 2026-07-12
Decided by: Brad (Coach) — the two escalate pins ratified per his standing
"sweep ALL sites, build it correctly" directive; mechanical pin applied.
Verdict: PROCEED

## Pin dispositions
1. **[escalate] Pin 1 — test_refs reader extension: RATIFIED.** Extend the
   shared spec reader (spec.Record.AC) to expose the spec-v1 `test_refs`
   field so internal/rtm's need->AC->test golden thread resolves on a
   spec.json-only release. test_refs is a real spec-v1 AC field (id/text/
   ears_pattern/test_refs) — exposing it completes the machine-contract
   read, not a new invention. AC-06 added (spec amended @release/v0.1.0).
2. **[escalate] Pin 2 — 3 more prose-read sites: FOLD IN.** gate/llmcheck.go:257,
   lint/touchpoints.go:117, cmd/sworn/task.go:131 get the same spec.json-
   preferred/spec.md-legacy-fallback treatment. Per Brad's "sweep ALL sites"
   directive — "every site" is literal, not the 9-site subset. Added to
   touchpoints + in_scope. (S01 stays grind: same mechanical pattern, wider.)
3. **[mechanical] design_decisions:** record CHOICE-A/B + the test_refs
   contract extension with Type classification before in_progress (Rule 9).

Use the shared spec.ReadRecord / a new spec.LoadSpec precedence helper so the
"spec.json-preferred, spec.md-legacy-fallback" rule is single-source (AC-04).
Preserve spec.md as the legacy fallback (do NOT delete it) — invert precedence.
Proceed to implementation.
