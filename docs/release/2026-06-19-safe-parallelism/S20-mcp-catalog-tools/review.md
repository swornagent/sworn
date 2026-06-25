# Captain review — S20-mcp-catalog-tools
Date: 2026-06-23
Design commit: 86165ead2c88ef3b0bbf9b4ceff107e3c82e1430

---

## Drift gate note

One commit (`73b036b — chore: materialise worktree for track T14-baton-integration`)
is in `release-wt/2026-06-19-safe-parallelism` but not in the T7 track branch.
It touches only `docs/release/2026-06-19-safe-parallelism/index.md` (bookkeeping
for a different track). It does not affect S20's spec, design, or any planned file.
Captain proceeds per judgment: spec is not stale.

---

## Pins

1. **[mechanical] §2b.1 — `design_decisions` absent from status.json**
   What I observed: S20's `status.json` has no `design_decisions` field. `design.md §2` lists 5 explicit decisions (tool-registration pattern, `plan_release` reuse of `CreateRelease`, doc format, options scaffold, overrides convention). S32-designfit-decisions-gate (verified) requires this field. S19's `status.json` (the immediately preceding T3 slice) has a properly populated `design_decisions` array with all 5 decisions in matching form. Trial log: this same missing-field pin triggered for S23 and S21.
   What to ask the implementer: Add `design_decisions` entries to `status.json` matching §2.1–§2.5 before transitioning to `in_progress`. All five appear Type-2 (implementation choices, not hard-to-reverse architectural commitments). Run `sworn designfit 2026-06-19-safe-parallelism` after adding to confirm the gate passes.

2. **[memory-cited] §2.1 — "stdlib-only approach" framing contradicts [[project_dep_policy]]**
   What I observed: Decision 1 says "Follow the existing `RegisterPlanTools` convention... Same stdlib-only approach." Memory [[project_dep_policy]] (revised 2026-06-20) explicitly says "Do not revert to 'stdlib only' framing when suggesting new deps" — the policy is now "minimal justified deps, each needs ADR." The decision itself (no new dep) is correct: catalog/decisions file ops are text manipulation; stdlib is clearly sufficient and no ADR is needed. But framing it as "stdlib-only approach" echoes the deprecated rule and could trigger S29-lint-deps' framing checks.
   What to ask the implementer: Update Decision 1 framing to "stdlib is sufficient for this slice's text-file ops; no new dependency, no ADR required." Confirm Decision 1 is type-classified and present in `status.json` `design_decisions` (covered by Pin 1).
   Citation: [[project_dep_policy]]

3. **[mechanical] §4 — prompt loading is `init()`-time, not request-time**
   What I observed: §4 states "resources.go already read from `internal/prompt` embed **at request time (closures)**." Verified: `internal/prompt/prompt.go` reads all embedded files in `init()` — the closures in `resources.go` (lines 16–50) call `prompt.Implementer()`, `prompt.Verifier()`, `prompt.Planner()` which are getter functions returning pre-loaded package-level strings. Reading happens at binary startup, not at request time. The *conclusion* ("no code change needed") is correct: S19's commit `7db0d0e` updated `internal/prompt/implementer.md` and `internal/prompt/verifier.md`, and this commit is confirmed present on the T7 branch. The S19-updated prompts are embedded at build time and will be served correctly.
   What to ask the implementer: Confirm §4's conclusion (no code change needed) while removing the "at request time" framing. Do not carry "request-time" into any implementation comment or documentation for this slice — it is factually incorrect and will confuse future readers of `resources.go`.

4. **[mechanical] §4 / Risk 1 — `create_release` references in `intake.md` unaddressed**
   What I observed: Spec Risk 1 states: "If any existing tests or documentation reference `create_release`, they must be updated in this slice." `docs/release/2026-06-19-safe-parallelism/intake.md` references `create_release` at two locations: (1) a UX flow sketch describing `sworn.create_release()` → creates intake.md + release folder, and (2) a tool list "Planning tools: `create_release`, `create_slice`, `set_track`, `update_intake`." `docs/mcp-setup.md` (user-facing) has no `create_release` references — that is clean. The design says "Not removing `create_release` from anywhere — it was never registered as a tool" but does not address `intake.md`.
   What to ask the implementer: Either (a) update `intake.md` to replace `create_release` with `plan_release` at both locations, or (b) declare `intake.md` exempt as historical planning context (not user-facing documentation), adding a Rule-2 tracking note explaining the deferral. If (b), the note must cite why, track it, and be acked — a silent skip is not acceptable per Risk 1's explicit requirement.

