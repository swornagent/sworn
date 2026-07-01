# Captain review — S01-render-drift-guard
Date: 2026-07-01
Design commit: ed0f30cde50ea25bf98154f0a893d62c08924d5e

## Pins

1. [mechanical] status.json — missing `planned_files` and `design_decisions` entirely
   What I observed: status.json (state: design_review) has neither `planned_files` nor
   `design_decisions` populated, unlike the sibling-release convention (e.g.
   `2026-07-01-release-hygiene/docs/release/2026-06-27-conformance-foundation/S06-invariant2-enforcement/status.json`,
   which records both at design_review). `internal/designfit/designfit.go`'s
   `impliesType1Work` fallback only fires when `planned_files` matches one of
   `cmd/sworn/`, `internal/state/`, `internal/verdict/` — this slice's own
   touchpoint `cmd/sworn/doctor.go` matches that prefix. With `planned_files`
   empty, `sworn designfit` cannot detect that this slice implies Type-1 work,
   so an empty `design_decisions` silently passes the Rule 9 gate instead of
   failing closed on it.
   What to ask the implementer: populate `status.json.planned_files` with the
   four touchpoint files, and add `design_decisions` entries for at least (a)
   byte-for-byte vs. normalised comparison and (b) removing `driftGuard`
   entirely with no transition period — both classifiable `Type-2` (narrow,
   reversible) per Rule 9, each with `rationale` and
   `architecturally_significant: false`. Confirm `sworn designfit
   2026-07-01-render-drift-reconciliation` passes once populated (it currently
   errors before reaching this slice — see pin 2).

