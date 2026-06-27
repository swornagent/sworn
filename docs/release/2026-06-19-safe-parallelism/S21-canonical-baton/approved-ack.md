<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design — 5 pins, all mechanical/memory. 3 are critical (1 + 3 + 4):

1. **ADR filename.** Use `docs/adr/0008-canonical-baton.md` (not 0005 — taken by T2). Update `planned_files` in status.json. Note divergence in proof.md.
2. **track-mode.md pre-exists.** S08c already created `internal/prompt/baton/track-mode.md`. Do not recreate or overwrite. Mark as pre-existing in proof.md. The embed extension to `baton/*` in prompt.go still needs to happen (replacing the explicit `baton/track-mode.md` in the embed directive).
3. **Populate `design_decisions` in status.json.** Record the five §2 decisions (all Type-2) before calling `sworn designfit`.
4. **AC7 test seed.** Use `## Engineering Process — Baton` (the real `adopt.BatonSectionHeading` constant) as the legacy-marker fixture string in `TestInitWarnsLegacyBaton`, not `<!-- baton:start -->`.
5. **T14 boundary.** No bash→sworn transforms in this slice — confirm S21 content is verbatim from sources. Transform is T14/S48's job per [[project_baton_sworn_architecture]].

Flags: (a) session-discipline.md + brainstorm-patterns.md sourced from `~/.claude/baton/` — fine for non-rule docs, note in proof.md if content diverges; (b) update prompt.go package comment to mention baton/ subdir.

§2 decisions 1–5 (all Type-2) ack. §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline corrections (wrong ADR filename, pre-existing file acknowledgement, missing status.json field, test seed string, memory ack) — none requires re-checking the design concept before code is safe.
-->
