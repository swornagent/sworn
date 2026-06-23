<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound. 2 mechanical pins, apply inline:

1. **Prefix set rationale.** Inside `impliesType1Work()`, add a brief comment explaining why `{cmd/sworn/, internal/state/, internal/verdict/}` is the intended scope (e.g. "CLI entrypoint, state machine, verdict contract — the artefact surface external consumers depend on; other internal packages are implementation detail"). First, read the full `internal/` directory listing and confirm no other package belongs in the set (candidates: `internal/run/`, `internal/scheduler/`, `internal/verify/`, `internal/supervisor/`). If any should be added, add them; if not, document why.

2. **D1 rationale gap.** Add one sentence to the `impliesType1Work()` function comment: "When design_decisions is empty, DesignDecision.ArchitecturallySignificant cannot be checked — planned_files prefix-matching is the correct fallback." This closes the gap between the spec's cited signal and the design's choice.

Flags (not pins): (a) Optionally record D1 as a Type-2 design_decision in status.json for harness completeness — not required by the gate but consistent with the harness intent; (b) `go vet` is implicit in the test run, no action needed.

§2 decisions D1–D5 ack (all Type-2, no human decision required). §6 empty ack.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: 2 apply-inline mechanical pins (prefix set rationale + D1 justification gap); design approach is sound and requires no redesign before code.
-->
