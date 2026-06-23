# Captain review — S32-designfit-decisions-gate
Date: 2026-06-22
Design commit: 5fb8387448ce8ac2dd4afcc89146d5bd6b6da308

## Pins

1. [mechanical] §2.D1 — Prefix set completeness not audited against full `internal/` package list
   What I observed: D1 chose `{cmd/sworn/, internal/state/, internal/verdict/}` as the architecturally-significant path prefixes, stating "these three packages are the contract/control-plane surface." The `internal/` directory contains 22 packages (`run/`, `scheduler/`, `supervisor/`, `verify/`, `agent/`, `board/`, `lint/`, `designfit/`, etc.). None of these appear in the prefix set. If a future slice modifies `internal/run/` (the run loop), `internal/scheduler/` (the concurrent scheduler from S02b), or `internal/verify/` without recording `design_decisions`, the new gate will not fire. The design does not document why these are excluded.
   What to ask the implementer: Either (a) add a brief comment inside `impliesType1Work()` explaining why the three prefixes represent the intended scope ("CLI entrypoint, state machine, verdict contract — the artefact surface an external consumer would depend on"), or (b) expand the prefix set to include other high-risk packages. Confirm by re-reading the `internal/` directory listing and making an explicit decision.

2. [mechanical] §2.D1 — Rationale omits why `DesignDecision.ArchitecturallySignificant` wasn't used
   What I observed: The spec's Risk mitigation explicitly says "drive the determination from an explicit artefact signal (see `internal/designfit/designfit.go:126` and the `DesignDecision`/`StakeClass` schema at `internal/state/state.go:83`)." The `DesignDecision` struct at `state.go:103` already has an `ArchitecturallySignificant bool` field — which would be the most natural "explicit artefact signal." D1's rationale says "uses existing data; no new schema field" but does not explain why the existing `ArchitecturallySignificant` field was passed over. The reasoning is correct (when `design_decisions` is empty, there are no `DesignDecision` objects to inspect — the bypass being fixed is precisely that the array is empty), but this is unstated.
   What to ask the implementer: Add one sentence to the `impliesType1Work()` function comment: "When design_decisions is empty, DesignDecision.ArchitecturallySignificant cannot be checked — planned_files prefix-matching is the correct fallback." This removes any doubt for future readers and makes D1 self-documenting.

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none. Both are documentation/rationale gaps; the gate logic itself is sound.

## Summary

2 mechanical pins, both apply-inline during implementation. The approach is correct: `planned_files` + path-prefix check is a sound explicit signal when `design_decisions` is empty. The only gaps are (1) the prefix set completeness rationale and (2) the unstated reason for not using `ArchitecturallySignificant`. Design is ready for code.

## Smaller flags (not pins, worth one-line ack)

- S32's own `status.json` has no `design_decisions`. Under the new gate, this is benign (`internal/designfit/` is not in the prefix set → benign-empty path). But D1 is a meaningful choice about gate behavior; the implementer may optionally add D1 as a Type-2 decision in status.json for harness completeness.
- `go vet ./internal/designfit/...` (AC5) is not named explicitly in the §5 reachability plan — it's implicit in the `go test` run. Trivially addressed.

## Suggested ack reply
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
