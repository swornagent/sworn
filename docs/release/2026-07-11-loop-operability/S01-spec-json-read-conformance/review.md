# Captain review — S01-spec-json-read-conformance
Date: 2026-07-11
Design commit: c9481d51b7a8132ad6d7b13274cf0d46363269fc

## Pins

1. [escalate] §2 site 9 / §3 PIN-1 — rtm required-tests forces a shared spec.Record contract extension (Coach nod).
   What I observed: Verified `spec.AC` (internal/spec/spec.go:24-28) exposes only `ID/Text/EARSPattern` — no `TestRefs` — while the spec-v1 schema permits `test_refs` (spec.go:19 comment). rtm sources its golden-thread required tests solely from the spec.md "Required tests" section (`parseRequiredTests`, internal/rtm/rtm.go:465). On a spec.json-ONLY release there is no spec.md, so rtm sees zero required tests → an AC→test trace break on exactly the releases this slice must make work. The design proposes adding `AC.TestRefs []string` to the shared `spec.Record`, which is consumed by ears/trace/coverage, sourced spec.json-preferred. It is presented as a single option; the legitimate alternative — leave rtm required-tests on the spec.md fallback and defer required-tests-from-spec.json as a tracked Rule 2 follow-up — is not weighed.
   What to ask the implementer: The Coach must bless one of: (a) extend `spec.Record.AC` with `test_refs` now (additive/`omitempty`, schema-sanctioned, but a shared read contract consumed across 4 packages — Rule 9 Type-1-ish, so record options+trade-offs), or (b) defer the rtm-required-tests-from-spec.json migration as a tracked Rule 2 item with why+tracking+acknowledgement. Do not silently pick.

