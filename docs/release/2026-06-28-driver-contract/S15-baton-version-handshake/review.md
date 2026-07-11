# Captain review — S15-baton-version-handshake
Date: 2026-07-11
Design commit: 1bc69dc1241885b86ca4ab10d30d0472f9ad251f

## Pins

1. [mechanical] §status.json — `design_decisions` field is missing entirely
   What I observed: design.md's "Decision list" table fully authors D1, D2, D3
   (each with a Decision, a Rule 9 Type — all Type-2 — and a Status of "Proposed
   default"), but `status.json` carries no `design_decisions` field at all. Every
   other design-reviewed sibling in this release (S01-S11, S13, S14) populates
   this field; S13-regress-go-module-cwd is the closest analog (an all-Type-2
   slice) and uses the shape `{id, choice, stake_class, rationale}` with no
   `human_decision` (correct for a Type-2 "noted default" — no Coach ratification
   required). `internal/designfit/designfit.go`'s deterministic gate reads this
   field directly; a design-review pass is the only place currently catching the
   omission by hand. This is a recurring gap — the trial log records the
   identical omission on S04, S08, and S11's first design pass — so I filed it
   as a standalone tooling finding (github.com/swornagent/sworn#94) rather than
   re-diagnosing it slice-by-slice; this pin is only the S15 instance.
   What to ask the implementer: transcribe D1-D3 from design.md into
   `status.json`'s `design_decisions` array before/while writing code, in the
   S13 shape: `id` ("D1"/"D2"/"D3"), `choice` (one-line), `stake_class`
   ("Type-2"), `rationale` (from design.md). No `human_decision` field needed.

2. [memory-cited] §Design-level pins for the reviewer, P2 — human-owned
   classification table + skew check vs. self-noticing drift
   What I observed: design.md's P2 argues that fully auto-deriving the
   graded/advisory classification (e.g. probing `Validate()`'s switch at runtime
   with sentinel payloads) was considered and rejected in favour of a small,
   human-authored 9-entry table compensated by `SchemaSkew()`'s fail-loud check
   — explicitly citing [[feedback_rules_capture_not_omniscience]] for the
   principle that cross-cutting drift needs a human-owned artefact + a gate, not
   a self-noticing heuristic. I confirmed the memory file exists and its content
   matches the citation (Rules 8/9/10 solicit and capture human judgement rather
   than expecting the loop or model to notice fidelity gaps on their own).
   Confirming: the citation is accurate and the design choice honours it — a
   human-owned table with a fail-loud skew check is the correct shape here, not
   an attempt at omniscient auto-derivation.
   Citation: [[feedback_rules_capture_not_omniscience]]

Pins: 2 total — 1 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none — pin 1 is a Rule 9 gate-field hygiene gap (the
deterministic gate does not currently fire on it, since S15's status.json also
carries no `planned_files` to trigger the fallback check), not a ship-blocking
defect; it is cheap to fix inline during implementation.

## Summary

Pins: 2 total — 1 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- **Manifest sourcing verified.** `SchemaManifest()`'s name/`$id`/version triple
  is 100% derived from `schemas.SchemaMap` (confirmed: exactly 9 entries in
  `internal/baton/schemas/embed.go`, each `$id` machine-parsed from the embedded
  JSON bytes) — no hand-typed name list. Only the graded/advisory
  *classification* is a hand-authored table, and `SchemaSkew()` is the
  compensating control (R-01's literal mitigation: "source from vendored files
  where possible... skew-check test asserts agreement" — a hand-maintained
  classification with a skew check satisfies this as written).
- **Graded/advisory split verified against live call sites.** `validator.go`'s
  `Validate()` switch has exactly 6 cases (slice-status-v1, board-v1, spec-v1,
  proof-v1, journeys-v1, attestations-v1); the only production `ValidateSchema(
  "verifier-verdict-v1", ...)` call site is `internal/verify/verify.go:283`.
  Graded = 7, Advisory = {contracts-v1, assembly-proof-v1} = 2, 7+2=9=
  `len(SchemaMap)` — matches design.md's ground-truth claim exactly.
- **Skew-check reachability confirmed feasible.** `cmd/sworn/doctor_test.go`
  already has a proven pattern for this exact shape of test
  (`TestDoctorFailsOnShaPin` uses `baton.SetVersionForTest` + `runDoctorInDir`
  to inject a fixture and assert `[ERROR]` in captured stdout); the design's
  plan to use `SetSchemaMapForTest` + `runDoctorInDir`/`cmdDoctor` to assert
  `[WARN]` for `baton/schema-skew` is the same proven pattern, not speculative.
  Satisfies Rule 1 (reachable through `cmdDoctor`, not just the leaf function).
- **P1 (group placement) resolved, no pin needed.** Confirmed the existing
  doctor group sequence is 1 → 2 → 2b → 3 → 4 with no existing "1b" — inserting
  "Group 1b: Baton schema manifest" directly after Group 1 is an open slot with
  no naming collision (`checkSchemaManifest`, `baton/schema-manifest/*`,
  `baton/schema-skew` — all new names, no collisions in `cmd/sworn/doctor.go`).
- **P3 (test fixture construction choice) — no pin needed**, as the implementer
  already flagged it correctly as not a design-level fork.
- **Cross-release ancestry checked, clean.** `git log release/v0.1.0..HEAD --
  cmd/sworn/doctor.go cmd/sworn/doctor_test.go` returns no commits — S11's spec
  declared `doctor_test.go` as a touchpoint but its actual landed diff never
  touched it, so there is no real collision with S15's planned edits to those
  files despite the shared spec.json touchpoint listing. `git log release/v0.1.0
  ..HEAD -- internal/baton/` returns exactly S11's three commits, which is what
  design.md's "Ground truth confirmed in the S11-landed tree" section already
  cites — no undocumented recent change.
- **`sworn llm-check --check design-review` not run.** The captain.md role
  prompt recommends this on a PROCEED verdict; it requires a paid model
  dispatch, which this session did not have explicit authorization to incur
  (consistent with this release's established no-paid-dispatch-without-ask
  posture per [[feedback_releaseverify_specmd_false_fail]] and prior sessions).
  Noted as a skipped optional step, not a gap in the pin-driven review.
- **Filed:** github.com/swornagent/sworn#94 — the recurring design_decisions
  omission pattern (S04, S08, S11, S15), as a tooling/process finding out of
  S15's own scope.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR clean, well-grounded design — every cited symbol (SchemaMap's 9 entries,
Validate()'s 6-case switch, the sole verifier-verdict-v1 ValidateSchema call
site, version_stub.go's Set/Clear pattern) checked out exactly against live
code. 1 pin + 6 flags:

1. **status.json is missing `design_decisions`.** Transcribe D1-D3 from
   design.md's Decision list table into `status.json`'s `design_decisions`
   array before/while writing code, using S13-regress-go-module-cwd's shape:
   `id` ("D1"/"D2"/"D3"), `choice` (one-line), `stake_class` ("Type-2"),
   `rationale` (from design.md). No `human_decision` needed — these are Type-2
   noted defaults, not Coach-ratified Type-1 choices. (Recurring pattern across
   the release — tracked separately as sworn#94; this is just the S15 fix.)

Flags (not pins): (a) manifest sourcing (names/$id/version from SchemaMap,
classification table + skew check) matches R-01's mitigation as written; (b)
graded=7/advisory=2 split verified against live `Validate()` switch +
`ValidateSchema` call sites, matches design exactly; (c) skew-check test plan
(`SetSchemaMapForTest` + `cmdDoctor` + assert `[WARN]`) matches the proven
`TestDoctorFailsOnShaPin` pattern already in doctor_test.go — reachable per
Rule 1; (d) P1 group placement ("Group 1b" after Group 1) has no naming
collision, confirmed clean; (e) no real touchpoint collision with S11 despite
shared spec.json listing (`doctor_test.go`) — S11's landed diff never touched
it; (f) `sworn llm-check --check design-review` skipped (paid dispatch, no
authorization this session).

§2 decisions D1 (memory-cited: [[feedback_rules_capture_not_omniscience]]), D2,
D3 acknowledged — all Type-2, no Coach decision required. No §6 questions in
this design (none raised, and none of P1-P3's reviewer-facing flags required
Coach authority).

Address pin 1 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both findings are apply-inline (a mechanical status.json transcription from content design.md already authored, and a memory-citation confirmation); no Type-1/architecturally-significant choice, no spec deviation, no product judgement call — design is sound to implement now.
-->
