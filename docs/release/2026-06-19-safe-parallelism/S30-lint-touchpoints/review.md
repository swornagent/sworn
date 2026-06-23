# Captain review — S30-lint-touchpoints
Date: 2026-06-22
Design commit: adf27aff36ac7cffe055244747d9f62d10858235

## Pins

**1. [mechanical] §2.D2 vs spec Risk — full-scan extraction causes false positives on S30's own spec**
> What I observed: Design D2 extracts "back-ticked tokens that contain `/` or end in `.go`/`.ts`/`.tsx`/`.md`" from all of spec.md. Grepping S30's own spec under that pattern yields undeclared paths that would fail: `cmd/sworn/main.go` (additive-invariant In-scope example), `internal/foo/bar.go` (Required-tests fixture example), `docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/spec.md` (Risk mitigation audit path), `docs/release/<release>/index.md` (template path), `proof.md`, and `index.md`. None of these are in planned_files. The spec Risk mitigation prescribes the exact fix: "scope extraction to back-ticked identifiers that look like paths... and **to the In-scope / Planned-touchpoints sections** — mirroring how `cmd/sworn/lint.go`'s `trace` target already locates structured content." Design D2 implements path-shaped filtering but ignores section-scoping. Result: the §5 reachability self-test claim ("should exit 0") is wrong — running `sworn lint touchpoints S30-lint-touchpoints 2026-06-19-safe-parallelism` against the real release would exit 1 due to false positives from S30's own spec.
> What to ask the implementer: Scope extraction to `## In scope` and `## Planned touchpoints` sections of spec.md only, consistent with how `cmdLintTrace` locates structured content. Combined with path-shape filtering, this eliminates Out-of-scope/Risk/tests false positives. Also perform the spec Risk audit step before wiring (see Pin #4).

**2. [mechanical] §2b design-fit gate — `design_decisions` absent from status.json**
> What I observed: S30's status.json has no `design_decisions` field. S29's verified status.json has 4 entries; this is the expected shape. `sworn designfit <release>` (the gate Step 2b confirms) would fail on an absent field. Design §2 has 5 decisions, all Type-2.
> What to ask the implementer: Add `design_decisions` to status.json before transitioning to in_progress. Five entries, all `"stake_class": "Type-2"`, one per §2 decision (D1–D5) with a concise rationale matching the design's language.

**3. [escalate] §4 vs spec In-scope — additive-invariant check replaced with weaker detection**
> What I observed: Spec In-scope explicitly includes: "additive-invariant check for documented-shared files. A file the matrix marks 'DOCUMENTED SHARED — additive …' may be touched by multiple tracks ONLY additively. Flag a slice whose design implies a **non-additive** (restructuring) edit to such a file… Report the non-additive edit + the other tracks sharing the file; the Coach/planner decides ownership." Design §4 says this detection is not implemented ("too fuzzy for a fail-closed gate") and substitutes: "flag any DOCUMENTED SHARED file that appears in the slice's planned_files and report it as an informational note." The substitution is a blunter signal — it flags every slice with a DOCUMENTED SHARED file in planned_files regardless of whether the edit is additive or restructuring. The 2026-06-21 `cmd/sworn/main.go` merge conflict (T9 extraction vs T2 no-args launch — the exact incident the spec cites as motivation) would NOT have been caught by the design's substitution: T9 and T2 would both have `cmd/sworn/main.go` in their informational notes, but so would every track doing a legitimate additive case-dispatch — making the signal noisy.
> What to ask the Coach: Accept the substitution (informational note when any DOCUMENTED SHARED file appears in planned_files) or require the non-additive detection the spec prescribes? Option (a) is simpler to implement and ships something useful, but will generate informational noise on every legitimate additive-dispatch edit to `cmd/sworn/main.go`. Option (b) requires prose heuristics to detect "restructuring vs appending" from design text — genuinely fuzzy. Coach picks.

**4. [mechanical] §3/Risk — spec Risk audit step not captured in design**
> What I observed: Spec Risk says "verify the section anchors against a real spec (`docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/spec.md`) before relying on them." Design has no corresponding step in §3, §4, or §6. Without verifying that `## In scope` and `## Planned touchpoints` exist as parseable headings across real specs, the section-scoped parser might misfire on heading case or wording differences.
> What to ask the implementer: Read S02b's spec.md during implementation and confirm both section headings exist with exact casing. If headings differ across specs, update the parser's anchor pattern to match reality and record the finding in the journal.

## Summary

Pins: 4 total — 3 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins: **Pin #1** — §5 reachability self-test exits 1 (not 0) on S30's own spec under D2's full-scan approach; proof.md would contain a false-positive artefact and fail Gate 4 at verification.

## Smaller flags (not pins, worth one-line ack)

(a) Usage string in `cmdLint` (current: "ac|trace" + "deps") needs updating to include `touchpoints` — implied by the design but not called out. Apply inline.

(b) `internal/foo/bar.go` in the Required-tests section is an example fixture path, not a real file — the section-scoped extraction fix (Pin #1) eliminates this concern.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound in structure but has one critical flaw and two apply-inline fixes. 4 pins + 1 flag:

1. **Section-scoped extraction (CRITICAL).** D2's full-scan extracts paths from Risk, Required-tests, and Out-of-scope sections of spec.md, producing false positives on S30's own spec (`cmd/sworn/main.go`, `internal/foo/bar.go`, `docs/release/…/S02b…/spec.md`, etc.). Scope extraction to `## In scope` and `## Planned touchpoints` sections only — as the spec Risk mitigation directs and as `cmdLintTrace` already does. Combined with your path-shape filter, this is the correct implementation.

2. **Add `design_decisions` to status.json.** Five entries (all Type-2, one per §2 decision D1–D5) before transitioning to in_progress.

3. **Coach must ack additive-invariant scope reduction.** Pin #3 is an [escalate] — the substitution (flag DOCUMENTED SHARED files in planned_files as informational) replaces the spec's "detect non-additive edits from design prose." Coach is deciding. Implement the substitution as designed; the Coach verdict will determine whether a follow-up spec amendment is needed.

4. **Spec Risk audit step.** Read `docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/spec.md` during implementation; confirm `## In scope` and `## Planned touchpoints` headings exist with exact casing before wiring the section-scoped parser. Record finding in journal.

Flag (not a pin): update `cmdLint` usage string to include `touchpoints` alongside `ac|trace|deps`.

§2 decisions D1–D5 ack (all Type-2, no Type-1). §6 empty ack.

Address pins 1, 2, 4 inline during implementation; pin 3 awaits Coach reply below. Proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin #3 is an explicit in-scope reduction from a spec In-scope item (additive-invariant check); Coach must decide whether to accept the substitution or require the prose-based detection the spec prescribes.
-->
