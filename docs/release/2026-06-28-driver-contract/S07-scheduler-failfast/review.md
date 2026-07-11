# Captain review — S07-scheduler-failfast
Date: 2026-07-10
Design commit: 584e73fe6be8a919ce72f77abcb523f79acd6863

## Pins

1. [escalate] Design risk section — AC-01's literal text ("resolve the implementer, verifier, AND CAPTAIN models... on any failure it SHALL exit non-zero") contradicts the just-ratified S06 precedent that a captain-leg Resolve failure is a non-fatal Rule 2 deferral.
   What I observed: Verified against live code. `RunSlice` (slice.go:281-284) already treats captain-leg resolution failure as non-fatal (`captainDriver, captainResolveErr := opts.Registry.Resolve(...)`, error captured not returned), per S06's Coach-amended AC-02 (spec.json AC-02 text confirmed verbatim: "...SHALL record that same descriptive role error as a durable Rule 2 deferral via the existing design-gate deferral path... and proceed"; S06 captain-proceed.md pin 1, 2026-07-10, ratified live). S07's own design (the D3 pseudocode block, cmd/sworn/run.go new sweep) implements the startup sweep to match — captain-leg failure surfaces as a stderr warning but does NOT return non-zero — which is the *opposite* of AC-01's literal text. The design.md author already self-flags this in a dedicated "Design risk requiring explicit ratification (escalate)" section and proposes amending AC-01's text in-place to match the fail-open captain policy, mirroring exactly how S06's AC-02 was amended. This is a real spec-vs-design conflict per the captain.md Step 1 Part B rubric ("Design choice contradicts the spec mitigation, with explicit acknowledgement and rationale") — it needs explicit Coach ratification before the implementer can build to a policy the verifier will actually grade against.
   What to ask the implementer / Coach: Coach picks one: (a) amend AC-01 in spec.json to read "...on any failure of the implementer, verifier, or escalation-list entries it SHALL exit non-zero; a captain-leg resolution failure SHALL be surfaced as a warning and SHALL NOT block startup" (the direction the design's own rationale argues for — policy coherence with the just-ratified S06 precedent, consistent with [[project-driver-contract-recut]]'s documented S06 AC-02 amendment), or (b) require literal enforcement of AC-01 as written (captain-leg failure blocks startup) — which would mean the startup sweep and every worker's in-flight RunSlice call enforce opposite policies for the identical role/model pair. Once ratified, record it as a Type-1 `design_decisions` entry (see pin 2) citing this acknowledgement, same shape as S06's D2.
   Citation: N/A (escalate — Coach authority, not a memory rule).

2. [memory-cited] status.json has no `design_decisions` field at all.
   What I observed: `status.json` for S07 carries no `design_decisions` key (confirmed by reading the live file). This is the identical Rule 9 gate gap [[project-driver-contract-recut]] documents being caught in S04's own `/design-review` ("status.json has NO `design_decisions` (Rule 9 gate can't pass)"), and every verified sibling in this track (S04, S05, S06) populated the field before `in_progress` in the exact `choice / stake_class / options / human_decision / rationale / architecturally_significant` shape (confirmed by reading all three live status.json files). S07's design.md names at least four distinct decisions that need recording: D1 (shared resolution helper, extract-not-duplicate — narrow/reversible, likely Type-2), D2 (registry re-constructed per-invocation rather than threaded through ParallelOptions — narrow/reversible, likely Type-2), D3 (startup sweep placed in cmd/sworn/run.go rather than inside run.RunParallel — narrow/reversible, likely Type-2), and the AC-01/captain-fail-open policy call from pin 1 (Type-1 — pending Coach ratification).
   What to ask the implementer: Populate `design_decisions` for D1–D3 (and the AC-01 policy call once ratified) before transitioning to `in_progress`, following the S04/S05/S06 record shape already established on this track.
   Citation: [[project-driver-contract-recut]] ("2026-07-09: ...status.json has NO `design_decisions` (Rule 9 gate can't pass)").

3. [mechanical] Design.md's characterization of `TestRunParallel_FailureCascade`'s existing coverage is inaccurate.
   What I observed: Design.md (§ "Files to touch", `internal/run/parallel_test.go` row, and the AC-03 traceability prose) states the extension proves "no phase-wide cascade cancel," a property "`TestRunParallel_FailureCascade` today does not explicitly assert for a *sibling* (it only asserts the *dependent* T3 is skipped)." I read the complete live function (`internal/run/parallel_test.go:197-279`): its only assertions are `err == nil` (fail) and `strings.Contains(err.Error(), "T1")` — it asserts **nothing** about T2's (the sibling's) or T3's (the dependent's) actual per-track outcome. The "asserts the dependent T3 is skipped" half of the claim does not hold against live code.
   What to ask the implementer: When extending this test for AC-03, add an explicit assertion that T3 (or whatever dependent stand-in is used) reaches `TrackSkipped`, in addition to the new T2-reaches-`TrackPass` assertion the design already plans — do not assume the T3-skip assertion already exists in the base test.
   Citation: N/A (mechanical — verified by reading `internal/run/parallel_test.go:197-279` directly).

