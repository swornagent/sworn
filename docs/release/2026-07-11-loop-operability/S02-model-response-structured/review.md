# Captain review — S02-model-response-structured
Date: 2026-07-12
Design commit: b8521bf265fddd5310a028c75a2308132e360c09

## Pins

1. [escalate] §2.D1 / §6.1 — Driver-contract Type-1 change needs a recorded Coach decision, not just a proposal.
   What I observed: D1 renames `DispatchInput.VerdictSchema` → `StructuredSchema` and gives `dispatchCaptain` a structured-output path so the two captain-family gates emit structured JSON. Verified live: `dispatchCaptain` (internal/driver/inprocess/inprocess.go:157) currently does a tool-less prose `meter.Chat` with nil tools; only `dispatchVerifier` (inprocess_verify.go:51) calls `ChatStructured` against `in.VerdictSchema`. This is a driver-contract edit (ADR-0012) — architecturally significant and hard to reverse. The design correctly presents two options with trade-offs (rename the field vs. add a parallel `CaptainSchema`), which satisfies the Rule 9 options requirement. What is missing is the recorded decision: Rule 9 forbids the model recording a Type-1 decision itself.
   What to ask the implementer: hold. The Coach picks rename (design's choice) vs. parallel field and the decision is recorded in status.json `design_decisions` (Type-1, architecturally_significant: true) plus an ADR-0012 amendment note, before code.

2. [mechanical] status.json §2b — Rule 9 design-fit gate fails closed: no `design_decisions` recorded.
   What I observed: status.json for this slice has NO `design_decisions` array, yet design.md declares D1 (Type-1, architecturally significant) and D3 (Type-1-adjacent contract addition). The verified sibling S01 recorded a full `design_decisions` array. The design-fit gate would fail closed on the current status.json.
   What to ask the implementer: populate `design_decisions` with D1 (Type-1) and D3 (Type-1-adjacent, new ErrKind) — carrying the Coach's D1 decision from pin 1 — and the D2/D4/D5 choices at their stated stake class, before transitioning to in_progress.

3. [escalate] §2.D2 / §6.2 — Schema home + whether reqverify needs a canonical validate-schema.
   What I observed: D2 chooses sworn-local inline emit schemas (lenient strict-subset, `title`-named) for design-tldr and reqverify-results, rather than canonical Baton `*-v1.json`. Spec in_scope explicitly sanctions this ("add a sworn-local one with a clear $id"), so the sworn-local choice is spec-blessed. The remaining open call is D2's own question: whether the DoR-results emission also needs an engine-side canonical validate-schema (`baton.ValidateSchema`) for parity with verifier-verdict-v1, which carries BOTH an inline emit-schema and a canonical validate-schema. Design recommends inline-only for design, inline + a lightweight sworn-local validate for reqverify.
   What to ask the implementer: Coach confirms inline-only vs inline+validate for the reqverify DoR-results path. (The design must NOT fork under an existing `$id` — spec in_scope.)

4. [memory-cited] §2.D3 / §6.3 — New `ErrKindUnsupported` touches the binding cross-driver ErrKind taxonomy.
   What I observed: AC-03 requires capability-absent to diverge from emission-failure into a declared Rule 2 deferral. Verified live: `dispatchVerifier` currently folds "client not structured-capable" into `ErrKindProtocol` (inprocess_verify.go:33-37: `so, ok := client.(model.StructuredOutput); if !ok { … ErrKindProtocol }`) — so the divergence D3 wants genuinely does not exist yet. D3 adds a dedicated `ErrKindUnsupported = "unsupported"` in internal/driver/subprocess.go. That file holds the cross-driver ErrKind vocabulary (`ErrKindAuth`, `ErrKindProtocol`, `ErrKindCredits`), which [[project_driver_contract_recut]] binds as a contract "for all future drivers".
   What to ask the implementer: confirm the new ErrKind is a deliberate addition to the binding taxonomy (not a local constant), and that subprocess-family drivers can produce/map it too — otherwise capability-absent stays indistinguishable on the subprocess path. Cite [[project_driver_contract_recut]].
   Citation: [[project_driver_contract_recut]]

5. [mechanical] §3 / §6 — Cross-track collision with T2-xai-driver (S03): verified-but-unmerged.
   What I observed: the board oracle reports S03 `verified`, but it reads T2's track ref — T2 is 6 commits ahead of `release-wt/2026-07-11-loop-operability` and NOT yet merged. T2 modifies internal/model (catalog/client/config/provider/xai + structured_test) and internal/driver/registry. VERIFIED FACT: T2 does NOT modify the driver-contract files S02 renames (driver.go, inprocess*.go, verify.go) — the VerdictSchema references on T2's branch are inherited base content, so there is no direct rename collision. The overlap is co-located-tree only.
   What to ask the implementer: no design change needed — this resolves at merge. Whichever of T1/T2 merges second forward-merges the first and re-runs the full `go test -count=1 -timeout 300s ./...` (merge-track backstops the affected-package regression). Do not infer S03's merge state from the board's `verified`.

6. [memory-cited] §5 / §6.5 — Newline-eating edit corruption discipline (confirmation).
   What I observed: design §6.5 already cites the corruption memory and prescribes the grep + gofmt -l + go vet + full-suite discipline after every .go edit. This is the correct mitigation.
   What to ask the implementer: acknowledge and hold to it — after every .go edit run `grep -nE '//.*\t+(return|[a-z]+\()'` on changed files, `gofmt -l`, `go vet`, and a full `go test -count=1 -timeout 300s ./...` before the state transition (this slice edits shared driver-contract files, so a fused `//`+code line would regress cross-package).
   Citation: [[project_newline_eating_edit_corruption]]

7. [escalate] §6.6 — Keep-one-slice vs. design/reqverify split.
   What I observed: the spec effort note flags a possible design-vs-reqverify split; design §6.6 recommends keeping one slice because D1 (the driver-contract edit) is the shared spine both gates depend on, so a split would duplicate or serialise the same contract change. The rationale is sound, but the spec explicitly surfaced this as a decision point.
   What to ask the implementer: Coach confirms no-split (one slice). If split, D1 must land first as its own slice both depend on.

8. [mechanical] §3 — Spec touchpoint filename is stale (design got it right).
   What I observed: spec.json `touchpoints` lists `internal/design/design_test.go`; the actual test file is `internal/design/tldr_test.go`, which design.md §3 uses correctly. No action for the design; the spec touchpoint is inaccurate.
   What to ask the implementer: proceed against tldr_test.go as design.md states; the stale spec touchpoint is cosmetic and does not gate.

## Summary

Pins: 8 total — 3 [mechanical], 2 [memory-cited], 3 [escalate] (pin 1 is Type-1-decision-bearing; pin 3 is schema-home + validate-schema; pin 7 is split — all Coach calls)
Critical pins (if any): Pin 1 (Type-1 driver-contract decision must be recorded by the Coach before code) and Pin 2 (Rule 9 design-fit gate fails closed without `design_decisions`). Pin 4 keeps AC-03's declared-deferral distinguishable — a silent-pass risk if the ErrKind is not taxonomy-honoured on the subprocess path.

## Smaller flags (not pins, worth one-line acknowledgement)

- §4 "interpreter/orchestrator scrapers … noted for a later audit": spec out_of_scope declares the why+boundary, but "noted for a later audit" without a tracked issue is a Rule 2 smell — file/cite an issue if the later sweep is real work, don't leave it as prose.
- AC-04 / R-01 alignment is clean: D4/D5 preserve acceptance semantics verbatim (schema-enforced six fields = the old `hasSixSections`; identical per-AC fail-closed grade logic replacing the `## RESULTS` scrape) and adapt tests rather than rewrite — a prior-model stub still passes, exactly as the spec risk mitigation binds.
- R-02 / AC-03 alignment is clean: capability-absent routes through the existing "not evaluated" DoR arm (internal/implement/ready.go:100) with a capability-naming reason, and a `recordDesignGateDeferral` precedent already exists at internal/run/slice.go:448 — the deferral lands on existing surfaces, not a new one.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). -->