2. [escalate] §2 audit / user_outcome — the 9-site enumeration under-covers "every site"; unguarded spec.md read/write sites remain.
   What I observed: An independent sweep of every non-test `spec.md` string reference (grep across internal/ + cmd/) found machine-contract sites the design's audit does NOT list:
     - internal/gate/llmcheck.go:257 — unguarded `os.ReadFile(spec.md)`; `sworn llm-check` (including the design-review LLM check the captain flow itself runs at session end) HARD-FAILS `llm-check: read spec.md` on a spec.json-only slice. This file is in the `internal/gate` package AC-04 already commits to, but §2 lists only coverage.go + trace.go.
     - internal/lint/touchpoints.go:117 — unguarded `extractSectionRefs(spec.md)`; `sworn lint touchpoints` hard-fails on a spec.json-only slice.
     - cmd/sworn/task.go:131 — the cmd-side `--task` on-ramp WRITES spec.md; CHOICE-B converts only run.go:285's `setupSlice` to spec.json, leaving this parallel write path emitting spec.md (inconsistent on-ramp, re-introduces the legacy contract).
     - (soft flag) internal/tui/blocked.go:212/:232 — reads spec.md with the error swallowed → empty spec context handed to Claude on a spec.json-only slice.
   Verified OK, no action: internal/mcp/context.go:48 and cmd/sworn/ledger.go:144 are already spec.json-first; internal/ears/ears.go:251 and internal/reqverify/reqverify.go:196 are the correct legacy fallbacks.
   Live evidence the gap is real: S01's own slice dir has spec.json and NO spec.md, so the captain's end-of-session `sworn llm-check --check design-review` would itself hard-fail today.
   What to ask the implementer: The spec `user_outcome` says "Every site in sworn that reads a slice's machine contract reads spec.json." The Coach decides: expand S01 to cover llmcheck + lint/touchpoints + cmd/task (grow §2 and AC-04's test scope accordingly), OR explicitly defer these sites as a tracked Rule 2 follow-up so "every site" is not silently under-delivered. At minimum, gate/llmcheck.go is inside AC-04's stated package scope and should be migrated in this slice.

3. [mechanical] §2b design-fit gate — status.json `design_decisions` is absent.
   What I observed: `status.json` has no `design_decisions` field, yet the design makes stakes-bearing choices: CHOICE-A (worker/`RunSliceFn` signature unchanged, Type-2), CHOICE-B (`--task` emits spec.json, Type-2), and the `spec.Record` contract extension (Pin 1, arguably architecturally-significant → Type-1). Rule 9's design-fit gate fails closed on an architecturally-significant choice not classified Type-1, or a Type-1 choice with no recorded human decision.
   What to ask the implementer: Record the three decisions in `status.json.design_decisions` with Type classification before code. The `spec.Record` extension (Pin 1) needs a recorded Coach decision if classified Type-1.

4. [memory-cited] §1/§2 — precedence inversion aligns with ADR-0009 / the driver-contract pattern.
   What I observed: The design applies the ears.go/reqverify.go "spec.json-preferred, spec.md legacy fallback, spec.json authoritative on disagreement" precedence (ADR-0009). Verified the reference block: internal/ears/ears.go:201-247 states the rule and calls `spec.ReadRecord` at :238, exactly as cited. `spec.ReadRecord` (spec.go:52) returns `(nil,nil)` on absence so callers fall back — confirmed.
   What to ask the implementer: Acknowledge the citation. Confirm `LoadSpec` fails CLOSED on a malformed spec.json (mirrors ReadRecord/ears.go:238-246) rather than falling through to spec.md.
   Citation: [[project_driver_contract_recut]]

## Summary
Pins: 4 total — 1 [mechanical], 1 [memory-cited], 2 [escalate]
Critical pins (if any): 1, 2 — both regress on exactly the spec.json-only releases this slice exists to enable (Pin 1: rtm golden-thread trace break; Pin 2: `sworn llm-check` and `sworn lint touchpoints` hard-fail).

## Smaller flags (not pins, worth one-line acknowledgement)
- (a) R-02 no-op guard: design site 2 makes `WriteSpecRecord` validate rather than regenerate when spec.json exists. Verified current spec_record.go:87 unconditionally overwrites and implement.go:141 calls it unconditionally — confirm the new guard is "spec.json exists AND parses" (fail closed on malformed) and that the AC-03 byte-equality test uses a planner spec.json carrying `ears_pattern`/`risks`.
- (b) CHOICE-A verified sound: implement.go:47 derives `sliceDir = filepath.Dir(specPath)`, so worker.go/run.go can keep the spec.md-anchored path with no `RunSliceFn` signature change (worker.go:104). Not an inference — confirmed in code.
- (c) design PIN-2 (specquality Examples spec.md-only) is correct: spec-v1 `Record` has no examples field; keeping `parseExamples` (specquality.go:521) on spec.md fallback is right. No contract change. Acknowledged, no action.
- (d) `LoadSpec(sliceDir) (rec *Record, mdText string, err error)` is a clean single-precedence primitive; it satisfies AC-04's "single shared helper" honestly (the package-local md parsers stay local because they return package-local types — that is expected, not duplication of the precedence).

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong, faithful application of the ADR-0009 ears.go pattern with well-verified anchors — but two escalate items need a Coach call before the "every site" outcome is honestly met. 4 pins + 4 flags:

1. **rtm required-tests / spec.Record extension.** `spec.AC` has no `test_refs`; rtm's required tests come only from spec.md's "Required tests" (rtm.go:465), so a spec.json-only release gets a golden-thread trace break. Apply the Coach's decision: EITHER extend `spec.Record.AC` with `test_refs` (additive, spec.json-preferred, keep spec.md fallback) and record it as a Type-1 decision, OR keep rtm required-tests on spec.md and defer the spec.json migration as a tracked Rule 2 item (why+tracking+acknowledgement). Do not pick silently.
2. **Audit completeness — missed unguarded sites.** Beyond the 9 audited sites, these read/write the machine contract unguarded: gate/llmcheck.go:257 (hard-fails `sworn llm-check` on spec.json-only — and it is inside AC-04's `internal/gate` scope), lint/touchpoints.go:117 (hard-fails `sworn lint touchpoints`), cmd/sworn/task.go:131 (writes spec.md — the on-ramp twin of the run.go path CHOICE-B converts), and tui/blocked.go:212/:232 (swallows the error → empty spec context). At minimum migrate gate/llmcheck.go in this slice (it is in AC-04's package scope). For lint/touchpoints + cmd/task, apply the Coach's decision: fold into S01, or defer as a tracked Rule 2 item — but do not leave "every site" silently under-delivered. (mcp/context.go, ledger.go, ears.go, reqverify.go verified already-correct.)
3. **Record design_decisions.** `status.json` has no `design_decisions`. Add CHOICE-A (Type-2), CHOICE-B (Type-2), and the spec.Record extension (Type-1 if you take Pin 1 option (a)) with classifications so the Rule 9 design-fit gate passes.
4. **ADR-0009 citation.** Acknowledged — the precedence matches ears.go:201-247. Confirm `LoadSpec` fails CLOSED on a malformed spec.json (mirror ReadRecord/ears.go:238-246) rather than silently falling to spec.md.

Flags (not pins): (a) confirm the `WriteSpecRecord` no-op guard is "exists AND parses" (fail closed) and the AC-03 byte-equality test uses a planner spec.json with ears_pattern/risks; (b) CHOICE-A confirmed sound in code — no RunSliceFn signature change needed; (c) design PIN-2 (specquality Examples spec.md-only) is correct, no action; (d) LoadSpec return shape is a clean single-precedence helper satisfying AC-04.

§2 decisions: CHOICE-A and CHOICE-B (Type-2, apply-inline) and the ADR-0009 precedence (memory-cited) acknowledged. §3 PIN-1 and the audit-completeness gap require the Coach calls above. §6: no separate open-questions block — the design's PIN-1 carries the one genuine open decision.

Address pins 3–4 and the flags inline during implementation; apply the Coach's decision on pins 1–2, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Two escalate pins need Coach judgement before code — extend the shared spec.Record contract (blast radius across ears/trace/coverage) vs defer, and expand scope to the unaudited unguarded sites (llmcheck/lint/task hard-fail on spec.json-only) vs a tracked Rule 2 deferral, since the spec's "every site" outcome is under-enumerated.
-->