4. [mechanical] Files-to-touch table plans to touch four files not listed in spec.json's `touchpoints`.
   What I observed: spec.json's `touchpoints` array is exactly `["cmd/sworn/run.go", "cmd/sworn/run_test.go", "internal/run/parallel.go", "internal/run/parallel_test.go", "internal/scheduler/worker_test.go"]`. Design.md's "Files to touch" table additionally plans to create `internal/run/resolve.go`, create `internal/run/resolve_test.go`, edit `internal/run/slice.go`, and edit `internal/run/imports_test.go` — none of which appear in spec.json. The design's own justification for each (resolve.go/slice.go: pure extraction, no behavioural change; imports_test.go: pure regression guard, verified empty today) checks out against live code, so this reads like a defensible, low-risk expansion rather than scope creep — but it is still an undeclared touchpoint surface relative to the spec.
   What to ask the implementer: Either amend spec.json's `touchpoints` via `/replan-release` to include the four files, or explicitly record the expansion (with the "pure refactor / regression guard" rationale already in design.md) in the `design_decisions` entry for D1, so Rule-8 traceability isn't silently under-scoped.
   Citation: N/A (mechanical — verified against live spec.json).

Pins: 4 total — 2 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): 1 — shipping without Coach ratification of the AC-01/captain-leg policy risks the implementer building to a policy the verifier grades as failing spec.json's literal text, or a Coach who is never asked to bless a real spec deviation.

## Summary

Pins: 4 total — 2 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): 1 (pin 1 — AC-01/captain-leg policy conflict)

## Smaller flags (not pins, worth one-line acknowledgement)

(a) Touchpoint overlap on `internal/run/slice.go` with sibling **S08-honest-cost-telemetry** (same track T4, still `planned`, not `in_progress`/`implemented`) — does not meet the Step 6 pin trigger (sibling must be `in_progress`/`implemented`), and T4 is a documented serial spine (one implementer, one worktree) per [[project-driver-contract-recut]], so sequencing resolves itself. Noted for awareness only.
(b) Spec Risks R-01 and R-02 are both already satisfied by the design as written: R-01's fail-closed-at-t=0 mitigation matches the sweep exactly; R-02 (AC-03 depends on S06 R-03's terminal-halt landing first) is satisfied — S06 is `verified` on this branch and I independently confirmed `driver.TerminalErrKind` (subprocess.go:54-56, set = {auth, credits}) is consumed at slice.go:525-529 exactly as design.md's grounding section describes.
(c) Rule 1 (Reachability Gate) compliance is exemplary: the first failing test (`TestParallelStartupFailFast`) is planned to drive `cmdRun` end-to-end (the real `sworn run --parallel` CLI entry point), not a leaf unit — explicitly reasoned in D3.
(d) Every numbered line citation in design.md that I spot-checked against live repo state (slice.go:201-203, 259-284, 269-272, 273-280, 281-284, 525-529, 954-969; subprocess.go:51-56; cmd/sworn/run.go:56,88-105,118-168,119,134-144,153; parallel.go:301,338-363,409-433; worker.go:320,377,463,320-365; imports_test.go `scannedPackages`; capabilities_test.go test names; S06 spec.json AC-02 text; S06 captain-proceed.md pin 1) checked out exactly. This is an unusually well-grounded design.

## Suggested acknowledgement reply

TL;DR Strongly grounded design (every cited line/quote independently re-verified against live code) with one real spec-vs-design conflict that needs your call before code, plus 3 apply-inline fixes:

1. **AC-01 captain-fail-fast reconciliation.** Coach: confirm AC-01 is amended to match S06's ratified captain-leg fail-open policy — matching how S06's own AC-02 was amended (recommended direction per the design's own rationale, consistent with the S06 precedent) — or require literal enforcement instead (captain-leg failure blocks startup, which would put the startup sweep and every in-flight `RunSlice` call on opposite policies for the same role/model pair). Once decided, implement to the ratified policy and record it as a Type-1 `design_decisions` entry citing this acknowledgement.
2. **Populate `design_decisions`.** status.json currently has none. Record D1 (shared resolution helper), D2 (registry re-constructed per-invocation), D3 (sweep placed in cmd/sworn/run.go), and the AC-01 policy call from pin 1, in the S04/S05/S06 record shape, before transitioning to `in_progress`.
3. **Correct the `TestRunParallel_FailureCascade` characterization and broaden the extension.** The live test asserts neither the sibling's nor the dependent's per-track outcome today (only that `RunParallel` returns an error mentioning "T1") — when extending it for AC-03, add an explicit T3-skipped assertion alongside the new T2-TrackPass assertion; don't assume T3-skip is already covered.
4. **Reconcile the touchpoint surface.** `internal/run/resolve.go`, `internal/run/resolve_test.go`, `internal/run/slice.go`, and `internal/run/imports_test.go` aren't in spec.json's `touchpoints`. Either amend touchpoints via `/replan-release` or record the expansion's rationale in the D1 `design_decisions` entry.

Flags (not pins): (a) touchpoint overlap with planned (not yet started) S08 on slice.go — sequencing resolves via the serial track; (b) R-01/R-02 spec risks already satisfied by the design as written; (c) Rule 1 reachability compliance is exemplary (CLI-entry-point test, not a leaf); (d) every spot-checked line citation in design.md verified exact against live code.

§2 decisions D1 (shared helper), D2 (registry re-construction), D3 (sweep placement) are clean, narrow, reversible — no memory conflicts found — acknowledged. The AC-01/captain-leg design-risk call (pin 1) is the one item needing your explicit ratification. §6 question (the design's own self-flagged risk section) acknowledged — see pin 1.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 1 is a genuine spec-vs-precedent policy conflict (AC-01's literal captain-fail-fast text vs S06's ratified captain-fail-open amendment) with no code-determinable answer — the Coach must ratify which policy AC-01 should state before the implementer builds to it.
-->
