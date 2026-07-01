# Captain review — S02-board-render (RE-REVIEW 2, post-IMPLEMENTER_FIX)
Date: 2026-07-01
Design commit: 17dba25720f34c33f9f5030af48c6d53b06d8856

**Supersedes:** the prior re-review (`DECISION: IMPLEMENTER_FIX` — design.md still
specified the pre-cutover tolerant `renderBoard` decoder). Prior version preserved
in git history. The IMPLEMENTER_FIX has been applied (commit `17dba25` "revise design
per IMPLEMENTER_FIX"); this pass re-checks the revised design.

## Context — what the revision changed

The prior re-review returned IMPLEMENTER_FIX on a single Verifier-invisible reader
choice: the design still described a local tolerant `renderBoard` decoder even though
the Coach had executed the S05 AC-06 cutover (board migrated string → object,
canonical strict `sworn` installed). The revised design.md now:

- **Choice 1** — decodes via canonical `board.ReadBoard`, strict, object-only; no
  local `renderBoard` struct, no string-form acceptance. Verified live: object board,
  `ReadBoard` succeeds (`sworn board --release … --json` → exit 0, 4 tracks).
- **Choice 2** — adds an `os.Stat` guard before `ReadBoard` so a *missing* board.json
  fails closed (AC-04) instead of falling through `ReadBoard`'s absent-file branch to
  `migrateFromIndex`, which would reconstruct the record from index.md and invert the
  data flow this slice exists to enforce.
- **Type-1 block** — records the reader choice (chosen: strict ReadBoard + object
  board; rejected: tolerant decoder) for transcription into `status.json` at
  `in_progress`.
- **Pin 3** — widens the test command set beyond `./internal/board/...`.

All four concrete code anchors the design cites were verified against live repo state:
`ReadBoard`@board.go:126, `Release.UnmarshalJSON`@board.go:54, `migrateFromIndex`
call@board.go:141, `ValidateIndex`@index.go:48. `render.go` does not yet exist in
either `internal/board/` or `cmd/sworn/` (new-files-only holds); `render` is not a
registered verb; `ship` uses the `<release> [project-root]` positional form the design
mirrors. Every prior pin is resolved; nothing in this pass requires re-checking the
design before code.

## Pins

### 1. [memory-cited] §Key-choices.1 — canonical strict `board.ReadBoard`, no second tolerant reader (prior critical Pin 1 resolved).
**Observed:** Choice 1 reads "Decode `board.json` via canonical `board.ReadBoard` —
strict, object-only … does **NOT** define a local `renderBoard` struct and does
**NOT** accept the bare-string `release` form." Verified live: board object-form,
`ReadBoard` succeeds.
**Ask:** Acknowledge the citation; confirm the strict-reader direction still holds
(no AC revision, no 2nd reader — as the memory prescribes).
**Citation:** [[project_board_v1_release_shape_skew]], [[feedback_releaseverify_specmd_false_fail]]

### 2. [mechanical] §Type-1 decision — transcribe into `status.json.design_decisions` at `in_progress`.
**Observed:** `status.json.design_decisions` is absent (correct for `design_review`).
The design carries a complete Type-1 block; the human decision already exists
(Coach-authorised S05 AC-06 cutover).
**Ask:** At `planned → in_progress`, write the Type-1 block into
`status.json.design_decisions` — transcription of a Coach-made decision, not a fresh
model-originated Type-1.

### 3. [mechanical] §Key-choices.2 — AC-04 `os.Stat` guard against `ReadBoard`'s absent-file lazy-migration.
**Observed:** Verified `board.go:126-141`: present board.json → strict decode (string
fails closed via `Release.UnmarshalJSON`@54); absent board.json (`os.IsNotExist`) →
`migrateFromIndex`@141 reconstructs from index.md. That absent-file branch would
invert the data flow and void AC-04. The `os.Stat`-first guard is the correct fix.
**Ask:** Confirm the guard is implemented and covered by a fail-closed test over a
board-less fixture dir (non-zero exit + no index.md written).

### 4. [memory-cited] §Pins.3 — widen test scope beyond `./internal/board/...`.
**Observed:** AC-06 names only `go test ./internal/board/...`, but the slice adds
`cmd/sworn/render.go`.
**Ask:** Also run `go test ./cmd/sworn/...` and a full `go test ./...` **with a
timeout**. Apply inline; the `/merge-track` affected-package gate backstops.
**Citation:** [[feedback_releaseverify_specmd_false_fail]], [[project_newline_eating_edit_corruption]]

### 5. [memory-cited] §Key-choices.4 — frontmatter-fusion kill confirmed sound.
**Observed:** Choice 4 runs rendered output through `board.ValidateIndex`
(`index.go:48`) + single-quoted YAML scalars — directly targets the `state: merged---`
fence-fusion class.
**Ask:** Confirm the AC-05 reachability step actually replaces the hand-authored
index.md with `sworn render`'s output, not just passes the golden fixture.
**Citation:** [[project_index_frontmatter_corruption_false_ready]]

### 6. [mechanical] Benign drift — T2 behind `release-wt` by 1 commit (not stale).
**Observed:** `rev-list --count T2..release-wt` = 1: commit `dd9cddf` (materialise T5
worktree). Its only board.json delta is T5's own `state`/`worktree_path` bookkeeping;
S02's spec.json is byte-identical on both branches and design.md exists only on T2.
The review is **not** stale.
**Ask:** Before `/merge-track`, T2 forward-merges `release-wt/` (implementer/verifier
drift gate — not a Captain action). No review impact.

## Summary

**6 pins — 3 [mechanical], 3 [memory-cited], 0 [escalate].**
Critical: **none.** The prior critical Pin 1 (forbidden tolerant decoder) is resolved
by the revision. Every remaining pin is apply-inline: a citation acknowledgement, a
Type-1 transcription, a guard/test confirmation, a test-scope widening, and a benign
forward-merge hygiene note. The design is sound, fully AC-traced, memory-aligned, and
all cited anchors verify live.

## Smaller flags (not pins, worth one-line acknowledgement)
- (a) The `sworn#20` lint drift-guard (committed index.md vs render output) is
  correctly scoped **out** as a tracked Rule-2 deferral — keep it tracked; do not pull
  it into this slice.
- (b) The reachability artefact here is CLI file output (the rendered `index.md`), not
  a screenshot — appropriate for a non-UI slice; AC-05 is the artefact.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically
     reads everything between this heading and the next ## heading (or EOF). Keep this
     content verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong revision — the IMPLEMENTER_FIX landed cleanly and every anchor verifies live. 6 pins, all apply-inline, 0 escalate:

1. **Strict reader confirmed.** Choice 1's canonical `board.ReadBoard` (no local tolerant decoder, no string form) is the settled cutover direction. Citation acknowledged — proceed as designed.
2. **Record the Type-1 decision.** At `planned → in_progress`, transcribe the design's Type-1 block into `status.json.design_decisions` (chosen: strict ReadBoard + object board; rejected: tolerant decoder). It's a Coach-made decision you're recording, not originating.
3. **AC-04 os.Stat guard.** Implement the `os.Stat`-before-`ReadBoard` guard and add a fail-closed test over a board-less fixture dir (assert non-zero exit + no index.md written) — `ReadBoard`'s absent-file branch lazy-migrates from index.md, which would void AC-04.
4. **Widen test scope.** Also run `go test ./cmd/sworn/...` and a full `go test ./...` with a timeout (a strict-reader change regressed cmd/sworn fixtures before; a fused newline once hung a test 10 min).
5. **Frontmatter kill.** Keep the `ValidateIndex` + single-quoted-scalar approach; make the AC-05 reachability step actually replace the hand-authored index.md with `sworn render`'s output.
6. **Forward-merge before merge-track.** T2 is 1 commit behind release-wt (benign T5 materialisation) — pull it in via the drift gate before `/merge-track`; no review impact.

Flags (not pins): (a) sworn#20 lint drift-guard stays a tracked Rule-2 deferral, out of this slice; (b) reachability is the rendered index.md (CLI output), not a screenshot.

§2 decisions 1–5 (Choice 1 [memory-cited: project_board_v1_release_shape_skew], Choices 2/3/5 clean, Choice 4 [memory-cited: project_index_frontmatter_corruption_false_ready]) acknowledged. No open §6 questions. Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Revised design resolves every prior pin — canonical strict ReadBoard replaces the forbidden tolerant decoder (Verifier-invisible choice now correct), os.Stat guard fixes the AC-04 lazy-migration interaction, Type-1 block ready to transcribe; all cited anchors verify live and remaining pins are apply-inline mechanical/memory-cited confirmations.
-->
