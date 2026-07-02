# Captain review — S06-core-loop-and-rtm-oracle-migration
Date: 2026-07-01
Design commit: afae9d5992f44db6efe1d74a1134533ee1e37fd1

## Pins

1. [mechanical] status.json/spec.json — effort_complexity uses an invalid "medium" enum, blocking state.Read entirely (CRITICAL)
   What I observed: S06's status.json and spec.json both carry `effort_complexity: {effort: "medium", complexity: "medium"}`. The strict schema (`internal/state.EffortComplexity.Validate`, ADR-0011 §3.7 / commit 763a5c5) only accepts `low`/`high` per axis. Verified live by isolating a copy of S06's status.json and running `sworn designfit` against it: hard error, not a warning —
   `effort_complexity: invalid axes effort="medium" complexity="medium" (each must be low|high)`.
   This is release-wide, not S06-specific — S01/S02/S03/S04 carry the same invalid "medium" values (only S05/S07 use valid low/low) — but for S06 it means `state.Read` cannot parse this slice's status.json at all right now, which blocks every tool that reads state (design-fit gate, loop dispatch, `/verify-slice`, `/merge-track`), not just this one gate.
   What to ask the implementer: before writing code, remap `effort_complexity` to a valid low/high pair in BOTH spec.json and status.json and re-derive `quadrant` via the documented `Quadrant()` mapping (chore/grind/puzzle/epic). This is a mechanical enum fix, not a product decision — the existing rationale ("three functions, same fix pattern... highest-severity slice") supports a defensible low/high pick without further judgement.

2. [mechanical] status.json — design_decisions is entirely absent despite ≥5 identifiable Type-2 choices in design.md
   What I observed: per the role prompt's Step 2b instruction to confirm the design-fit gate would pass on this slice's current status.json — it has no `design_decisions` field at all. design.md documents at least five concrete choices: (a) avoid `internal/board` edits, keep `trackInfosFromBoardTracks` local; (b) delete-and-delegate `parseDocumentedSharedFiles` to `router.ParseDocumentedShared`, fail-open on parse error; (c) read `board.json` directly in `rtm.go` via a local anonymous struct rather than extend `Build`'s signature; (d) retain `parseReleaseBenefit`/`parseOrgObjective` as a legacy fallback; (e) the AC-06 reachability-artefact substitute (see pin 7). The sibling-release convention (e.g. `2026-06-27-conformance-foundation/S06-invariant2-enforcement/status.json`) records even narrow Type-2 choices as `{choice, stake_class, rationale, architecturally_significant}`. Mechanically this slice still passes today's gate — `internal/designfit.impliesType1Work` only trips on `cmd/sworn/`, `internal/state/`, `internal/verdict/` path prefixes, none of which S06 touches — so an empty `design_decisions` is the gate's benign-empty case, not a violation. Not blocking, but an audit-trail gap against this project's own convention.
   What to ask the implementer: backfill the ~5 decisions above into status.json's `design_decisions` (all Type-2, none architecturally significant) during implementation.

3. [mechanical] rtm.go's new board.json reader duplicates internal/board's canonical, tested unmarshal of the same shape
   What I observed: `readBoardVerticalTrace` (AC-04) hand-rolls a second, independently-typed `json.Unmarshal` of `board.json`'s `release.vertical_trace` object, deliberately bypassing `board.ReadBoard`/`BoardRecord` (justified: avoids rippling `rtm.Build`'s signature into `internal/implement/ready.go:69`, confirmed the sole external caller). This is exactly the drift class this release exists to close (its own `vertical_trace.benefit` text: "parses of a rendered-markdown format that changed underneath them ... across ~15 sites") — a second, hand-maintained reader of the same JSON shape means a future field rename only breaks the untested copy loudly if someone remembers it exists; `internal/board`'s copy has test coverage, this one currently would not. Given T1-drift-guard's exclusive ownership of `internal/board/board.go` this release, a shared-type fix isn't available without a track collision.
   What to ask the implementer: add a one-line comment in the new anonymous struct cross-referencing `board.Release`'s JSON tags, and/or an `rtm_test.go` case that round-trips a real `board.Render`-produced board.json through both readers and asserts agreement — inline insurance, not a redesign.

4. [memory-cited] board.json `release` shape assumption confirmed against [[project_board_v1_release_shape_skew]]
   What I observed: `readBoardVerticalTrace` unmarshals top-level `release` as an OBJECT with nested `vertical_trace.benefit`/`.org_objective`. [[project_board_v1_release_shape_skew]] records the board-v1 `release` field shape as DECIDED OBJECT/strict (cutover executed 2026-07-01, today). Confirmed directly against this release's own live `board.json`: matches exactly, and — per the design's own audit — no real board.json today carries `org_objective` (checked both `2026-07-01-render-drift-reconciliation/board.json` and `2026-07-01-loop-cli-ux/board.json`, confirmed by me independently: only `vertical_trace.benefit` present in both). No conflict; acknowledging confirms the citation.
   Citation: [[project_board_v1_release_shape_skew]]

