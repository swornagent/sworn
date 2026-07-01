# Captain review — S01-d6-record-reconciliation
Date: 2026-07-01
Design commit: 5b0ee7d866b9fb8e025838be4c9b1f06df55956f

## Pins

1. [escalate] §Risks.1 / user_outcome — Real fired deferrals fail write-back schema validation (`acknowledgement` vs `acknowledged_by`).
   What I observed: `slice-status-v1.json` `open_deferrals.items` carries `required: [why, tracking, acknowledgement]`. Fired's real deferrals are `{id, description, why, tracking, acknowledged_by}` — no `acknowledgement`. `state.Read` (Go unmarshal) ignores `required`, so READ succeeds; `state.Write` runs `baton.ValidateSchema`, which enforces `required`, so WRITE-BACK of a real fired deferral fails closed. The AC-02 round-trip fixture deliberately includes `acknowledgement` (to stay schema-valid), so the in-repo test proves field-*preservation* but never exercises write-back of fired's actual (acknowledgement-less) shape. The slice's user outcome says sworn "round-trips it without dropping any field" — but for real fired data the write side is rejected wholesale, not field-dropped, and AC-08's "proceed past the D6 failure point" is contingent on the loop NOT writing that status back during the smoke. The design author surfaced this verbatim and asked the reviewer to confirm.
   What to ask the implementer: This is a scope/coherence decision for the Coach, not a code fix. Determinable facts (settled): schema requires `acknowledgement`; fired data lacks it; AC-02 fixture sidesteps it. Open judgement (Coach): is read-side-only sufficient for S01 — with write-back of non-schema-compliant coach deferrals tracked as an explicit Rule-2 follow-up — or must S01 also resolve the schema↔coach-producer mismatch (the schema's `required:[acknowledgement]` vs the coach emitting `acknowledged_by`)? The latter is a spec/contract change (`/replan-release` or an upstream coach fix), not in S01's AC set.

2. [escalate] §D1 / status.json design_decisions[0] — Type-1 architecturally-significant decision has empty `human_decision`.
   What I observed: D1 (carrier representation: structs + `Extra` overflow map + custom `(Un)MarshalJSON`) is recorded `stake_class: "Type-1"`, `architecturally_significant: true`, `human_decision: ""`. The design says "human_decision left for the Captain." Rule 9: a Type-1 choice requires a recorded human decision, and the model/Captain may NOT self-record it. The design DOES present 3 options with trade-offs + prior art (spec AC-03), so the Rule-9 "≥2 options for Type-1" requirement is satisfied — what is missing is the Coach's recorded ratification of the representation mechanism + its determinism guarantee.
   What to ask the implementer: The Coach ratifies D1 (acknowledgement IS the decision); implementer then writes the Coach's decision into `design_decisions[0].human_decision` before transitioning to in_progress. The design-fit gate fails closed until that field is populated.

3. [memory-cited] §D4 + §AC-07 — verdict-layer-stays-string and inconclusive-enum align with the keystone plan.
   What I observed: D4 keeps `verdict.Result.Violations []string` (confirmed at `internal/verdict/verdict.go:42`) and wraps into `[]Violation` via a helper; AC-07 adds `"inconclusive"` to the slice-status-v1 `verification.result` enum. Both match [[project_keystone_structured_outputs]]'s explicit decisions: "Map violations[].description→[]string (keep Go state's []string; objects = D6)" and "INCONCLUSIVE = Option A (DEFER) ... Deferred D4 leaf-enum add tracked as issue #37 (bundle w/ D6/1b)." This slice IS that D6/1b bundle.
   What to ask the implementer: Confirm the citation holds — yes, the keystone deferred the object form and the #37 leaf-enum to this slice. Acknowledging confirms it. (Note: keystone step-3 commit 869f07c is present on this base, so D4's `verdict.Result.Violations` premise is live.)
   Citation: [[project_keystone_structured_outputs]]

4. [mechanical] §Risks.2 — Marshal determinism: confirm a byte-stable round-trip assertion in the fixture test.
   What I observed: `status.json` is rewritten every transition; non-deterministic key order would produce phantom diffs that break the drift gate (the drift gate counts commit ancestry, but content churn still spins the loop — see [[feedback_replan_propagate_by_merge_not_copy]]). The design proposes marshalling via a `map` (encoding/json sorts map keys) and asserts "a byte-stable round-trip assertion in the fixture test."
   What to ask the implementer: Confirm the AC-02 fixture test asserts byte-stable output (read→write→read produces identical bytes), not just field presence. The map-based MarshalJSON is the mechanism; the assertion is the proof.

5. [mechanical] §Risks.3 / §3 — Confirm all `[]string`→`[]Deferral`/`[]Violation` assignment sites thread the new type and compile.
   What I observed: `verify.Input.OpenDeferrals` is `[]string` (verify.go:35) and feeds `RunFirstPass`→`CheckBoundaryMocks` (verify.go:65,386) / `isDeclared` (verify.go:487); `run/slice.go:712` does `st.Verification.Violations = lastVerdict.Violations` and `:718` does `= []string{fallback}` — both will fail to compile once `Verification.Violations` is `[]Violation` unless they go through the `violationsFromStrings` helper (D4). `internal/mcp/tools_plan.go:70` initialises `OpenDeferrals: []string{}` (must become `[]Deferral{}`).
   What to ask the implementer: After the type change, confirm `go build ./...` passes — i.e. every `[]string` literal/assignment against `OpenDeferrals`/`Verification.Violations` (slice.go:712,718; tools_ops.go:601; tools_plan.go:70; verify.Input threading through RunFirstPass) is updated. The migration is atomic by nature; the compiler is the first gate.

