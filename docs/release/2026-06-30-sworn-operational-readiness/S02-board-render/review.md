# Captain review — S02-board-render
Date: 2026-07-01
Design commit: 2ab361ad48624dd1b0a6ecd145f7b17b2579ec30

## Pins

1. [escalate] §Choices.1 / Pin 1 — AC-04 ↔ AC-05 are in tension, and the fix is a Type-1 architectural choice (a second, tolerant board.json reader).
   What I observed: The design (Choice 1, Pin 1) picks a *local tolerant* `renderBoard` decode over the canonical `board.ReadBoard`, because `ReadBoard` rejects this release's `board.json` (`release` is a bare string). I verified all of this live: `board.ReadBoard` exists (`internal/board/board.go:126`) and rejects a string release with "board release: not a canonical {name} object" (`board.go:59`); the live `board.json` `release` field is the string `"2026-06-30-sworn-operational-readiness"`; `BoardTrack`/`StringList` exist and are tolerant (`board.go:84-119`). So the tension is a real, determinable fact — not an inference. But it is an *internal spec contradiction*: AC-04 says fail closed when `board.json` is "invalid against board-v1" (and board-v1's canonical reader requires `release` = object), while AC-05 requires `sworn render` to *succeed* against this exact string-shaped board. Both cannot hold under the strict reading. Introducing a second board.json reader with different strictness than the canonical one is also an architecturally-significant (Type-1) choice — it creates a lasting reader-divergence surface.
   What to ask the implementer: This is a Coach call, not an implementer pick. Option (a): render tolerates the dual `release` form (string-or-object) and does NOT enforce `release=object` — AC-05 passes now; AC-04's fail-closed teeth narrow to genuine corruption; a tolerant reader coexists with the strict `ReadBoard`. Option (b): render enforces strict board-v1 (`release=object`) — requires migrating `board.json` (a T4 touchpoint) first, re-scoping S02 or adding a T4 dependency; AC-05 cannot pass until then. The Coach picks (a) or (b), or resolves the AC-04/AC-05 contradiction via `/replan-release`.

2. [mechanical] §Choices / status.json — Rule 9 design-fit gate: `design_decisions` is absent and Choice 1 is unclassified.
   What I observed: `status.json` has no `design_decisions` field (verified). The design-fit gate has nothing to check, yet Choice 1 is architecturally-significant (a divergent reader path) and is presented as a single option — Rule 9 requires it be classified Type-1 with a recorded human decision.
   What to ask the implementer: Once the Coach resolves Pin 1, record that resolution in `status.json.design_decisions` as a Type-1 decision (chosen option + rationale + the two options above) before writing code.

3. [memory-cited] §Choices.1 / AC-06 — the tolerant-reader direction is memory-consistent, but AC-06's test scope misses the package the memory says a reader change regressed.
   What I observed: Choice 1 aligns with [[feedback_releaseverify_specmd_false_fail]], whose tail records that "a tightened reader/contract can regress test fixtures in other packages (S05 strict reader broke board.json string-form fixtures in internal/board + cmd/sworn)." The design's tolerant reader is the right response to that pain. But AC-06 scopes verification to `go test ./internal/board/...`, while this slice *adds* `cmd/sworn/render.go` and a new reader path — `cmd/sworn` is exactly the package the memory flags.
   What to ask the implementer: Confirm the memory applies, and run `go test ./cmd/sworn/...` (not just `./internal/board/...`) before claiming done — or rely on the `/merge-track` affected-package regression gate and note that explicitly in the proof. Also run full `go test ./...` with a timeout per [[project_newline_eating_edit_corruption]].
   Citation: [[feedback_releaseverify_specmd_false_fail]]