5. **[mechanical] §1 / AC2 — `slice_count: 24` is stale; confirm test does not hardcode it**
   What I observed: AC2 says `plan_release("2026-06-19-safe-parallelism")` (existing release) returns `{exists: true, slice_count: 24}`. The release board currently shows ~59 slices. The unit test `TestPlanReleaseExisting` uses a temp-dir fixture with a controlled slice count, so the test itself should not be affected. But the spec AC says "24" literally — a Verifier running `plan_release` against the real release for the reachability check will observe a count far above 24, which could cause a BLOCKED verdict if the AC is interpreted strictly.
   What to ask the implementer: Confirm `TestPlanReleaseExisting` asserts `exists: true` against the *fixture's* count (whatever the fixture has, not literally 24). In `proof.md`, explicitly note that the real release's `slice_count` differs from the 24 in AC2 (the count was accurate at spec-authoring time), and the AC passes on the fixture. This pre-empts a Verifier BLOCKED on the literal value.

---

## Summary

Pins: **5 total — 4 [mechanical], 1 [memory-cited], 0 [escalate]**
Critical pins: **None** — no pin causes the slice to ship broken if unaddressed, but Pin 1 (missing `design_decisions`) will cause the designfit gate to trivially pass empty, hiding the Type classification audit, and Pin 5 (stale AC2 count) risks a Verifier BLOCKED.

---

## Smaller flags (not pins, worth one-line ack)

(a) **`plan_release` Returns schema inconsistency in spec.** The formal "Returns:" line in spec §1 omits `slice_count` and `release_worktree_branch` that appear in the descriptive text and AC2. Implementation must include all fields from the description (not just the Returns line). This is not a design defect — the design correctly follows the description — but the implementer should document the full return shape in the tool handler comment for the Verifier's benefit.

(b) **Screenshots directory must be created.** The §5 reachability plan writes a screenshot to `docs/release/2026-06-19-safe-parallelism/screenshots/S20-mcp-catalog-tools-induction-status.png`. This directory does not exist yet. Either create it or update the screenshot path in `proof.md` to wherever the screenshot lands.

---

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Clean design on a well-scoped new-file slice. 5 pins — all apply-inline:

1. **Add `design_decisions` to status.json.** §2 lists 5 decisions; all appear Type-2. Add them to the `design_decisions` array in `status.json` before coding starts and run `sworn designfit 2026-06-19-safe-parallelism` to confirm the gate passes.
2. **Fix Decision 1 framing.** Change "Same stdlib-only approach" → "stdlib is sufficient for this slice's text-file ops; no new dependency, no ADR required." The dep policy is now [[project_dep_policy]] "minimal justified deps + ADR," and "stdlib-only" echoes the deprecated rule.
3. **Drop "request-time" from §4.** `prompt.go` loads embedded files at `init()`. The conclusion (no code change needed) is correct — S19's prompt changes are confirmed on the T7 branch. Do not carry "at request time" into any code comment for this slice.
4. **Resolve `create_release` in `intake.md`.** Either update the two references to `plan_release`, or declare `intake.md` exempt as historical context with a Rule-2 tracking note (why + tracking + ack). Silent skip is blocked by Risk 1's explicit requirement.
5. **Guard AC2's `slice_count: 24` in proof.md.** Confirm `TestPlanReleaseExisting` asserts against the fixture count (not literally 24). Note in `proof.md` that the real release count exceeds 24 and AC2 passes on the fixture.

Flags (not pins): (a) `plan_release` Returns schema in spec omits `slice_count`/`release_worktree_branch` — implement from the descriptive text, not just the Returns line; (b) create the screenshots directory before writing the proof.md reachability artefact.

§2 decisions 1–5 ack (subject to Pin 1 + 2 corrections). §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline corrections — missing status.json field, framing fix, comment guard, intake.md triage, AC fixture confirmation. Design is structurally sound with correct registration pattern, file plan, and stdlib choice. No redesign needed before code.
-->