6. [memory-cited] §AC-09 / process — Edit-corruption guard + full-suite discipline for a ~15-file migration.
   What I observed: this is a ~15-file mechanical type migration — exactly the surface where [[project_newline_eating_edit_corruption]] bit three times (statement fused onto a trailing `//` comment line, silently commenting out code), and where the in-loop verifier did not reliably re-run tests ([[project_loop_verifier_fidelity]]). A HUNG test (not just a failing one) was the prior signature.
   What to ask the implementer: After edits, run `grep -rnE '//.*\t+(return|[a-z]+\()'` over the touched files; and satisfy AC-09 with a FULL `go test ./...` carrying a per-package timeout (not the in-loop judge) before claiming green.
   Citation: [[project_newline_eating_edit_corruption]]

## Summary
Pins: 6 total — 2 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins (if any): 1 (write-back validation gap may mean the user outcome / AC-08 is not actually unblocked for real fired data). Pin 2 is a hard design-fit gate (cannot reach in_progress until the Coach records the Type-1 decision).

## Smaller flags (not pins, worth one-line acknowledgement)
(a) `internal/mcp/tools_plan.go:70` (`OpenDeferrals: []string{}`) is a confirmed touchpoint — the design listed it conditionally ("if it writes deferrals"); it does. Fold it into the touchpoint set.
(b) D4's anchor "verdict.go:40" is under-qualified — actual file is `internal/verdict/verdict.go` (field at :42, comment at :40). That comment ("Kept as []string to match state.Verification.Violations until the [D6] migration") goes stale once this slice lands — update it inline.
(c) AC-07 enum addition is safe: the merge gate keys on slice STATE (`merge.go:220`, `router.go:58`), not `verification.result` — nothing grants a pass on the result value, so `"inconclusive"` can never accidentally pass. Confirm no exhaustive `switch result` defaults `inconclusive` into a pass path.
(d) Oracle `blockedReason = s.Verification.Violations[0]` (oracle.go:236) will read `ViolationStrings()[0]` after D2 — confirm the projected string is acceptable as the blocked-reason display. ([[project_oracle_blocked_invisible]] — oracle reads `.state` not `.verification.result` — is a separate known bug, NOT in S01 scope.)
(e) §4 "not touched" report types (reqverify/ears/gate/designaudit/specquality/lint/rtm + scrape-layer `sv.Violations` at verify.go:228-232): AC-04 makes this binding and `sv.Violations` is confirmed a distinct scrape-local type. Grep-confirm none alias `state.Verification.Violations` before relying on the guard.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong, well-decomposed design — anchors all verified live, scope correctly bounded, the round-trip trap surfaced honestly. 6 pins + 5 flags:

1. **Write-back validation gap (CRITICAL — Coach to resolve scope).** The schema requires `acknowledgement` on deferrals; fired's real data uses `acknowledged_by`. Read succeeds, write-back fails closed on validation. AC-02's fixture includes `acknowledgement` so it doesn't exercise this. Coach decision: is S01 read-side-only (write-back of non-compliant coach deferrals tracked as a Rule-2 follow-up issue), or must S01 also resolve the schema↔coach `required:[acknowledgement]` mismatch (spec/replan)? Do not implement past this without the scope call. If read-only is blessed, file the follow-up issue at find time and cite the number.
2. **D1 Type-1 ratification.** Coach records the carrier-representation decision (structs + Extra overflow + custom marshalers); write it into `status.json design_decisions[0].human_decision` before in_progress. Options + trade-offs are already present — this is ratify-and-record, not redesign.
3. **Keystone alignment (confirm).** D4 (verdict layer stays `[]string`, wrap into `[]Violation`) and AC-07 (inconclusive enum) match the keystone plan that deferred the object form + #37 to this D6/1b bundle. Acknowledged.
4. **Byte-stable round-trip.** Make the AC-02 fixture assert identical bytes on read→write→read (map-based MarshalJSON for sorted keys), not just field presence — phantom diffs break the drift gate.
5. **Compile-thread the new types.** Update every `[]string` site: slice.go:712/718 (via `violationsFromStrings`), tools_ops.go:601, tools_plan.go:70 (`[]Deferral{}`), and `verify.Input.OpenDeferrals` through `RunFirstPass`/`CheckBoundaryMocks`/`isDeclared`. `go build ./...` is the first gate.
6. **Edit-corruption guard.** After edits run `grep -rnE '//.*\t+(return|[a-z]+\()'` on touched files, and satisfy AC-09 with a FULL `go test ./...` + per-package timeout (a hung test is the signature) — do not trust the in-loop judge.

Flags (not pins): (a) tools_plan.go:70 is a confirmed touchpoint, not conditional; (b) D4 anchor is `internal/verdict/verdict.go:42` — and update its now-stale `// Kept as []string ...` comment inline; (c) AC-07 enum is safe (merge gates on state, not result) — confirm no `switch result` defaults inconclusive into pass; (d) oracle blockedReason now reads `ViolationStrings()[0]` — confirm the projected string is acceptable; (e) grep-confirm the §4 not-touched report types don't alias `state.Verification.Violations`.

§2 decisions: D1 [escalate — Type-1 ratify], D2/D3 (Type-2, clean), D4 [memory-cited — keystone] acknowledged. §Risks 1 [escalate], 2/3 [mechanical] acknowledged. AC-07 enum [memory-cited — #37] acknowledged.

Address pins 3–6 and flags inline during implementation. Pins 1 and 2 need the Coach's call first; once acknowledged, proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: D1 is a Type-1 decision with empty human_decision that only the Coach can record (Rule 9), and Risk-1 (schema requires `acknowledgement`, fired data uses `acknowledged_by`, so real-data write-back fails closed) is a genuine scope/spec-coherence judgement — read-only-S01 vs resolve-the-mismatch — with no single right answer.
-->