4. [memory-cited] §Choices.3 / AC-03 — confirmation: the slice structurally kills the frontmatter-fusion failure class.
   What I observed: Choice 3 (render through `board.ValidateIndex` — verified at `internal/board/index.go:48`) plus AC-03's single-quoted YAML scalars directly targets [[project_index_frontmatter_corruption_false_ready]]. The structural fix (render deterministically instead of hand-authoring/hand-editing) removes the newline-eating edit path that caused the false merge-ready. The spec rationale scopes OUT the lint drift-guard (sworn#20) as an acknowledged follow-up — Rule 2 satisfied.
   What to ask the implementer: Acknowledge the citation. No change needed; confirm the render output is what replaces the hand-authored index.md (AC-05 reachability artefact) and that the drift-guard deferral remains tracked (sworn#20).
   Citation: [[project_index_frontmatter_corruption_false_ready]]

## Summary
Pins: 4 total — 1 [mechanical], 2 [memory-cited], 1 [escalate]
Critical pins (if any): Pin 1 — the AC-04↔AC-05 contradiction determines whether AC-05 can pass at all; building either strictness without Coach authority ships a spec-deviating slice.

## Smaller flags (not pins, worth one-line acknowledgement)
- (a) Pin 2 in the design (test fixtures under `internal/board/testdata/render/` sit outside the 3-file touchpoint list) is conventional and fine — inert fixtures owned by `render_test.go`, no decision needed.
- (b) No touchpoint collisions: S02's three files appear in no sibling's touchpoints; S04/S05 (both merged) authored the `board.go` symbols S02 only *reads*. The design's touchpoint-disjoint claim (which AC-05 asserts) independently holds.
- (c) Drift gate reported the track 2 commits behind release-wt — a false positive: both are worktree-materialisation bookkeeping commits and `spec.json` is byte-identical across refs. Reviewed against the track worktree's design.md (the freshest, and the only ref that has it).

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Solid, well-anchored design — every cited symbol and the board.json string-shape all verified true; one load-bearing spec contradiction needs a Coach call. 4 pins + 3 flags:

1. **AC-04↔AC-05 contradiction (Coach decision).** AC-04 fails-closed on "invalid against board-v1" (canonical reader wants `release`=object); AC-05 requires render to succeed against this release's string-shaped board. Do NOT pick unilaterally. Await the Coach's choice: (a) render tolerates dual string-or-object form and does not enforce `release`=object, or (b) render enforces strict board-v1 and this slice depends on migrating board.json (T4). If unresolved, this routes to `/replan-release`.
2. **Record the Type-1 decision.** Once Pin 1 is resolved, write it into `status.json.design_decisions` as a Type-1 choice (chosen option + rationale + both options) before coding — Rule 9 design-fit gate; the field is currently absent.
3. **Widen the test scope.** AC-06 only names `go test ./internal/board/...`, but you add `cmd/sworn/render.go` + a new reader path — the package a strict-reader change regressed before (feedback_releaseverify_specmd_false_fail). Run `go test ./cmd/sworn/...` too, plus full `go test ./...` with a timeout, before claiming done.
4. **Frontmatter fix — acknowledged.** Choice 3 (`ValidateIndex` + single-quoted scalars, AC-03) correctly kills the index-frontmatter-fusion failure class; keep the sworn#20 drift-guard deferral tracked.

Flags (not pins): (a) test fixtures outside the 3-file touchpoint list is fine; (b) no sibling touchpoint collisions, disjointness holds; (c) the drift-gate "2 commits behind" is a bookkeeping false positive, spec byte-identical.

§2 decisions 2 ([[feedback_releaseverify_specmd_false_fail]]), 3 ([[project_index_frontmatter_corruption_false_ready]]) memory-cited; 4, 5 clean and acknowledged. §6 Pin 2 (fixtures) acknowledged; §6 Pin 1 is the escalate above.

Address pins 2–4 inline during implementation once Pin 1 is resolved, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: AC-04 (fail closed on invalid board-v1 = release-object) and AC-05 (must render this release's string-shaped board) are an internal spec contradiction; resolving it also commits a Type-1 tolerant-reader architecture — a spec-coherence + design-fidelity judgement only the Coach can make.
-->