2. [escalate] spec.json — `effort_complexity.effort: "medium"` is not a valid
   enum value; breaks `sworn designfit` for the whole release
   What I observed: `internal/baton/schemas/spec-v1.json` defines `effort` and
   `complexity` as strict two-value enums (`low`|`high` only — no `medium`).
   5 of 7 slices in this release (S01, S02, S03, S04, S06) use `"effort":
   "medium"`. Running `sworn designfit 2026-07-01-render-drift-reconciliation`
   against the live worktree today does not produce a violations report — it
   hard-errors before evaluating any slice: `state: ... effort_complexity:
   invalid axes effort="medium" complexity="low" (each must be low|high)`.
   This is a release-wide planner defect, not specific to S01, but S01 is the
   first slice to reach design_review so it's the first to surface it.
   What to ask the implementer: this is a spec.json defect across multiple
   slices — per the Captain's identity contract, spec defects are flagged and
   routed to `/replan-release`, not corrected inline by the implementer or the
   Captain. Recommend a lightweight, non-scope-changing replan pass to correct
   `effort` to `low` or `high` (planner's call on which, per slice) across
   S01/S02/S03/S04/S06 before `sworn designfit` can run cleanly for this
   release.

3. [escalate] AC-05 — the reachability artefact this AC demands cannot be
   produced today, for reasons outside this slice's (and this release's) scope
   What I observed: I simulated `checkRenderDrift`'s logic (re-render each
   `docs/release/*/board.json`-backed release via the existing `board.Render`
   and diff against the committed `index.md`) against the live repo's current
   state, across all 5 releases that currently carry a `board.json`:
   - `2026-06-27-conformance-foundation`: `board.Render` **hard-errors** —
     its `board.json` still uses the legacy bare-string `"release":
     "2026-06-27-conformance-foundation"` form rather than the canonical
     `{"name": ...}` object. Per design.md's own step 4, a Render error
     "fails closed" and is surfaced as an ERROR naming the release. This
     release's tracks are all already `state: merged`; nobody in T1-T5 is
     touching it.
   - `2026-07-01-release-hygiene`: `board.Render`'s output **genuinely
     differs** from the committed `index.md` (confirmed via diff — the
     committed file is explicitly hand-authored per its own note: "This file
     is hand-authored until S02-board-render... ships a renderer; board.json
     is the source of truth" and "board.json uses the `release: string` form
     (what the installed oracle reads today)").
   Both would report as ERROR the moment `checkRenderDrift` ships, making
   `sworn doctor` exit non-zero for the *whole repo* — not because of
   anything S01–S07 introduce or regress, but because two already-known,
   already-deferred releases have never been migrated to the canonical
   board.json shape / re-rendered index.md. `intake.md` (line 76) already
   records a Rule-2 deferral of `2026-07-01-release-hygiene` (and
   `2026-07-01-loop-cli-ux`) relative to this release, acknowledged by Brad —
   but that deferral's stated reason is about *those releases'* own
   ergonomic/cosmetic priority, not about the *new* consequence that S01's
   own AC-05 proof step will trip over them. `2026-06-27-conformance-foundation`
   isn't mentioned in the deferral at all.
   AC-05, as written ("sworn doctor SHALL report zero drift errors"), is not
   achievable by S01 alone without either touching two out-of-scope,
   already-merged/deferred releases, or narrowing what AC-05 actually
   requires.
   What to ask the implementer: this needs a Coach call, not an inline pick —
   there is no single right answer. Options: (a) narrow AC-05 to scope the
   "zero drift errors" claim to only the releases this release's tracks
   touch (excluding pre-existing/deferred releases, tracked separately in
   #44/#45), (b) expand scope to migrate `2026-06-27-conformance-foundation`'s
   board.json shape (#44) and re-render `2026-07-01-release-hygiene`'s
   `index.md` (#45) as part of this release, or (c) accept that `sworn
   doctor` goes red repo-wide on landing and treat #44/#45 as tracked,
   acknowledged follow-up. The Coach picks; each has different scope and
   CI-blast-radius implications.

   Tracked: swornagent/sworn#44 (conformance-foundation legacy board.json
   shape), swornagent/sworn#45 (release-hygiene stale index.md) — filed at
   review time per Rule 2 capture discipline.

4. [memory-cited] approach aligns with [[project_index_frontmatter_corruption_false_ready]]
   What I observed: this slice's design deletes `driftGuard`'s
   `ParseTracks(extractFrontmatterBody(...))`-on-raw-`index.md` pattern
   entirely (AC-04) rather than patching it, and replaces it with a
   render-and-diff that never re-parses `index.md` as a source of truth. That
   is exactly the direction the tracked follow-up in this memory
   (`swornagent/sworn#20`) anticipated, at a different layer — the memory's
   incident was caused by treating `index.md` frontmatter as re-parseable
   source of truth; this design retires that class of read path for driftGuard
   specifically. Non-trivial alignment, worth an explicit acknowledgement.
   Citation: [[project_index_frontmatter_corruption_false_ready]]

5. [memory-cited] AC-06's scoped test command is a known false-negative risk
   for the proof bundle
   What I observed: this slice's spec.json is spec-v1 (`spec.json`, not
   `spec.md`) and AC-06 only requires `go test ./internal/board/...
   ./cmd/sworn/...`. [[feedback_releaseverify_specmd_false_fail]] documents
   two relevant lessons for this exact slice shape: (a) `release-verify.sh`'s
   first-pass will emit a residual `spec.md missing` FAIL — a known false
   negative, not a real gap, don't manufacture a spec.md to silence it — and
   (b) a scoped test run has previously missed a real cross-package
   regression (S05's stricter reader broke `board.json` fixtures in a
   different package that only `go test ./...` surfaced). This slice removes
   `driftGuard`'s call site inside `WriteBoard` in `internal/board/board.go`,
   a shared, multi-consumer file.
   What to ask the implementer: document the `spec.md missing` FAIL as a known
   false negative in the proof bundle's first-pass section, and run full `go
   test ./...` (not just the AC-06-scoped packages) before claiming the proof
   bundle done.
   Citation: [[feedback_releaseverify_specmd_false_fail]]

Pins: 5 total — 1 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins (if any): 3 (AC-05's reachability artefact is not achievable as
written given current live-repo state; would make `sworn doctor` exit
non-zero repo-wide on landing for reasons outside this slice's control)

## Summary

Pins: 5 total — 1 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins (if any): 3

## Smaller flags (not pins, worth one-line acknowledgement)

- design.md uses descriptive prose headings (User outcome / Approach / Files
  touched / Design-level risks / Out of scope) rather than the canonical
  §1–§6 numbering; content substance maps across cleanly, this is a labeling
  difference only, not a completeness gap.
- All line/function citations in design.md (`board.go:173`, `driftGuard` at
  `board.go:227`, `render.go:46`, the 5 `log.Printf` calls, the
  `trackInfosToBoardTracks`/`boardTracksToTrackInfos` call sites at
  `board.go:213` and `oracle.go:391`) were independently verified against
  live repo state and are accurate.
- Touchpoint disjointness against S02–S07 (§4/Out of scope's claim) was
  independently verified — no file-level overlap across any sibling spec.json.
- No cross-release ancestry commits on `internal/board/board.go` or
  `cmd/sworn/doctor.go` since `release/v0.1.0` — clean base, no unacknowledged
  recent-commit drift surface.

## Suggested acknowledgement reply

TL;DR Solid, well-verified mechanical design — every code citation checked out
against live repo state. 5 pins + 0 additional flags beyond what's noted below:

1. **Populate status.json Rule 9 fields.** Add `planned_files` (the four
   touchpoints) and `design_decisions` (byte-for-byte-vs-normalised comparison;
   full driftGuard removal with no transition period — both Type-2) to
   status.json before implementation. Confirm `sworn designfit
   2026-07-01-render-drift-reconciliation` passes afterward.
2. **effort_complexity schema fix (release-wide).** `spec.json`'s `"effort":
   "medium"` is invalid (schema is `low`|`high` only) — affects S01, S02, S03,
   S04, S06. Route through `/replan-release` for a mechanical correction; this
   is a planner-territory fix, not something to patch inline.
3. **AC-05 scope decision (Coach call).** Live-repo simulation shows the
   render-drift check will report ERROR for `2026-06-27-conformance-foundation`
   (legacy string-shaped board.json — Render() hard-errors, tracked in #44)
   and `2026-07-01-release-hygiene` (index.md genuinely stale vs. its
   board.json, tracked in #45) the moment it ships — neither is in scope for
   T1–T5. Coach picks: (a) narrow AC-05 to this release's own touched
   releases, (b) add scope to fix both (#44/#45), or (c) accept and track the
   repo-wide `sworn doctor` regression as a deliberate, separately-tracked
   consequence. `[[insert Coach's pick here]]`

Flags (not pins): (a) design.md's headings are prose-labeled rather than
§1–§6 numbered — no content gap, just a naming difference; (b) all code
citations in design.md verified accurate against live repo state.

§2 decisions: byte-for-byte comparison choice and the driftGuard-removal
approach are sound and [[project_index_frontmatter_corruption_false_ready]]-
aligned — acknowledged. §6: no open questions were raised by the implementer;
none found on review either, beyond pins 2–3 above.

Address pins 1–2 inline during implementation. For pin 3, apply the Coach's
picked resolution above, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: AC-05's reachability artefact is not achievable as written given two already-known, out-of-scope releases with malformed/drifted board.json-index.md pairs — a genuine scope trade-off (narrow the AC vs. expand scope vs. accept a repo-wide doctor regression) with no single right answer.
-->