TL;DR strong design — cited symbols all verified live, acceptance semantics preserved verbatim (R-01/R-02 mitigations honoured), reachability threads through the real loop integration points (slice.go:335, ready.go:106). It carries genuine Coach-authority decisions (Type-1 driver-contract change). 8 pins + 3 flags:

1. **D1 driver-contract (Type-1).** Coach decides rename `VerdictSchema`→`StructuredSchema` (design's pick) vs. parallel `CaptainSchema` field, then RECORD it in status.json `design_decisions` (Type-1, architecturally_significant: true) + an ADR-0012 amendment note. The model may not self-record a Type-1 decision.
2. **Rule 9 gate.** status.json has no `design_decisions`; populate it (D1 per the Coach call above, plus D3 Type-1-adjacent and D2/D4/D5 at their stake class) before in_progress — the design-fit gate fails closed otherwise.
3. **D2 schema home.** Sworn-local inline emit schemas are spec-blessed (do NOT fork under an existing `$id`). Coach confirms inline-only vs inline+lightweight-validate for the reqverify DoR-results path.
4. **D3 new ErrKind.** `ErrKindUnsupported` is a deliberate addition to the binding cross-driver taxonomy (subprocess.go), not a local constant — confirm subprocess-family drivers can produce/map it, else AC-03's declared deferral collapses back into `ErrKindProtocol` on that path. (cites [[project_driver_contract_recut]])
5. **T2 cross-track.** Resolves at merge, no design change: T2-xai-driver (S03) is verified but 6 commits UNMERGED to release-wt and touches internal/model + internal/driver/registry; it does NOT modify the files you rename. Whoever lands second re-runs the full suite. Don't trust the board's `verified` as "merged".
6. **Newline-eating discipline.** Keep §6.5's grep + gofmt -l + go vet + full `go test -count=1 -timeout 300s ./...` after every .go edit — this slice edits shared driver-contract files. (cites [[project_newline_eating_edit_corruption]])
7. **Keep one slice.** Coach confirms no design/reqverify split (D1 is the shared spine). If split, D1 lands first.
8. **Stale spec touchpoint.** Build against `internal/design/tldr_test.go` (design.md is right); the spec's `design_test.go` touchpoint is cosmetic.

Flags (not pins): (a) §4 "later audit" of interpreter/orchestrator scrapers — file/cite an issue if real, don't leave as prose; (b) AC-04/R-01 semantics-preservation is clean; (c) AC-03/R-02 deferral lands on the existing "not evaluated" arm + recordDesignGateDeferral precedent — clean.

§2 decisions: D1 [escalate/Type-1], D2 [escalate], D3 [memory-cited: [[project_driver_contract_recut]]], D4/D5 [Type-2, acknowledged]. §6 questions 1-6 all surfaced above.

Address pins 2, 4, 5, 6, 8 inline during implementation; pins 1, 3, 7 carry the Coach's decisions. Then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: D1 is a Type-1 architecturally-significant driver-contract change (rename VerdictSchema->StructuredSchema + structured dispatchCaptain, ADR-0012) the model cannot self-record per Rule 9; D2 schema-home and D3 new-ErrKind-taxonomy are contract calls needing Coach ratification.
-->
