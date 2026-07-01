# Captain review — S01-render-drift-guard
Date: 2026-07-02
Design commit: 1184572fd3eec721b5cf70f6202937c8a6b64440

## Pins

1. [mechanical] status.json — still missing `planned_files` and `design_decisions`
   What I observed: this is unresolved from the previous review pass. `status.json`
   (state: `design_review`) still has neither `planned_files` nor `design_decisions`
   populated. `internal/designfit/designfit.go`'s `impliesType1Work` fallback only
   fires when `planned_files` matches one of `cmd/sworn/`, `internal/state/`,
   `internal/verdict/` — this slice's own touchpoint `cmd/sworn/doctor.go` matches
   that prefix. I ran `sworn designfit 2026-07-01-render-drift-reconciliation` live
   just now: it reports `DESIGNFIT PASS — 7 slice(s) checked, all design-fit gates
   clear`, exit 0. That PASS is vacuous for this slice specifically — with
   `planned_files` empty, the gate has no touchpoint to match against `cmd/sworn/`,
   so it cannot evaluate Rule 9 here at all; it isn't confirming compliance, it's
   blind to this slice. Do not read the green `designfit` run as clearing this pin.
   What to ask the implementer: populate `status.json.planned_files` with the four
   touchpoint files, and add `design_decisions` entries for at least (a) byte-for-byte
   vs. normalised comparison and (b) removing `driftGuard` entirely with no transition
   period — both classifiable `Type-2` (narrow, reversible), each with `rationale` and
   `architecturally_significant: false`. Re-run `sworn designfit
   2026-07-01-render-drift-reconciliation` afterward and confirm the PASS is now a
   real evaluation of this slice, not a vacuous one.