5. [mechanical] extractReleaseWorktreePath's two other independently-named duplicates confirmed already owned by sibling slices — no orphaned drift site
   What I observed: the same-named function (independent per-package copies of the identical frontmatter-parsing bug) also exists at `internal/mcp/tools_ops.go:568` and `cmd/sworn/regress.go:95`. design.md's dead-code-deletion claim for `internal/run`'s copy is correct as far as it goes (Go's package scoping means deleting one doesn't affect the others) but doesn't mention the other two exist. Verified both are already in scope elsewhere in this release: S04-mcp-oracle-migration AC-01 explicitly covers `tools_ops.go`; S05-cli-merge-regress-oracle-migration AC-02 explicitly names `extractReleaseWorktreePath` in `cmd/sworn/regress.go`. No action needed — recorded as a confirming cross-slice check.
   What to ask the implementer: none — informational, confirms no gap.

6. [mechanical] design's own Pin 1 (touchpoint-matrix collision avoidance) — confirmed correct
   What I observed: design.md's own pin asks the reviewer to confirm no `internal/board` edits are needed and the six declared touchpoints are sufficient. Independently verified: cross-checked all 7 slices' spec.json touchpoints in this release — only S01-render-drift-guard (T1-drift-guard) touches `internal/board/board.go`/`board_test.go`; no other slice, including S06, does. `board.ReadBoard`, `BoardRecord`, `BoardTrack`, `TrackInfo` are confirmed exported and read-only-sufficient for AC-01/AC-02.
   What to ask the implementer: none — confirms the design's own question with a yes.

