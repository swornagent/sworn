# Captain review — S02-tui-oracle-migration
Date: 2026-07-01
Design commit: d84e0009e815c72be95f27e69ed6798eeaab3d05

## Pins

1. [memory-cited] Design choices §DC-1 — proof.json reader scope vs. future proof-visibility theme
   What I observed: DC-1 picks a local minimal struct in blocked.go to read proof.json's
   not_delivered field, rather than exporting a reader from internal/implement.
   [[project_proof_visibility_theme]] plans making the Rule-6 proof bundle a first-class,
   actively-surfaced artefact across CLI/board-UI/MCP/TUI panel/notification — as its OWN
   future release, planned after R3, not ad hoc per-slice.
   What to ask the implementer: confirm this is understood as a narrow, spec-scoped bug fix
   (matching AC-03 exactly) rather than a stand-in implementation of the future proof-panel
   theme — so the later proof-visibility release isn't surprised by an ad hoc reader pattern
   it needs to unify or replace.
   Citation: [[project_proof_visibility_theme]]

2. [memory-cited] Design choices §DC-2 — lazy-migration write side effect vs. board-v1 shape contract
   What I observed: DC-2 acknowledges board.ReadBoard's migrateFromIndex path writes board.json
   as a side effect of a TUI browse. [[project_board_v1_release_shape_skew]] documents the
   board-v1 release field as OBJECT-ONLY/strict (S05 AC-03) with a 2026-07-01 cutover — a stale
   binary or reader writing the legacy bare-string form would produce a board.json the strict
   reader then fails closed on.
   I independently verified this against live code: migrateFromIndex constructs the release via
   StringRelease(...), which board_release_test.go:TestStringRelease_EmitsCanonicalObject confirms
   emits the canonical object form ({"name":"..."}), never the bare string. DC-2's write path is
   already consistent with the strict reader landed in this track's internal/board package.
   What to ask the implementer: no action needed — cite this confirmation in the acknowledgement
   so it's on record that the cross-check was done, not assumed.
   Citation: [[project_board_v1_release_shape_skew]]

3. [mechanical] §3 Files to touch — touchpoint overlap with sibling S03-tui-chrome-rework
   What I observed: internal/tui/tui_test.go is a declared touchpoint of both this slice and
   S03-tui-chrome-rework (same track T2-tui). S03 is currently state=planned (not in_progress),
   so this isn't a live collision, but S02's design substantially rewrites tui_test.go (new
   writeBoardFixture helper, several tests converted to the real render path).
   What to ask the implementer: no design change needed — single serial implementer per track
   worktree means S02 lands first and S03 inherits the regenerated fixture helper. Just flag it
   in the acknowledgement so S03's implementer isn't surprised the fixture shape moved
   underneath it.

## Summary

Pins: 3 total — 1 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: none

## Smaller flags (not pins, worth one-line acknowledgement)

- `sworn designfit 2026-07-01-render-drift-reconciliation` passes cleanly for all 7 slices
  (checked live) — Rule 9 design-fit gate satisfied despite no `status.json.design_decisions`
  field on any slice in this release (a release-wide convention, not S02-specific).
- Verified the design's factual claims against live code: `TrackInfo.Depends` has no other
  consumer (grep confirms), `internal/implement`'s import set is genuinely heavy
  (agent/git/prompt/reqverify/state), and AC-05's named test release
  (`2026-06-30-sworn-operational-readiness`) has a committed, 5-track, object-shape `board.json`.
  No inference-dressed-as-fact issues found.
- Cross-release ancestry on all three touched files (`board.go`, `blocked.go`, `tui_test.go`)
  since `release/v0.1.0`: no commits — clean.
- Step 4 (cross-stack drift) not applicable — single Go runtime, no FE/BE boundary in this slice.
- Spec-completeness gate: spec.json ACs are concrete (file paths, function names, specific test
  names, a named release for AC-05) — not thin.
- Track drift gate: this review's Step 0 initially found the T2-tui track worktree 2 commits
  behind release-wt/ (worktree-materialisation commits, then a legitimate AC-05/effort_complexity
  schema replan landed on release-wt mid-session). Forward-merged before this review proceeded;
  confirmed `rev-list --count HEAD..release-wt/2026-07-01-render-drift-reconciliation` = 0.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Clean design, no blocking issues — 2 memory citations to acknowledge and 1 sequencing
flag. 3 pins:

1. **Proof-visibility theme scope check.** DC-1's local proof.json struct is a narrow,
   spec-scoped bug fix (AC-03) — confirm it's understood that way and not a stand-in for the
   future proof-panel theme ([[project_proof_visibility_theme]]), so that release isn't
   surprised later by a pattern it needs to unify.
2. **Board-v1 shape cross-check — already confirmed clean.** DC-2's migrateFromIndex write path
   uses StringRelease, which emits the canonical object form per the strict S05 reader
   ([[project_board_v1_release_shape_skew]]). No action — just recording the citation.
3. **tui_test.go touchpoint overlap with S03-tui-chrome-rework.** Same track, S03 still
   planned — no live collision. S02 lands first; S03's implementer will build on your
   regenerated writeBoardFixture helper.

Flags (not pins): sworn designfit passes clean; AC-05's target release/board.json verified as
claimed; design's other factual claims (Depends field usage, internal/implement weight)
verified true by grep.

§2 decisions DC-1 and DC-2 acknowledged (both Type-2, both memory-cross-checked clean).
No §6 questions were raised; none needed — nothing in §1–5 surfaced a Coach-authority call.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline acknowledgements (2 memory citations already independently confirmed clean, 1 sequencing note with no design impact) — no re-review of the design is needed before code.
-->