2. [mechanical] design.md's AC-05 discussion predates the replan; also, `sworn doctor`
   already exits non-zero today for reasons unrelated to this slice
   What I observed: design.md's "AC-05 reachability" bullet was written at the
   original design commit (`ed0f30c`), before the `/replan-release` pass (`cfc0a2c`,
   forward-merged as this track's current HEAD) that narrowed AC-05's text to scope
   "zero drift errors" to this release's own tracks (T1-T5) and carved out
   `swornagent/sworn#44` (`2026-06-27-conformance-foundation`, legacy string-form
   board.json) and `#45` (`2026-07-01-release-hygiene`, stale hand-authored
   `index.md`) as tracked, non-blocking exceptions. design.md's current text doesn't
   mention either exception, and its own conclusion ("not a cross-track coupling
   risk") is about a different concern (sibling tracks regressing other releases)
   than the AC-05 rewording addresses.
   I independently verified both exceptions are still live, not stale, right now:
   `2026-06-27-conformance-foundation`'s `board.json.release` is still a bare string
   (`'2026-06-27-conformance-foundation'`) — the S05-hardened reader is object-only
   and fails closed on this, confirming #44 still applies. For #45, I ran `sworn
   render 2026-07-01-release-hygiene` against the live repo (in the primary
   checkout, then immediately `git checkout --` reverted the resulting diff — this
   review does not modify production files) and it produced a 34-line diff against
   the committed `index.md`, confirming `release-hygiene`'s index.md still genuinely
   drifts from `Render()`'s output today. Both exceptions are real and current.
   Separately, and more load-bearing for the proof bundle: I ran `sworn doctor` for
   real against this repo just now. It exits 1 today, entirely because of Group 2b
   ("[ERROR] status timestamps — 95 violation(s) across scanned releases", future-dated
   `last_updated_at`/`verifier_verdict_at` values in unrelated releases like
   `2026-06-19-safe-parallelism` and `2026-06-27-conformance-foundation`) — nothing to
   do with render drift, since `checkRenderDrift` doesn't exist yet. AC-05's proof
   step ("confirm the drift-guard check reports no errors beyond the tracked
   exceptions") is a claim about the *new check's own reported output*, not about
   `sworn doctor`'s overall exit code — the overall exit code is already non-zero
   today and will remain so regardless of whether this slice is implemented
   correctly.
   What to ask the implementer: when writing the proof bundle, don't attempt to show
   `sworn doctor` exiting 0 — it can't, for reasons outside this slice's control.
   Instead capture the render-drift check's own section of the output, showing OK
   for all in-scope releases and (if it surfaces them at all, depending on how the
   check is scoped) ERROR only for the two tracked exceptions. Worth a one-line
   update to design.md or a note in journal.md so a future reader isn't confused by
   a non-zero `sworn doctor` exit code next to an "AC-05 satisfied" claim.

3. [memory-cited] driftGuard/ParseTracks retirement aligns with
   [[project_index_frontmatter_corruption_false_ready]]
   What I observed: this slice's design deletes `driftGuard`'s
   `ParseTracks(extractFrontmatterBody(...))`-on-raw-`index.md` pattern entirely
   (AC-04) rather than patching it, replacing it with a render-and-diff that never
   re-parses `index.md` as a source of truth. That's exactly the direction the
   memory's tracked follow-up (`swornagent/sworn#20`) anticipated, at a different
   layer — the memory's original incident was caused by treating `index.md`
   frontmatter as re-parseable source of truth; this design retires that class of
   read path for `driftGuard` specifically. Re-confirmed still accurate; no change
   since the last review.
   Citation: [[project_index_frontmatter_corruption_false_ready]]

4. [memory-cited] AC-06's scoped test command is a known false-negative risk for the
   proof bundle
   What I observed: unchanged from the last review. `spec.json` is spec-v1 (not
   spec.md), and AC-06 only requires `go test ./internal/board/... ./cmd/sworn/...`.
   [[feedback_releaseverify_specmd_false_fail]] documents two relevant lessons: (a)
   `release-verify.sh`'s first-pass will emit a residual `spec.md missing` FAIL — a
   known false negative, don't manufacture a spec.md to silence it — and (b) a
   scoped test run has previously missed a real cross-package regression. This slice
   removes `driftGuard`'s call site inside `WriteBoard` in `internal/board/board.go`,
   a shared, multi-consumer file.
   What to ask the implementer: document the `spec.md missing` FAIL as a known false
   negative in the proof bundle's first-pass section, and run full `go test ./...`
   (not just the AC-06-scoped packages) before claiming the proof bundle done.
   Citation: [[feedback_releaseverify_specmd_false_fail]]

Pins: 4 total — 2 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): none — both mechanical pins are apply-inline corrections
(populate two status.json fields; clarify what the AC-05 proof step actually
measures), not design changes.

## Summary

Pins: 4 total — 2 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- Both previously-escalated pins from the first review round (effort_complexity
  schema violation; AC-05 scope) are confirmed resolved by the `/replan-release`
  pass now on this track's branch — `effort_complexity` is `low`/`low`/`chore`
  (valid enum today; the `chore`->`quick` rename is tracked separately in
  `sworn#48`, not yet vendored, not this slice's concern), and AC-05's text now
  carries the T1-T5 scope + `#44`/`#45` exceptions.
- Touchpoint disjointness against S02-S07 re-verified directly from live sibling
  spec.json files (not re-used from the prior review) — still clean, no overlap.
- No commits on any of this slice's 4 touchpoint files since the release base
  (`release/v0.1.0..HEAD` is empty for all four) — clean base, nothing to reconcile.
- design.md's descriptive prose headings (User outcome / Approach / Files touched /
  Design-level risks / Out of scope) rather than canonical §1-§6 numbering —
  labeling difference only, not a completeness gap, same as last review.
- `sworn designfit` for the release overall now passes cleanly (previously
  hard-errored pre-replan) — but see pin 1 for why that PASS doesn't clear this
  specific slice's Rule 9 gap.

## Suggested acknowledgement reply

TL;DR Design is sound and unchanged since the last review — the replan already
resolved both prior escalations (effort_complexity schema, AC-05 scope). 4 pins,
all apply-inline, no new judgement calls:

1. **Populate status.json Rule 9 fields.** Add `planned_files` (the four
   touchpoints) and `design_decisions` (byte-for-byte-vs-normalised comparison;
   full driftGuard removal with no transition period — both Type-2) to
   status.json. Re-run `sworn designfit 2026-07-01-render-drift-reconciliation`
   afterward and confirm it's evaluating this slice for real, not vacuously
   passing on an empty `planned_files`.
2. **AC-05 proof step: don't chase `sworn doctor` exiting 0.** It already exits 1
   today for unrelated reasons (95 pre-existing status-timestamp violations in
   other releases). Capture the render-drift check's own output showing OK across
   in-scope releases and ERROR only for the two tracked exceptions (`#44`, `#45`
   — both independently confirmed still live). Note this distinction in the proof
   bundle or journal.md.
3. **Document the `spec.md missing` false-negative.** `release-verify.sh`'s
   first-pass will FAIL on a missing `spec.md` for this spec-v1 slice — known,
   not a real gap. Note it in the proof bundle's first-pass section.
4. **Run full `go test ./...`, not just the AC-06-scoped packages**, before
   claiming the proof bundle done — this slice touches `WriteBoard`, a shared
   multi-consumer function.

Flags (not pins): (a) design.md's headings stay prose-labeled rather than
numbered §1-§6 — no content gap; (b) touchpoint disjointness against S02-S07
re-verified clean; (c) no ancestry drift on any of the four touchpoint files
since release base.

§2 decisions (byte-for-byte comparison; driftGuard removal) remain sound and
[[project_index_frontmatter_corruption_false_ready]]-aligned — acknowledged.

Address pins 1-4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 4 pins are apply-inline corrections (populate two status.json fields; clarify what the AC-05 proof step measures given sworn doctor's pre-existing unrelated non-zero exit; document a known false-negative; run the full test suite) — none require re-checking the design itself, which is unchanged and sound since the last review. Both prior escalations are already resolved by the replan on this branch.
-->