7. [escalate] AC-06 reachability-artefact substitute needs Coach sign-off
   What I observed: design.md's own Pin 2. AC-06's literal text calls for capturing `sworn loop --release <name> --parallel` actually finding its tracks against this repo's own multi-track release. The design proposes a substitute — a Go-level `run.RunParallel` invocation against the real release's board.json with a no-op `RunSliceFn` — because the literal CLI invocation would dispatch live slice work against T1/T2/T3's worktrees, which I confirmed via `git worktree list` are genuinely separate, currently-materialised, in-flight track worktrees (T1-drift-guard, T2-tui, T3-mcp all exist on disk right now). This is a real side-effect risk, not a hypothetical one.
   Deeper trace (post-review, on request): read `internal/router/router.go` + `internal/scheduler/worker.go` to see exactly what the literal CLI would do to each track right now, not just in the abstract. Findings: (a) T5 (this slice) is actually safe to run for real, now that review.md is committed — the router returns `coach_decision` for S06 and the worker correctly pauses without touching code. (b) T2-tui's S03 is `planned` — the router unconditionally returns `implement` for `planned`, so the literal CLI would genuinely dispatch a real `/implement-slice` run against T2's live worktree (it would halt before code per Rule 9's design-TL;DR gate, but would still commit a fresh design.md nobody asked for yet). (c) T2-tui's S02 is `design_review` with a design.md but no review.md yet — the router returns `NextReview` ("review"), and `runTrackRouter`'s dispatch switch has **no case for "review"** — it falls to `default:` and calls `releaseTrack(supervisor.StateFailed)`, i.e. running the literal CLI right now would mark the entire T2 track FAILED in the supervisor DB, for a slice that isn't actually broken. Filed as [swornagent/sworn#46](https://github.com/swornagent/sworn/issues/46) — a real bug, independent of S06's scope. This confirms the literal-CLI option is not merely "has some side effects" but currently trips a genuine dispatch-loop defect on a sibling track.
   What to ask the Coach: given #46, is the Go-level no-op-`RunSliceFn` substitute acceptable as AC-06's reachability artefact (recommended — it also sidesteps #46 entirely, since a no-op RunSliceFn never reaches the point of writing code, though it may still hit the #46 default-case DB-state mutation on T2 if run with a real Router and real DB — recommend the implementer point the substitute at a throwaway DB, as the existing unit-test harness pattern already does), or is the literal CLI invocation required regardless (accepting both the T2-tui side effects and #46 triggering)? This is not determinable from repo state alone — it is a scope/risk-tolerance call for the Coach.

Pins: 7 total — 5 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins: #1 (effort_complexity schema violation blocks state.Read on this slice entirely — must be fixed before any state-transitioning tool touches it, but the fix itself is mechanical)

## Summary

Design is technically sound and stays cleanly inside its six declared touchpoints — the "no internal/board changes needed" reasoning holds under independent verification, all cited line numbers/function names/call sites check out against live code, and no touchpoint collision exists with any sibling slice. One release-wide mechanical defect (invalid effort_complexity enum) currently blocks `state.Read` on this and four other slices and must be fixed before implementation proceeds. One genuine Coach-authority question remains open (AC-06's reachability substitute) — the design correctly self-flagged it rather than picking silently.

## Smaller flags (not pins, worth one-line acknowledgement)

- design.md's router_test.go section frames `internal/board` as a new test-file import; it is already imported there today (existing helpers) — cosmetic inaccuracy, no risk.
- Line-number citations for `RunParallel`'s frontmatter block (design says "~144-173"; live code spans 123-172) are close enough to be useful navigation aids, not exact — no action needed.

## Suggested acknowledgement reply

TL;DR sound design, stays inside its declared touchpoints, but blocked on one release-wide schema defect and one open Coach question. 6 pins + 2 flags:

1. **Fix the effort_complexity enum.** Remap `effort`/`complexity` from "medium"/"medium" to a valid low/high pair in both spec.json and status.json, re-deriving `quadrant` via `Quadrant()`. This currently hard-blocks `state.Read` on this slice — confirmed by running `sworn designfit` directly.
2. **Backfill design_decisions.** Record the ~5 Type-2 choices from design.md (avoid internal/board edits; delete-and-delegate parseDocumentedSharedFiles; direct board.json read in rtm.go; retain legacy parseReleaseBenefit/parseOrgObjective fallback; AC-06 substitute) into status.json's design_decisions during implementation.
3. **Add drift insurance around the new rtm.go board.json reader.** One-line comment cross-referencing board.Release's JSON tags, or a round-trip test against a real board.Render output, so this new reader doesn't become the next silent-drift site.
4. **[[project_board_v1_release_shape_skew]] confirmed** — rtm.go's OBJECT-shape assumption for board.json's release field matches the decided/live shape. No action.
5. **extractReleaseWorktreePath duplicates in tools_ops.go/regress.go confirmed already owned** by S04/S05 respectively. No action.
6. **Touchpoint-matrix collision avoidance confirmed correct** — no internal/board changes needed, six declared touchpoints are sufficient. No action.
7. **AC-06 reachability substitute — RESOLVED (Coach decision, see below).** Capture AC-06's reachability artefact via a Go-level `run.RunParallel` invocation against the real release's board.json with a no-op `RunSliceFn`, using a throwaway DB/EventDB (same pattern the existing unit-test harness already uses) rather than the real `.sworn/sworn.db` — this also avoids swornagent/sworn#46's default-case DB-state mutation, not just the T2-tui side effects. Do not use the literal CLI invocation for this AC.

Flags (not pins): (a) router_test.go already imports internal/board today — the "new import" framing in design.md is a minor inaccuracy, not a risk; (b) RunParallel's cited line range is approximate but close enough to navigate by.

§2 decisions (a)-(e) above acknowledged as Type-2/non-architecturally-significant. §6/Pins-for-Captain question 1 (touchpoint collision) acknowledged — confirmed correct, no internal/board changes needed. §6/Pins-for-Captain question 2 (AC-06 substitute) resolved — see Coach decision below.

Address pins 1–7 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 7 (AC-06 reachability-artefact substitute vs literal CLI invocation with real side effects on T1-T3's live worktrees) is a genuine scope/safety trade-off with no single right answer, explicitly self-flagged by the design as needing sign-off before implementation. All other pins are apply-inline and do not require re-checking the design.
-->

## Coach decision (post-review)

Date: 2026-07-01
Decision-maker: Brad (Coach), in conversation following this review

**Pin 7 (AC-06 reachability-artefact substitute): Option (a) selected.**

- Option (a) — Go-level `run.RunParallel` invocation against the real release's board.json, no-op `RunSliceFn`, throwaway DB. Prioritises isolation from sibling in-flight tracks and sidesteps swornagent/sworn#46 (whose existence was discovered while tracing this decision).
- Option (b) — literal `sworn loop --release <name> --parallel` CLI invocation. Rejected: confirmed to trigger a real `/implement-slice` dispatch against T2-tui's S03 (planned) and to trip #46 on T2-tui's S02 (marks the track FAILED for a slice that isn't actually broken) — side effects unrelated to what AC-06 is trying to prove.

Rationale: option (a) exercises the exact logic AC-01/02/03 fix (`board.ReadBoard`/`trackInfosFromBoardTracks`/`parseDocumentedSharedFiles` via `RunParallel`) against real board.json data, without depending on unrelated, out-of-scope dispatch-loop code (`internal/scheduler/worker.go`, not a declared touchpoint) that is currently broken. If literal CLI-entrypoint wiring needs separate confidence later, that is a distinct smoke test, tracked separately (not a gate on this slice) — and naturally follows once swornagent/sworn#46 is fixed.

This closes pin 7. Slice is now unblocked pending pins 1–6 (all apply-inline).
